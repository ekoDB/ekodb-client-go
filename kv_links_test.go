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
		"GET /api/kv/my-key/links": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"collection": "users", "document_id": "doc_1"},
				{"collection": "users", "document_id": "doc_2"},
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.KVGetLinks("my-key")
	if err != nil {
		t.Fatalf("KVGetLinks failed: %v", err)
	}
	if len(result) != 2 || result[0]["document_id"] != "doc_1" {
		t.Fatalf("unexpected links: %#v", result)
	}
}

func TestKVLink(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/kv/my-key/links/users/doc_1": func(w http.ResponseWriter, r *http.Request) {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if len(body) != 0 {
				t.Errorf("Expected empty link options, got %#v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(nil)
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	if err := client.KVLink("my-key", "users", "doc_1"); err != nil {
		t.Fatalf("KVLink failed: %v", err)
	}
}

func TestKVUnlink(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"DELETE /api/kv/my-key/links/users/doc_1": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(nil)
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	if err := client.KVUnlink("my-key", "users", "doc_1"); err != nil {
		t.Fatalf("KVUnlink failed: %v", err)
	}
}

func TestKVLinkEscapesEveryPathSegment(t *testing.T) {
	var got capturedRequest
	server := newCapturingServer(t, &got)
	defer server.Close()

	client := createTestClient(t, server)
	if err := client.KVLink("a/b", "users/archive", "doc/1"); err != nil {
		t.Fatalf("KVLink failed: %v", err)
	}
	if got.method != http.MethodPost {
		t.Fatalf("method = %s, want POST", got.method)
	}
	if got.escapedPath != "/api/kv/a%2Fb/links/users%2Farchive/doc%2F1" {
		t.Fatalf("escaped path = %s", got.escapedPath)
	}
}

// ============================================================================
// Error Tests
// ============================================================================

func TestKVGetLinksNotFound(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/kv/missing-key/links": func(w http.ResponseWriter, r *http.Request) {
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
		"POST /api/kv/bad-key/links/users/doc_1": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("Internal Server Error"))
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	if err := client.KVLink("bad-key", "users", "doc_1"); err == nil {
		t.Fatal("Expected error for server error")
	}
}

func TestKVUnlinkNotFound(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"DELETE /api/kv/missing-key/links/users/doc_1": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("Link not found"))
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	if err := client.KVUnlink("missing-key", "users", "doc_1"); err == nil {
		t.Fatal("Expected error for non-existent link")
	}
}

func TestKVGetLinksEmpty(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/kv/empty-key/links": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]interface{}{})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.KVGetLinks("empty-key")
	if err != nil {
		t.Fatalf("KVGetLinks failed: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("Expected 0 links, got %d", len(result))
	}
}
