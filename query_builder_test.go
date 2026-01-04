package ekodb

import (
	"encoding/json"
	"testing"
)

// ============================================================================
// QueryBuilder Basic Tests
// ============================================================================

func TestNewQueryBuilder(t *testing.T) {
	qb := NewQueryBuilder()
	if qb == nil {
		t.Fatal("NewQueryBuilder returned nil")
	}
	if len(qb.filters) != 0 {
		t.Errorf("Expected empty filters, got %d", len(qb.filters))
	}
	if len(qb.sortFields) != 0 {
		t.Errorf("Expected empty sortFields, got %d", len(qb.sortFields))
	}
}

func TestQueryBuilderBuildEmpty(t *testing.T) {
	qb := NewQueryBuilder()
	query := qb.Build()

	if len(query) != 0 {
		t.Errorf("Expected empty query, got %v", query)
	}
}

// ============================================================================
// Equality Operators Tests
// ============================================================================

func TestQueryBuilderEq(t *testing.T) {
	qb := NewQueryBuilder().Eq("status", "active")
	query := qb.Build()

	filter, ok := query["filter"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected filter to be a map")
	}
	if filter["type"] != "Condition" {
		t.Errorf("Expected type Condition, got %v", filter["type"])
	}

	content := filter["content"].(map[string]interface{})
	if content["operator"] != "Eq" {
		t.Errorf("Expected operator Eq, got %v", content["operator"])
	}
	if content["field"] != "status" {
		t.Errorf("Expected field status, got %v", content["field"])
	}
	if content["value"] != "active" {
		t.Errorf("Expected value active, got %v", content["value"])
	}
}

func TestQueryBuilderNe(t *testing.T) {
	qb := NewQueryBuilder().Ne("status", "deleted")
	query := qb.Build()

	filter := query["filter"].(map[string]interface{})
	content := filter["content"].(map[string]interface{})
	if content["operator"] != "Ne" {
		t.Errorf("Expected operator Ne, got %v", content["operator"])
	}
}

// ============================================================================
// Comparison Operators Tests
// ============================================================================

func TestQueryBuilderGt(t *testing.T) {
	qb := NewQueryBuilder().Gt("age", 18)
	query := qb.Build()

	filter := query["filter"].(map[string]interface{})
	content := filter["content"].(map[string]interface{})
	if content["operator"] != "Gt" {
		t.Errorf("Expected operator Gt, got %v", content["operator"])
	}
	if content["value"] != 18 {
		t.Errorf("Expected value 18, got %v", content["value"])
	}
}

func TestQueryBuilderGte(t *testing.T) {
	qb := NewQueryBuilder().Gte("score", 80)
	query := qb.Build()

	filter := query["filter"].(map[string]interface{})
	content := filter["content"].(map[string]interface{})
	if content["operator"] != "Gte" {
		t.Errorf("Expected operator Gte, got %v", content["operator"])
	}
}

func TestQueryBuilderLt(t *testing.T) {
	qb := NewQueryBuilder().Lt("price", 100.50)
	query := qb.Build()

	filter := query["filter"].(map[string]interface{})
	content := filter["content"].(map[string]interface{})
	if content["operator"] != "Lt" {
		t.Errorf("Expected operator Lt, got %v", content["operator"])
	}
}

func TestQueryBuilderLte(t *testing.T) {
	qb := NewQueryBuilder().Lte("count", 1000)
	query := qb.Build()

	filter := query["filter"].(map[string]interface{})
	content := filter["content"].(map[string]interface{})
	if content["operator"] != "Lte" {
		t.Errorf("Expected operator Lte, got %v", content["operator"])
	}
}

// ============================================================================
// Array Operators Tests
// ============================================================================

func TestQueryBuilderIn(t *testing.T) {
	qb := NewQueryBuilder().In("status", []interface{}{"active", "pending"})
	query := qb.Build()

	filter := query["filter"].(map[string]interface{})
	content := filter["content"].(map[string]interface{})
	if content["operator"] != "In" {
		t.Errorf("Expected operator In, got %v", content["operator"])
	}

	values := content["value"].([]interface{})
	if len(values) != 2 {
		t.Errorf("Expected 2 values, got %d", len(values))
	}
}

func TestQueryBuilderNin(t *testing.T) {
	qb := NewQueryBuilder().Nin("role", []interface{}{"blocked", "deleted"})
	query := qb.Build()

	filter := query["filter"].(map[string]interface{})
	content := filter["content"].(map[string]interface{})
	if content["operator"] != "NotIn" {
		t.Errorf("Expected operator NotIn, got %v", content["operator"])
	}
}

// ============================================================================
// String Operators Tests
// ============================================================================

func TestQueryBuilderContains(t *testing.T) {
	qb := NewQueryBuilder().Contains("email", "@example.com")
	query := qb.Build()

	filter := query["filter"].(map[string]interface{})
	content := filter["content"].(map[string]interface{})
	if content["operator"] != "Contains" {
		t.Errorf("Expected operator Contains, got %v", content["operator"])
	}
	if content["value"] != "@example.com" {
		t.Errorf("Expected value @example.com, got %v", content["value"])
	}
}

func TestQueryBuilderStartsWith(t *testing.T) {
	qb := NewQueryBuilder().StartsWith("name", "John")
	query := qb.Build()

	filter := query["filter"].(map[string]interface{})
	content := filter["content"].(map[string]interface{})
	if content["operator"] != "StartsWith" {
		t.Errorf("Expected operator StartsWith, got %v", content["operator"])
	}
}

func TestQueryBuilderEndsWith(t *testing.T) {
	qb := NewQueryBuilder().EndsWith("filename", ".pdf")
	query := qb.Build()

	filter := query["filter"].(map[string]interface{})
	content := filter["content"].(map[string]interface{})
	if content["operator"] != "EndsWith" {
		t.Errorf("Expected operator EndsWith, got %v", content["operator"])
	}
}

func TestQueryBuilderRegex(t *testing.T) {
	qb := NewQueryBuilder().Regex("phone", "^\\+1")
	query := qb.Build()

	filter := query["filter"].(map[string]interface{})
	content := filter["content"].(map[string]interface{})
	if content["operator"] != "Regex" {
		t.Errorf("Expected operator Regex, got %v", content["operator"])
	}
}

// ============================================================================
// Logical Operators Tests
// ============================================================================

func TestQueryBuilderAnd(t *testing.T) {
	conditions := []map[string]interface{}{
		{
			"type": "Condition",
			"content": map[string]interface{}{
				"field":    "status",
				"operator": "Eq",
				"value":    "active",
			},
		},
		{
			"type": "Condition",
			"content": map[string]interface{}{
				"field":    "age",
				"operator": "Gt",
				"value":    18,
			},
		},
	}

	qb := NewQueryBuilder().And(conditions)
	query := qb.Build()

	filter := query["filter"].(map[string]interface{})
	if filter["type"] != "Logical" {
		t.Errorf("Expected type Logical, got %v", filter["type"])
	}

	content := filter["content"].(map[string]interface{})
	if content["operator"] != "And" {
		t.Errorf("Expected operator And, got %v", content["operator"])
	}

	expressions := content["expressions"].([]map[string]interface{})
	if len(expressions) != 2 {
		t.Errorf("Expected 2 expressions, got %d", len(expressions))
	}
}

func TestQueryBuilderOr(t *testing.T) {
	conditions := []map[string]interface{}{
		{
			"type": "Condition",
			"content": map[string]interface{}{
				"field":    "role",
				"operator": "Eq",
				"value":    "admin",
			},
		},
		{
			"type": "Condition",
			"content": map[string]interface{}{
				"field":    "role",
				"operator": "Eq",
				"value":    "super_admin",
			},
		},
	}

	qb := NewQueryBuilder().Or(conditions)
	query := qb.Build()

	filter := query["filter"].(map[string]interface{})
	content := filter["content"].(map[string]interface{})
	if content["operator"] != "Or" {
		t.Errorf("Expected operator Or, got %v", content["operator"])
	}
}

func TestQueryBuilderNot(t *testing.T) {
	condition := map[string]interface{}{
		"type": "Condition",
		"content": map[string]interface{}{
			"field":    "deleted",
			"operator": "Eq",
			"value":    true,
		},
	}

	qb := NewQueryBuilder().Not(condition)
	query := qb.Build()

	filter := query["filter"].(map[string]interface{})
	content := filter["content"].(map[string]interface{})
	if content["operator"] != "Not" {
		t.Errorf("Expected operator Not, got %v", content["operator"])
	}
}

// ============================================================================
// Multiple Filters (Auto AND) Tests
// ============================================================================

func TestQueryBuilderMultipleFilters(t *testing.T) {
	qb := NewQueryBuilder().
		Eq("status", "active").
		Gt("age", 18).
		Contains("email", "@company.com")

	query := qb.Build()

	filter := query["filter"].(map[string]interface{})
	if filter["type"] != "Logical" {
		t.Errorf("Expected type Logical for multiple filters, got %v", filter["type"])
	}

	content := filter["content"].(map[string]interface{})
	if content["operator"] != "And" {
		t.Errorf("Expected auto-AND for multiple filters, got %v", content["operator"])
	}

	expressions := content["expressions"].([]map[string]interface{})
	if len(expressions) != 3 {
		t.Errorf("Expected 3 expressions, got %d", len(expressions))
	}
}

// ============================================================================
// Sorting Tests
// ============================================================================

func TestQueryBuilderSortAscending(t *testing.T) {
	qb := NewQueryBuilder().SortAscending("name")
	query := qb.Build()

	sort := query["sort"].([]map[string]interface{})
	if len(sort) != 1 {
		t.Fatalf("Expected 1 sort field, got %d", len(sort))
	}
	if sort[0]["field"] != "name" {
		t.Errorf("Expected field name, got %v", sort[0]["field"])
	}
	if sort[0]["ascending"] != true {
		t.Errorf("Expected ascending true, got %v", sort[0]["ascending"])
	}
}

func TestQueryBuilderSortDescending(t *testing.T) {
	qb := NewQueryBuilder().SortDescending("created_at")
	query := qb.Build()

	sort := query["sort"].([]map[string]interface{})
	if sort[0]["ascending"] != false {
		t.Errorf("Expected ascending false, got %v", sort[0]["ascending"])
	}
}

func TestQueryBuilderMultipleSorts(t *testing.T) {
	qb := NewQueryBuilder().
		SortDescending("created_at").
		SortAscending("name")

	query := qb.Build()

	sort := query["sort"].([]map[string]interface{})
	if len(sort) != 2 {
		t.Errorf("Expected 2 sort fields, got %d", len(sort))
	}
}

// ============================================================================
// Pagination Tests
// ============================================================================

func TestQueryBuilderLimit(t *testing.T) {
	qb := NewQueryBuilder().Limit(10)
	query := qb.Build()

	if query["limit"] != 10 {
		t.Errorf("Expected limit 10, got %v", query["limit"])
	}
}

func TestQueryBuilderSkip(t *testing.T) {
	qb := NewQueryBuilder().Skip(20)
	query := qb.Build()

	if query["skip"] != 20 {
		t.Errorf("Expected skip 20, got %v", query["skip"])
	}
}

func TestQueryBuilderPage(t *testing.T) {
	// Page 2 with 20 items per page = skip 40
	qb := NewQueryBuilder().Page(2, 20)
	query := qb.Build()

	if query["limit"] != 20 {
		t.Errorf("Expected limit 20, got %v", query["limit"])
	}
	if query["skip"] != 40 {
		t.Errorf("Expected skip 40, got %v", query["skip"])
	}
}

func TestQueryBuilderPageZero(t *testing.T) {
	// Page 0 with 10 items = skip 0
	qb := NewQueryBuilder().Page(0, 10)
	query := qb.Build()

	if query["skip"] != 0 {
		t.Errorf("Expected skip 0, got %v", query["skip"])
	}
}

// ============================================================================
// Join Tests
// ============================================================================

func TestQueryBuilderJoin(t *testing.T) {
	joinConfig := map[string]interface{}{
		"collections":   []string{"users"},
		"local_field":   "user_id",
		"foreign_field": "id",
		"as_field":      "user",
	}

	qb := NewQueryBuilder().Join(joinConfig)
	query := qb.Build()

	join := query["join"].(map[string]interface{})
	if join["local_field"] != "user_id" {
		t.Errorf("Expected local_field user_id, got %v", join["local_field"])
	}
}

// ============================================================================
// Bypass Flags Tests
// ============================================================================

func TestQueryBuilderBypassCache(t *testing.T) {
	qb := NewQueryBuilder().BypassCache(true)
	query := qb.Build()

	if query["bypass_cache"] != true {
		t.Errorf("Expected bypass_cache true, got %v", query["bypass_cache"])
	}
}

func TestQueryBuilderBypassCacheFalse(t *testing.T) {
	qb := NewQueryBuilder().BypassCache(false)
	query := qb.Build()

	// Should not include bypass_cache when false
	if _, ok := query["bypass_cache"]; ok {
		t.Error("bypass_cache should not be included when false")
	}
}

func TestQueryBuilderBypassRipple(t *testing.T) {
	qb := NewQueryBuilder().BypassRipple(true)
	query := qb.Build()

	if query["bypass_ripple"] != true {
		t.Errorf("Expected bypass_ripple true, got %v", query["bypass_ripple"])
	}
}

// ============================================================================
// Chaining Tests
// ============================================================================

func TestQueryBuilderChaining(t *testing.T) {
	qb := NewQueryBuilder().
		Eq("status", "active").
		Gt("age", 18).
		SortDescending("created_at").
		SortAscending("name").
		Limit(10).
		Skip(20).
		BypassCache(true)

	query := qb.Build()

	// Check filter exists
	if _, ok := query["filter"]; !ok {
		t.Error("Expected filter in query")
	}

	// Check sort exists
	sort := query["sort"].([]map[string]interface{})
	if len(sort) != 2 {
		t.Errorf("Expected 2 sort fields, got %d", len(sort))
	}

	// Check pagination
	if query["limit"] != 10 {
		t.Errorf("Expected limit 10, got %v", query["limit"])
	}
	if query["skip"] != 20 {
		t.Errorf("Expected skip 20, got %v", query["skip"])
	}

	// Check bypass flag
	if query["bypass_cache"] != true {
		t.Errorf("Expected bypass_cache true, got %v", query["bypass_cache"])
	}
}

// ============================================================================
// BuildJSON Tests
// ============================================================================

func TestQueryBuilderBuildJSON(t *testing.T) {
	qb := NewQueryBuilder().
		Eq("status", "active").
		Limit(10)

	jsonBytes, err := qb.BuildJSON()
	if err != nil {
		t.Fatalf("BuildJSON failed: %v", err)
	}

	// Verify it's valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("BuildJSON produced invalid JSON: %v", err)
	}

	if parsed["limit"] != float64(10) { // JSON numbers are float64
		t.Errorf("Expected limit 10 in JSON, got %v", parsed["limit"])
	}
}

func TestQueryBuilderBuildJSONEmpty(t *testing.T) {
	qb := NewQueryBuilder()
	jsonBytes, err := qb.BuildJSON()
	if err != nil {
		t.Fatalf("BuildJSON failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("BuildJSON produced invalid JSON: %v", err)
	}

	if len(parsed) != 0 {
		t.Errorf("Expected empty JSON object, got %v", parsed)
	}
}

// ============================================================================
// SortOrder Constants Tests
// ============================================================================

func TestSortOrderConstants(t *testing.T) {
	if SortAsc != "asc" {
		t.Errorf("SortAsc = %v, want asc", SortAsc)
	}
	if SortDesc != "desc" {
		t.Errorf("SortDesc = %v, want desc", SortDesc)
	}
}
