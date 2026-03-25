package ekodb

import (
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
			json.NewDecoder(r.Body).Decode(&body)

			if body["tool"] != "count_records" {
				t.Errorf("Expected tool count_records, got %v", body["tool"])
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
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
			json.NewDecoder(r.Body).Decode(&body)

			if body["chat_id"] != "chat_456" {
				t.Errorf("Expected chat_id chat_456, got %v", body["chat_id"])
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
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
			json.NewEncoder(w).Encode(map[string]interface{}{
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
			w.Write([]byte("Not Found"))
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
			w.Write([]byte("Internal Server Error"))
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
			json.NewEncoder(w).Encode(map[string]interface{}{
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
			json.NewEncoder(w).Encode(map[string]interface{}{
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
			json.NewEncoder(w).Encode([]map[string]interface{}{
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
			json.NewEncoder(w).Encode([]map[string]interface{}{
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
			w.Write([]byte("{}"))
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
			w.Write([]byte("{}"))
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
			w.Write([]byte("Not Found"))
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.GetUserFunction("nonexistent")
	if err == nil {
		t.Fatal("Expected error for non-existent function")
	}
}
