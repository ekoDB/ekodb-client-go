package ekodb

import (
	"encoding/json"
	"net/http"
	"testing"
)

// ============================================================================
// Convenience Methods Tests
// ============================================================================

func TestUpsert_UpdatePath(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/find/users/user_123": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(Record{"id": "user_123", "name": "Alice"})
		},
		"PUT /api/update/users/user_123": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(Record{"id": "user_123", "name": "Alice Updated"})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	record := Record{"name": "Alice Updated"}
	result, err := client.Upsert("users", "user_123", record)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	if result["id"] != "user_123" {
		t.Errorf("Expected id user_123, got %v", result["id"])
	}
}

func TestUpsert_ForwardsBypassCacheToExistenceProbe(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/find/users/user_123": func(w http.ResponseWriter, r *http.Request) {
			if got := r.URL.Query().Get("bypass_cache"); got != "true" {
				t.Fatalf("existence probe bypass_cache = %q, want true", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(Record{"id": "user_123"})
		},
		"PUT /api/update/users/user_123": func(w http.ResponseWriter, r *http.Request) {
			if got := r.URL.Query().Get("bypass_cache"); got != "true" {
				t.Fatalf("update bypass_cache = %q, want true", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(Record{"id": "user_123", "name": "updated"})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	bypassCache := true
	if _, err := client.Upsert(
		"users",
		"user_123",
		Record{"name": "updated"},
		UpsertOptions{BypassCache: &bypassCache},
	); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
}

func TestUpsert_InsertPath(t *testing.T) {
	callCount := 0
	var insertBody Record
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/find/users/new_id": func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Not found"})
		},
		"POST /api/insert/users": func(w http.ResponseWriter, r *http.Request) {
			callCount++
			if err := json.NewDecoder(r.Body).Decode(&insertBody); err != nil {
				t.Errorf("decode insert body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(Record{"id": "server_generated_id", "name": "Bob"})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	record := Record{"name": "Bob"}
	result, err := client.Upsert("users", "new_id", record)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	if callCount != 2 {
		t.Errorf("Expected 2 calls (existence check + insert), got %d", callCount)
	}
	if _, ok := insertBody["id"]; ok {
		t.Errorf("insert body unexpectedly contains caller-supplied id: %v", insertBody)
	}
	if result["id"] != "server_generated_id" {
		t.Errorf("Expected server-generated id, got %v", result["id"])
	}
}

func TestUpsert_DoesNotTrustSuccessfulMissingUpdate(t *testing.T) {
	putCalled := false
	var insertBody Record
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/find/users/new_id": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Not found"})
		},
		"PUT /api/update/users/new_id": func(w http.ResponseWriter, r *http.Request) {
			putCalled = true
			_ = json.NewEncoder(w).Encode(Record{"id": "new_id", "name": "Bob"})
		},
		"POST /api/insert/users": func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&insertBody); err != nil {
				t.Errorf("decode insert body: %v", err)
			}
			_ = json.NewEncoder(w).Encode(Record{"id": "server_generated_id", "name": "Bob"})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.Upsert("users", "new_id", Record{"name": "Bob"})
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	if putCalled {
		t.Fatal("Upsert called Update after the existence check reported not found")
	}
	if _, ok := insertBody["id"]; ok {
		t.Errorf("insert body unexpectedly contains caller-supplied id: %v", insertBody)
	}
	if result["id"] != "server_generated_id" {
		t.Errorf("Expected server-generated id, got %v", result["id"])
	}
}

func TestFindOne_Found(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/find/users": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]Record{
				{"id": "user_123", "email": "alice@example.com"},
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.FindOne("users", "email", "alice@example.com")
	if err != nil {
		t.Fatalf("FindOne failed: %v", err)
	}
	if result == nil {
		t.Fatal("Expected result, got nil")
	}
	if result["id"] != "user_123" {
		t.Errorf("Expected id user_123, got %v", result["id"])
	}
}

func TestFindOne_NotFound(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/find/users": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]Record{})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.FindOne("users", "email", "nonexistent@example.com")
	if err != nil {
		t.Fatalf("FindOne failed: %v", err)
	}
	if result != nil {
		t.Errorf("Expected nil, got %v", result)
	}
}

func TestExists_True(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/find/users/user_123": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(Record{"id": "user_123", "name": "Alice"})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	exists, err := client.Exists("users", "user_123")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("Expected exists=true, got false")
	}
}

func TestExists_False(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/find/users/nonexistent": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Not found"})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	exists, err := client.Exists("users", "nonexistent")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Error("Expected exists=false, got true")
	}
}

func TestPaginate_Page1(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/find/users": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]Record{
				{"id": "1"}, {"id": "2"}, {"id": "3"},
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	results, err := client.Paginate("users", 1, 10)
	if err != nil {
		t.Fatalf("Paginate failed: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}
}

func TestPaginate_Page2(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/find/users": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]Record{
				{"id": "11"}, {"id": "12"},
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	results, err := client.Paginate("users", 2, 10)
	if err != nil {
		t.Fatalf("Paginate failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}
