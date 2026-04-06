package ekodb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// ============================================================================
// Chat Message Stream Tests
// ============================================================================

func TestExecuteToolSuccess(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/tools/execute": func(w http.ResponseWriter, r *http.Request) {
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("Failed to decode request body: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			if body["tool"] != "count_records" {
				t.Errorf("Expected tool count_records, got %v", body["tool"])
			}

			// Verify params are sent correctly
			params, ok := body["params"].(map[string]interface{})
			if !ok {
				t.Errorf("Expected params to be a map, got %T", body["params"])
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if params["collection"] != "users" {
				t.Errorf("Expected params.collection=users, got %v", params["collection"])
			}

			// Verify chat_id is omitted when empty
			if _, exists := body["chat_id"]; exists {
				t.Errorf("Expected chat_id to be omitted when empty, but it was present: %v", body["chat_id"])
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"result":  map[string]interface{}{"count": 42},
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.ExecuteTool("count_records", map[string]interface{}{"collection": "users"}, "")
	if err != nil {
		t.Fatalf("ExecuteTool failed: %v", err)
	}
	if result["count"] != float64(42) {
		t.Errorf("Expected count 42, got %v", result["count"])
	}
}

func TestExecuteToolWithChatID(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/tools/execute": func(w http.ResponseWriter, r *http.Request) {
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("Failed to decode request body: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			if body["chat_id"] != "chat_456" {
				t.Errorf("Expected chat_id chat_456, got %v", body["chat_id"])
			}

			// Verify params are sent correctly
			params, ok := body["params"].(map[string]interface{})
			if !ok {
				t.Errorf("Expected params to be a map, got %T", body["params"])
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if params["key"] != "greeting" {
				t.Errorf("Expected params.key=greeting, got %v", params["key"])
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"result":  map[string]interface{}{"value": "hello"},
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.ExecuteTool("kv_get", map[string]interface{}{"key": "greeting"}, "chat_456")
	if err != nil {
		t.Fatalf("ExecuteTool failed: %v", err)
	}
	if result["value"] != "hello" {
		t.Errorf("Expected value hello, got %v", result["value"])
	}
}

func TestExecuteToolFailure(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/tools/execute": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "permission denied",
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.ExecuteTool("delete_collection", map[string]interface{}{"collection": "system"}, "")
	if err == nil {
		t.Fatal("Expected error for failed tool execution")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("Expected permission denied error, got %v", err)
	}
}

func TestExecuteToolNotFound(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/tools/execute": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("Not Found"))
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.ExecuteTool("count_records", map[string]interface{}{"collection": "users"}, "")
	if err != nil {
		t.Fatalf("Expected nil error for 404, got %v", err)
	}
	if result != nil {
		t.Errorf("Expected nil result for 404, got %v", result)
	}
}

func TestExecuteToolMethodNotAllowed(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/tools/execute": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_, _ = w.Write([]byte("Method Not Allowed"))
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.ExecuteTool("count_records", map[string]interface{}{"collection": "users"}, "")
	if err != nil {
		t.Fatalf("Expected nil error for 405, got %v", err)
	}
	if result != nil {
		t.Errorf("Expected nil result for 405, got %v", result)
	}
}

func TestChatMessageStream(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/session_1/messages/stream": func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Accept") != "text/event-stream" {
				t.Errorf("Expected Accept: text/event-stream, got %s", r.Header.Get("Accept"))
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Fatal("ResponseWriter does not support Flusher")
			}

			// Send chunk events
			fmt.Fprintf(w, "data:{\"token\":\"Hello\"}\n\n")
			flusher.Flush()
			fmt.Fprintf(w, "data:{\"token\":\" world\"}\n\n")
			flusher.Flush()
			// Send end event
			fmt.Fprintf(w, "data:{\"content\":\"Hello world\",\"message_id\":\"msg_1\",\"execution_time_ms\":42}\n\n")
			flusher.Flush()
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	ch, err := client.ChatMessageStream("session_1", ChatMessageRequest{Message: "Hi"})
	if err != nil {
		t.Fatalf("ChatMessageStream failed: %v", err)
	}

	var chunks []string
	var endEvent *ChatStreamEvent
	for event := range ch {
		switch event.Type {
		case "chunk":
			chunks = append(chunks, event.Content)
		case "end":
			endEvent = &event
		case "error":
			t.Fatalf("Unexpected error event: %s", event.Error)
		}
	}

	if len(chunks) != 2 {
		t.Errorf("Expected 2 chunks, got %d", len(chunks))
	}
	if chunks[0] != "Hello" {
		t.Errorf("Expected first chunk 'Hello', got '%s'", chunks[0])
	}
	if chunks[1] != " world" {
		t.Errorf("Expected second chunk ' world', got '%s'", chunks[1])
	}
	if endEvent == nil {
		t.Fatal("Expected end event")
	}
	if endEvent.MessageID != "msg_1" {
		t.Errorf("Expected message_id msg_1, got %s", endEvent.MessageID)
	}
	if endEvent.ExecutionTimeMs != 42 {
		t.Errorf("Expected execution_time_ms 42, got %d", endEvent.ExecutionTimeMs)
	}
}

func TestChatMessageStreamError(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/session_1/messages/stream": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			fmt.Fprintf(w, "data:{\"error\":\"Model unavailable\"}\n\n")
			flusher.Flush()
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	ch, err := client.ChatMessageStream("session_1", ChatMessageRequest{Message: "Hi"})
	if err != nil {
		t.Fatalf("ChatMessageStream failed: %v", err)
	}

	var gotError bool
	for event := range ch {
		if event.Type == "error" {
			gotError = true
			if event.Error != "Model unavailable" {
				t.Errorf("Expected error 'Model unavailable', got '%s'", event.Error)
			}
		}
	}
	if !gotError {
		t.Fatal("Expected error event")
	}
}

func TestChatMessageStreamHTTPError(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/session_1/messages/stream": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("Internal Server Error"))
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.ChatMessageStream("session_1", ChatMessageRequest{Message: "Hi"})
	if err == nil {
		t.Fatal("Expected error for HTTP 500")
	}
}

// ============================================================================
// Raw Completion Stream With Progress Tests
// ============================================================================

func TestRawCompletionStreamWithProgress(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/complete/stream": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			fmt.Fprintf(w, "data:{\"token\":\"The \"}\n\n")
			flusher.Flush()
			fmt.Fprintf(w, "data:{\"token\":\"answer \"}\n\n")
			flusher.Flush()
			fmt.Fprintf(w, "data:{\"token\":\"is 42\"}\n\n")
			flusher.Flush()
			fmt.Fprintf(w, "data:{\"content\":\"The answer is 42\"}\n\n")
			flusher.Flush()
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	var tokens []string
	result, err := client.RawCompletionStreamWithProgress(RawCompletionRequest{
		SystemPrompt: "You are a test assistant.",
		Message:      "What is the answer?",
	}, func(token string) {
		tokens = append(tokens, token)
	})
	if err != nil {
		t.Fatalf("RawCompletionStreamWithProgress failed: %v", err)
	}

	if len(tokens) != 3 {
		t.Errorf("Expected 3 tokens, got %d", len(tokens))
	}
	if result.Content != "The answer is 42" {
		t.Errorf("Expected content 'The answer is 42', got '%s'", result.Content)
	}
}

func TestRawCompletionStreamWithProgressError(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/complete/stream": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			fmt.Fprintf(w, "data:{\"token\":\"partial\"}\n\n")
			flusher.Flush()
			fmt.Fprintf(w, "data:{\"error\":\"context length exceeded\"}\n\n")
			flusher.Flush()
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	var tokens []string
	_, err := client.RawCompletionStreamWithProgress(RawCompletionRequest{
		SystemPrompt: "Test",
		Message:      "Test",
	}, func(token string) {
		tokens = append(tokens, token)
	})
	if err == nil {
		t.Fatal("Expected error for LLM error event")
	}
	if len(tokens) != 1 {
		t.Errorf("Expected 1 token before error, got %d", len(tokens))
	}
}

func TestRawCompletionStreamWithProgressNilCallback(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/complete/stream": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			fmt.Fprintf(w, "data:{\"token\":\"OK\"}\n\n")
			flusher.Flush()
			fmt.Fprintf(w, "data:{\"content\":\"OK\"}\n\n")
			flusher.Flush()
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.RawCompletionStreamWithProgress(RawCompletionRequest{
		SystemPrompt: "Test",
		Message:      "Test",
	}, nil)
	if err != nil {
		t.Fatalf("RawCompletionStreamWithProgress with nil callback failed: %v", err)
	}
	if result.Content != "OK" {
		t.Errorf("Expected content 'OK', got '%s'", result.Content)
	}
}

// ============================================================================
// User Function CRUD Tests
// ============================================================================

func TestSaveUserFunction(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/functions": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "created", "id": "fn_1",
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	id, err := client.SaveUserFunction(UserFunction{
		Label: "my_fn",
		Name:  "My Function",
		Functions: []FunctionStageConfig{
			StageFindAll("users"),
		},
	})
	if err != nil {
		t.Fatalf("SaveUserFunction failed: %v", err)
	}
	if id != "fn_1" {
		t.Errorf("Expected id fn_1, got %s", id)
	}
}

func TestGetUserFunction(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/functions/my_fn": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"label": "my_fn", "name": "My Function",
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	fn, err := client.GetUserFunction("my_fn")
	if err != nil {
		t.Fatalf("GetUserFunction failed: %v", err)
	}
	if fn.Label != "my_fn" {
		t.Errorf("Expected label my_fn, got %s", fn.Label)
	}
	if fn.Name != "My Function" {
		t.Errorf("Expected name 'My Function', got '%s'", fn.Name)
	}
}

func TestListUserFunctions(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/functions": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"label": "fn_a", "name": "Function A"},
				{"label": "fn_b", "name": "Function B"},
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	fns, err := client.ListUserFunctions(nil)
	if err != nil {
		t.Fatalf("ListUserFunctions failed: %v", err)
	}
	if len(fns) != 2 {
		t.Errorf("Expected 2 functions, got %d", len(fns))
	}
}

func TestListUserFunctionsWithTags(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/functions*": func(w http.ResponseWriter, r *http.Request) {
			tags := r.URL.Query().Get("tags")
			if tags == "" {
				t.Error("Expected tags parameter")
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"label": "fn_a", "name": "Function A"},
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	fns, err := client.ListUserFunctions([]string{"etl", "cron"})
	if err != nil {
		t.Fatalf("ListUserFunctions with tags failed: %v", err)
	}
	if len(fns) != 1 {
		t.Errorf("Expected 1 function, got %d", len(fns))
	}
}

func TestUpdateUserFunction(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"PUT /api/functions/my_fn": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	err := client.UpdateUserFunction("my_fn", UserFunction{
		Label: "my_fn",
		Name:  "Updated Function",
		Functions: []FunctionStageConfig{
			StageFindAll("orders"),
		},
	})
	if err != nil {
		t.Fatalf("UpdateUserFunction failed: %v", err)
	}
}

func TestDeleteUserFunction(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"DELETE /api/functions/my_fn": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	err := client.DeleteUserFunction("my_fn")
	if err != nil {
		t.Fatalf("DeleteUserFunction failed: %v", err)
	}
}

func TestGetUserFunctionNotFound(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/functions/nonexistent": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("Not Found"))
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.GetUserFunction("nonexistent")
	if err == nil {
		t.Fatal("Expected error for non-existent function")
	}
}

// ============================================================================
// agent_id Tests
// ============================================================================

func TestCreateChatSessionRequestAgentID(t *testing.T) {
	agentName := "my-agent"
	req := CreateChatSessionRequest{
		Collections: []CollectionConfig{{CollectionName: "docs"}},
		LLMProvider: "openai",
		AgentID:     &agentName,
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	if m["agent_id"] != "my-agent" {
		t.Errorf("Expected agent_id=my-agent, got %v", m["agent_id"])
	}
}

func TestCreateChatSessionRequestAgentIDOmitted(t *testing.T) {
	req := CreateChatSessionRequest{
		Collections: []CollectionConfig{{CollectionName: "docs"}},
		LLMProvider: "openai",
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	if _, exists := m["agent_id"]; exists {
		t.Error("Expected agent_id to be omitted when nil")
	}
}

func TestChatSessionAgentIDDeserialization(t *testing.T) {
	raw := `{"chat_id":"c1","created_at":"2026-01-01","updated_at":"2026-01-01","llm_provider":"openai","llm_model":"gpt-4","collections":[],"agent_id":"bot-1","message_count":0}`
	var session ChatSession
	if err := json.Unmarshal([]byte(raw), &session); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	if session.AgentID == nil || *session.AgentID != "bot-1" {
		t.Errorf("Expected agent_id=bot-1, got %v", session.AgentID)
	}
}

func TestChatSessionAgentIDMissing(t *testing.T) {
	raw := `{"chat_id":"c1","created_at":"2026-01-01","updated_at":"2026-01-01","llm_provider":"openai","llm_model":"gpt-4","collections":[],"message_count":0}`
	var session ChatSession
	if err := json.Unmarshal([]byte(raw), &session); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	if session.AgentID != nil {
		t.Error("Expected agent_id to be nil when absent")
	}
}

// ============================================================================
// client_tools / confirm_tools / exclude_tools Tests
// ============================================================================

func TestChatMessageRequestClientTools(t *testing.T) {
	req := ChatMessageRequest{
		Message: "hello",
		ClientTools: []ClientToolDef{
			{Name: "weather", Description: "Get weather", Parameters: map[string]interface{}{"type": "object"}},
		},
		ConfirmTools: []string{"shell_exec"},
		ExcludeTools: []string{"file_delete"},
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	tools := m["client_tools"].([]interface{})
	if len(tools) != 1 {
		t.Fatalf("Expected 1 client tool, got %d", len(tools))
	}
	tool := tools[0].(map[string]interface{})
	if tool["name"] != "weather" {
		t.Errorf("Expected tool name=weather, got %v", tool["name"])
	}
	confirm := m["confirm_tools"].([]interface{})
	if len(confirm) != 1 || confirm[0] != "shell_exec" {
		t.Errorf("Unexpected confirm_tools: %v", confirm)
	}
	exclude := m["exclude_tools"].([]interface{})
	if len(exclude) != 1 || exclude[0] != "file_delete" {
		t.Errorf("Unexpected exclude_tools: %v", exclude)
	}
}

func TestChatMessageRequestToolsOmittedWhenNil(t *testing.T) {
	req := ChatMessageRequest{Message: "hi"}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	for _, field := range []string{"client_tools", "confirm_tools", "exclude_tools"} {
		if _, exists := m[field]; exists {
			t.Errorf("Expected %s to be omitted when nil", field)
		}
	}
}

func TestClientToolDefSerialization(t *testing.T) {
	tool := ClientToolDef{
		Name:        "calc",
		Description: "Calculator",
		Parameters:  map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
	}
	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	if m["name"] != "calc" {
		t.Errorf("Expected name=calc, got %v", m["name"])
	}
	if m["description"] != "Calculator" {
		t.Errorf("Expected description=Calculator, got %v", m["description"])
	}
}

// ============================================================================
// SubmitChatToolResult Tests
// ============================================================================

func TestSubmitChatToolResult(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/chat-123/tool-result": func(w http.ResponseWriter, r *http.Request) {
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("Failed to decode request body: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if body["call_id"] != "call-456" {
				t.Errorf("Expected call_id=call-456, got %v", body["call_id"])
			}
			if body["success"] != true {
				t.Errorf("Expected success=true, got %v", body["success"])
			}
			result := body["result"].(map[string]interface{})
			if result["temp"] != "72F" {
				t.Errorf("Expected result.temp=72F, got %v", result["temp"])
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	err := client.SubmitChatToolResult("chat-123", "call-456", true, map[string]interface{}{"temp": "72F"}, "")
	if err != nil {
		t.Fatalf("SubmitChatToolResult failed: %v", err)
	}
}

func TestSubmitChatToolResultError(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/chat-123/tool-result": func(w http.ResponseWriter, r *http.Request) {
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("Failed to decode request body: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if body["success"] != false {
				t.Errorf("Expected success=false, got %v", body["success"])
			}
			if body["error"] != "tool crashed" {
				t.Errorf("Expected error='tool crashed', got %v", body["error"])
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	err := client.SubmitChatToolResult("chat-123", "call-456", false, nil, "tool crashed")
	if err != nil {
		t.Fatalf("SubmitChatToolResult failed: %v", err)
	}
}

// ============================================================================
// SubscribeSSE Tests
// ============================================================================

func TestSubscribeSSEParsesMutations(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/subscribe/orders": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, "event: subscribed\ndata: {}\n\n")
			_, _ = fmt.Fprintf(w, "event: mutation\ndata: %s\n\n",
				`{"collection":"orders","event":"insert","record_ids":["r1"],"timestamp":"2026-04-01T00:00:00Z"}`)
			_, _ = fmt.Fprintf(w, "event: mutation\ndata: %s\n\n",
				`{"collection":"orders","event":"update","record_ids":["r2"],"timestamp":"2026-04-01T00:01:00Z"}`)
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	sub, err := client.SubscribeSSE(context.Background(), "orders", nil)
	if err != nil {
		t.Fatalf("SubscribeSSE failed: %v", err)
	}

	var events []MutationNotification
	for n := range sub.Events {
		events = append(events, n)
	}

	if len(events) != 2 {
		t.Fatalf("Expected 2 events, got %d", len(events))
	}
	if events[0].Event != "insert" || events[0].RecordIDs[0] != "r1" {
		t.Errorf("Unexpected first event: %+v", events[0])
	}
	if events[1].Event != "update" || events[1].RecordIDs[0] != "r2" {
		t.Errorf("Unexpected second event: %+v", events[1])
	}
	// Verify no stream errors
	if streamErr := <-sub.Err; streamErr != nil {
		t.Errorf("Unexpected stream error: %v", streamErr)
	}
}

func TestSubscribeSSEWithFilter(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/subscribe/orders": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("filter_field") != "status" {
				t.Errorf("Expected filter_field=status, got %s", r.URL.Query().Get("filter_field"))
			}
			if r.URL.Query().Get("filter_value") != "active" {
				t.Errorf("Expected filter_value=active, got %s", r.URL.Query().Get("filter_value"))
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, "event: mutation\ndata: %s\n\n",
				`{"collection":"orders","event":"insert","record_ids":["r1"],"timestamp":"t"}`)
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	sub, err := client.SubscribeSSE(context.Background(), "orders", &SubscribeSSEOptions{
		FilterField: "status",
		FilterValue: "active",
	})
	if err != nil {
		t.Fatalf("SubscribeSSE failed: %v", err)
	}

	var events []MutationNotification
	for n := range sub.Events {
		events = append(events, n)
	}
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}
}

func TestSubscribeSSEAuthFailure(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{})
	defer server.Close()

	// Create a client with a bad token that will fail auth
	client, err := NewClientWithConfig(ClientConfig{
		BaseURL:     server.URL,
		APIKey:      "bad-key",
		ShouldRetry: false,
		Format:      JSON,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	// Override token to bypass token refresh
	client.tokenMu.Lock()
	client.token = "bad-token"
	client.tokenMu.Unlock()

	_, sseErr := client.SubscribeSSE(context.Background(), "orders", nil)
	if sseErr == nil {
		t.Fatal("Expected error for unauthorized SSE")
	}
	if !strings.Contains(sseErr.Error(), "401") && !strings.Contains(sseErr.Error(), "Unauthorized") {
		t.Errorf("Expected auth error, got: %v", sseErr)
	}
}

func TestSubscribeSSEOptionsStruct(t *testing.T) {
	opts := SubscribeSSEOptions{
		FilterField: "type",
		FilterValue: "order",
	}
	if opts.FilterField != "type" {
		t.Errorf("Expected FilterField=type, got %s", opts.FilterField)
	}
	if opts.FilterValue != "order" {
		t.Errorf("Expected FilterValue=order, got %s", opts.FilterValue)
	}
}
