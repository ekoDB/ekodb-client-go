package ekodb

import (
	"encoding/json"
	"net/http"
	"testing"
)

// ============================================================================
// KV Links Tests
// ============================================================================

func TestKVGetLinks(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/kv/links/my-key": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"links": []map[string]interface{}{
					{"collection": "users", "document_id": "doc_1"},
					{"collection": "users", "document_id": "doc_2"},
				},
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.KVGetLinks("my-key")
	if err != nil {
		t.Fatalf("KVGetLinks failed: %v", err)
	}
	if result["links"] == nil {
		t.Error("Expected links field")
	}
}

func TestKVLink(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/kv/link": func(w http.ResponseWriter, r *http.Request) {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["key"] != "my-key" {
				t.Errorf("Expected key my-key, got %v", body["key"])
			}
			if body["collection"] != "users" {
				t.Errorf("Expected collection users, got %v", body["collection"])
			}
			if body["document_id"] != "doc_1" {
				t.Errorf("Expected document_id doc_1, got %v", body["document_id"])
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true, "key": "my-key", "collection": "users", "document_id": "doc_1",
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.KVLink("my-key", "users", "doc_1")
	if err != nil {
		t.Fatalf("KVLink failed: %v", err)
	}
	if result["ok"] != true {
		t.Errorf("Expected ok true, got %v", result["ok"])
	}
}

func TestKVUnlink(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/kv/unlink": func(w http.ResponseWriter, r *http.Request) {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["key"] != "my-key" {
				t.Errorf("Expected key my-key, got %v", body["key"])
			}
			if body["collection"] != "users" {
				t.Errorf("Expected collection users, got %v", body["collection"])
			}
			if body["document_id"] != "doc_1" {
				t.Errorf("Expected document_id doc_1, got %v", body["document_id"])
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.KVUnlink("my-key", "users", "doc_1")
	if err != nil {
		t.Fatalf("KVUnlink failed: %v", err)
	}
	if result["ok"] != true {
		t.Errorf("Expected ok true, got %v", result["ok"])
	}
}

// ============================================================================
// Error Tests
// ============================================================================

func TestKVGetLinksNotFound(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/kv/links/missing-key": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("Key not found"))
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.KVGetLinks("missing-key")
	if err == nil {
		t.Fatal("Expected error for non-existent key")
	}
}

func TestKVLinkServerError(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/kv/link": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("Internal Server Error"))
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.KVLink("bad-key", "users", "doc_1")
	if err == nil {
		t.Fatal("Expected error for server error")
	}
}

func TestKVUnlinkNotFound(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/kv/unlink": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("Link not found"))
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.KVUnlink("missing-key", "users", "doc_1")
	if err == nil {
		t.Fatal("Expected error for non-existent link")
	}
}

func TestKVGetLinksEmpty(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/kv/links/empty-key": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"links": []interface{}{},
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.KVGetLinks("empty-key")
	if err != nil {
		t.Fatalf("KVGetLinks failed: %v", err)
	}
	links, ok := result["links"].([]interface{})
	if !ok {
		t.Fatal("Expected links to be an array")
	}
	if len(links) != 0 {
		t.Errorf("Expected 0 links, got %d", len(links))
	}
}
