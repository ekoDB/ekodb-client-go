package ekodb

import (
	"encoding/json"
	"net/http"
	"testing"
)

// ============================================================================
// KV Batch Operations Tests
// ============================================================================

func TestKVBatchGetSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/kv/batch/get": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			response := []map[string]interface{}{
				{"data": "value1"},
				{"data": "value2"},
				{"data": "value3"},
			}
			json.NewEncoder(w).Encode(response)
		},
	}

	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	keys := []string{"key1", "key2", "key3"}
	results, err := client.KVBatchGet(keys)
	if err != nil {
		t.Fatalf("KVBatchGet failed: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}
}

func TestKVBatchSetSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/kv/batch/set": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			response := [][]interface{}{
				{"key1", true},
				{"key2", true},
				{"key3", true},
			}
			json.NewEncoder(w).Encode(response)
		},
	}

	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	entries := []map[string]interface{}{
		{"key": "key1", "value": map[string]interface{}{"data": "value1"}},
		{"key": "key2", "value": map[string]interface{}{"data": "value2"}},
		{"key": "key3", "value": map[string]interface{}{"data": "value3"}},
	}
	results, err := client.KVBatchSet(entries)
	if err != nil {
		t.Fatalf("KVBatchSet failed: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}
	// Verify all were successful
	for i, result := range results {
		if len(result) != 2 {
			t.Errorf("Result %d: expected 2 elements, got %d", i, len(result))
			continue
		}
		if wasSet, ok := result[1].(bool); !ok || !wasSet {
			t.Errorf("Result %d: expected success=true, got %v", i, result[1])
		}
	}
}

func TestKVBatchSetWithTTL(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/kv/batch/set": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			response := [][]interface{}{
				{"key1", true},
				{"key2", true},
			}
			json.NewEncoder(w).Encode(response)
		},
	}

	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	entries := []map[string]interface{}{
		{"key": "key1", "value": map[string]interface{}{"data": "value1"}, "ttl": 3600},
		{"key": "key2", "value": map[string]interface{}{"data": "value2"}, "ttl": 3600},
	}
	results, err := client.KVBatchSet(entries)
	if err != nil {
		t.Fatalf("KVBatchSet with TTL failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

func TestKVBatchDeleteSuccess(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"DELETE /api/kv/batch/delete": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			response := [][]interface{}{
				{"key1", true},
				{"key2", true},
				{"key3", false},
			}
			json.NewEncoder(w).Encode(response)
		},
	}

	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	keys := []string{"key1", "key2", "key3"}
	results, err := client.KVBatchDelete(keys)
	if err != nil {
		t.Fatalf("KVBatchDelete failed: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}
	// Verify first two were deleted, third was not found
	if wasDeleted, ok := results[0][1].(bool); !ok || !wasDeleted {
		t.Errorf("Expected key1 to be deleted")
	}
	if wasDeleted, ok := results[1][1].(bool); !ok || !wasDeleted {
		t.Errorf("Expected key2 to be deleted")
	}
	if wasDeleted, ok := results[2][1].(bool); !ok || wasDeleted {
		t.Errorf("Expected key3 to not be found")
	}
}

func TestKVBatchGetEmptyKeys(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/kv/batch/get": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{})
		},
	}

	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	results, err := client.KVBatchGet([]string{})
	if err != nil {
		t.Fatalf("KVBatchGet with empty keys failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results for empty keys, got %d", len(results))
	}
}

func TestKVBatchSetPartialFailure(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/kv/batch/set": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			response := [][]interface{}{
				{"key1", true},
				{"key2", false},
				{"key3", true},
			}
			json.NewEncoder(w).Encode(response)
		},
	}

	server := createTestServer(t, handlers)
	defer server.Close()

	client := createTestClient(t, server)
	entries := []map[string]interface{}{
		{"key": "key1", "value": map[string]interface{}{"data": "value1"}},
		{"key": "key2", "value": map[string]interface{}{"data": "value2"}},
		{"key": "key3", "value": map[string]interface{}{"data": "value3"}},
	}
	results, err := client.KVBatchSet(entries)
	if err != nil {
		t.Fatalf("KVBatchSet failed: %v", err)
	}

	// Verify partial success
	if wasSet, ok := results[0][1].(bool); !ok || !wasSet {
		t.Errorf("Expected key1 to succeed")
	}
	if wasSet, ok := results[1][1].(bool); !ok || wasSet {
		t.Errorf("Expected key2 to fail")
	}
	if wasSet, ok := results[2][1].(bool); !ok || !wasSet {
		t.Errorf("Expected key3 to succeed")
	}
}
