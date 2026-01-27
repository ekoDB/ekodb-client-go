package ekodb

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// TestQueryBuilderSelectFields tests the SelectFields method
func TestQueryBuilderSelectFields(t *testing.T) {
	query := NewQueryBuilder().
		Eq("status", "active").
		SelectFields("name", "email", "created_at").
		Build()

	if query["select_fields"] == nil {
		t.Fatal("Expected select_fields to be set")
	}

	fields, ok := query["select_fields"].([]string)
	if !ok {
		t.Fatal("Expected select_fields to be []string")
	}

	if len(fields) != 3 {
		t.Fatalf("Expected 3 fields, got %d", len(fields))
	}

	expectedFields := map[string]bool{"name": true, "email": true, "created_at": true}
	for _, field := range fields {
		if !expectedFields[field] {
			t.Errorf("Unexpected field: %s", field)
		}
	}
}

// TestQueryBuilderExcludeFields tests the ExcludeFields method
func TestQueryBuilderExcludeFields(t *testing.T) {
	query := NewQueryBuilder().
		Eq("user_role", "admin").
		ExcludeFields("password", "api_key", "secret_token").
		Build()

	if query["exclude_fields"] == nil {
		t.Fatal("Expected exclude_fields to be set")
	}

	fields, ok := query["exclude_fields"].([]string)
	if !ok {
		t.Fatal("Expected exclude_fields to be []string")
	}

	if len(fields) != 3 {
		t.Fatalf("Expected 3 fields, got %d", len(fields))
	}

	expectedFields := map[string]bool{"password": true, "api_key": true, "secret_token": true}
	for _, field := range fields {
		if !expectedFields[field] {
			t.Errorf("Unexpected field: %s", field)
		}
	}
}

// TestQueryBuilderBothProjections tests using both select and exclude.
// When both are used, select is applied first, then exclude removes fields
// from the selected set. This is useful for selecting a group of fields
// but excluding specific nested fields (e.g., select "metadata" but exclude "metadata.internal").
func TestQueryBuilderBothProjections(t *testing.T) {
	query := NewQueryBuilder().
		Eq("status", "active").
		SelectFields("name", "email", "metadata").
		ExcludeFields("metadata.internal").
		Build()

	if query["select_fields"] == nil {
		t.Fatal("Expected select_fields to be set")
	}

	if query["exclude_fields"] == nil {
		t.Fatal("Expected exclude_fields to be set")
	}

	selectFields := query["select_fields"].([]string)
	excludeFields := query["exclude_fields"].([]string)

	if len(selectFields) != 3 {
		t.Fatalf("Expected 3 select fields, got %d", len(selectFields))
	}

	if len(excludeFields) != 1 {
		t.Fatalf("Expected 1 exclude field, got %d", len(excludeFields))
	}
}

// TestQueryBuilderProjectionWithComplexQuery tests projection with complex filters
func TestQueryBuilderProjectionWithComplexQuery(t *testing.T) {
	query := NewQueryBuilder().
		Eq("status", "active").
		Gte("age", 18).
		Lt("age", 65).
		SelectFields("id", "name", "email").
		SortDescending("created_at").
		Limit(10).
		Build()

	// Verify filter exists
	if query["filter"] == nil {
		t.Fatal("Expected filter to be set")
	}

	// Verify projection
	if query["select_fields"] == nil {
		t.Fatal("Expected select_fields to be set")
	}

	fields := query["select_fields"].([]string)
	if len(fields) != 3 {
		t.Fatalf("Expected 3 fields, got %d", len(fields))
	}

	// Verify sort
	if query["sort"] == nil {
		t.Fatal("Expected sort to be set")
	}

	// Verify limit
	if query["limit"] == nil {
		t.Fatal("Expected limit to be set")
	}

	limit := query["limit"].(int)
	if limit != 10 {
		t.Fatalf("Expected limit 10, got %d", limit)
	}
}

// TestProjectionPreservesOtherFields tests that projection doesn't interfere with other query params
func TestProjectionPreservesOtherFields(t *testing.T) {
	query := NewQueryBuilder().
		Eq("type", "user").
		SelectFields("username", "email").
		BypassCache(true).
		BypassRipple(true).
		Skip(20).
		Build()

	// Check all fields are present
	if query["filter"] == nil {
		t.Error("filter should be present")
	}
	if query["select_fields"] == nil {
		t.Error("select_fields should be present")
	}
	if query["bypass_cache"] != true {
		t.Error("bypass_cache should be true")
	}
	if query["bypass_ripple"] != true {
		t.Error("bypass_ripple should be true")
	}
	if query["skip"] != 20 {
		t.Errorf("skip should be 20, got %v", query["skip"])
	}
}

// TestEmptyProjection tests that empty projection doesn't add fields
func TestEmptyProjection(t *testing.T) {
	query := NewQueryBuilder().
		Eq("status", "active").
		SelectFields().
		Build()

	// Empty SelectFields() should not add the field to the query
	if query["select_fields"] != nil {
		fields := query["select_fields"].([]string)
		if len(fields) != 0 {
			t.Fatalf("Expected no select_fields or empty array, got %d fields", len(fields))
		}
	}
	// Test passes if select_fields is nil or empty
}

// ============================================================================
// FindByIDWithProjection Client Tests
// ============================================================================

// Note: Uses createTestServer from client_test.go

func TestFindByIDWithProjectionSelectFields(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/find/users": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// Return only the selected fields
			json.NewEncoder(w).Encode([]Record{
				{"id": "user_123", "name": "Alice", "email": "alice@example.com"},
			})
		},
	}

	server := createTestServer(t, handlers)
	defer server.Close()

	client, err := NewClientWithConfig(ClientConfig{
		BaseURL:     server.URL,
		APIKey:      "test-api-key",
		ShouldRetry: false,
		Timeout:     5 * time.Second,
		Format:      JSON,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	result, err := client.FindByIDWithProjection("users", "user_123", []string{"name", "email"}, nil)
	if err != nil {
		t.Fatalf("FindByIDWithProjection failed: %v", err)
	}
	if result["id"] != "user_123" {
		t.Errorf("Expected id user_123, got %v", result["id"])
	}
	if result["name"] != "Alice" {
		t.Errorf("Expected name Alice, got %v", result["name"])
	}
}

func TestFindByIDWithProjectionExcludeFields(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/find/users": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// Return record without excluded fields
			json.NewEncoder(w).Encode([]Record{
				{"id": "user_123", "name": "Alice"},
			})
		},
	}

	server := createTestServer(t, handlers)
	defer server.Close()

	client, err := NewClientWithConfig(ClientConfig{
		BaseURL:     server.URL,
		APIKey:      "test-api-key",
		ShouldRetry: false,
		Timeout:     5 * time.Second,
		Format:      JSON,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	result, err := client.FindByIDWithProjection("users", "user_123", nil, []string{"password", "secret"})
	if err != nil {
		t.Fatalf("FindByIDWithProjection failed: %v", err)
	}
	if result["id"] != "user_123" {
		t.Errorf("Expected id user_123, got %v", result["id"])
	}
}

func TestFindByIDWithProjectionNotFound(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"POST /api/find/users": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// Return empty array for not found
			json.NewEncoder(w).Encode([]Record{})
		},
	}

	server := createTestServer(t, handlers)
	defer server.Close()

	client, err := NewClientWithConfig(ClientConfig{
		BaseURL:     server.URL,
		APIKey:      "test-api-key",
		ShouldRetry: false,
		Timeout:     5 * time.Second,
		Format:      JSON,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	_, err = client.FindByIDWithProjection("users", "nonexistent", []string{"name"}, nil)
	if err == nil {
		t.Fatal("Expected error for not found document")
	}

	// Verify it returns HTTPError with 404 for consistency with FindByID
	httpErr, ok := err.(*HTTPError)
	if !ok {
		t.Fatalf("Expected HTTPError, got %T", err)
	}
	if !httpErr.IsNotFound() {
		t.Errorf("Expected 404 status, got %d", httpErr.StatusCode)
	}
}
