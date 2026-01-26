package ekodb

import (
	"encoding/json"
	"testing"
)

// TestSWRStageSerialization tests that SWR stage serializes correctly
func TestSWRStageSerialization(t *testing.T) {
	stage := StageSWR(
		"user:{{user_id}}",
		"15m",
		"https://api.example.com/users/{{user_id}}",
		"GET",
		map[string]string{"User-Agent": "ekoDB-Client"},
		nil,
		nil,
		nil,
		nil,
	)

	data, err := json.Marshal(stage)
	if err != nil {
		t.Fatalf("Failed to marshal SWR stage: %v", err)
	}

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal SWR stage: %v", err)
	}

	// FunctionStageConfig uses custom MarshalJSON that flattens structure
	if result["type"] != "SWR" {
		t.Errorf("Expected type 'SWR', got %v", result["type"])
	}
	if result["cache_key"] != "user:{{user_id}}" {
		t.Errorf("Expected cache_key 'user:{{user_id}}', got %v", result["cache_key"])
	}
	if result["ttl"] != "15m" {
		t.Errorf("Expected ttl '15m', got %v", result["ttl"])
	}
	if result["url"] != "https://api.example.com/users/{{user_id}}" {
		t.Errorf("Expected url 'https://api.example.com/users/{{user_id}}', got %v", result["url"])
	}
	if result["method"] != "GET" {
		t.Errorf("Expected method 'GET', got %v", result["method"])
	}
}

// TestSWRStageWithAuditCollection tests SWR stage with audit trail
func TestSWRStageWithAuditCollection(t *testing.T) {
	outputField := "product"
	collection := "swr_audit_trail"

	stage := StageSWR(
		"product:{{id}}",
		"1h",
		"https://api.example.com/products/{{id}}",
		"GET",
		nil,
		nil,
		nil,
		&outputField,
		&collection,
	)

	data, err := json.Marshal(stage)
	if err != nil {
		t.Fatalf("Failed to marshal SWR stage: %v", err)
	}

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal SWR stage: %v", err)
	}

	if result["output_field"] != "product" {
		t.Errorf("Expected output_field 'product', got %v", result["output_field"])
	}
	if result["collection"] != "swr_audit_trail" {
		t.Errorf("Expected collection 'swr_audit_trail', got %v", result["collection"])
	}
}

// TestSWRStageWithPOSTBody tests SWR stage with POST method and body
func TestSWRStageWithPOSTBody(t *testing.T) {
	body := map[string]interface{}{
		"query": "{{search_term}}",
		"limit": 10,
	}

	stage := StageSWR(
		"search:{{search_term}}",
		900, // Integer seconds
		"https://api.example.com/search",
		"POST",
		nil,
		body,
		nil,
		nil,
		nil,
	)

	data, err := json.Marshal(stage)
	if err != nil {
		t.Fatalf("Failed to marshal SWR stage: %v", err)
	}

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal SWR stage: %v", err)
	}

	if result["method"] != "POST" {
		t.Errorf("Expected method 'POST', got %v", result["method"])
	}

	bodyVal, ok := result["body"]
	if !ok {
		t.Fatalf("Expected 'body' field in result, got: %v", result)
	}
	bodyMap, ok := bodyVal.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected 'body' to be a map[string]interface{}, got: %T (%v)", bodyVal, bodyVal)
	}
	if bodyMap["query"] != "{{search_term}}" {
		t.Errorf("Expected body.query '{{search_term}}', got %v", bodyMap["query"])
	}
}

// TestSWRStageTTLFormats tests various TTL format support
func TestSWRStageTTLFormats(t *testing.T) {
	tests := []struct {
		name        string
		ttl         interface{}
		expectedTTL interface{}
	}{
		{"Duration string", "30m", "30m"},
		{"Integer seconds", 1800, float64(1800)}, // JSON numbers are float64
		{"String seconds", "1800", "1800"},
		{"ISO timestamp", "2026-01-27T12:00:00Z", "2026-01-27T12:00:00Z"}, // Server parses ISO timestamps
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stage := StageSWR(
				"test",
				tt.ttl,
				"https://example.com",
				"GET",
				nil,
				nil,
				nil,
				nil,
				nil,
			)

			data, err := json.Marshal(stage)
			if err != nil {
				t.Fatalf("Failed to marshal SWR stage: %v", err)
			}

			var result map[string]interface{}
			err = json.Unmarshal(data, &result)
			if err != nil {
				t.Fatalf("Failed to unmarshal SWR stage: %v", err)
			}

			if result["ttl"] != tt.expectedTTL {
				t.Errorf("Expected ttl %v, got %v", tt.expectedTTL, result["ttl"])
			}
		})
	}
}

// TestSWRStageWithTimeout tests SWR stage with custom timeout
func TestSWRStageWithTimeout(t *testing.T) {
	timeout := 120

	stage := StageSWR(
		"slow:api",
		"5m",
		"https://slow-api.example.com/data",
		"GET",
		nil,
		nil,
		&timeout,
		nil,
		nil,
	)

	data, err := json.Marshal(stage)
	if err != nil {
		t.Fatalf("Failed to marshal SWR stage: %v", err)
	}

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal SWR stage: %v", err)
	}

	if result["timeout_seconds"] != float64(120) {
		t.Errorf("Expected timeout_seconds 120, got %v", result["timeout_seconds"])
	}
}

// TestSWRStageOptionalFieldsOmitted tests that nil optional fields are omitted
func TestSWRStageOptionalFieldsOmitted(t *testing.T) {
	stage := StageSWR(
		"minimal",
		"15m",
		"https://example.com",
		"GET",
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	data, err := json.Marshal(stage)
	if err != nil {
		t.Fatalf("Failed to marshal SWR stage: %v", err)
	}

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal SWR stage: %v", err)
	}

	// Optional fields should not be present when nil
	optionalFields := []string{"headers", "body", "timeout_seconds", "output_field", "collection"}
	for _, field := range optionalFields {
		if _, exists := result[field]; exists {
			t.Errorf("Optional field '%s' should not be present when nil", field)
		}
	}
}
