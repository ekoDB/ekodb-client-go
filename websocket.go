package ekodb

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
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
	token string
	conn  *websocket.Conn

	mu               sync.Mutex
	pendingRequests  map[string]chan wsResponse
	subscriptions    map[string]chan MutationNotification
	chatStreams      map[string]chan ChatStreamEvent
	registerToolsAck chan wsResponse
	dispatcherDone   chan struct{}
	ctx              context.Context
	cancel           context.CancelFunc
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
		token:           c.getToken(),
		pendingRequests: make(map[string]chan wsResponse),
		subscriptions:   make(map[string]chan MutationNotification),
		chatStreams:     make(map[string]chan ChatStreamEvent),
		ctx:             ctx,
		cancel:          cancel,
	}

	if err := ws.connect(); err != nil {
		cancel()
		return nil, err
	}

	ws.dispatcherDone = make(chan struct{})
	go ws.readLoop()

	return ws, nil
}

// connect establishes a WebSocket connection.
func (ws *WebSocketClient) connect() error {
	url := ws.wsURL
	if len(url) < 7 || url[len(url)-7:] != "/api/ws" {
		url += "/api/ws"
	}
	url += "?token=" + ws.token

	header := make(map[string][]string)
	header["Authorization"] = []string{"Bearer " + ws.token}

	conn, _, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		return fmt.Errorf("websocket connection failed: %w", err)
	}

	ws.conn = conn
	return nil
}

func (ws *WebSocketClient) genMessageID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// readLoop is the dispatcher goroutine that routes incoming messages.
func (ws *WebSocketClient) readLoop() {
	defer close(ws.dispatcherDone)

	for {
		_, data, err := ws.conn.ReadMessage()
		if err != nil {
			ws.mu.Lock()
			// Notify all pending requests
			for id, ch := range ws.pendingRequests {
				ch <- wsResponse{Err: fmt.Errorf("connection closed: %w", err)}
				delete(ws.pendingRequests, id)
			}
			// Close all chat streams
			for id, ch := range ws.chatStreams {
				ch <- ChatStreamEvent{Type: "error", Error: "connection closed"}
				close(ch)
				delete(ws.chatStreams, id)
			}
			// Close all subscriptions
			for id, ch := range ws.subscriptions {
				close(ch)
				delete(ws.subscriptions, id)
			}
			if ws.registerToolsAck != nil {
				ws.registerToolsAck <- wsResponse{Err: fmt.Errorf("connection closed")}
				ws.registerToolsAck = nil
			}
			ws.mu.Unlock()
			return
		}

		var msg map[string]json.RawMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		var msgType string
		if t, ok := msg["type"]; ok {
			json.Unmarshal(t, &msgType)
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
	}
}

func (ws *WebSocketClient) routeRequestResponse(msgType string, msg map[string]json.RawMessage) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	// Try to extract messageId from payload
	var messageID string
	if payloadRaw, ok := msg["payload"]; ok {
		var payload map[string]json.RawMessage
		if json.Unmarshal(payloadRaw, &payload) == nil {
			if midRaw, ok := payload["message_id"]; ok {
				json.Unmarshal(midRaw, &messageID)
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

	if target == nil && ws.registerToolsAck != nil {
		target = ws.registerToolsAck
		ws.registerToolsAck = nil
	}

	if target == nil && len(ws.pendingRequests) == 1 {
		for id, ch := range ws.pendingRequests {
			target = ch
			delete(ws.pendingRequests, id)
			break
		}
	}

	if target != nil {
		if msgType == "Error" {
			var errMsg string
			if raw, ok := msg["message"]; ok {
				json.Unmarshal(raw, &errMsg)
			}
			if errMsg == "" {
				errMsg = "unknown error"
			}
			target <- wsResponse{Err: fmt.Errorf("websocket error: %s", errMsg)}
		} else {
			target <- wsResponse{Payload: msg["payload"]}
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

	ws.mu.Lock()
	ch, ok := ws.subscriptions[notification.Collection]
	ws.mu.Unlock()

	if ok {
		select {
		case ch <- notification:
		default:
			// Drop if channel full
		}
	}
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
		json.Unmarshal(raw, &chatID)
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
		json.Unmarshal(raw, &payload)
	}

	ws.mu.Lock()
	ch, ok := ws.chatStreams[chatID]
	ws.mu.Unlock()

	if ok {
		ch <- ChatStreamEvent{Type: "chunk", Content: payload.Content}
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
	}
	if raw, ok := msg["payload"]; ok {
		json.Unmarshal(raw, &payload)
	}

	ws.mu.Lock()
	ch, ok := ws.chatStreams[chatID]
	if ok {
		delete(ws.chatStreams, chatID)
	}
	ws.mu.Unlock()

	if ok {
		ch <- ChatStreamEvent{
			Type:            "end",
			MessageID:       payload.MessageID,
			TokenUsage:      payload.TokenUsage,
			ToolCallHistory: payload.ToolCallHistory,
			ExecutionTimeMs: payload.ExecutionTimeMs,
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
		json.Unmarshal(raw, &payload)
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
		ch <- ChatStreamEvent{Type: "error", Error: errMsg}
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
		json.Unmarshal(raw, &payload)
	}

	ws.mu.Lock()
	ch, ok := ws.chatStreams[chatID]
	ws.mu.Unlock()

	if ok {
		ch <- ChatStreamEvent{
			Type:      "toolCall",
			ChatID:    chatID,
			CallID:    payload.CallID,
			ToolName:  payload.ToolName,
			Arguments: payload.Arguments,
		}
	}
}

func (ws *WebSocketClient) sendRequest(request interface{}, messageID string) (json.RawMessage, error) {
	ch := make(chan wsResponse, 1)

	ws.mu.Lock()
	ws.pendingRequests[messageID] = ch
	ws.mu.Unlock()

	if err := ws.conn.WriteJSON(request); err != nil {
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
	case <-ws.ctx.Done():
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
		return []Record{}, nil
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

	// Register subscription channel before sending
	ch := make(chan MutationNotification, 64)
	ws.mu.Lock()
	ws.subscriptions[collection] = ch
	ws.mu.Unlock()

	_, err := ws.sendRequest(request, messageID)
	if err != nil {
		ws.mu.Lock()
		delete(ws.subscriptions, collection)
		ws.mu.Unlock()
		return nil, err
	}

	return ch, nil
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
	ws.chatStreams[chatID] = ch
	ws.mu.Unlock()

	if err := ws.conn.WriteJSON(request); err != nil {
		ws.mu.Lock()
		delete(ws.chatStreams, chatID)
		ws.mu.Unlock()
		return nil, fmt.Errorf("failed to send chat request: %w", err)
	}

	return ch, nil
}

// RegisterClientTools registers client-side tool definitions for a chat session.
func (ws *WebSocketClient) RegisterClientTools(chatID string, tools []ClientToolDefinition) error {
	request := map[string]interface{}{
		"type": "RegisterClientTools",
		"payload": map[string]interface{}{
			"chat_id": chatID,
			"tools":   tools,
		},
	}

	ackCh := make(chan wsResponse, 1)
	ws.mu.Lock()
	ws.registerToolsAck = ackCh
	ws.mu.Unlock()

	if err := ws.conn.WriteJSON(request); err != nil {
		ws.mu.Lock()
		ws.registerToolsAck = nil
		ws.mu.Unlock()
		return fmt.Errorf("failed to send tool registration: %w", err)
	}

	select {
	case resp := <-ackCh:
		return resp.Err
	case <-ws.ctx.Done():
		return fmt.Errorf("context cancelled")
	}
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

	return ws.conn.WriteJSON(request)
}

// Close closes the WebSocket connection and cleans up resources.
func (ws *WebSocketClient) Close() error {
	ws.cancel()
	if ws.conn != nil {
		err := ws.conn.Close()
		if ws.dispatcherDone != nil {
			<-ws.dispatcherDone
		}
		return err
	}
	return nil
}
