package ekodb

import (
	"testing"
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

// TestQueryBuilderBothProjections tests using both select and exclude
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
