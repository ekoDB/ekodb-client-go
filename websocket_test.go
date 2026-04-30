package ekodb

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

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
