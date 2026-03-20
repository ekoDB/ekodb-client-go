package ekodb

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// Test Helpers
// ============================================================================

// mockTokenHandler returns a handler that responds with a valid token
func mockTokenHandler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/auth/token" {
			t.Errorf("Expected /api/auth/token, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"token": "test-jwt-token"})
	}
}

// createTestServer creates a test server with token auth and custom handlers
func createTestServer(t *testing.T, handlers map[string]http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle token endpoint
		if r.URL.Path == "/api/auth/token" {
			mockTokenHandler(t)(w, r)
			return
		}

		// Check for auth header on non-token requests
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-jwt-token" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized"))
			return
		}

		// Find matching handler
		key := r.Method + " " + r.URL.Path
		if handler, ok := handlers[key]; ok {
			handler(w, r)
			return
		}

		// Check for path prefix handlers (for dynamic paths)
		for pattern, handler := range handlers {
			if matchesPattern(pattern, r.Method+" "+r.URL.Path) {
				handler(w, r)
				return
			}
		}

		t.Errorf("Unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
}

// matchesPattern checks if a path matches a pattern with wildcards
func matchesPattern(pattern, path string) bool {
	// Simple prefix matching for now
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		return len(path) >= len(pattern)-1 && path[:len(pattern)-1] == pattern[:len(pattern)-1]
	}
	return pattern == path
}

// createTestClient creates a test client pointing to the test server
func createTestClient(t *testing.T, server *httptest.Server) *Client {
	client, err := NewClientWithConfig(ClientConfig{
		BaseURL:     server.URL,
		APIKey:      "test-api-key",
		ShouldRetry: false, // Disable retries for predictable tests
		Timeout:     5 * time.Second,
		Format:      JSON, // Use JSON for test compatibility
	})
	if err != nil {
		t.Fatalf("Failed to create test client: %v", err)
	}
	return client
}

// ============================================================================
// Client Configuration Tests
// ============================================================================

func TestNewClient(t *testing.T) {
	server := createTestServer(t, nil)
	defer server.Close()

	client, err := NewClient(server.URL, "test-api-key")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if client == nil {
		t.Fatal("Expected non-nil client")
	}
}

func TestNewClientWithConfig(t *testing.T) {
	server := createTestServer(t, nil)
	defer server.Close()

	client, err := NewClientWithConfig(ClientConfig{
		BaseURL:     server.URL,
		APIKey:      "test-api-key",
		ShouldRetry: true,
		MaxRetries:  5,
		Timeout:     60 * time.Second,
		Format:      MessagePack,
	})
	if err != nil {
		t.Fatalf("NewClientWithConfig failed: %v", err)
	}
	if client == nil {
		t.Fatal("Expected non-nil client")
	}
}

func TestNewClientWithDefaults(t *testing.T) {
	server := createTestServer(t, nil)
	defer server.Close()

	client, err := NewClientWithConfig(ClientConfig{
		BaseURL: server.URL,
		APIKey:  "test-api-key",
		// All other fields use defaults
	})
	if err != nil {
		t.Fatalf("NewClientWithConfig with defaults failed: %v", err)
	}
	if client == nil {
		t.Fatal("Expected non-nil client")
	}
}

func TestClientAuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Invalid API key"))
	}))
	defer server.Close()

	_, err := NewClient(server.URL, "invalid-key")
	if err == nil {
		t.Fatal("Expected error for invalid API key")
	}
}

// ============================================================================
// RateLimitInfo Tests
// ============================================================================

func TestRateLimitInfoIsNearLimit(t *testing.T) {
	tests := []struct {
		limit     int
		remaining int
		expected  bool
	}{
		{1000, 50, true},   // 5% remaining - near limit
		{1000, 100, true},  // 10% remaining - at threshold
		{1000, 500, false}, // 50% remaining - not near
		{1000, 0, true},    // 0% remaining - definitely near
	}

	for _, tt := range tests {
		info := &RateLimitInfo{Limit: tt.limit, Remaining: tt.remaining}
		result := info.IsNearLimit()
		if result != tt.expected {
			t.Errorf("IsNearLimit(%d/%d) = %v, want %v", tt.remaining, tt.limit, result, tt.expected)
		}
	}
}

func TestRateLimitInfoIsExceeded(t *testing.T) {
	tests := []struct {
		remaining int
		expected  bool
	}{
		{0, true},
		{1, false},
		{100, false},
	}

	for _, tt := range tests {
		info := &RateLimitInfo{Limit: 1000, Remaining: tt.remaining}
		result := info.IsExceeded()
		if result != tt.expected {
			t.Errorf("IsExceeded(%d) = %v, want %v", tt.remaining, result, tt.expected)
		}
	}
}

func TestRateLimitInfoRemainingPercentage(t *testing.T) {
	tests := []struct {
		limit     int
		remaining int
		expected  float64
	}{
		{1000, 250, 25.0},
		{1000, 0, 0.0},
		{1000, 1000, 100.0},
		{100, 50, 50.0},
	}

	for _, tt := range tests {
		info := &RateLimitInfo{Limit: tt.limit, Remaining: tt.remaining}
		result := info.RemainingPercentage()
		if result != tt.expected {
			t.Errorf("RemainingPercentage(%d/%d) = %v, want %v", tt.remaining, tt.limit, result, tt.expected)
		}
	}
}

// ============================================================================
// Error Type Tests
// ============================================================================

func TestRateLimitError(t *testing.T) {
	err := &RateLimitError{RetryAfterSecs: 60}
	expected := "rate limit exceeded, retry after 60 seconds"
	if err.Error() != expected {
		t.Errorf("RateLimitError.Error() = %q, want %q", err.Error(), expected)
	}

	err2 := &RateLimitError{RetryAfterSecs: 30, Message: "Custom message"}
	if err2.Error() != "Custom message" {
		t.Errorf("RateLimitError with message = %q, want %q", err2.Error(), "Custom message")
	}
}

func TestHTTPError(t *testing.T) {
	err := &HTTPError{StatusCode: 500, Message: "Internal Server Error"}
	expected := "request failed with status 500: Internal Server Error"
	if err.Error() != expected {
		t.Errorf("HTTPError.Error() = %q, want %q", err.Error(), expected)
	}
}

func TestHTTPErrorIsNotFound(t *testing.T) {
	tests := []struct {
		statusCode int
		expected   bool
	}{
		{404, true},
		{200, false},
		{500, false},
		{401, false},
	}

	for _, tt := range tests {
		err := &HTTPError{StatusCode: tt.statusCode}
		if err.IsNotFound() != tt.expected {
			t.Errorf("IsNotFound(%d) = %v, want %v", tt.statusCode, err.IsNotFound(), tt.expected)
		}
	}
}

// ============================================================================
// Health Check Tests
// ============================================================================

func TestHealthSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"GET /api/health": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	err := client.Health()
	if err != nil {
		t.Errorf("Health() failed: %v", err)
	}
}

func TestHealthFailure(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"GET /api/health": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "unhealthy"})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	err := client.Health()
	if err == nil {
		t.Error("Expected Health() to fail for unhealthy status")
	}
}

// ============================================================================
// Insert Tests
// ============================================================================

func TestInsertSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/insert/users": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(Record{
				"id":   "user_123",
				"name": "John Doe",
				"age":  30,
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	record := Record{"name": "John Doe", "age": 30}
	result, err := client.Insert("users", record)
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}
	if result["id"] != "user_123" {
		t.Errorf("Insert result id = %v, want user_123", result["id"])
	}
}

func TestInsertWithTTL(t *testing.T) {
	var receivedRecord Record
	handlers := map[string]http.HandlerFunc{
		"POST /api/insert/users": func(w http.ResponseWriter, r *http.Request) {
			json.NewDecoder(r.Body).Decode(&receivedRecord)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(Record{"id": "user_123"})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	record := Record{"name": "John"}
	_, err := client.Insert("users", record, InsertOptions{TTL: "1h"})
	if err != nil {
		t.Fatalf("Insert with TTL failed: %v", err)
	}
	if receivedRecord["ttl"] != "1h" {
		t.Errorf("TTL not set correctly, got %v", receivedRecord["ttl"])
	}
}

// ============================================================================
// Find Tests
// ============================================================================

func TestFindSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/find/users": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]Record{
				{"id": "user_1", "name": "Alice"},
				{"id": "user_2", "name": "Bob"},
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	query := map[string]interface{}{"limit": 10}
	results, err := client.Find("users", query)
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Find returned %d records, want 2", len(results))
	}
}

func TestFindEmptyResult(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/find/users": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]Record{})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	results, err := client.Find("users", nil)
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Find returned %d records, want 0", len(results))
	}
}

func TestFindByIDSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"GET /api/find/users/user_123": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(Record{"id": "user_123", "name": "Alice"})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.FindByID("users", "user_123")
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if result["id"] != "user_123" {
		t.Errorf("FindByID result id = %v, want user_123", result["id"])
	}
}

// ============================================================================
// Update Tests
// ============================================================================

func TestUpdateSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"PUT /api/update/users/user_123": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(Record{
				"id":   "user_123",
				"name": "Alice Updated",
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	record := Record{"name": "Alice Updated"}
	result, err := client.Update("users", "user_123", record)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if result["name"] != "Alice Updated" {
		t.Errorf("Update result name = %v, want Alice Updated", result["name"])
	}
}

// ============================================================================
// Delete Tests
// ============================================================================

func TestDeleteSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"DELETE /api/delete/users/user_123": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"deleted": true})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	err := client.Delete("users", "user_123")
	if err != nil {
		t.Errorf("Delete failed: %v", err)
	}
}

// ============================================================================
// Atomic Field Action Tests
// ============================================================================

func TestUpdateWithActionIncrement(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"PUT /api/update/counters/rec_1/action/increment": func(w http.ResponseWriter, r *http.Request) {
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("Failed to decode request body: %v", err)
			}
			if body["field"] != "views" {
				t.Errorf("Expected field=views, got %v", body["field"])
			}
			if body["value"] != float64(1) {
				t.Errorf("Expected value=1, got %v", body["value"])
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(Record{"id": "rec_1", "views": float64(42)})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.UpdateWithAction("counters", "rec_1", "increment", "views", 1)
	if err != nil {
		t.Fatalf("UpdateWithAction failed: %v", err)
	}
	if result["id"] != "rec_1" {
		t.Errorf("Expected id=rec_1, got %v", result["id"])
	}
}

func TestUpdateWithActionPush(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"PUT /api/update/lists/rec_2/action/push": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(Record{"id": "rec_2", "tags": []string{"rust", "new-tag"}})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.UpdateWithAction("lists", "rec_2", "push", "tags", "new-tag")
	if err != nil {
		t.Fatalf("UpdateWithAction push failed: %v", err)
	}
	if result["id"] != "rec_2" {
		t.Errorf("Expected id=rec_2, got %v", result["id"])
	}
}

func TestUpdateWithActionClear(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"PUT /api/update/data/rec_3/action/clear": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(Record{"id": "rec_3", "temp": float64(0)})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.UpdateWithAction("data", "rec_3", "clear", "temp", nil)
	if err != nil {
		t.Fatalf("UpdateWithAction clear failed: %v", err)
	}
	if result["id"] != "rec_3" {
		t.Errorf("Expected id=rec_3, got %v", result["id"])
	}
}

func TestUpdateWithActionSequence(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"PUT /api/update/sequence/game/player_1": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(Record{
				"id":    "player_1",
				"score": float64(110),
				"lives": float64(2),
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	actions := [][3]interface{}{
		{"increment", "score", 10},
		{"decrement", "lives", 1},
		{"push", "log", "hit"},
	}
	result, err := client.UpdateWithActionSequence("game", "player_1", actions)
	if err != nil {
		t.Fatalf("UpdateWithActionSequence failed: %v", err)
	}
	if result["id"] != "player_1" {
		t.Errorf("Expected id=player_1, got %v", result["id"])
	}
}

func TestUpdateWithActionNotFound(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"PUT /api/update/counters/missing/action/increment": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"error": "Record not found"})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.UpdateWithAction("counters", "missing", "increment", "views", 1)
	if err == nil {
		t.Error("Expected error for missing record, got nil")
	}
}

// ============================================================================
// Batch Operation Tests
// ============================================================================

func TestBatchInsertSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/batch/insert/users": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"successful": []string{"id_1", "id_2", "id_3"},
				"failed":     []interface{}{},
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	records := []Record{
		{"name": "User 1"},
		{"name": "User 2"},
		{"name": "User 3"},
	}
	results, err := client.BatchInsert("users", records)
	if err != nil {
		t.Fatalf("BatchInsert failed: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("BatchInsert returned %d records, want 3", len(results))
	}
}

func TestBatchDeleteSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"DELETE /api/batch/delete/users": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"successful": []string{"id_1", "id_2"},
				"failed":     []interface{}{},
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	ids := []string{"id_1", "id_2"}
	count, err := client.BatchDelete("users", ids)
	if err != nil {
		t.Fatalf("BatchDelete failed: %v", err)
	}
	if count != 2 {
		t.Errorf("BatchDelete returned count %d, want 2", count)
	}
}

func TestBatchUpdateSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"PUT /api/batch/update/users": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"successful": []string{"id_1", "id_2"},
				"failed":     []interface{}{},
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	updates := map[string]Record{
		"id_1": {"name": "Updated 1"},
		"id_2": {"name": "Updated 2"},
	}
	results, err := client.BatchUpdate("users", updates)
	if err != nil {
		t.Fatalf("BatchUpdate failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("BatchUpdate returned %d records, want 2", len(results))
	}
}

// ============================================================================
// KV Store Tests
// ============================================================================

func TestKVSetSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/kv/set/my_key": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	err := client.KVSet("my_key", map[string]string{"data": "value"})
	if err != nil {
		t.Errorf("KVSet failed: %v", err)
	}
}

func TestKVGetSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"GET /api/kv/get/my_key": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"value": map[string]string{"data": "stored_value"},
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	value, err := client.KVGet("my_key")
	if err != nil {
		t.Fatalf("KVGet failed: %v", err)
	}
	if value == nil {
		t.Error("KVGet returned nil value")
	}
}

func TestKVDeleteSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"DELETE /api/kv/delete/my_key": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"deleted": true})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	err := client.KVDelete("my_key")
	if err != nil {
		t.Errorf("KVDelete failed: %v", err)
	}
}

func TestKVExistsSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"GET /api/kv/get/existing_key": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"value": "data"})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	exists, err := client.KVExists("existing_key")
	if err != nil {
		t.Fatalf("KVExists failed: %v", err)
	}
	if !exists {
		t.Error("KVExists returned false, want true")
	}
}

func TestKVExistsNotFound(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"GET /api/kv/get/missing_key": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	exists, err := client.KVExists("missing_key")
	if err != nil {
		t.Fatalf("KVExists failed: %v", err)
	}
	if exists {
		t.Error("KVExists returned true, want false")
	}
}

// ============================================================================
// Transaction Tests
// ============================================================================

func TestBeginTransactionSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/transactions": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"transaction_id": "tx_123456"})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	txID, err := client.BeginTransaction("READ_COMMITTED")
	if err != nil {
		t.Fatalf("BeginTransaction failed: %v", err)
	}
	if txID != "tx_123456" {
		t.Errorf("BeginTransaction returned %q, want tx_123456", txID)
	}
}

func TestBeginTransactionInvalidIsolation(t *testing.T) {
	server := createTestServer(t, nil)
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.BeginTransaction("INVALID_LEVEL")
	if err == nil {
		t.Error("Expected error for invalid isolation level")
	}
}

func TestBeginTransactionAllIsolationLevels(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/transactions": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"transaction_id": "tx_123"})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)

	levels := []string{"READ_UNCOMMITTED", "READ_COMMITTED", "REPEATABLE_READ", "SERIALIZABLE"}
	for _, level := range levels {
		_, err := client.BeginTransaction(level)
		if err != nil {
			t.Errorf("BeginTransaction(%s) failed: %v", level, err)
		}
	}
}

func TestCommitTransactionSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/transactions/tx_123/commit": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	err := client.CommitTransaction("tx_123")
	if err != nil {
		t.Errorf("CommitTransaction failed: %v", err)
	}
}

func TestRollbackTransactionSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/transactions/tx_123/rollback": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	err := client.RollbackTransaction("tx_123")
	if err != nil {
		t.Errorf("RollbackTransaction failed: %v", err)
	}
}

// ============================================================================
// Collection Tests
// ============================================================================

func TestListCollectionsSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"GET /api/collections": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string][]string{
				"collections": {"users", "posts", "comments"},
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	collections, err := client.ListCollections()
	if err != nil {
		t.Fatalf("ListCollections failed: %v", err)
	}
	if len(collections) != 3 {
		t.Errorf("ListCollections returned %d collections, want 3", len(collections))
	}
}

func TestDeleteCollectionSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"DELETE /api/collections/test_collection": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	err := client.DeleteCollection("test_collection")
	if err != nil {
		t.Errorf("DeleteCollection failed: %v", err)
	}
}

func TestCollectionExistsTrue(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"GET /api/collections": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string][]string{
				"collections": {"users", "posts", "comments"},
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	exists, err := client.CollectionExists("users")
	if err != nil {
		t.Fatalf("CollectionExists failed: %v", err)
	}
	if !exists {
		t.Error("CollectionExists returned false, want true")
	}
}

func TestCollectionExistsFalse(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"GET /api/collections": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string][]string{
				"collections": {"users", "posts", "comments"},
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	exists, err := client.CollectionExists("nonexistent")
	if err != nil {
		t.Fatalf("CollectionExists failed: %v", err)
	}
	if exists {
		t.Error("CollectionExists returned true, want false")
	}
}

func TestCountDocuments(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/find/users": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]Record{
				{"id": "1", "name": "Alice"},
				{"id": "2", "name": "Bob"},
				{"id": "3", "name": "Carol"},
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	count, err := client.CountDocuments("users")
	if err != nil {
		t.Fatalf("CountDocuments failed: %v", err)
	}
	if count != 3 {
		t.Errorf("CountDocuments returned %d, want 3", count)
	}
}

func TestGetChatModels(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"GET /api/chat_models": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(ChatModels{
				OpenAI:     []string{"gpt-4", "gpt-3.5-turbo"},
				Anthropic:  []string{"claude-3-opus", "claude-3-sonnet"},
				Perplexity: []string{"llama-3.1-sonar-small"},
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	models, err := client.GetChatModels()
	if err != nil {
		t.Fatalf("GetChatModels failed: %v", err)
	}
	if len(models.OpenAI) != 2 {
		t.Errorf("Expected 2 OpenAI models, got %d", len(models.OpenAI))
	}
	if len(models.Anthropic) != 2 {
		t.Errorf("Expected 2 Anthropic models, got %d", len(models.Anthropic))
	}
}

func TestGetChatTools(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"GET /api/chat/tools": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"name": "web_search", "description": "Search the web"},
				{"name": "http_fetch", "description": "Fetch a URL"},
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	tools, err := client.GetChatTools()
	if err != nil {
		t.Fatalf("GetChatTools failed: %v", err)
	}
	if len(tools) != 2 {
		t.Errorf("Expected 2 tools, got %d", len(tools))
	}
	if tools[0]["name"] != "web_search" {
		t.Errorf("Expected web_search, got %v", tools[0]["name"])
	}
}

func TestGetChatToolsError(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"GET /api/chat/tools": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal server error"))
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.GetChatTools()
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
}

func TestGetChatModel(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"GET /api/chat_models/openai": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]string{"gpt-4", "gpt-3.5-turbo", "gpt-4-turbo"})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	models, err := client.GetChatModel("openai")
	if err != nil {
		t.Fatalf("GetChatModel failed: %v", err)
	}
	if len(models) != 3 {
		t.Errorf("Expected 3 models, got %d", len(models))
	}
}

// ============================================================================
// Error Handling Tests
// ============================================================================

func TestServerError(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/insert/users": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal Server Error"))
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.Insert("users", Record{"name": "Test"})
	if err == nil {
		t.Error("Expected error for server error response")
	}
	httpErr, ok := err.(*HTTPError)
	if !ok {
		t.Errorf("Expected HTTPError, got %T", err)
	} else if httpErr.StatusCode != 500 {
		t.Errorf("Expected status 500, got %d", httpErr.StatusCode)
	}
}

func TestNotFoundError(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"GET /api/find/users/nonexistent": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.FindByID("users", "nonexistent")
	if err == nil {
		t.Error("Expected error for not found response")
	}
	httpErr, ok := err.(*HTTPError)
	if !ok {
		t.Errorf("Expected HTTPError, got %T", err)
	} else if !httpErr.IsNotFound() {
		t.Error("Expected IsNotFound() to return true")
	}
}

func TestRateLimitErrorResponse(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/insert/users": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("Rate limit exceeded"))
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.Insert("users", Record{"name": "Test"})
	if err == nil {
		t.Error("Expected error for rate limit response")
	}
	rateErr, ok := err.(*RateLimitError)
	if !ok {
		t.Errorf("Expected RateLimitError, got %T: %v", err, err)
	} else if rateErr.RetryAfterSecs != 60 {
		t.Errorf("Expected RetryAfterSecs 60, got %d", rateErr.RetryAfterSecs)
	}
}

// ============================================================================
// Rate Limit Info Extraction Tests
// ============================================================================

func TestRateLimitInfoExtraction(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"GET /api/health": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-RateLimit-Limit", "1000")
			w.Header().Set("X-RateLimit-Remaining", "999")
			w.Header().Set("X-RateLimit-Reset", "1234567890")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)

	// Initially nil
	if client.GetRateLimitInfo() != nil {
		t.Error("Expected nil rate limit info before any request")
	}

	// Make a request
	err := client.Health()
	if err != nil {
		t.Fatalf("Health failed: %v", err)
	}

	// Check rate limit info
	info := client.GetRateLimitInfo()
	if info == nil {
		t.Fatal("Expected non-nil rate limit info after request")
	}
	if info.Limit != 1000 {
		t.Errorf("Limit = %d, want 1000", info.Limit)
	}
	if info.Remaining != 999 {
		t.Errorf("Remaining = %d, want 999", info.Remaining)
	}
	if info.Reset != 1234567890 {
		t.Errorf("Reset = %d, want 1234567890", info.Reset)
	}
}

func TestIsNearRateLimit(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"GET /api/health": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-RateLimit-Limit", "100")
			w.Header().Set("X-RateLimit-Remaining", "5") // 5% remaining
			w.Header().Set("X-RateLimit-Reset", "1234567890")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)

	// Initially false
	if client.IsNearRateLimit() {
		t.Error("Expected IsNearRateLimit() = false before any request")
	}

	// Make a request
	client.Health()

	// Check near limit
	if !client.IsNearRateLimit() {
		t.Error("Expected IsNearRateLimit() = true with 5% remaining")
	}
}

// ============================================================================
// Restore Operations Tests
// ============================================================================

func TestRestoreRecordSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/trash/users/record_123": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "restored"})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	err := client.RestoreRecord("users", "record_123")
	if err != nil {
		t.Errorf("RestoreRecord failed: %v", err)
	}
}

func TestRestoreCollectionSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/trash/users": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":           "restored",
				"collection":       "users",
				"records_restored": 5,
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	count, err := client.RestoreCollection("users")
	if err != nil {
		t.Fatalf("RestoreCollection failed: %v", err)
	}
	if count != 5 {
		t.Errorf("RestoreCollection returned count %d, want 5", count)
	}
}

// ============================================================================
// Collection Management Tests
// ============================================================================

func TestCreateCollectionSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/collections/new_collection": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "created"})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	schema := NewSchemaBuilder().
		AddField("name", NewFieldTypeSchemaBuilder("String").Build()).
		AddField("age", NewFieldTypeSchemaBuilder("Integer").Build()).
		Build()
	err := client.CreateCollection("new_collection", schema)
	if err != nil {
		t.Errorf("CreateCollection failed: %v", err)
	}
}

func TestGetCollectionSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"GET /api/collections/users": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"collection": map[string]interface{}{
					"fields": map[string]interface{}{
						"name": map[string]string{"field_type": "String"},
					},
				},
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	info, err := client.GetCollection("users")
	if err != nil {
		t.Fatalf("GetCollection failed: %v", err)
	}
	if info == nil {
		t.Error("GetCollection returned nil")
	}
}

func TestGetSchemaSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"GET /api/collections/users": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"collection": map[string]interface{}{
					"fields": map[string]interface{}{
						"name":  map[string]string{"field_type": "String"},
						"email": map[string]string{"field_type": "String"},
					},
				},
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	schema, err := client.GetSchema("users")
	if err != nil {
		t.Fatalf("GetSchema failed: %v", err)
	}
	if schema == nil {
		t.Error("GetSchema returned nil")
	}
}

// ============================================================================
// Search Tests
// ============================================================================

func TestSearchSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/search/documents": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{
					{"id": "doc_1", "score": 0.95, "title": "Result 1"},
					{"id": "doc_2", "score": 0.85, "title": "Result 2"},
				},
				"total": 2,
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	query := NewSearchQueryBuilder("search terms").Limit(10).Build()
	result, err := client.Search("documents", query)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(result.Results) != 2 {
		t.Errorf("Search returned %d results, want 2", len(result.Results))
	}
}

func TestTextSearchSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/search/documents": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{
					{"id": "doc_1", "title": "Matching document"},
				},
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	results, err := client.TextSearch("documents", "matching text", 10)
	if err != nil {
		t.Fatalf("TextSearch failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("TextSearch returned %d results, want 1", len(results))
	}
}

func TestHybridSearchSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/search/documents": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{
					{"id": "doc_1", "score": 0.9},
					{"id": "doc_2", "score": 0.8},
				},
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	vector := []float64{0.1, 0.2, 0.3, 0.4}
	results, err := client.HybridSearch("documents", "query text", vector, 10)
	if err != nil {
		t.Fatalf("HybridSearch failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("HybridSearch returned %d results, want 2", len(results))
	}
}

func TestSearchQueryBuilderSelectFields(t *testing.T) {
	fields := []string{"title", "content"}
	query := NewSearchQueryBuilder("test query").
		SelectFields(fields).
		Build()

	if query.SelectFields == nil {
		t.Fatal("SelectFields not set")
	}
	if len(query.SelectFields) != 2 {
		t.Errorf("SelectFields length = %d, want 2", len(query.SelectFields))
	}
	if query.SelectFields[0] != "title" {
		t.Errorf("SelectFields[0] = %s, want title", query.SelectFields[0])
	}
	if query.SelectFields[1] != "content" {
		t.Errorf("SelectFields[1] = %s, want content", query.SelectFields[1])
	}
}

func TestSearchQueryBuilderExcludeFields(t *testing.T) {
	fields := []string{"metadata", "internal_id"}
	query := NewSearchQueryBuilder("test query").
		ExcludeFields(fields).
		Build()

	if query.ExcludeFields == nil {
		t.Fatal("ExcludeFields not set")
	}
	if len(query.ExcludeFields) != 2 {
		t.Errorf("ExcludeFields length = %d, want 2", len(query.ExcludeFields))
	}
	if query.ExcludeFields[0] != "metadata" {
		t.Errorf("ExcludeFields[0] = %s, want metadata", query.ExcludeFields[0])
	}
	if query.ExcludeFields[1] != "internal_id" {
		t.Errorf("ExcludeFields[1] = %s, want internal_id", query.ExcludeFields[1])
	}
}

// ============================================================================
// KV Find/Query Tests
// ============================================================================

func TestKVFindSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/kv/find": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"key": "user:1", "value": map[string]string{"name": "Alice"}},
				{"key": "user:2", "value": map[string]string{"name": "Bob"}},
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	results, err := client.KVFind("user:*", false)
	if err != nil {
		t.Fatalf("KVFind failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("KVFind returned %d results, want 2", len(results))
	}
}

// ============================================================================
// Embed Tests
// ============================================================================

func TestEmbed(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/embed": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"embeddings": [][]float64{{0.1, 0.2, 0.3}},
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	embedding, err := client.Embed("Hello world", "test-model")
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}
	expected := []float64{0.1, 0.2, 0.3}
	if len(embedding) != len(expected) {
		t.Fatalf("Embed returned %d dimensions, want %d", len(embedding), len(expected))
	}
	for i, v := range expected {
		if embedding[i] != v {
			t.Errorf("embedding[%d] = %f, want %f", i, embedding[i], v)
		}
	}
}

func TestEmbedBatch(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/embed": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"embeddings": [][]float64{{0.1, 0.2}, {0.3, 0.4}},
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	embeddings, err := client.EmbedBatch([]string{"Hello", "World"}, "test-model")
	if err != nil {
		t.Fatalf("EmbedBatch failed: %v", err)
	}
	if len(embeddings) != 2 {
		t.Fatalf("EmbedBatch returned %d embeddings, want 2", len(embeddings))
	}
	if embeddings[0][0] != 0.1 || embeddings[0][1] != 0.2 {
		t.Errorf("embeddings[0] = %v, want [0.1, 0.2]", embeddings[0])
	}
	if embeddings[1][0] != 0.3 || embeddings[1][1] != 0.4 {
		t.Errorf("embeddings[1] = %v, want [0.3, 0.4]", embeddings[1])
	}
}

func TestEmbedBatchEmpty(t *testing.T) {
	// No server needed - should fail before making request
	handlers := map[string]http.HandlerFunc{}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.EmbedBatch([]string{}, "test-model")
	if err == nil {
		t.Fatal("Expected error for empty texts, got nil")
	}
	if !contains(err.Error(), "texts must not be empty") {
		t.Errorf("Expected error containing 'texts must not be empty', got: %v", err)
	}
}

func TestEmbedBatchMismatch(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/embed": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// Return 1 embedding for 2 input texts
			json.NewEncoder(w).Encode(map[string]interface{}{
				"embeddings": [][]float64{{0.1, 0.2}},
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.EmbedBatch([]string{"Hello", "World"}, "test-model")
	if err == nil {
		t.Fatal("Expected error for mismatched embedding count, got nil")
	}
	if !contains(err.Error(), "does not match") {
		t.Errorf("Expected error containing 'does not match', got: %v", err)
	}
}

// contains checks if s contains substr (helper for error message assertions)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ============================================================================
// Functions/Scripts Tests
// ============================================================================

func TestSaveScriptSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/functions": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":    "func_123",
				"label": "my_function",
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	script := Script{
		Label: "my_function",
		Name:  "my_function",
		Functions: []FunctionStageConfig{
			StageFindAll("users"),
		},
	}
	result, err := client.SaveScript(script)
	if err != nil {
		t.Fatalf("SaveScript failed: %v", err)
	}
	if result == "" {
		t.Error("SaveScript returned empty result")
	}
}

func TestCallScriptSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/functions/my_function": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []map[string]interface{}{
					{"id": "user_1", "name": "Alice"},
				},
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	params := map[string]interface{}{"limit": 10}
	result, err := client.CallScript("my_function", params)
	if err != nil {
		t.Fatalf("CallScript failed: %v", err)
	}
	if result == nil {
		t.Error("Expected non-nil result from CallScript")
	}
}

func TestGetScriptSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"GET /api/functions/func_123": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":    "func_123",
				"label": "my_function",
				"name":  "my_function",
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.GetScript("func_123")
	if err != nil {
		t.Fatalf("GetScript failed: %v", err)
	}
	if result == nil {
		t.Error("GetScript returned nil")
	}
}

func TestListScriptsSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"GET /api/functions": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "func_1", "label": "function_1", "name": "function_1"},
				{"id": "func_2", "label": "function_2", "name": "function_2"},
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	results, err := client.ListScripts(nil)
	if err != nil {
		t.Fatalf("ListScripts failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("ListScripts returned %d scripts, want 2", len(results))
	}
}

func TestDeleteScriptSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"DELETE /api/functions/func_123": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	err := client.DeleteScript("func_123")
	if err != nil {
		t.Errorf("DeleteScript failed: %v", err)
	}
}

// ============================================================================
// Chat Tests
// ============================================================================

func TestCreateChatSessionSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/chat": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"chat_id":    "chat_123",
				"message_id": "msg_001",
				"responses":  []string{"Hello! How can I help you?"},
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	request := CreateChatSessionRequest{
		LLMProvider: "openai",
		LLMModel:    strPtr("gpt-4"),
	}
	result, err := client.CreateChatSession(request)
	if err != nil {
		t.Fatalf("CreateChatSession failed: %v", err)
	}
	if result == nil {
		t.Error("CreateChatSession returned nil")
	}
}

func TestChatMessageSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/chat/chat_123/messages": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"chat_id":    "chat_123",
				"message_id": "msg_002",
				"responses":  []string{"Here's my response to your question."},
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	request := ChatMessageRequest{Message: "What is the answer?"}
	result, err := client.ChatMessage("chat_123", request)
	if err != nil {
		t.Fatalf("ChatMessage failed: %v", err)
	}
	if result == nil {
		t.Error("ChatMessage returned nil")
	}
}

func TestListChatSessionsSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"GET /api/chat": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"sessions": []map[string]interface{}{
					{"id": "chat_1", "created_at": "2024-01-01T00:00:00Z"},
					{"id": "chat_2", "created_at": "2024-01-02T00:00:00Z"},
				},
				"total": 2,
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.ListChatSessions(nil)
	if err != nil {
		t.Fatalf("ListChatSessions failed: %v", err)
	}
	if result == nil {
		t.Error("ListChatSessions returned nil")
	}
}

func TestGetChatSessionSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"GET /api/chat/chat_123": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":         "chat_123",
				"created_at": "2024-01-01T00:00:00Z",
				"messages":   []interface{}{},
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.GetChatSession("chat_123")
	if err != nil {
		t.Fatalf("GetChatSession failed: %v", err)
	}
	if result == nil {
		t.Error("GetChatSession returned nil")
	}
}

func TestDeleteChatSessionSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"DELETE /api/chat/chat_123": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	err := client.DeleteChatSession("chat_123")
	if err != nil {
		t.Errorf("DeleteChatSession failed: %v", err)
	}
}

// Helper function for string pointers in tests
func strPtr(s string) *string {
	return &s
}

// ============================================================================
// Missing Method Tests - Added Jan 4, 2026
// ============================================================================

func TestFindAllSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/find/users": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "user_1", "name": "Alice"},
				{"id": "user_2", "name": "Bob"},
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	results, err := client.FindAll("users", 100)
	if err != nil {
		t.Fatalf("FindAll failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("FindAll returned %d results, want 2", len(results))
	}
}

func TestKVQuerySuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/kv/find": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"key": "config:app", "value": map[string]string{"setting": "value1"}},
				{"key": "config:db", "value": map[string]string{"setting": "value2"}},
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	results, err := client.KVQuery("config:*", false)
	if err != nil {
		t.Fatalf("KVQuery failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("KVQuery returned %d results, want 2", len(results))
	}
}

func TestGetTransactionStatusSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"GET /api/transactions/tx_123": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"transaction_id": "tx_123",
				"status":         "active",
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	status, err := client.GetTransactionStatus("tx_123")
	if err != nil {
		t.Fatalf("GetTransactionStatus failed: %v", err)
	}
	if status["status"] != "active" {
		t.Errorf("GetTransactionStatus status = %v, want active", status["status"])
	}
}

func TestUpdateScriptSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"PUT /api/functions/func_123": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":    "func_123",
				"label": "updated_function",
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	script := Script{
		Label: "updated_function",
		Name:  "updated_function",
		Functions: []FunctionStageConfig{
			StageFindAll("users"),
		},
	}
	err := client.UpdateScript("func_123", script)
	if err != nil {
		t.Errorf("UpdateScript failed: %v", err)
	}
}

func TestBranchChatSessionSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/chat/branch": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"chat_id":       "chat_456",
				"branched_from": "chat_123",
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	request := CreateChatSessionRequest{
		LLMProvider: "openai",
	}
	result, err := client.BranchChatSession(request)
	if err != nil {
		t.Fatalf("BranchChatSession failed: %v", err)
	}
	if result == nil {
		t.Error("BranchChatSession returned nil")
	}
}

func TestMergeChatSessionsSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/chat/merge": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"chat_id": "chat_123",
				"merged":  true,
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	request := MergeSessionsRequest{
		SourceChatIDs: []string{"chat_456"},
		TargetChatID:  "chat_123",
	}
	result, err := client.MergeChatSessions(request)
	if err != nil {
		t.Fatalf("MergeChatSessions failed: %v", err)
	}
	if result == nil {
		t.Error("MergeChatSessions returned nil")
	}
}

func TestUpdateChatSessionSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"PUT /api/chat/chat_123": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"chat_id": "chat_123",
				"updated": true,
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	title := "New Title"
	request := UpdateSessionRequest{Title: &title}
	result, err := client.UpdateChatSession("chat_123", request)
	if err != nil {
		t.Fatalf("UpdateChatSession failed: %v", err)
	}
	if result == nil {
		t.Error("UpdateChatSession returned nil")
	}
}

func TestGetChatSessionMessagesSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"GET /api/chat/chat_123/messages": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"messages": []map[string]interface{}{
					{"id": "msg_1", "role": "user", "content": "Hello"},
					{"id": "msg_2", "role": "assistant", "content": "Hi there"},
				},
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.GetChatSessionMessages("chat_123", nil)
	if err != nil {
		t.Fatalf("GetChatSessionMessages failed: %v", err)
	}
	if result == nil {
		t.Error("GetChatSessionMessages returned nil")
	}
}

func TestDeleteChatMessageSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"DELETE /api/chat/chat_123/messages/msg_001": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	err := client.DeleteChatMessage("chat_123", "msg_001")
	if err != nil {
		t.Errorf("DeleteChatMessage failed: %v", err)
	}
}

func TestUpdateChatMessageSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"PUT /api/chat/chat_123/messages/msg_001": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	err := client.UpdateChatMessage("chat_123", "msg_001", "Updated content")
	if err != nil {
		t.Errorf("UpdateChatMessage failed: %v", err)
	}
}

func TestRegenerateChatMessageSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/chat/chat_123/messages/msg_001/regenerate": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"message_id": "msg_002",
				"content":    "Regenerated response",
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.RegenerateChatMessage("chat_123", "msg_001")
	if err != nil {
		t.Fatalf("RegenerateChatMessage failed: %v", err)
	}
	if result == nil {
		t.Error("RegenerateChatMessage returned nil")
	}
}

func TestToggleForgottenMessageSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"PATCH /api/chat/chat_123/messages/msg_001/forgotten": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	err := client.ToggleForgottenMessage("chat_123", "msg_001", true)
	if err != nil {
		t.Errorf("ToggleForgottenMessage failed: %v", err)
	}
}

// ============================================================================
// Distinct Values Tests
// ============================================================================

func TestDistinctValuesSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/distinct/products/category": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(DistinctValuesResponse{
				Collection: "products",
				Field:      "category",
				Values:     []interface{}{"books", "electronics", "food"},
				Count:      3,
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	resp, err := client.DistinctValues("products", "category", DistinctValuesQuery{})
	if err != nil {
		t.Fatalf("DistinctValues failed: %v", err)
	}
	if resp.Count != 3 {
		t.Errorf("DistinctValues count = %d, want 3", resp.Count)
	}
	if len(resp.Values) != 3 {
		t.Errorf("DistinctValues values len = %d, want 3", len(resp.Values))
	}
	if resp.Collection != "products" {
		t.Errorf("DistinctValues collection = %q, want %q", resp.Collection, "products")
	}
	if resp.Field != "category" {
		t.Errorf("DistinctValues field = %q, want %q", resp.Field, "category")
	}
}

func TestDistinctValuesEmpty(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/distinct/empty/tag": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(DistinctValuesResponse{
				Collection: "empty",
				Field:      "tag",
				Values:     []interface{}{},
				Count:      0,
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	resp, err := client.DistinctValues("empty", "tag", DistinctValuesQuery{})
	if err != nil {
		t.Fatalf("DistinctValues failed: %v", err)
	}
	if resp.Count != 0 {
		t.Errorf("DistinctValues count = %d, want 0", resp.Count)
	}
}

func TestDistinctValuesWithFilter(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/distinct/orders/status": func(w http.ResponseWriter, r *http.Request) {
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("Failed to decode request body: %v", err)
			}
			if body["filter"] == nil {
				t.Errorf("Expected filter in request body, got nil")
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(DistinctValuesResponse{
				Collection: "orders",
				Field:      "status",
				Values:     []interface{}{"active", "pending"},
				Count:      2,
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	query := DistinctValuesQuery{
		Filter: map[string]interface{}{
			"type": "Condition",
			"content": map[string]interface{}{
				"field": "region", "operator": "Eq", "value": "us",
			},
		},
	}
	resp, err := client.DistinctValues("orders", "status", query)
	if err != nil {
		t.Fatalf("DistinctValues failed: %v", err)
	}
	if resp.Count != 2 {
		t.Errorf("DistinctValues count = %d, want 2", resp.Count)
	}
}

func TestDistinctValuesServerError(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/distinct/bad/field": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"internal error"}`))
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.DistinctValues("bad", "field", DistinctValuesQuery{})
	if err == nil {
		t.Error("Expected error from server error, got nil")
	}
}

// ============================================================================
// RawCompletion Tests
// ============================================================================

func TestRawCompletionSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/chat/complete": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(RawCompletionResponse{
				Content: "The answer is 42.",
			})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	resp, err := client.RawCompletion(RawCompletionRequest{
		SystemPrompt: "You are a helpful assistant.",
		Message:      "What is the answer?",
	})
	if err != nil {
		t.Fatalf("RawCompletion failed: %v", err)
	}
	if resp.Content != "The answer is 42." {
		t.Errorf("RawCompletion content = %q, want %q", resp.Content, "The answer is 42.")
	}
}

func TestRawCompletionWithOptionalFields(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/chat/complete": func(w http.ResponseWriter, r *http.Request) {
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			if body["provider"] != "openai" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if body["model"] != "gpt-4o" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(RawCompletionResponse{Content: "Response."})
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	provider := "openai"
	model := "gpt-4o"
	maxTokens := 512
	client := createTestClient(t, server)
	resp, err := client.RawCompletion(RawCompletionRequest{
		SystemPrompt: "System.",
		Message:      "User.",
		Provider:     &provider,
		Model:        &model,
		MaxTokens:    &maxTokens,
	})
	if err != nil {
		t.Fatalf("RawCompletion failed: %v", err)
	}
	if resp.Content != "Response." {
		t.Errorf("RawCompletion content = %q, want %q", resp.Content, "Response.")
	}
}

func TestRawCompletionServerError(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/chat/complete": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"llm unavailable"}`))
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.RawCompletion(RawCompletionRequest{
		SystemPrompt: "S.",
		Message:      "M.",
	})
	if err == nil {
		t.Error("Expected error from server error, got nil")
	}
}

func TestRawCompletionStreamSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/chat/complete/stream": func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Accept") != "text/event-stream" {
				t.Errorf("Expected Accept: text/event-stream, got: %s", r.Header.Get("Accept"))
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, `data: {"token":"Hello"}`)
			fmt.Fprintln(w, `data: {"token":" world"}`)
			fmt.Fprintln(w, `data: {"content":"Hello world","done":true}`)
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.RawCompletionStream(RawCompletionRequest{
		SystemPrompt: "System.",
		Message:      "User.",
	})
	if err != nil {
		t.Fatalf("RawCompletionStream failed: %v", err)
	}
	if result.Content != "Hello world" {
		t.Errorf("Expected 'Hello world', got '%s'", result.Content)
	}
}

func TestRawCompletionStreamTokenAccumulation(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/chat/complete/stream": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, `data: {"token":"chunk1"}`)
			fmt.Fprintln(w, `data: {"token":"chunk2"}`)
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.RawCompletionStream(RawCompletionRequest{
		SystemPrompt: "S.",
		Message:      "M.",
	})
	if err != nil {
		t.Fatalf("RawCompletionStream failed: %v", err)
	}
	if result.Content != "chunk1chunk2" {
		t.Errorf("Expected 'chunk1chunk2', got '%s'", result.Content)
	}
}

func TestRawCompletionStreamError(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/chat/complete/stream": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, `data: {"error":"LLM timeout"}`)
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.RawCompletionStream(RawCompletionRequest{
		SystemPrompt: "S.",
		Message:      "M.",
	})
	if err == nil {
		t.Error("Expected error from SSE error event, got nil")
	}
	if !strings.Contains(err.Error(), "LLM timeout") {
		t.Errorf("Expected error to contain 'LLM timeout', got: %s", err.Error())
	}
}

func TestRawCompletionStreamHTTPError(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/chat/complete/stream": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized"))
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.RawCompletionStream(RawCompletionRequest{
		SystemPrompt: "S.",
		Message:      "M.",
	})
	if err == nil {
		t.Error("Expected error from 401 response, got nil")
	}
}
