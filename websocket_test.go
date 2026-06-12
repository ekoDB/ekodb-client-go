package ekodb

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vmihailenco/msgpack/v5"
)

// performServerHandshake mirrors the real server's additive negotiation: it
// consumes the client's Hello and replies with a Welcome offering the given
// format ("json" or "msgpack") so the test client proceeds without stalling on
// its Welcome read. Tolerant by design — if the first frame can't be read (the
// client closed before handshaking, e.g. the closing-leak test) or isn't a
// Hello, it returns silently so the handler can still hand off the conn.
func performServerHandshake(t *testing.T, conn *websocket.Conn, format string) {
	t.Helper()
	// Bound the read: a client that dials without handshaking (e.g. a direct
	// connect() unit test) must not hang the handler forever. Clear the deadline
	// after so the test's own reads on this conn are unaffected.
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	mt, data, err := conn.ReadMessage()
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil {
		return
	}
	var hello map[string]interface{}
	if mt == websocket.TextMessage && json.Unmarshal(data, &hello) == nil && hello["type"] == "Hello" {
		// t.Errorf (not Fatalf) is goroutine-safe; the handler runs off-test.
		if werr := conn.WriteJSON(map[string]interface{}{
			"type":    "Welcome",
			"payload": map[string]interface{}{"format": format},
		}); werr != nil {
			t.Errorf("handshake Welcome write failed: %v", werr)
		}
	}
}

// Test helper: write JSON to server conn, fail test on error
func mustWriteJSON(t *testing.T, conn *websocket.Conn, v interface{}) {
	t.Helper()
	if err := conn.WriteJSON(v); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
}

// Test helper: create a mock WS server and return the URL + server connection channel
func setupTestWSServer(t *testing.T) (string, chan *websocket.Conn, *httptest.Server) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	connCh := make(chan *websocket.Conn, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			// Cannot use t.Fatalf from non-test goroutine
			t.Errorf("upgrade failed: %v", err)
			return
		}
		// Mirror the real server: answer the client's Hello so connect() doesn't
		// stall on its Welcome read. "json" keeps these tests on the text
		// transport they assert against.
		performServerHandshake(t, conn, "json")
		connCh <- conn
	}))

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/ws"
	return wsURL, connCh, server
}

// Read a JSON message from server-side connection
func readMessage(t *testing.T, conn *websocket.Conn) map[string]interface{} {
	var msg map[string]interface{}
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("failed to read message: %v", err)
	}
	return msg
}

// Regression: connect() must not store (and thus leak) a freshly-dialed
// connection if Close() already flipped `closing` while the context-less Dial
// was in flight. Close() tears down the previous conn and never sees the new
// one, so storing it would leave an open socket that is never closed.
func TestWebSocketConnectDoesNotLeakWhenClosing(t *testing.T) {
	wsURL, connCh, server := setupTestWSServer(t)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ws := &WebSocketClient{
		wsURL:         wsURL,
		tokenProvider: func() string { return "test-token" },
		ctx:           ctx,
		cancel:        cancel,
	}
	// Simulate Close() having already set `closing` before connect() finishes dialing.
	ws.closing = true

	if err := ws.connect(); err == nil {
		t.Fatal("connect() must return an error when the client is already closing")
	}

	ws.writeMu.Lock()
	conn := ws.conn
	ws.writeMu.Unlock()
	if conn != nil {
		t.Fatal("connect() must not store a connection when closing (would leak an open socket)")
	}

	// If the dial reached the server, close that side so the test server shuts down cleanly.
	select {
	case sc := <-connCh:
		sc.Close()
	case <-time.After(time.Second):
	}
}

func TestWebSocketFindAll(t *testing.T) {
	wsURL, connCh, server := setupTestWSServer(t)
	defer server.Close()

	// Create a minimal client to get token
	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	// Get server connection
	serverConn := <-connCh
	defer serverConn.Close()

	// Start findAll in goroutine
	resultCh := make(chan []Record, 1)
	errCh := make(chan error, 1)
	go func() {
		records, err := ws.FindAll("users")
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- records
	}()

	// Read request from server
	msg := readMessage(t, serverConn)
	if msg["type"] != "FindAll" {
		t.Fatalf("expected FindAll, got %v", msg["type"])
	}

	payload := msg["payload"].(map[string]interface{})
	if payload["collection"] != "users" {
		t.Fatalf("expected collection users, got %v", payload["collection"])
	}

	// Send response
	resp := map[string]interface{}{
		"type": "Success",
		"payload": map[string]interface{}{
			"message_id": msg["messageId"],
			"data": []map[string]interface{}{
				{"id": "1", "name": "Alice"},
			},
		},
	}
	mustWriteJSON(t, serverConn, resp)

	select {
	case records := <-resultCh:
		if len(records) != 1 {
			t.Fatalf("expected 1 record, got %d", len(records))
		}
		if records[0]["name"] != "Alice" {
			t.Fatalf("expected name Alice, got %v", records[0]["name"])
		}
	case err := <-errCh:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for FindAll result")
	}
}

func TestWebSocketFindAllError(t *testing.T) {
	wsURL, connCh, server := setupTestWSServer(t)
	defer server.Close()

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	serverConn := <-connCh
	defer serverConn.Close()

	errCh := make(chan error, 1)
	go func() {
		_, err := ws.FindAll("nonexistent")
		errCh <- err
	}()

	msg := readMessage(t, serverConn)

	resp := map[string]interface{}{
		"type":    "Error",
		"message": "Collection not found",
		"payload": map[string]interface{}{
			"message_id": msg["messageId"],
		},
	}
	mustWriteJSON(t, serverConn, resp)

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Collection not found") {
			t.Fatalf("expected 'Collection not found', got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestWebSocketSubscribe(t *testing.T) {
	wsURL, connCh, server := setupTestWSServer(t)
	defer server.Close()

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	serverConn := <-connCh
	defer serverConn.Close()

	subCh := make(chan (<-chan MutationNotification), 1)
	subErr := make(chan error, 1)
	go func() {
		ch, err := ws.Subscribe("orders", SubscribeOptions{
			FilterField: "status",
			FilterValue: "active",
		})
		if err != nil {
			subErr <- err
			return
		}
		subCh <- ch
	}()

	msg := readMessage(t, serverConn)
	if msg["type"] != "Subscribe" {
		t.Fatalf("expected Subscribe, got %v", msg["type"])
	}
	payload := msg["payload"].(map[string]interface{})
	if payload["filter_field"] != "status" {
		t.Fatalf("expected filter_field status, got %v", payload["filter_field"])
	}

	// Ack
	mustWriteJSON(t, serverConn, map[string]interface{}{
		"type":    "Success",
		"payload": map[string]interface{}{"message_id": msg["messageId"]},
	})

	var notifCh <-chan MutationNotification
	select {
	case notifCh = <-subCh:
	case err := <-subErr:
		t.Fatalf("subscribe failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for subscribe")
	}

	// Send mutation notification
	mustWriteJSON(t, serverConn, map[string]interface{}{
		"type": "MutationNotification",
		"payload": map[string]interface{}{
			"collection": "orders",
			"event":      "insert",
			"record_ids": []string{"order-1"},
			"timestamp":  "2026-03-13T00:00:00Z",
		},
	})

	select {
	case n := <-notifCh:
		if n.Event != "insert" {
			t.Fatalf("expected insert event, got %v", n.Event)
		}
		if len(n.RecordIDs) != 1 || n.RecordIDs[0] != "order-1" {
			t.Fatalf("expected record_ids [order-1], got %v", n.RecordIDs)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for notification")
	}
}

func TestWebSocketChatSend(t *testing.T) {
	wsURL, connCh, server := setupTestWSServer(t)
	defer server.Close()

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	serverConn := <-connCh
	defer serverConn.Close()

	eventCh, err := ws.ChatSend("chat-1", "Hello")
	if err != nil {
		t.Fatalf("failed to send chat: %v", err)
	}

	msg := readMessage(t, serverConn)
	if msg["type"] != "ChatSend" {
		t.Fatalf("expected ChatSend, got %v", msg["type"])
	}
	payload := msg["payload"].(map[string]interface{})
	if payload["chat_id"] != "chat-1" {
		t.Fatalf("expected chat_id chat-1, got %v", payload["chat_id"])
	}

	// Send chunks
	mustWriteJSON(t, serverConn, map[string]interface{}{
		"type":    "ChatStreamChunk",
		"payload": map[string]interface{}{"chat_id": "chat-1", "content": "Hi "},
	})
	mustWriteJSON(t, serverConn, map[string]interface{}{
		"type":    "ChatStreamChunk",
		"payload": map[string]interface{}{"chat_id": "chat-1", "content": "there!"},
	})
	mustWriteJSON(t, serverConn, map[string]interface{}{
		"type": "ChatStreamEnd",
		"payload": map[string]interface{}{
			"chat_id":           "chat-1",
			"message_id":        "msg-1",
			"execution_time_ms": 150,
		},
	})

	var events []ChatStreamEvent
	for event := range eventCh {
		events = append(events, event)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].Type != "chunk" || events[0].Content != "Hi " {
		t.Fatalf("unexpected first event: %+v", events[0])
	}
	if events[1].Type != "chunk" || events[1].Content != "there!" {
		t.Fatalf("unexpected second event: %+v", events[1])
	}
	if events[2].Type != "end" || events[2].MessageID != "msg-1" {
		t.Fatalf("unexpected end event: %+v", events[2])
	}
}

func TestWebSocketChatStreamError(t *testing.T) {
	wsURL, connCh, server := setupTestWSServer(t)
	defer server.Close()

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	serverConn := <-connCh
	defer serverConn.Close()

	eventCh, err := ws.ChatSend("chat-2", "test")
	if err != nil {
		t.Fatalf("failed to send chat: %v", err)
	}

	readMessage(t, serverConn) // consume the ChatSend message

	mustWriteJSON(t, serverConn, map[string]interface{}{
		"type": "ChatStreamError",
		"payload": map[string]interface{}{
			"chat_id": "chat-2",
			"error":   "Model unavailable",
		},
	})

	var events []ChatStreamEvent
	for event := range eventCh {
		events = append(events, event)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "error" || events[0].Error != "Model unavailable" {
		t.Fatalf("unexpected error event: %+v", events[0])
	}
}

func TestWebSocketRegisterClientTools(t *testing.T) {
	wsURL, connCh, server := setupTestWSServer(t)
	defer server.Close()

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	serverConn := <-connCh
	defer serverConn.Close()

	tools := []ClientToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get weather for a location",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"location": map[string]string{"type": "string"},
				},
			},
		},
	}

	regErr := make(chan error, 1)
	go func() {
		regErr <- ws.RegisterClientTools("chat-1", tools)
	}()

	msg := readMessage(t, serverConn)
	if msg["type"] != "RegisterClientTools" {
		t.Fatalf("expected RegisterClientTools, got %v", msg["type"])
	}

	// Ack with messageId echoed back
	mustWriteJSON(t, serverConn, map[string]interface{}{
		"type": "Success",
		"payload": map[string]interface{}{
			"message_id": msg["messageId"],
			"status":     "tools_registered",
		},
	})

	select {
	case err := <-regErr:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestWebSocketSendToolResult(t *testing.T) {
	wsURL, connCh, server := setupTestWSServer(t)
	defer server.Close()

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	serverConn := <-connCh
	defer serverConn.Close()

	// Start a chat stream to have context
	eventCh, err := ws.ChatSend("chat-1", "test")
	if err != nil {
		t.Fatalf("failed to start chat: %v", err)
	}
	readMessage(t, serverConn) // consume ChatSend

	// Server sends tool call
	mustWriteJSON(t, serverConn, map[string]interface{}{
		"type": "ClientToolCall",
		"payload": map[string]interface{}{
			"chat_id":   "chat-1",
			"call_id":   "call-123",
			"tool_name": "get_weather",
			"arguments": map[string]string{"location": "NYC"},
		},
	})

	// Read tool call event
	event := <-eventCh
	if event.Type != "toolCall" {
		t.Fatalf("expected toolCall, got %v", event.Type)
	}
	if event.ToolName != "get_weather" {
		t.Fatalf("expected get_weather, got %v", event.ToolName)
	}

	// Send tool result
	err = ws.SendToolResult("chat-1", "call-123", true, map[string]string{"temp": "72F"}, "")
	if err != nil {
		t.Fatalf("failed to send tool result: %v", err)
	}

	// Verify server received it
	msg := readMessage(t, serverConn)
	if msg["type"] != "ClientToolResult" {
		t.Fatalf("expected ClientToolResult, got %v", msg["type"])
	}
	payload := msg["payload"].(map[string]interface{})
	if payload["call_id"] != "call-123" {
		t.Fatalf("expected call_id call-123, got %v", payload["call_id"])
	}
	if payload["success"] != true {
		t.Fatalf("expected success true")
	}

	// End the stream
	mustWriteJSON(t, serverConn, map[string]interface{}{
		"type": "ChatStreamEnd",
		"payload": map[string]interface{}{
			"chat_id":           "chat-1",
			"message_id":        "msg-1",
			"execution_time_ms": 500,
		},
	})

	// Drain remaining events
	for range eventCh {
	}
}

func TestWebSocketCancelChat(t *testing.T) {
	// Wire-format guard for the server-side cancel path. The ekoDB
	// server matches on the literal type tag "CancelChat" and the
	// nested payload.chat_id field — any rename here silently
	// breaks cancellation across the version boundary, so pin the
	// JSON shape in a test rather than relying on map[string]
	// ordering staying stable.
	wsURL, connCh, server := setupTestWSServer(t)
	defer server.Close()

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	serverConn := <-connCh
	defer serverConn.Close()

	if err := ws.CancelChat("chat-xyz"); err != nil {
		t.Fatalf("CancelChat returned error: %v", err)
	}

	msg := readMessage(t, serverConn)
	if msg["type"] != "CancelChat" {
		t.Fatalf("expected type CancelChat, got %v", msg["type"])
	}
	payload, ok := msg["payload"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected payload to be an object, got %T", msg["payload"])
	}
	if payload["chat_id"] != "chat-xyz" {
		t.Fatalf("expected payload.chat_id chat-xyz, got %v", payload["chat_id"])
	}
	// Payload should carry exactly chat_id and nothing else —
	// extra fields would force a wire-compat shim on older
	// servers.
	if len(payload) != 1 {
		t.Fatalf("expected payload to have exactly 1 field, got %d: %v", len(payload), payload)
	}
}

func TestWebSocketChatSendWithOptions(t *testing.T) {
	wsURL, connCh, server := setupTestWSServer(t)
	defer server.Close()

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	serverConn := <-connCh
	defer serverConn.Close()

	bypass := true
	maxIter := uint32(5)
	eventCh, err := ws.ChatSend("chat-3", "Hello", ChatSendOptions{
		BypassRipple:  &bypass,
		MaxIterations: &maxIter,
		ExcludeTools:  []string{"shell_exec"},
	})
	if err != nil {
		t.Fatalf("failed to send chat: %v", err)
	}

	msg := readMessage(t, serverConn)
	payload := msg["payload"].(map[string]interface{})

	if payload["bypass_ripple"] != true {
		t.Fatalf("expected bypass_ripple true")
	}
	if payload["max_iterations"] != float64(5) {
		t.Fatalf("expected max_iterations 5, got %v", payload["max_iterations"])
	}

	excludeTools := payload["exclude_tools"].([]interface{})
	if len(excludeTools) != 1 || excludeTools[0] != "shell_exec" {
		t.Fatalf("unexpected exclude_tools: %v", excludeTools)
	}

	// End stream
	mustWriteJSON(t, serverConn, map[string]interface{}{
		"type": "ChatStreamEnd",
		"payload": map[string]interface{}{
			"chat_id":           "chat-3",
			"message_id":        "msg-x",
			"execution_time_ms": 10,
		},
	})

	for range eventCh {
	}
}

func TestWebSocketMutationNotificationTypes(t *testing.T) {
	// Test MutationNotification JSON marshaling
	n := MutationNotification{
		Collection: "users",
		Event:      "insert",
		RecordIDs:  []string{"1", "2"},
		Timestamp:  "2026-03-13T00:00:00Z",
	}

	data, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var n2 MutationNotification
	if err := json.Unmarshal(data, &n2); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if n2.Collection != "users" || n2.Event != "insert" || len(n2.RecordIDs) != 2 {
		t.Fatalf("roundtrip failed: %+v", n2)
	}
}

func TestChatStreamEventTypes(t *testing.T) {
	events := []ChatStreamEvent{
		{Type: "chunk", Content: "Hello"},
		{Type: "end", MessageID: "msg-1", ExecutionTimeMs: 100},
		{Type: "error", Error: "something broke"},
		{Type: "toolCall", ChatID: "c1", CallID: "call-1", ToolName: "get_weather"},
	}

	for _, e := range events {
		data, err := json.Marshal(e)
		if err != nil {
			t.Fatalf("failed to marshal %s event: %v", e.Type, err)
		}

		var e2 ChatStreamEvent
		if err := json.Unmarshal(data, &e2); err != nil {
			t.Fatalf("failed to unmarshal %s event: %v", e.Type, err)
		}

		if e2.Type != e.Type {
			t.Fatalf("type mismatch: expected %s, got %s", e.Type, e2.Type)
		}
	}
}

func TestChatStreamEndContextWindow(t *testing.T) {
	// Test that context_window field is correctly marshalled/unmarshalled
	event := ChatStreamEvent{
		Type:            "end",
		MessageID:       "msg-cw",
		ExecutionTimeMs: 250,
		ContextWindow:   128000,
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var e2 ChatStreamEvent
	if err := json.Unmarshal(data, &e2); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if e2.ContextWindow != 128000 {
		t.Fatalf("expected context_window 128000, got %d", e2.ContextWindow)
	}

	// Test that context_window is omitted when zero
	event2 := ChatStreamEvent{Type: "end", MessageID: "msg-2"}
	data2, _ := json.Marshal(event2)
	dataStr := string(data2)
	if json.Valid(data2) {
		var raw map[string]interface{}
		if err := json.Unmarshal(data2, &raw); err != nil {
			t.Fatalf("failed to unmarshal ChatStreamEvent: %v", err)
		}
		if _, exists := raw["context_window"]; exists {
			t.Fatalf("context_window should be omitted when zero, got: %s", dataStr)
		}
	}
}

func TestWebSocketChatSendWithContextWindow(t *testing.T) {
	wsURL, connCh, server := setupTestWSServer(t)
	defer server.Close()

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	serverConn := <-connCh
	defer serverConn.Close()

	eventCh, err := ws.ChatSend("chat-cw", "test")
	if err != nil {
		t.Fatalf("failed to send chat: %v", err)
	}

	readMessage(t, serverConn)

	// Send end event with context_window
	mustWriteJSON(t, serverConn, map[string]interface{}{
		"type": "ChatStreamEnd",
		"payload": map[string]interface{}{
			"chat_id":           "chat-cw",
			"message_id":        "msg-cw",
			"execution_time_ms": 100,
			"context_window":    128000,
		},
	})

	var events []ChatStreamEvent
	for event := range eventCh {
		events = append(events, event)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "end" {
		t.Fatalf("expected end event, got %s", events[0].Type)
	}
	if events[0].ContextWindow != 128000 {
		t.Fatalf("expected context_window 128000, got %d", events[0].ContextWindow)
	}
}

func TestClientToolDefinitionJSON(t *testing.T) {
	tool := ClientToolDefinition{
		Name:        "get_weather",
		Description: "Get current weather",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"location": map[string]string{"type": "string"},
			},
		},
	}

	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var tool2 ClientToolDefinition
	if err := json.Unmarshal(data, &tool2); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if tool2.Name != "get_weather" || tool2.Description != "Get current weather" {
		t.Fatalf("roundtrip failed: %+v", tool2)
	}
}

func TestWebSocketRawCompletion(t *testing.T) {
	wsURL, connCh, server := setupTestWSServer(t)
	defer server.Close()

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	serverConn := <-connCh
	defer serverConn.Close()

	resultCh := make(chan *RawCompletionResponse, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, err := ws.RawCompletion(RawCompletionRequest{
			SystemPrompt: "You are helpful.",
			Message:      "Say hello.",
		})
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- resp
	}()

	msg := readMessage(t, serverConn)
	if msg["type"] != "RawComplete" {
		t.Fatalf("expected RawComplete, got %v", msg["type"])
	}

	payload := msg["payload"].(map[string]interface{})
	if payload["system_prompt"] != "You are helpful." {
		t.Fatalf("expected system_prompt, got %v", payload["system_prompt"])
	}
	if payload["message"] != "Say hello." {
		t.Fatalf("expected message, got %v", payload["message"])
	}

	resp := map[string]interface{}{
		"type": "Success",
		"payload": map[string]interface{}{
			"data": map[string]interface{}{
				"content": "Hello! How can I help?",
			},
		},
	}
	mustWriteJSON(t, serverConn, resp)

	select {
	case result := <-resultCh:
		if result.Content != "Hello! How can I help?" {
			t.Fatalf("expected content, got %v", result.Content)
		}
	case err := <-errCh:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for RawCompletion result")
	}
}

func TestWebSocketRawCompletionWithOptionalFields(t *testing.T) {
	wsURL, connCh, server := setupTestWSServer(t)
	defer server.Close()

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	serverConn := <-connCh
	defer serverConn.Close()

	provider := "openai"
	model := "gpt-4o-mini"
	maxTokens := 512

	resultCh := make(chan *RawCompletionResponse, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, err := ws.RawCompletion(RawCompletionRequest{
			SystemPrompt: "System.",
			Message:      "User.",
			Provider:     &provider,
			Model:        &model,
			MaxTokens:    &maxTokens,
		})
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- resp
	}()

	msg := readMessage(t, serverConn)
	payload := msg["payload"].(map[string]interface{})
	if payload["provider"] != "openai" {
		t.Fatalf("expected provider openai, got %v", payload["provider"])
	}
	if payload["model"] != "gpt-4o-mini" {
		t.Fatalf("expected model gpt-4o-mini, got %v", payload["model"])
	}

	resp := map[string]interface{}{
		"type": "Success",
		"payload": map[string]interface{}{
			"data": map[string]interface{}{"content": "Done."},
		},
	}
	mustWriteJSON(t, serverConn, resp)

	select {
	case result := <-resultCh:
		if result.Content != "Done." {
			t.Fatalf("expected Done., got %v", result.Content)
		}
	case err := <-errCh:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

// =========================================================================
// WS CRUD Tests
// =========================================================================

func TestWebSocketInsert(t *testing.T) {
	wsURL, connCh, server := setupTestWSServer(t)
	defer server.Close()

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	serverConn := <-connCh
	defer serverConn.Close()

	resultCh := make(chan json.RawMessage, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := ws.Insert("users", map[string]interface{}{
			"name": "Alice", "email": "a@b.com",
		})
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()

	msg := readMessage(t, serverConn)
	if msg["type"] != "Insert" {
		t.Fatalf("expected Insert, got %v", msg["type"])
	}
	payload := msg["payload"].(map[string]interface{})
	if payload["collection"] != "users" {
		t.Fatalf("expected collection users, got %v", payload["collection"])
	}
	record := payload["record"].(map[string]interface{})
	if record["name"] != "Alice" {
		t.Fatalf("expected name Alice, got %v", record["name"])
	}

	mustWriteJSON(t, serverConn, map[string]interface{}{
		"type": "Success",
		"payload": map[string]interface{}{
			"message_id": msg["messageId"],
			"data":       map[string]interface{}{"id": "new-1", "name": "Alice", "email": "a@b.com"},
		},
	})

	select {
	case result := <-resultCh:
		var rec map[string]interface{}
		if err := json.Unmarshal(result, &rec); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}
		if rec["id"] != "new-1" {
			t.Fatalf("expected id new-1, got %v", rec["id"])
		}
	case err := <-errCh:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestWebSocketInsertWithBypassRipple(t *testing.T) {
	wsURL, connCh, server := setupTestWSServer(t)
	defer server.Close()

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	serverConn := <-connCh
	defer serverConn.Close()

	errCh := make(chan error, 1)
	go func() {
		_, err := ws.Insert("users", map[string]interface{}{"name": "Bob"}, true)
		errCh <- err
	}()

	msg := readMessage(t, serverConn)
	payload := msg["payload"].(map[string]interface{})
	if payload["bypass_ripple"] != true {
		t.Fatalf("expected bypass_ripple true, got %v", payload["bypass_ripple"])
	}

	mustWriteJSON(t, serverConn, map[string]interface{}{
		"type": "Success",
		"payload": map[string]interface{}{
			"message_id": msg["messageId"],
			"data":       map[string]interface{}{"id": "new-2", "name": "Bob"},
		},
	})

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestWebSocketQuery(t *testing.T) {
	wsURL, connCh, server := setupTestWSServer(t)
	defer server.Close()

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	serverConn := <-connCh
	defer serverConn.Close()

	resultCh := make(chan json.RawMessage, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := ws.Query("users", QueryOptions{
			Limit: 10,
			Sort:  "name",
		})
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()

	msg := readMessage(t, serverConn)
	if msg["type"] != "Query" {
		t.Fatalf("expected Query, got %v", msg["type"])
	}
	payload := msg["payload"].(map[string]interface{})
	if payload["collection"] != "users" {
		t.Fatalf("expected collection users, got %v", payload["collection"])
	}
	if payload["limit"] != float64(10) {
		t.Fatalf("expected limit 10, got %v", payload["limit"])
	}

	mustWriteJSON(t, serverConn, map[string]interface{}{
		"type": "Success",
		"payload": map[string]interface{}{
			"message_id": msg["messageId"],
			"data": []map[string]interface{}{
				{"id": "1", "name": "Alice"},
				{"id": "2", "name": "Bob"},
			},
		},
	})

	select {
	case result := <-resultCh:
		var records []map[string]interface{}
		if err := json.Unmarshal(result, &records); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if len(records) != 2 {
			t.Fatalf("expected 2 records, got %d", len(records))
		}
	case err := <-errCh:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestWebSocketUpdate(t *testing.T) {
	wsURL, connCh, server := setupTestWSServer(t)
	defer server.Close()

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	serverConn := <-connCh
	defer serverConn.Close()

	resultCh := make(chan json.RawMessage, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := ws.Update("users", "u-1", map[string]interface{}{"name": "Alice Updated"})
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()

	msg := readMessage(t, serverConn)
	if msg["type"] != "Update" {
		t.Fatalf("expected Update, got %v", msg["type"])
	}
	payload := msg["payload"].(map[string]interface{})
	if payload["id"] != "u-1" {
		t.Fatalf("expected id u-1, got %v", payload["id"])
	}

	mustWriteJSON(t, serverConn, map[string]interface{}{
		"type": "Success",
		"payload": map[string]interface{}{
			"message_id": msg["messageId"],
			"data":       map[string]interface{}{"id": "u-1", "name": "Alice Updated"},
		},
	})

	select {
	case result := <-resultCh:
		var rec map[string]interface{}
		if err := json.Unmarshal(result, &rec); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if rec["name"] != "Alice Updated" {
			t.Fatalf("expected 'Alice Updated', got %v", rec["name"])
		}
	case err := <-errCh:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestWebSocketDelete(t *testing.T) {
	wsURL, connCh, server := setupTestWSServer(t)
	defer server.Close()

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	serverConn := <-connCh
	defer serverConn.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- ws.Delete("users", "u-1")
	}()

	msg := readMessage(t, serverConn)
	if msg["type"] != "Delete" {
		t.Fatalf("expected Delete, got %v", msg["type"])
	}
	payload := msg["payload"].(map[string]interface{})
	if payload["id"] != "u-1" {
		t.Fatalf("expected id u-1, got %v", payload["id"])
	}

	mustWriteJSON(t, serverConn, map[string]interface{}{
		"type": "Success",
		"payload": map[string]interface{}{
			"message_id": msg["messageId"],
		},
	})

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestWebSocketBatchInsert(t *testing.T) {
	wsURL, connCh, server := setupTestWSServer(t)
	defer server.Close()

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	serverConn := <-connCh
	defer serverConn.Close()

	resultCh := make(chan json.RawMessage, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := ws.BatchInsert("users", []map[string]interface{}{
			{"name": "Alice"},
			{"name": "Bob"},
		})
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()

	msg := readMessage(t, serverConn)
	if msg["type"] != "BatchInsert" {
		t.Fatalf("expected BatchInsert, got %v", msg["type"])
	}

	mustWriteJSON(t, serverConn, map[string]interface{}{
		"type": "Success",
		"payload": map[string]interface{}{
			"message_id": msg["messageId"],
			"data": map[string]interface{}{
				"inserted": 2,
				"ids":      []string{"id-1", "id-2"},
			},
		},
	})

	select {
	case <-resultCh:
		// success
	case err := <-errCh:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestWebSocketCRUDError(t *testing.T) {
	wsURL, connCh, server := setupTestWSServer(t)
	defer server.Close()

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	serverConn := <-connCh
	defer serverConn.Close()

	errCh := make(chan error, 1)
	go func() {
		_, err := ws.FindByID("users", "nonexistent")
		errCh <- err
	}()

	msg := readMessage(t, serverConn)
	if msg["type"] != "FindById" {
		t.Fatalf("expected FindById, got %v", msg["type"])
	}

	mustWriteJSON(t, serverConn, map[string]interface{}{
		"type":    "Error",
		"code":    float64(404),
		"message": "Record not found",
		"payload": map[string]interface{}{
			"message_id": msg["messageId"],
		},
	})

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Record not found") {
			t.Fatalf("expected 'Record not found', got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

// =========================================================================
// SchemaChanged Routing Tests
// =========================================================================

func TestWebSocketSchemaChangedUpdatesCache(t *testing.T) {
	wsURL, connCh, server := setupTestWSServer(t)
	defer server.Close()

	cache := NewSchemaCache(SchemaCacheConfig{
		Enabled:    true,
		MaxEntries: 10,
		TTL:        60 * time.Second,
	})
	cache.Insert("users", "id", 1)

	client := &Client{token: "test-token", schemaCache: cache}
	ws, err := client.WebSocket(wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	serverConn := <-connCh
	defer serverConn.Close()

	// Server pushes SchemaChanged with newer version
	mustWriteJSON(t, serverConn, map[string]interface{}{
		"type": "SchemaChanged",
		"payload": map[string]interface{}{
			"collection":        "users",
			"version":           float64(2),
			"primary_key_alias": "user_id",
		},
	})

	// Poll until the cache is updated (avoids flaky time.Sleep)
	deadline := time.Now().Add(2 * time.Second)
	for {
		if entry := cache.Get("users"); entry != nil && entry.Version == 2 {
			if entry.PrimaryKeyAlias != "user_id" {
				t.Errorf("expected alias 'user_id', got '%s'", entry.PrimaryKeyAlias)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for SchemaChanged to update cache")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestWebSocketSchemaChangedIgnoresOlderVersion(t *testing.T) {
	wsURL, connCh, server := setupTestWSServer(t)
	defer server.Close()

	cache := NewSchemaCache(SchemaCacheConfig{
		Enabled:    true,
		MaxEntries: 10,
		TTL:        60 * time.Second,
	})
	cache.Insert("users", "user_id", 5)

	client := &Client{token: "test-token", schemaCache: cache}
	ws, err := client.WebSocket(wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	serverConn := <-connCh
	defer serverConn.Close()

	// Server pushes SchemaChanged with OLDER version
	mustWriteJSON(t, serverConn, map[string]interface{}{
		"type": "SchemaChanged",
		"payload": map[string]interface{}{
			"collection":        "users",
			"version":           float64(3),
			"primary_key_alias": "id",
		},
	})

	// Can't poll for "nothing changed" — wait briefly for the message to be processed,
	// then verify the cache was not overwritten.
	time.Sleep(50 * time.Millisecond)

	entry := cache.Get("users")
	if entry == nil {
		t.Fatal("expected cache entry")
	}
	if entry.PrimaryKeyAlias != "user_id" {
		t.Errorf("expected alias 'user_id' unchanged, got '%s'", entry.PrimaryKeyAlias)
	}
	if entry.Version != 5 {
		t.Errorf("expected version 5 unchanged, got %d", entry.Version)
	}
}

func TestWebSocketSchemaChangedWithoutCache(t *testing.T) {
	wsURL, connCh, server := setupTestWSServer(t)
	defer server.Close()

	// No schema cache attached
	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	serverConn := <-connCh
	defer serverConn.Close()

	// Should not panic when no cache is attached
	mustWriteJSON(t, serverConn, map[string]interface{}{
		"type": "SchemaChanged",
		"payload": map[string]interface{}{
			"collection":        "users",
			"version":           float64(1),
			"primary_key_alias": "id",
		},
	})

	time.Sleep(20 * time.Millisecond)
	// If we get here without panic, test passes
}

func TestWebSocketRawCompletionError(t *testing.T) {
	wsURL, connCh, server := setupTestWSServer(t)
	defer server.Close()

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	serverConn := <-connCh
	defer serverConn.Close()

	errCh := make(chan error, 1)
	go func() {
		_, err := ws.RawCompletion(RawCompletionRequest{
			SystemPrompt: "System.",
			Message:      "User.",
		})
		errCh <- err
	}()

	readMessage(t, serverConn)

	resp := map[string]interface{}{
		"type":    "Error",
		"message": "Model not found",
	}
	mustWriteJSON(t, serverConn, resp)

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

// =========================================================================
// Auto-reconnect tests (#37)
// =========================================================================

// reconnectTestServer is a mock WS server that accepts multiple upgrades and
// records each incoming connection plus the Authorization/token seen on its
// upgrade request. Used to verify auto-reconnect behaviour.
type reconnectTestServer struct {
	server  *httptest.Server
	wsURL   string
	connCh  chan *websocket.Conn
	authCh  chan string // Authorization header per upgrade
	tokenCh chan string // ?token= query param per upgrade
}

func setupReconnectTestServer(t *testing.T) *reconnectTestServer {
	t.Helper()
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	rts := &reconnectTestServer{
		connCh:  make(chan *websocket.Conn, 8),
		authCh:  make(chan string, 8),
		tokenCh: make(chan string, 8),
	}
	rts.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Record auth context before the upgrade hijacks the request.
		select {
		case rts.authCh <- r.Header.Get("Authorization"):
		default:
		}
		select {
		case rts.tokenCh <- r.URL.Query().Get("token"):
		default:
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		// Every (re)dial begins with the client's Hello; answer it (JSON) so the
		// reconnect path negotiates and proceeds, exactly like the real server.
		performServerHandshake(t, conn, "json")
		rts.connCh <- conn
	}))
	rts.wsURL = "ws" + strings.TrimPrefix(rts.server.URL, "http") + "/api/ws"
	return rts
}

// TestWebSocketReconnectResumesSubscription verifies that a subscription
// survives a mid-stream server disconnect: the client reconnects, re-sends the
// Subscribe request, and the SAME channel delivers a post-reconnect mutation.
func TestWebSocketReconnectResumesSubscription(t *testing.T) {
	rts := setupReconnectTestServer(t)
	defer rts.server.Close()

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(rts.wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	conn1 := <-rts.connCh

	// Subscribe (in a goroutine — it blocks on the server ack).
	subCh := make(chan (<-chan MutationNotification), 1)
	subErr := make(chan error, 1)
	go func() {
		ch, err := ws.Subscribe("orders")
		if err != nil {
			subErr <- err
			return
		}
		subCh <- ch
	}()

	msg := readMessage(t, conn1)
	if msg["type"] != "Subscribe" {
		t.Fatalf("expected Subscribe, got %v", msg["type"])
	}
	mustWriteJSON(t, conn1, map[string]interface{}{
		"type":    "Success",
		"payload": map[string]interface{}{"message_id": msg["messageId"]},
	})

	var notifCh <-chan MutationNotification
	select {
	case notifCh = <-subCh:
	case err := <-subErr:
		t.Fatalf("subscribe failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for subscribe")
	}

	// Deliver one mutation, then drop the connection.
	mustWriteJSON(t, conn1, map[string]interface{}{
		"type": "MutationNotification",
		"payload": map[string]interface{}{
			"collection": "orders",
			"event":      "insert",
			"record_ids": []string{"order-1"},
			"timestamp":  "2026-03-13T00:00:00Z",
		},
	})
	select {
	case n := <-notifCh:
		if n.Event != "insert" || len(n.RecordIDs) != 1 || n.RecordIDs[0] != "order-1" {
			t.Fatalf("unexpected pre-drop notification: %+v", n)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for pre-drop notification")
	}

	// Server drops the connection unexpectedly.
	conn1.Close()

	// Client should reconnect and re-send the Subscribe request.
	var conn2 *websocket.Conn
	select {
	case conn2 = <-rts.connCh:
	case <-time.After(10 * time.Second):
		t.Fatal("client did not reconnect after drop")
	}
	defer conn2.Close()

	resubMsg := readMessage(t, conn2)
	if resubMsg["type"] != "Subscribe" {
		t.Fatalf("expected re-sent Subscribe after reconnect, got %v", resubMsg["type"])
	}
	rpayload := resubMsg["payload"].(map[string]interface{})
	if rpayload["collection"] != "orders" {
		t.Fatalf("expected re-subscribe to orders, got %v", rpayload["collection"])
	}
	// Ack the resubscribe (server normally would).
	mustWriteJSON(t, conn2, map[string]interface{}{
		"type":    "Success",
		"payload": map[string]interface{}{"message_id": resubMsg["messageId"]},
	})

	// A post-reconnect mutation must arrive on the SAME channel.
	mustWriteJSON(t, conn2, map[string]interface{}{
		"type": "MutationNotification",
		"payload": map[string]interface{}{
			"collection": "orders",
			"event":      "update",
			"record_ids": []string{"order-2"},
			"timestamp":  "2026-03-13T00:01:00Z",
		},
	})
	select {
	case n := <-notifCh:
		if n.Event != "update" || len(n.RecordIDs) != 1 || n.RecordIDs[0] != "order-2" {
			t.Fatalf("unexpected post-reconnect notification: %+v", n)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for post-reconnect notification on same channel")
	}
}

// TestWebSocketReconnectDialsWithToken verifies that a reconnect dial carries a
// token (both the Authorization header and the ?token= query param).
func TestWebSocketReconnectDialsWithToken(t *testing.T) {
	rts := setupReconnectTestServer(t)
	defer rts.server.Close()

	client := &Client{token: "secret-jwt"}
	ws, err := client.WebSocket(rts.wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	conn1 := <-rts.connCh

	// Drain the first connection's auth context.
	<-rts.authCh
	<-rts.tokenCh

	// Establish a subscription so reconnect has something to resume.
	subErr := make(chan error, 1)
	go func() {
		_, err := ws.Subscribe("orders")
		subErr <- err
	}()
	msg := readMessage(t, conn1)
	mustWriteJSON(t, conn1, map[string]interface{}{
		"type":    "Success",
		"payload": map[string]interface{}{"message_id": msg["messageId"]},
	})
	if err := <-subErr; err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	// Drop to force a reconnect.
	conn1.Close()

	select {
	case <-rts.connCh:
	case <-time.After(10 * time.Second):
		t.Fatal("client did not reconnect")
	}

	// The reconnect dial must have carried the token.
	select {
	case auth := <-rts.authCh:
		if auth != "Bearer secret-jwt" {
			t.Fatalf("expected reconnect Authorization 'Bearer secret-jwt', got %q", auth)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no Authorization header captured for reconnect dial")
	}
	select {
	case tok := <-rts.tokenCh:
		if tok != "secret-jwt" {
			t.Fatalf("expected reconnect token query 'secret-jwt', got %q", tok)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no token query param captured for reconnect dial")
	}
}

// TestWebSocketCloseStopsReconnection verifies that Close() halts reconnection:
// after Close, the server sees no further dial attempts even though a
// subscription was active when the connection dropped.
func TestWebSocketCloseStopsReconnection(t *testing.T) {
	rts := setupReconnectTestServer(t)
	defer rts.server.Close()

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(rts.wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}

	conn1 := <-rts.connCh

	// Establish a subscription.
	subErr := make(chan error, 1)
	go func() {
		_, err := ws.Subscribe("orders")
		subErr <- err
	}()
	msg := readMessage(t, conn1)
	mustWriteJSON(t, conn1, map[string]interface{}{
		"type":    "Success",
		"payload": map[string]interface{}{"message_id": msg["messageId"]},
	})
	if err := <-subErr; err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	// Intentional close — must not trigger a reconnect.
	if err := ws.Close(); err != nil {
		t.Fatalf("close returned error: %v", err)
	}
	conn1.Close()

	// No new connection should arrive after an intentional Close.
	select {
	case c := <-rts.connCh:
		c.Close()
		t.Fatal("client attempted to reconnect after Close()")
	case <-time.After(2 * time.Second):
		// Expected: no reconnect.
	}
}

// TestWebSocketUnsubscribeRaceWithDelivery exercises the deliver-vs-close race:
// the server streams mutation notifications while the client Unsubscribes (which
// closes the subscription channel). Before the fix, the dispatcher sent on the
// channel outside the lock and could panic with "send on closed channel". Run
// under `-race` (and `-count`) this is a regression guard for #37 (#5/#6).
func TestWebSocketUnsubscribeRaceWithDelivery(t *testing.T) {
	rts := setupReconnectTestServer(t)
	defer rts.server.Close()

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(rts.wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	conn := <-rts.connCh

	subCh := make(chan (<-chan MutationNotification), 1)
	go func() {
		ch, err := ws.Subscribe("orders")
		if err == nil {
			subCh <- ch
		}
	}()
	msg := readMessage(t, conn)
	mustWriteJSON(t, conn, map[string]interface{}{
		"type":    "Success",
		"payload": map[string]interface{}{"message_id": msg["messageId"]},
	})

	var notifCh <-chan MutationNotification
	select {
	case notifCh = <-subCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for subscribe ack")
	}

	// Server streams notifications continuously until told to stop.
	stop := make(chan struct{})
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		payload := map[string]interface{}{
			"type": "MutationNotification",
			"payload": map[string]interface{}{
				"collection": "orders", "event": "insert",
				"record_ids": []string{"x"}, "timestamp": "2026-03-13T00:00:00Z",
			},
		}
		for {
			select {
			case <-stop:
				return
			default:
			}
			if err := conn.WriteJSON(payload); err != nil {
				return
			}
		}
	}()

	// A consumer drains so most deliveries take the send branch (maximizing the
	// window the old code raced on), then we Unsubscribe mid-stream.
	go func() {
		for range notifCh {
		}
	}()
	time.Sleep(30 * time.Millisecond)
	ws.Unsubscribe("orders")
	time.Sleep(30 * time.Millisecond)

	close(stop)
	<-writerDone
	// Reaching here without a panic means deliver-vs-close is safe.
}

// TestWebSocketNoReconnectWithoutSubscriptions guards that an unexpected drop
// with no active subscriptions is terminal — the client must NOT spin a
// background reconnect loop (which would add latency to one-shot/chat WS flows).
func TestWebSocketNoReconnectWithoutSubscriptions(t *testing.T) {
	rts := setupReconnectTestServer(t)
	defer rts.server.Close()

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(rts.wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	conn := <-rts.connCh
	// No Subscribe — drop the connection unexpectedly.
	conn.Close()

	select {
	case c := <-rts.connCh:
		c.Close()
		t.Fatal("client reconnected despite having no active subscriptions")
	case <-time.After(2 * time.Second):
		// Expected: no reconnect.
	}
}

// TestWebSocketUnsubscribeSendsServerFrame verifies that Unsubscribe sends a
// best-effort Unsubscribe frame to the server (so the server stops streaming),
// not just a local teardown.
func TestWebSocketUnsubscribeSendsServerFrame(t *testing.T) {
	rts := setupReconnectTestServer(t)
	defer rts.server.Close()

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(rts.wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	conn := <-rts.connCh

	subErr := make(chan error, 1)
	go func() {
		_, e := ws.Subscribe("orders")
		subErr <- e
	}()
	msg := readMessage(t, conn)
	if msg["type"] != "Subscribe" {
		t.Fatalf("expected Subscribe, got %v", msg["type"])
	}
	mustWriteJSON(t, conn, map[string]interface{}{
		"type":    "Success",
		"payload": map[string]interface{}{"message_id": msg["messageId"]},
	})
	if e := <-subErr; e != nil {
		t.Fatalf("subscribe failed: %v", e)
	}

	ws.Unsubscribe("orders")

	msg = readMessage(t, conn)
	if msg["type"] != "Unsubscribe" {
		t.Fatalf("expected Unsubscribe frame, got %v", msg["type"])
	}
	payload, ok := msg["payload"].(map[string]interface{})
	if !ok || payload["collection"] != "orders" {
		t.Fatalf("expected Unsubscribe payload collection=orders, got %v", msg["payload"])
	}
}

// TestWebSocketReconnectExitsWhenSubscriptionsRemovedDuringBackoff covers the
// case where a drop hands off to reconnect() WITH an active subscription, but
// that subscription is removed (e.g. an in-flight Subscribe failed and deleted
// it) before reconnect dials. reconnect() must exit instead of reviving a
// zombie connection with nothing to replay.
func TestWebSocketReconnectExitsWhenSubscriptionsRemovedDuringBackoff(t *testing.T) {
	rts := setupReconnectTestServer(t)
	defer rts.server.Close()

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(rts.wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	conn1 := <-rts.connCh

	subErr := make(chan error, 1)
	go func() {
		_, e := ws.Subscribe("orders")
		subErr <- e
	}()
	msg := readMessage(t, conn1)
	if msg["type"] != "Subscribe" {
		t.Fatalf("expected Subscribe, got %v", msg["type"])
	}
	mustWriteJSON(t, conn1, map[string]interface{}{
		"type":    "Success",
		"payload": map[string]interface{}{"message_id": msg["messageId"]},
	})
	if e := <-subErr; e != nil {
		t.Fatalf("subscribe failed: %v", e)
	}

	// Drop from the server side; with an active subscription the client hands
	// off to reconnect(), which starts a ~200ms backoff before its first dial.
	_ = conn1.Close()

	// Deterministically wait until readLoop has observed the drop and flipped
	// the reconnecting flag (set under ws.mu before reconnect() is spawned),
	// rather than racing a fixed sleep. Once it's set, reconnect() is in its
	// pre-dial backoff window; removing the only subscription now forces its
	// pre-dial check to see zero subscriptions and exit without dialing.
	deadline := time.Now().Add(2 * time.Second)
	for {
		ws.mu.Lock()
		reconnecting := ws.reconnecting
		ws.mu.Unlock()
		if reconnecting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for reconnect loop to start")
		}
		time.Sleep(time.Millisecond)
	}
	ws.Unsubscribe("orders")

	select {
	case c := <-rts.connCh:
		c.Close()
		t.Fatal("reconnect dialed despite all subscriptions being removed during backoff")
	case <-time.After(2 * time.Second):
		// Expected: reconnect exited without dialing.
	}
}

// TestWebSocketConnectFallsBackToBackgroundContext verifies connect() handles a
// nil ws.ctx (e.g. a manually constructed client): DialContext panics on a nil
// context, so connect() must initialize a cancelable context. It must also leave
// the client in a consistent state — Close() (which calls ws.cancel()) must not
// panic afterward.
func TestWebSocketConnectFallsBackToBackgroundContext(t *testing.T) {
	rts := setupReconnectTestServer(t)
	defer rts.server.Close()

	// A bare client with NO context set, mimicking a manually constructed one.
	// connect() reads only wsURL + tokenProvider; ctx is left nil on purpose.
	ws := &WebSocketClient{
		wsURL:         rts.wsURL,
		tokenProvider: func() string { return "test-token" },
	}

	// Must not panic on the nil context.
	if err := ws.connect(); err != nil {
		t.Fatalf("connect() with nil ctx should succeed: %v", err)
	}

	// connect() must have initialized ws.ctx and ws.cancel so later operations
	// don't panic on nil.
	if ws.ctx == nil || ws.cancel == nil {
		t.Fatal("connect() must initialize ws.ctx and ws.cancel when they were nil")
	}

	// The dial reached the server and connect() stored a usable conn.
	select {
	case c := <-rts.connCh:
		_ = c.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("dial did not reach the server")
	}

	// Close() calls ws.cancel(); it must not panic now that connect() set it.
	if err := ws.Close(); err != nil {
		t.Fatalf("Close() after nil-ctx connect should not error: %v", err)
	}
}

// TestWebSocketReconnectClosesSocketWhenSubsRemovedDuringDial covers the race the
// pre-dial check cannot: the last subscription is removed WHILE connect() is
// dialing (after the pre-dial check already passed). The just-opened socket then
// has nothing to replay, so reconnect() must tear it down rather than leak a
// zombie connection + readLoop.
func TestWebSocketReconnectClosesSocketWhenSubsRemovedDuringDial(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	connCh := make(chan *websocket.Conn, 8)
	dialStarted := make(chan struct{}, 1)
	proceed := make(chan struct{})
	var dialN int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Gate only the SECOND dial (the reconnect). The first dial (the initial
		// connect) proceeds immediately so the subscription can be established.
		if atomic.AddInt32(&dialN, 1) == 2 {
			dialStarted <- struct{}{}
			<-proceed
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		performServerHandshake(t, conn, "json")
		connCh <- conn
	}))
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/ws"

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	conn1 := <-connCh

	// Establish a subscription so a server-side drop hands off to reconnect().
	subErr := make(chan error, 1)
	go func() {
		_, e := ws.Subscribe("orders")
		subErr <- e
	}()
	msg := readMessage(t, conn1)
	if msg["type"] != "Subscribe" {
		t.Fatalf("expected Subscribe, got %v", msg["type"])
	}
	mustWriteJSON(t, conn1, map[string]interface{}{
		"type":    "Success",
		"payload": map[string]interface{}{"message_id": msg["messageId"]},
	})
	if e := <-subErr; e != nil {
		t.Fatalf("subscribe failed: %v", e)
	}

	// Drop server-side; with an active subscription the client hands off to
	// reconnect(), whose pre-dial check still sees the subscription and dials.
	_ = conn1.Close()

	// Once the reconnect dial is in flight (pre-dial check already passed),
	// remove the only subscription so the POST-dial check is the one that must
	// catch the now-empty set, then let the dial complete.
	select {
	case <-dialStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("reconnect did not dial")
	}
	ws.Unsubscribe("orders")
	close(proceed)

	// The reconnect socket must be torn down (nothing to replay). A teardown
	// closes the socket, so the server's read returns a non-timeout error
	// promptly; a leaked zombie would stay open and the read would block until
	// the deadline (a timeout).
	conn2 := <-connCh
	_ = conn2.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, readErr := conn2.ReadMessage()
	if readErr == nil {
		t.Fatal("reconnect unexpectedly replayed onto the socket")
	}
	var ne net.Error
	if errors.As(readErr, &ne) && ne.Timeout() {
		t.Fatal("reconnect left the socket open (zombie connection): the read timed " +
			"out waiting for data instead of observing a client-side close")
	}
}

// newRoutingTestClient builds a bare WebSocketClient with just the maps needed
// to drive routeRequestResponse directly (no live socket).
func newRoutingTestClient() *WebSocketClient {
	return &WebSocketClient{
		pendingRequests: make(map[string]chan wsResponse),
		subscriptions:   make(map[string]chan MutationNotification),
		subParams:       make(map[string]SubscribeOptions),
		chatStreams:     make(map[string]chan ChatStreamEvent),
	}
}

// rawMsg marshals a JSON object into the map[string]json.RawMessage shape that
// routeRequestResponse consumes.
func rawMsg(t *testing.T, obj map[string]interface{}) map[string]json.RawMessage {
	t.Helper()
	b, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

// TestRouteRequestResponseDoesNotMisrouteUnmatchedID verifies that when a
// Success carries a message id that is PRESENT but does not match any pending
// request, the single-pending-request fallback is suppressed — the stray ack
// must not be delivered to an unrelated in-flight request. This covers the
// Unsubscribe ack case (a present id that matches nothing) and a parseable but
// unmatched id (a late ack for an already-settled request).
func TestRouteRequestResponseDoesNotMisrouteUnmatchedID(t *testing.T) {
	cases := []struct {
		name string
		msg  map[string]interface{}
	}{
		{
			name: "present unmatched id top-level",
			msg: map[string]interface{}{
				"type":      "Success",
				"messageId": "unsub-999",
				"payload":   map[string]interface{}{"ok": true},
			},
		},
		{
			name: "present unmatched id in payload",
			msg: map[string]interface{}{
				"type":    "Success",
				"payload": map[string]interface{}{"message_id": "unsub-999"},
			},
		},
		{
			name: "present but malformed (numeric) id",
			msg: map[string]interface{}{
				"type":      "Success",
				"messageId": 12345,
				"payload":   map[string]interface{}{"ok": true},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ws := newRoutingTestClient()
			pending := make(chan wsResponse, 1)
			ws.pendingRequests["the-real-request"] = pending

			ws.routeRequestResponse("Success", rawMsg(t, tc.msg))

			select {
			case <-pending:
				t.Fatal("stray ack was misrouted to the unrelated pending request")
			default:
				// Expected: the real request is untouched.
			}
			ws.mu.Lock()
			_, stillPending := ws.pendingRequests["the-real-request"]
			ws.mu.Unlock()
			if !stillPending {
				t.Fatal("the real request was incorrectly settled")
			}
		})
	}
}

// TestRouteRequestResponseSinglePendingFallback verifies the legitimate
// fallback still works: when the server echoes NO message id anywhere and
// exactly one request is pending, the response is delivered to it.
func TestRouteRequestResponseSinglePendingFallback(t *testing.T) {
	ws := newRoutingTestClient()
	pending := make(chan wsResponse, 1)
	ws.pendingRequests["only-request"] = pending

	ws.routeRequestResponse("Success", rawMsg(t, map[string]interface{}{
		"type":    "Success",
		"payload": map[string]interface{}{"ok": true},
	}))

	select {
	case <-pending:
		// Expected: sequential request/response delivered.
	case <-time.After(time.Second):
		t.Fatal("single-pending fallback did not deliver the response")
	}
	ws.mu.Lock()
	_, stillPending := ws.pendingRequests["only-request"]
	ws.mu.Unlock()
	if stillPending {
		t.Fatal("the request should have been settled and removed")
	}
}

// TestRouteRequestResponseMatchedID verifies an id that DOES match a pending
// request is delivered to exactly that request even when others are pending.
func TestRouteRequestResponseMatchedID(t *testing.T) {
	ws := newRoutingTestClient()
	want := make(chan wsResponse, 1)
	other := make(chan wsResponse, 1)
	ws.pendingRequests["req-A"] = want
	ws.pendingRequests["req-B"] = other

	ws.routeRequestResponse("Success", rawMsg(t, map[string]interface{}{
		"type":      "Success",
		"messageId": "req-A",
		"payload":   map[string]interface{}{"ok": true},
	}))

	select {
	case <-want:
		// Expected.
	case <-time.After(time.Second):
		t.Fatal("matched id was not delivered to its request")
	}
	select {
	case <-other:
		t.Fatal("response leaked to the wrong pending request")
	default:
	}
}

// TestWebSocketBinaryNegotiation drives the full binary path end-to-end: the
// server Welcomes msgpack, the client must then send its request as a binary
// msgpack frame, and a binary msgpack response must decode back into records
// transparently. This is the Go mirror of the server's
// test_ws_binary_msgpack_round_trip.
func TestWebSocketBinaryNegotiation(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	connCh := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		performServerHandshake(t, conn, "msgpack")
		connCh <- conn
	}))
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/ws"

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	// After a Welcome{msgpack}, the connection must be in binary mode.
	if !ws.binary.Load() {
		t.Fatal("client did not negotiate binary (msgpack) mode after Welcome")
	}

	serverConn := <-connCh
	defer serverConn.Close()

	resultCh := make(chan []Record, 1)
	errCh := make(chan error, 1)
	go func() {
		records, err := ws.FindAll("users")
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- records
	}()

	// The request must arrive as a binary msgpack frame.
	mt, data, err := serverConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read request: %v", err)
	}
	if mt != websocket.BinaryMessage {
		t.Fatalf("expected a binary request frame, got message type %d", mt)
	}
	var req map[string]interface{}
	if err := msgpack.Unmarshal(data, &req); err != nil {
		t.Fatalf("request frame is not valid msgpack: %v", err)
	}
	if req["type"] != "FindAll" {
		t.Fatalf("expected FindAll, got %v", req["type"])
	}

	// Respond with a binary msgpack Success frame.
	resp := map[string]interface{}{
		"type": "Success",
		"payload": map[string]interface{}{
			"message_id": req["messageId"],
			"data": []map[string]interface{}{
				{"id": "1", "name": "Alice"},
			},
		},
	}
	respBytes, err := msgpack.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal msgpack response: %v", err)
	}
	if err := serverConn.WriteMessage(websocket.BinaryMessage, respBytes); err != nil {
		t.Fatalf("failed to write msgpack response: %v", err)
	}

	select {
	case records := <-resultCh:
		if len(records) != 1 {
			t.Fatalf("expected 1 record, got %d", len(records))
		}
		if records[0]["name"] != "Alice" {
			t.Fatalf("expected name Alice, got %v", records[0]["name"])
		}
	case err := <-errCh:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for binary FindAll result")
	}
}

// TestWebSocketJSONStaysJSONWhenServerDeclinesMsgpack proves back-compat: a
// server that Welcomes only "json" (or, like an older server, never Welcomes
// at all and the read times out / errors) leaves the connection on JSON text.
func TestWebSocketJSONStaysJSONWhenServerDeclinesMsgpack(t *testing.T) {
	wsURL, connCh, server := setupTestWSServer(t) // Welcomes "json"
	defer server.Close()

	client := &Client{token: "test-token"}
	ws, err := client.WebSocket(wsURL)
	if err != nil {
		t.Fatalf("failed to create WebSocket client: %v", err)
	}
	defer ws.Close()

	if ws.binary.Load() {
		t.Fatal("client must stay on JSON when the server does not Welcome msgpack")
	}

	serverConn := <-connCh
	defer serverConn.Close()

	errCh := make(chan error, 1)
	go func() {
		_, err := ws.FindAll("users")
		errCh <- err
	}()

	// The request must arrive as a JSON text frame, not binary.
	mt, _, err := serverConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read request: %v", err)
	}
	if mt != websocket.TextMessage {
		t.Fatalf("expected a JSON text request frame, got message type %d", mt)
	}
	// Drain the in-flight call so the goroutine doesn't leak.
	_ = serverConn.Close()
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
	}
}

// TestMsgpackToJSONTranscode verifies the binary->JSON transcode is
// value-identical to the JSON wire shape, including the key edge case: a
// msgpack bin (a Binary field) must become a number array, not base64, so
// decoded data is the same regardless of negotiated transport.
func TestMsgpackToJSONTranscode(t *testing.T) {
	source := map[string]interface{}{
		"type": "Success",
		"payload": map[string]interface{}{
			"message_id": "m1",
			"data": map[string]interface{}{
				"name":   "Alice",
				"count":  42,
				"active": true,
				"photo":  []byte{255, 216, 255},
				"tags":   []interface{}{"x", "y"},
			},
		},
	}
	packed, err := msgpack.Marshal(source)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	jsonBytes, err := msgpackToJSON(packed)
	if err != nil {
		t.Fatalf("transcode: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &got); err != nil {
		t.Fatalf("transcoded output is not valid JSON: %v", err)
	}
	if got["type"] != "Success" {
		t.Fatalf("type lost in transcode: %v", got["type"])
	}
	payload := got["payload"].(map[string]interface{})
	data := payload["data"].(map[string]interface{})
	if data["name"] != "Alice" {
		t.Fatalf("string field lost: %v", data["name"])
	}
	if data["active"] != true {
		t.Fatalf("bool field lost: %v", data["active"])
	}
	// Binary must be a number array matching the server's JSON wire shape, not
	// a base64 string.
	photo, ok := data["photo"].([]interface{})
	if !ok {
		t.Fatalf("binary field is not a number array: %T = %v", data["photo"], data["photo"])
	}
	if len(photo) != 3 || photo[0].(float64) != 255 || photo[1].(float64) != 216 || photo[2].(float64) != 255 {
		t.Fatalf("binary field bytes wrong: %v", photo)
	}
}
