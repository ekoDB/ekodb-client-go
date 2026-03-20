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
	serverConn.WriteJSON(resp)

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
	serverConn.WriteJSON(resp)

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
	serverConn.WriteJSON(map[string]interface{}{
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
	serverConn.WriteJSON(map[string]interface{}{
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
	serverConn.WriteJSON(map[string]interface{}{
		"type":    "ChatStreamChunk",
		"payload": map[string]interface{}{"chat_id": "chat-1", "content": "Hi "},
	})
	serverConn.WriteJSON(map[string]interface{}{
		"type":    "ChatStreamChunk",
		"payload": map[string]interface{}{"chat_id": "chat-1", "content": "there!"},
	})
	serverConn.WriteJSON(map[string]interface{}{
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

	serverConn.WriteJSON(map[string]interface{}{
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
	serverConn.WriteJSON(map[string]interface{}{
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
	serverConn.WriteJSON(map[string]interface{}{
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
	serverConn.WriteJSON(map[string]interface{}{
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
	serverConn.WriteJSON(map[string]interface{}{
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
	serverConn.WriteJSON(resp)

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
	serverConn.WriteJSON(resp)

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
	serverConn.WriteJSON(resp)

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}
