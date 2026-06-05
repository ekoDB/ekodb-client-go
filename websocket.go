package ekodb

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// MutationNotification represents a subscription event from collection changes.
type MutationNotification struct {
	Collection string          `json:"collection"`
	Event      string          `json:"event"`
	RecordIDs  []string        `json:"record_ids"`
	Records    json.RawMessage `json:"records,omitempty"`
	Timestamp  string          `json:"timestamp"`
}

// ChatStreamEvent represents an event from a streaming chat response.
type ChatStreamEvent struct {
	Type            string          `json:"type"` // "chunk", "end", "toolCall", "error"
	Content         string          `json:"content,omitempty"`
	MessageID       string          `json:"message_id,omitempty"`
	TokenUsage      json.RawMessage `json:"token_usage,omitempty"`
	ToolCallHistory json.RawMessage `json:"tool_call_history,omitempty"`
	ExecutionTimeMs uint64          `json:"execution_time_ms,omitempty"`
	ContextWindow   uint32          `json:"context_window,omitempty"` // Model's context window size in tokens
	ChatID          string          `json:"chat_id,omitempty"`
	CallID          string          `json:"call_id,omitempty"`
	ToolName        string          `json:"tool_name,omitempty"`
	Arguments       json.RawMessage `json:"arguments,omitempty"`
	Error           string          `json:"error,omitempty"`
}

// ClientToolDefinition defines a client-side tool the LLM can call.
type ClientToolDefinition struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

// ChatSendOptions are optional parameters for ChatSend.
type ChatSendOptions struct {
	BypassRipple  *bool                  `json:"bypass_ripple,omitempty"`
	ClientTools   []ClientToolDefinition `json:"client_tools,omitempty"`
	MaxIterations *uint32                `json:"max_iterations,omitempty"`
	ConfirmTools  []string               `json:"confirm_tools,omitempty"`
	ExcludeTools  []string               `json:"exclude_tools,omitempty"`
}

// SubscribeOptions are optional parameters for Subscribe.
type SubscribeOptions struct {
	FilterField string `json:"filter_field,omitempty"`
	FilterValue string `json:"filter_value,omitempty"`
}

// WebSocketClient represents a WebSocket connection to ekoDB with full dispatcher.
type WebSocketClient struct {
	wsURL string
	conn  *websocket.Conn

	// tokenProvider returns a fresh auth token on every (re)connect. It is
	// read on each dial so a since-expired JWT can be refreshed transparently.
	tokenProvider func() string

	writeMu         sync.Mutex // serializes all writes to ws.conn
	mu              sync.Mutex // protects maps + closing/reconnecting flags
	pendingRequests map[string]chan wsResponse
	subscriptions   map[string]chan MutationNotification
	// subParams records the parameters used for each active subscription so
	// the subscribe request can be replayed after an automatic reconnect.
	subParams      map[string]SubscribeOptions
	chatStreams    map[string]chan ChatStreamEvent
	dispatcherDone chan struct{}
	ctx            context.Context
	cancel         context.CancelFunc
	messageCounter atomic.Int64
	schemaCache    *SchemaCache // optional, for auto-invalidation on SchemaChanged

	// closing is set by Close() so the reconnect loop exits cleanly and an
	// intentional shutdown is never mistaken for a transient drop.
	closing bool
	// reconnecting indicates a reconnect loop is currently running (set when a
	// drop is handed off to reconnect(), cleared once a new dispatcher is up).
	// Close() reads it to skip waiting on dispatcherDone when no dispatcher
	// goroutine is active (e.g. the reconnect loop is mid-backoff).
	reconnecting bool
}

type wsResponse struct {
	Payload json.RawMessage
	Err     error
}

// WebSocket creates a new WebSocket client with dispatcher.
func (c *Client) WebSocket(wsURL string) (*WebSocketClient, error) {
	ctx, cancel := context.WithCancel(context.Background())
	ws := &WebSocketClient{
		wsURL:           wsURL,
		tokenProvider:   c.getToken,
		pendingRequests: make(map[string]chan wsResponse),
		subscriptions:   make(map[string]chan MutationNotification),
		subParams:       make(map[string]SubscribeOptions),
		chatStreams:     make(map[string]chan ChatStreamEvent),
		ctx:             ctx,
		cancel:          cancel,
	}

	if err := ws.connect(); err != nil {
		cancel()
		return nil, err
	}

	// Attach schema cache before starting readLoop to avoid a race
	// where a SchemaChanged message arrives before the cache is set.
	if c.schemaCache != nil {
		ws.schemaCache = c.schemaCache
	}

	ws.dispatcherDone = make(chan struct{})
	go ws.readLoop(ws.conn)

	return ws, nil
}

// ConnectWS creates a WebSocket client by deriving the WS URL from the base URL.
// Converts http→ws, https→wss (path /api/ws is appended during connect).
func (c *Client) ConnectWS() (*WebSocketClient, error) {
	wsURL := strings.Replace(c.baseURL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	return c.WebSocket(wsURL)
}

// EnableSchemaCache enables the in-memory schema cache on this client.
func (c *Client) EnableSchemaCache(ttl time.Duration, maxEntries int) {
	c.schemaCache = NewSchemaCache(SchemaCacheConfig{
		Enabled:    true,
		MaxEntries: maxEntries,
		TTL:        ttl,
	})
}

// GetSchemaCache returns the client's schema cache (may be nil if not enabled).
func (c *Client) GetSchemaCache() *SchemaCache {
	return c.schemaCache
}

// ExtractRecordID extracts the record ID from a record map using the
// schema cache's primary_key_alias for the collection.
func (c *Client) ExtractRecordID(collection string, record map[string]interface{}) string {
	if c.schemaCache != nil {
		return c.schemaCache.ExtractRecordID(collection, record)
	}
	// No cache — try defaults
	for _, key := range []string{"id", "_id"} {
		if id, ok := record[key]; ok {
			if s, ok := id.(string); ok {
				return s
			}
			// Handle typed wrapper {"type": "String", "value": "..."}
			if m, ok := id.(map[string]interface{}); ok {
				if v, ok := m["value"]; ok {
					if s, ok := v.(string); ok {
						return s
					}
				}
			}
		}
	}
	return ""
}

// connect establishes a WebSocket connection. A FRESH token is fetched from
// tokenProvider on every call so a since-expired JWT is refreshed (via the
// parent client's proactive-refresh mechanism) before each (re)connect.
func (ws *WebSocketClient) connect() error {
	u, err := url.Parse(ws.wsURL)
	if err != nil {
		return fmt.Errorf("invalid websocket URL: %w", err)
	}

	if u.Path == "" || u.Path == "/" {
		u.Path = "/api/ws"
	} else if !strings.HasSuffix(u.Path, "/api/ws") {
		u.Path = strings.TrimRight(u.Path, "/") + "/api/ws"
	}

	var token string
	if ws.tokenProvider != nil {
		token = ws.tokenProvider()
	}

	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()

	header := make(map[string][]string)
	header["Authorization"] = []string{"Bearer " + token}

	// DialContext(ws.ctx) so Close()/cancel() can abort an in-flight dial
	// (DefaultDialer.HandshakeTimeout still bounds a hung handshake). This keeps
	// a reconnect-loop dial from blocking a clean shutdown.
	conn, _, err := websocket.DefaultDialer.DialContext(ws.ctx, u.String(), header)
	if err != nil {
		return fmt.Errorf("websocket connection failed: %w", err)
	}

	// A dial that completes in the instant before Close() runs still needs this
	// guard: don't store a connection Close() has already moved past, or it
	// leaks an open socket. Holding ws.mu across the closing check and the store
	// keeps it atomic vs Close(), which sets ws.closing under ws.mu before
	// nil'ing ws.conn.
	ws.mu.Lock()
	if ws.closing {
		ws.mu.Unlock()
		_ = conn.Close()
		return fmt.Errorf("websocket client closed during connect")
	}
	ws.writeMu.Lock()
	ws.conn = conn
	ws.writeMu.Unlock()
	ws.mu.Unlock()
	return nil
}

// Reconnect backoff bounds for the automatic reconnect loop.
const (
	reconnectBaseDelay = 200 * time.Millisecond
	reconnectMaxDelay  = 5 * time.Second
)

// reconnect runs the automatic reconnect loop after an unexpected disconnect.
// It dials again with a FRESH token using capped exponential backoff + jitter,
// re-sends the subscribe request for every tracked subscription, then resumes
// the dispatcher. It exits cleanly if Close() is called while it is running.
func (ws *WebSocketClient) reconnect() {
	defer func() {
		ws.mu.Lock()
		ws.reconnecting = false
		ws.mu.Unlock()
	}()

	delay := reconnectBaseDelay
	for {
		// Bail out if the caller intentionally closed (or context cancelled).
		ws.mu.Lock()
		closing := ws.closing
		ws.mu.Unlock()
		if closing {
			return
		}
		select {
		case <-ws.ctx.Done():
			return
		default:
		}

		// Sleep the backoff window with jitter, but wake immediately on close.
		jitter := time.Duration(rand.Int64N(int64(delay)/2 + 1))
		wait := delay + jitter
		select {
		case <-ws.ctx.Done():
			return
		case <-time.After(wait):
		}

		ws.mu.Lock()
		closing = ws.closing
		hasSubs := len(ws.subscriptions) > 0
		ws.mu.Unlock()
		// Bail if closed, or if every subscription was removed while we were
		// backing off (e.g. an in-flight Subscribe failed and deleted its sub
		// after the drop had already spawned this loop). Reviving a connection
		// with nothing to replay would leak a zombie socket + readLoop. After
		// this exit a later Subscribe won't auto-redial, which is already true
		// today since Subscribe never calls connect().
		if closing || !hasSubs {
			return
		}

		if err := ws.connect(); err != nil {
			// Dial failed — grow the backoff (capped) and retry.
			delay *= 2
			if delay > reconnectMaxDelay {
				delay = reconnectMaxDelay
			}
			continue
		}

		// Connected. Re-send every tracked subscription so the server resumes
		// pushing notifications onto the still-open caller channels.
		ws.resubscribeAll()

		// Install a fresh dispatcherDone for the new readLoop and resume.
		ws.mu.Lock()
		if ws.closing {
			ws.mu.Unlock()
			return
		}
		ws.dispatcherDone = make(chan struct{})
		ws.mu.Unlock()

		// ws.conn is written by connect(), cleared by Close(), and read by
		// writeJSON() all under writeMu — read it under the same lock here. A
		// concurrent Close() may have nil'd it, in which case there is nothing
		// to resume.
		ws.writeMu.Lock()
		conn := ws.conn
		ws.writeMu.Unlock()
		if conn == nil {
			return
		}

		go ws.readLoop(conn)
		return
	}
}

// resubscribeAll replays the subscribe request for every tracked subscription
// after a reconnect. It reads the tracked params under the lock, then sends
// each request with the lock released (writeJSON does its own locking).
func (ws *WebSocketClient) resubscribeAll() {
	ws.mu.Lock()
	params := make(map[string]SubscribeOptions, len(ws.subParams))
	for collection, opts := range ws.subParams {
		params[collection] = opts
	}
	ws.mu.Unlock()

	for collection, opts := range params {
		payload := map[string]interface{}{
			"collection": collection,
		}
		if opts.FilterField != "" {
			payload["filter_field"] = opts.FilterField
		}
		if opts.FilterValue != "" {
			payload["filter_value"] = opts.FilterValue
		}
		request := map[string]interface{}{
			"type":      "Subscribe",
			"messageId": ws.genMessageID(),
			"payload":   payload,
		}
		// Fire-and-forget: the subscribe ack is not awaited here. A write
		// failure means the connection dropped again; the next readLoop error
		// will retrigger the reconnect loop.
		_ = ws.writeJSON(request)
	}
}

func (ws *WebSocketClient) genMessageID() string {
	counter := ws.messageCounter.Add(1)
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), counter)
}

// writeJSON serializes all writes to the WebSocket connection.
func (ws *WebSocketClient) writeJSON(v interface{}) error {
	ws.writeMu.Lock()
	defer ws.writeMu.Unlock()
	if ws.conn == nil {
		return fmt.Errorf("websocket connection closed")
	}
	return ws.conn.WriteJSON(v)
}

// readLoop is the dispatcher goroutine that routes incoming messages.
// conn is passed as a parameter to avoid a data race with Close() which nils ws.conn.
func (ws *WebSocketClient) readLoop(conn *websocket.Conn) {
	// done is captured at loop start; reconnect() installs a fresh
	// dispatcherDone before spawning each new readLoop, so closing the
	// captured channel always signals the right Close() waiter.
	ws.mu.Lock()
	done := ws.dispatcherDone
	ws.mu.Unlock()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			// Connection dropped. Fail in-flight request/chat callers so they
			// don't hang, but KEEP subscription channels alive — a transient
			// drop must not permanently kill subscriptions. If this was an
			// intentional Close(), tear down for good; otherwise hand off to
			// the reconnect loop.
			ws.mu.Lock()
			pendingChans := make(map[string]chan wsResponse)
			for id, ch := range ws.pendingRequests {
				pendingChans[id] = ch
				delete(ws.pendingRequests, id)
			}
			chatChans := make(map[string]chan ChatStreamEvent)
			for id, ch := range ws.chatStreams {
				chatChans[id] = ch
				delete(ws.chatStreams, id)
			}
			closing := ws.closing
			// Reconnect exists solely to keep ACTIVE subscriptions alive across a
			// transient drop. With nothing to replay (a one-shot request or a
			// finished chat stream), an unexpected drop is terminal — tear down
			// instead of spinning a background reconnect loop.
			reconnect := !closing && len(ws.subscriptions) > 0
			// Collect subscription channels to close unless we're reconnecting
			// (in which case they stay open and get re-subscribed).
			var subChans map[string]chan MutationNotification
			if reconnect {
				ws.reconnecting = true
			} else {
				subChans = make(map[string]chan MutationNotification)
				for id, ch := range ws.subscriptions {
					subChans[id] = ch
					delete(ws.subscriptions, id)
				}
			}
			ws.mu.Unlock()

			// Send/close outside the lock to avoid deadlock
			for _, ch := range pendingChans {
				select {
				case ch <- wsResponse{Err: fmt.Errorf("connection closed: %w", err)}:
				default:
				}
			}
			for _, ch := range chatChans {
				select {
				case ch <- ChatStreamEvent{Type: "error", Error: "connection closed"}:
				default:
				}
				close(ch)
			}
			for _, ch := range subChans {
				close(ch)
			}

			// Mark connection as closed so subsequent writes fail fast
			ws.writeMu.Lock()
			ws.conn = nil
			ws.writeMu.Unlock()

			if !reconnect {
				// Terminal: either an intentional Close() or an unexpected drop
				// with no subscriptions to replay. End the dispatcher. Only an
				// intentional Close() cancels the context (so a still-usable
				// client isn't invalidated by a transient drop).
				if closing {
					ws.cancel()
				}
				close(done)
				return
			}

			// Unexpected drop WITH active subscriptions: spawn the reconnect loop
			// and end this dispatcher. reconnect() re-arms a fresh readLoop (and a
			// fresh dispatcherDone) on success. Signal this waiter, then hand off.
			close(done)
			go ws.reconnect()
			return
		}

		var msg map[string]json.RawMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		var msgType string
		if t, ok := msg["type"]; ok {
			_ = json.Unmarshal(t, &msgType)
		}

		ws.routeMessage(msgType, msg, data)
	}
}

func (ws *WebSocketClient) routeMessage(msgType string, msg map[string]json.RawMessage, raw []byte) {
	switch msgType {
	case "Success", "Error":
		ws.routeRequestResponse(msgType, msg)

	case "MutationNotification":
		ws.routeMutationNotification(msg)

	case "ChatStreamChunk":
		ws.routeChatStreamChunk(msg)

	case "ChatStreamEnd":
		ws.routeChatStreamEnd(msg)

	case "ChatStreamError":
		ws.routeChatStreamError(msg)

	case "ClientToolCall":
		ws.routeClientToolCall(msg)

	case "SchemaChanged":
		ws.routeSchemaChanged(msg)
	}
}

func (ws *WebSocketClient) routeRequestResponse(msgType string, msg map[string]json.RawMessage) {
	ws.mu.Lock()

	// Extract the messageId from the top level, then from the payload. The
	// server echoes it top-level for Success/Error, but some message shapes
	// carry it in the payload, so check both before giving up.
	var messageID string
	for _, key := range []string{"messageId", "message_id"} {
		if midRaw, ok := msg[key]; ok {
			_ = json.Unmarshal(midRaw, &messageID)
			if messageID != "" {
				break
			}
		}
	}
	if messageID == "" {
		if payloadRaw, ok := msg["payload"]; ok {
			var payload map[string]json.RawMessage
			if json.Unmarshal(payloadRaw, &payload) == nil {
				for _, key := range []string{"message_id", "messageId"} {
					if midRaw, ok := payload[key]; ok {
						_ = json.Unmarshal(midRaw, &messageID)
						if messageID != "" {
							break
						}
					}
				}
			}
		}
	}

	var target chan wsResponse

	if messageID != "" {
		if ch, ok := ws.pendingRequests[messageID]; ok {
			target = ch
			delete(ws.pendingRequests, messageID)
		}
	}

	// Fallback: the server didn't echo a messageId ANYWHERE — if exactly one
	// request is pending, deliver to it (sequential request/response). Gate on
	// `messageID == ""`, not "no top-level id field": a present-but-unmatched id
	// (e.g. a best-effort Unsubscribe ack, or a late response for an
	// already-settled request) must NOT be misrouted to an unrelated in-flight
	// request. (Matches the TypeScript client's routing.)
	if target == nil && messageID == "" && len(ws.pendingRequests) == 1 {
		for id, ch := range ws.pendingRequests {
			target = ch
			delete(ws.pendingRequests, id)
			break
		}
	}

	ws.mu.Unlock()

	if target != nil {
		var resp wsResponse
		if msgType == "Error" {
			var errMsg string
			if raw, ok := msg["message"]; ok {
				_ = json.Unmarshal(raw, &errMsg)
			}
			if errMsg == "" {
				errMsg = "unknown error"
			}
			resp = wsResponse{Err: fmt.Errorf("websocket error: %s", errMsg)}
		} else {
			resp = wsResponse{Payload: msg["payload"]}
		}
		select {
		case target <- resp:
		default:
		}
	}
}

func (ws *WebSocketClient) routeMutationNotification(msg map[string]json.RawMessage) {
	payloadRaw, ok := msg["payload"]
	if !ok {
		return
	}

	var notification MutationNotification
	if err := json.Unmarshal(payloadRaw, &notification); err != nil {
		return
	}

	// Hold ws.mu across the lookup AND the send. The send is non-blocking
	// (select/default), so holding the lock cannot deadlock. Close() and
	// Unsubscribe() remove the channel from the map (and Close() closes it)
	// under this same lock, so a delivery here can never race a close() and
	// panic with "send on closed channel".
	ws.mu.Lock()
	if ch, ok := ws.subscriptions[notification.Collection]; ok {
		select {
		case ch <- notification:
		default:
			// Drop if the consumer is not keeping up.
		}
	}
	ws.mu.Unlock()
}

func (ws *WebSocketClient) extractChatID(msg map[string]json.RawMessage) string {
	payloadRaw, ok := msg["payload"]
	if !ok {
		return ""
	}
	var payload map[string]json.RawMessage
	if json.Unmarshal(payloadRaw, &payload) != nil {
		return ""
	}
	var chatID string
	if raw, ok := payload["chat_id"]; ok {
		_ = json.Unmarshal(raw, &chatID)
	}
	return chatID
}

func (ws *WebSocketClient) routeChatStreamChunk(msg map[string]json.RawMessage) {
	chatID := ws.extractChatID(msg)
	if chatID == "" {
		return
	}

	var payload struct {
		Content string `json:"content"`
	}
	if raw, ok := msg["payload"]; ok {
		if err := json.Unmarshal(raw, &payload); err != nil {
			return
		}
	}

	ws.mu.Lock()
	ch, ok := ws.chatStreams[chatID]
	ws.mu.Unlock()

	if ok {
		select {
		case ch <- ChatStreamEvent{Type: "chunk", Content: payload.Content}:
		default:
			// Drop if channel full — consumer is too slow
		}
	}
}

func (ws *WebSocketClient) routeChatStreamEnd(msg map[string]json.RawMessage) {
	chatID := ws.extractChatID(msg)
	if chatID == "" {
		return
	}

	var payload struct {
		MessageID       string          `json:"message_id"`
		TokenUsage      json.RawMessage `json:"token_usage"`
		ToolCallHistory json.RawMessage `json:"tool_call_history"`
		ExecutionTimeMs uint64          `json:"execution_time_ms"`
		ContextWindow   uint32          `json:"context_window"`
	}
	var unmarshalErr error
	if raw, ok := msg["payload"]; ok {
		unmarshalErr = json.Unmarshal(raw, &payload)
	}

	ws.mu.Lock()
	ch, ok := ws.chatStreams[chatID]
	if ok {
		delete(ws.chatStreams, chatID)
	}
	ws.mu.Unlock()

	if ok {
		if unmarshalErr != nil {
			// Send error event so consumer isn't left hanging
			select {
			case ch <- ChatStreamEvent{Type: "error", Error: "malformed end payload: " + unmarshalErr.Error()}:
			default:
			}
		} else {
			select {
			case ch <- ChatStreamEvent{
				Type:            "end",
				MessageID:       payload.MessageID,
				TokenUsage:      payload.TokenUsage,
				ToolCallHistory: payload.ToolCallHistory,
				ExecutionTimeMs: payload.ExecutionTimeMs,
				ContextWindow:   payload.ContextWindow,
			}:
			default:
			}
		}
		close(ch)
	}
}

func (ws *WebSocketClient) routeChatStreamError(msg map[string]json.RawMessage) {
	chatID := ws.extractChatID(msg)
	if chatID == "" {
		return
	}

	var payload struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if raw, ok := msg["payload"]; ok {
		if err := json.Unmarshal(raw, &payload); err != nil {
			// Payload is malformed — still need to close the stream
			payload.Error = "malformed error payload: " + err.Error()
		}
	}

	errMsg := payload.Error
	if errMsg == "" {
		errMsg = payload.Message
	}
	if errMsg == "" {
		errMsg = "unknown error"
	}

	ws.mu.Lock()
	ch, ok := ws.chatStreams[chatID]
	if ok {
		delete(ws.chatStreams, chatID)
	}
	ws.mu.Unlock()

	if ok {
		select {
		case ch <- ChatStreamEvent{Type: "error", Error: errMsg}:
		default:
		}
		close(ch)
	}
}

func (ws *WebSocketClient) routeClientToolCall(msg map[string]json.RawMessage) {
	chatID := ws.extractChatID(msg)
	if chatID == "" {
		return
	}

	var payload struct {
		CallID    string          `json:"call_id"`
		ToolName  string          `json:"tool_name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if raw, ok := msg["payload"]; ok {
		if err := json.Unmarshal(raw, &payload); err != nil {
			return
		}
	}

	ws.mu.Lock()
	ch, ok := ws.chatStreams[chatID]
	ws.mu.Unlock()

	if ok {
		select {
		case ch <- ChatStreamEvent{
			Type:      "toolCall",
			ChatID:    chatID,
			CallID:    payload.CallID,
			ToolName:  payload.ToolName,
			Arguments: payload.Arguments,
		}:
		default:
			// Drop if channel full — consumer is too slow
		}
	}
}

func (ws *WebSocketClient) sendRequest(request interface{}, messageID string) (json.RawMessage, error) {
	ch := make(chan wsResponse, 1)

	ctx, cancel := context.WithTimeout(ws.ctx, 30*time.Second)
	defer cancel()

	ws.mu.Lock()
	ws.pendingRequests[messageID] = ch
	ws.mu.Unlock()

	if err := ws.writeJSON(request); err != nil {
		ws.mu.Lock()
		delete(ws.pendingRequests, messageID)
		ws.mu.Unlock()
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	select {
	case resp := <-ch:
		if resp.Err != nil {
			return nil, resp.Err
		}
		return resp.Payload, nil
	case <-ctx.Done():
		ws.mu.Lock()
		delete(ws.pendingRequests, messageID)
		ws.mu.Unlock()
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("request timeout")
		}
		return nil, fmt.Errorf("context cancelled")
	}
}

// FindAll finds all records in a collection via WebSocket.
func (ws *WebSocketClient) FindAll(collection string) ([]Record, error) {
	messageID := ws.genMessageID()
	request := map[string]interface{}{
		"type":      "FindAll",
		"messageId": messageID,
		"payload": map[string]string{
			"collection": collection,
		},
	}

	payloadRaw, err := ws.sendRequest(request, messageID)
	if err != nil {
		return nil, err
	}

	var payload struct {
		Data []Record `json:"data"`
	}
	if err := json.Unmarshal(payloadRaw, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal FindAll response: %w", err)
	}

	return payload.Data, nil
}

// Subscribe subscribes to mutation notifications on a collection.
// Returns a channel that receives MutationNotification events.
func (ws *WebSocketClient) Subscribe(collection string, opts ...SubscribeOptions) (<-chan MutationNotification, error) {
	messageID := ws.genMessageID()

	payload := map[string]interface{}{
		"collection": collection,
	}
	if len(opts) > 0 {
		if opts[0].FilterField != "" {
			payload["filter_field"] = opts[0].FilterField
		}
		if opts[0].FilterValue != "" {
			payload["filter_value"] = opts[0].FilterValue
		}
	}

	request := map[string]interface{}{
		"type":      "Subscribe",
		"messageId": messageID,
		"payload":   payload,
	}

	// Capture the params for replay after an automatic reconnect.
	var subOpts SubscribeOptions
	if len(opts) > 0 {
		subOpts = opts[0]
	}

	// Register subscription channel before sending
	ch := make(chan MutationNotification, 64)
	ws.mu.Lock()
	if _, exists := ws.subscriptions[collection]; exists {
		ws.mu.Unlock()
		return nil, fmt.Errorf("already subscribed to collection %q", collection)
	}
	ws.subscriptions[collection] = ch
	ws.subParams[collection] = subOpts
	ws.mu.Unlock()

	_, err := ws.sendRequest(request, messageID)
	if err != nil {
		ws.mu.Lock()
		delete(ws.subscriptions, collection)
		delete(ws.subParams, collection)
		ws.mu.Unlock()
		return nil, err
	}

	return ch, nil
}

// Unsubscribe stops delivery for a collection: it sends a best-effort
// Unsubscribe frame to the server (so the server stops streaming mutations for
// this collection on this connection), removes the local subscription so it is
// no longer replayed on reconnect, and closes its notification channel.
// It is safe to call for a collection that is not currently subscribed (no-op).
//
// The server frame is best-effort: if the socket is already gone the local
// teardown is sufficient, since the server drops all subscriptions when the
// connection closes. A unique messageId is attached so the server's Success ack
// is matched-and-dropped by the dispatcher rather than misrouted.
func (ws *WebSocketClient) Unsubscribe(collection string) {
	ws.mu.Lock()
	ch, ok := ws.subscriptions[collection]
	delete(ws.subscriptions, collection)
	delete(ws.subParams, collection)
	ws.mu.Unlock()
	if !ok {
		return
	}
	close(ch)

	_ = ws.writeJSON(map[string]interface{}{
		"type":      "Unsubscribe",
		"messageId": ws.genMessageID(),
		"payload":   map[string]interface{}{"collection": collection},
	})
}

// ChatSend sends a chat message and returns a channel of streaming events.
// The channel is closed when the stream ends or errors.
func (ws *WebSocketClient) ChatSend(chatID, message string, opts ...ChatSendOptions) (<-chan ChatStreamEvent, error) {
	payload := map[string]interface{}{
		"chat_id": chatID,
		"message": message,
	}

	if len(opts) > 0 {
		o := opts[0]
		if o.BypassRipple != nil {
			payload["bypass_ripple"] = *o.BypassRipple
		}
		if o.ClientTools != nil {
			payload["client_tools"] = o.ClientTools
		}
		if o.MaxIterations != nil {
			payload["max_iterations"] = *o.MaxIterations
		}
		if o.ConfirmTools != nil {
			payload["confirm_tools"] = o.ConfirmTools
		}
		if o.ExcludeTools != nil {
			payload["exclude_tools"] = o.ExcludeTools
		}
	}

	request := map[string]interface{}{
		"type":    "ChatSend",
		"payload": payload,
	}

	ch := make(chan ChatStreamEvent, 64)
	ws.mu.Lock()
	if _, exists := ws.chatStreams[chatID]; exists {
		ws.mu.Unlock()
		return nil, fmt.Errorf("chat stream already active for chatID %q", chatID)
	}
	ws.chatStreams[chatID] = ch
	ws.mu.Unlock()

	if err := ws.writeJSON(request); err != nil {
		ws.mu.Lock()
		delete(ws.chatStreams, chatID)
		ws.mu.Unlock()
		return nil, fmt.Errorf("failed to send chat request: %w", err)
	}

	return ch, nil
}

// RegisterClientTools registers client-side tool definitions for a chat session.
func (ws *WebSocketClient) RegisterClientTools(chatID string, tools []ClientToolDefinition) error {
	messageID := ws.genMessageID()
	request := map[string]interface{}{
		"type":      "RegisterClientTools",
		"messageId": messageID,
		"payload": map[string]interface{}{
			"chat_id": chatID,
			"tools":   tools,
		},
	}

	_, err := ws.sendRequest(request, messageID)
	return err
}

// CancelChat aborts an in-flight chat stream by chat_id. The server
// fires the matching CancellationToken, drops the LLM HTTP call, and
// skips persisting the assistant message. Pre-fix on the Rust client
// side, dropping the stream channel only halted local chunk delivery —
// the LLM kept generating server-side and the "cancelled" turn still
// landed in /history. The Go client never had that bug because there
// was no cancel API at all; this method closes the gap.
//
// Connection: requires an active WS (caller must have already called
// ChatSend or another method that establishes the connection). The
// cancel itself is idempotent on the server — sending it for a
// chat_id with no in-flight stream is a no-op and does not error.
func (ws *WebSocketClient) CancelChat(chatID string) error {
	request := map[string]interface{}{
		"type": "CancelChat",
		"payload": map[string]interface{}{
			"chat_id": chatID,
		},
	}
	return ws.writeJSON(request)
}

// SendToolResult sends a tool result back to the server during a chat stream.
func (ws *WebSocketClient) SendToolResult(chatID, callID string, success bool, result interface{}, errMsg string) error {
	payload := map[string]interface{}{
		"chat_id": chatID,
		"call_id": callID,
		"success": success,
	}
	if result != nil {
		payload["result"] = result
	}
	if errMsg != "" {
		payload["error"] = errMsg
	}

	request := map[string]interface{}{
		"type":    "ClientToolResult",
		"payload": payload,
	}

	return ws.writeJSON(request)
}

// RawCompletion performs a stateless raw LLM completion via WebSocket.
//
// Sends a RawComplete message and waits for the Success response.
// Preferred over HTTP for deployed instances: the persistent WSS
// connection is already authenticated and won't be killed by reverse
// proxy timeouts.
func (ws *WebSocketClient) RawCompletion(request RawCompletionRequest) (*RawCompletionResponse, error) {
	messageID := ws.genMessageID()

	payload := map[string]interface{}{
		"system_prompt": request.SystemPrompt,
		"message":       request.Message,
	}
	if request.Provider != nil {
		payload["provider"] = *request.Provider
	}
	if request.Model != nil {
		payload["model"] = *request.Model
	}
	if request.MaxTokens != nil {
		payload["max_tokens"] = *request.MaxTokens
	}

	req := map[string]interface{}{
		"type":      "RawComplete",
		"messageId": messageID,
		"payload":   payload,
	}

	payloadRaw, err := ws.sendRequest(req, messageID)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data struct {
			Content string `json:"content"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payloadRaw, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal RawCompletion response: %w", err)
	}

	return &RawCompletionResponse{Content: result.Data.Content}, nil
}

// SetSchemaCache attaches a schema cache for automatic invalidation on SchemaChanged events.
func (ws *WebSocketClient) SetSchemaCache(cache *SchemaCache) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.schemaCache = cache
}

func (ws *WebSocketClient) routeSchemaChanged(msg map[string]json.RawMessage) {
	payloadRaw, ok := msg["payload"]
	if !ok {
		return
	}
	var payload struct {
		Collection      string `json:"collection"`
		Version         uint64 `json:"version"`
		PrimaryKeyAlias string `json:"primary_key_alias"`
	}
	if json.Unmarshal(payloadRaw, &payload) != nil {
		return
	}
	ws.mu.Lock()
	cache := ws.schemaCache
	ws.mu.Unlock()
	if cache != nil {
		cache.HandleSchemaChanged(payload.Collection, payload.Version, payload.PrimaryKeyAlias)
	}
}

// sendCRUD is a helper for all CRUD operations: build request, send, extract data from response.
func (ws *WebSocketClient) sendCRUD(msgType string, payload map[string]interface{}) (json.RawMessage, error) {
	messageID := ws.genMessageID()
	request := map[string]interface{}{
		"type":      msgType,
		"messageId": messageID,
		"payload":   payload,
	}
	return ws.sendRequest(request, messageID)
}

// extractData pulls the "data" field from a response payload.
func extractData(payloadRaw json.RawMessage) (json.RawMessage, error) {
	var wrapper struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payloadRaw, &wrapper); err != nil {
		return payloadRaw, nil // Return raw if no "data" wrapper
	}
	if wrapper.Data != nil {
		return wrapper.Data, nil
	}
	return payloadRaw, nil
}

// =========================================================================
// WS CRUD Methods — Full Parity with Server
// =========================================================================

// Insert inserts a single record into a collection via WebSocket.
func (ws *WebSocketClient) Insert(collection string, record map[string]interface{}, bypassRipple ...bool) (json.RawMessage, error) {
	payload := map[string]interface{}{
		"collection": collection,
		"record":     record,
	}
	if len(bypassRipple) > 0 {
		payload["bypass_ripple"] = bypassRipple[0]
	}
	resp, err := ws.sendCRUD("Insert", payload)
	if err != nil {
		return nil, err
	}
	return extractData(resp)
}

// Query queries records from a collection via WebSocket.
func (ws *WebSocketClient) Query(collection string, opts ...QueryOptions) (json.RawMessage, error) {
	payload := map[string]interface{}{
		"collection": collection,
	}
	if len(opts) > 0 {
		o := opts[0]
		if o.Filter != nil {
			payload["filter"] = o.Filter
		}
		if o.Sort != nil {
			payload["sort"] = o.Sort
		}
		if o.Limit > 0 {
			payload["limit"] = o.Limit
		}
		if o.Skip > 0 {
			payload["skip"] = o.Skip
		}
	}
	resp, err := ws.sendCRUD("Query", payload)
	if err != nil {
		return nil, err
	}
	return extractData(resp)
}

// QueryOptions are optional parameters for WS Query.
type QueryOptions struct {
	Filter interface{} `json:"filter,omitempty"`
	Sort   interface{} `json:"sort,omitempty"`
	Limit  int         `json:"limit,omitempty"`
	Skip   int         `json:"skip,omitempty"`
}

// FindByID finds a single record by ID via WebSocket.
func (ws *WebSocketClient) FindByID(collection, id string) (json.RawMessage, error) {
	resp, err := ws.sendCRUD("FindById", map[string]interface{}{
		"collection": collection,
		"id":         id,
	})
	if err != nil {
		return nil, err
	}
	return extractData(resp)
}

// Update updates a record by ID via WebSocket.
func (ws *WebSocketClient) Update(collection, id string, record map[string]interface{}, bypassRipple ...bool) (json.RawMessage, error) {
	payload := map[string]interface{}{
		"collection": collection,
		"id":         id,
		"record":     record,
	}
	if len(bypassRipple) > 0 {
		payload["bypass_ripple"] = bypassRipple[0]
	}
	resp, err := ws.sendCRUD("Update", payload)
	if err != nil {
		return nil, err
	}
	return extractData(resp)
}

// Delete deletes a record by ID via WebSocket.
func (ws *WebSocketClient) Delete(collection, id string, bypassRipple ...bool) error {
	payload := map[string]interface{}{
		"collection": collection,
		"id":         id,
	}
	if len(bypassRipple) > 0 {
		payload["bypass_ripple"] = bypassRipple[0]
	}
	_, err := ws.sendCRUD("Delete", payload)
	return err
}

// BatchInsert inserts multiple records at once via WebSocket.
func (ws *WebSocketClient) BatchInsert(collection string, records []map[string]interface{}, bypassRipple ...bool) (json.RawMessage, error) {
	payload := map[string]interface{}{
		"collection": collection,
		"records":    records,
	}
	if len(bypassRipple) > 0 {
		payload["bypass_ripple"] = bypassRipple[0]
	}
	resp, err := ws.sendCRUD("BatchInsert", payload)
	if err != nil {
		return nil, err
	}
	return extractData(resp)
}

// BatchUpdate updates multiple records at once via WebSocket.
// Each update is a [id, data] pair.
func (ws *WebSocketClient) BatchUpdate(collection string, updates [][2]interface{}, bypassRipple ...bool) (json.RawMessage, error) {
	payload := map[string]interface{}{
		"collection": collection,
		"updates":    updates,
	}
	if len(bypassRipple) > 0 {
		payload["bypass_ripple"] = bypassRipple[0]
	}
	resp, err := ws.sendCRUD("BatchUpdate", payload)
	if err != nil {
		return nil, err
	}
	return extractData(resp)
}

// BatchDelete deletes multiple records by IDs via WebSocket.
func (ws *WebSocketClient) BatchDelete(collection string, ids []string, bypassRipple ...bool) error {
	payload := map[string]interface{}{
		"collection": collection,
		"ids":        ids,
	}
	if len(bypassRipple) > 0 {
		payload["bypass_ripple"] = bypassRipple[0]
	}
	_, err := ws.sendCRUD("BatchDelete", payload)
	return err
}

// TextSearch performs full-text search via WebSocket.
func (ws *WebSocketClient) TextSearch(collection, query string, fields []string, limit int) (json.RawMessage, error) {
	payload := map[string]interface{}{
		"collection": collection,
		"query":      query,
	}
	opts := map[string]interface{}{}
	if len(fields) > 0 {
		opts["fields"] = fields
	}
	if limit > 0 {
		opts["limit"] = limit
	}
	if len(opts) > 0 {
		payload["options"] = opts
	}
	resp, err := ws.sendCRUD("TextSearch", payload)
	if err != nil {
		return nil, err
	}
	return extractData(resp)
}

// DistinctValues returns distinct values for a field via WebSocket.
func (ws *WebSocketClient) DistinctValues(collection, field string, filter ...interface{}) (json.RawMessage, error) {
	payload := map[string]interface{}{
		"collection": collection,
		"field":      field,
	}
	if len(filter) > 0 && filter[0] != nil {
		payload["filter"] = filter[0]
	}
	resp, err := ws.sendCRUD("DistinctValues", payload)
	if err != nil {
		return nil, err
	}
	return extractData(resp)
}

// UpdateWithAction applies an atomic field action to a record via WebSocket.
func (ws *WebSocketClient) UpdateWithAction(collection, id, action, field string, value ...interface{}) (json.RawMessage, error) {
	payload := map[string]interface{}{
		"collection": collection,
		"id":         id,
		"action":     action,
		"field":      field,
	}
	if len(value) > 0 && value[0] != nil {
		payload["value"] = value[0]
	}
	resp, err := ws.sendCRUD("UpdateWithAction", payload)
	if err != nil {
		return nil, err
	}
	return extractData(resp)
}

// CreateCollection creates a new collection with optional schema via WebSocket.
func (ws *WebSocketClient) CreateCollection(name string, schema ...interface{}) error {
	payload := map[string]interface{}{
		"name": name,
	}
	if len(schema) > 0 && schema[0] != nil {
		payload["schema"] = schema[0]
	} else {
		payload["schema"] = map[string]interface{}{}
	}
	_, err := ws.sendCRUD("CreateCollection", payload)
	return err
}

// ListCollections lists all collections via WebSocket.
func (ws *WebSocketClient) ListCollections() ([]string, error) {
	resp, err := ws.sendCRUD("GetCollections", map[string]interface{}{})
	if err != nil {
		return nil, err
	}
	data, err := extractData(resp)
	if err != nil {
		return nil, err
	}
	var collections []string
	if err := json.Unmarshal(data, &collections); err != nil {
		return nil, fmt.Errorf("failed to unmarshal collections: %w", err)
	}
	return collections, nil
}

// DeleteCollection deletes a collection via WebSocket.
func (ws *WebSocketClient) DeleteCollection(name string) error {
	_, err := ws.sendCRUD("DeleteCollection", map[string]interface{}{
		"name": name,
	})
	return err
}

// Close closes the WebSocket connection and cleans up resources. It marks the
// client as closing FIRST so any in-flight reconnect loop exits cleanly and the
// dispatcher does not treat the resulting read error as a transient drop.
func (ws *WebSocketClient) Close() error {
	ws.mu.Lock()
	if ws.closing {
		// Already closing/closed — idempotent.
		ws.mu.Unlock()
		return nil
	}
	ws.closing = true
	reconnecting := ws.reconnecting
	done := ws.dispatcherDone
	// Close subscription channels so callers ranging over them unblock. The
	// reconnect loop is gated on ws.closing, so it will not replay these.
	for collection, ch := range ws.subscriptions {
		delete(ws.subscriptions, collection)
		delete(ws.subParams, collection)
		close(ch)
	}
	ws.mu.Unlock()

	// Cancel context first so the reconnect loop's sleep/dial wakes and exits.
	ws.cancel()

	ws.writeMu.Lock()
	conn := ws.conn
	ws.conn = nil
	ws.writeMu.Unlock()

	var err error
	if conn != nil {
		err = conn.Close()
	}

	// Only wait on the dispatcher if a readLoop is actually running. When a
	// reconnect loop is mid-backoff there is no live dispatcher to drain (its
	// previous done was already closed), so waiting would block forever.
	if conn != nil && !reconnecting && done != nil {
		<-done
	}
	return err
}
