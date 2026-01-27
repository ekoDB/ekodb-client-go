package ekodb

import (
	"encoding/json"
	"testing"
)

// ============================================================================
// ScriptCondition Serialization Tests
// ============================================================================

func TestConditionHasRecordsSerialization(t *testing.T) {
	cond := ConditionHasRecords()
	data, err := json.Marshal(cond)
	if err != nil {
		t.Fatalf("Failed to marshal HasRecords: %v", err)
	}

	expected := `{"type":"HasRecords"}`
	if string(data) != expected {
		t.Errorf("Expected %s, got %s", expected, string(data))
	}
}

func TestConditionFieldExistsSerialization(t *testing.T) {
	cond := ConditionFieldExists("value")
	data, err := json.Marshal(cond)
	if err != nil {
		t.Fatalf("Failed to marshal FieldExists: %v", err)
	}

	// Parse to verify structure
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if result["type"] != "FieldExists" {
		t.Errorf("Expected type FieldExists, got %v", result["type"])
	}

	value, ok := result["value"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected value to be a map")
	}

	if value["field"] != "value" {
		t.Errorf("Expected field 'value', got %v", value["field"])
	}
}

func TestConditionFieldEqualsSerialization(t *testing.T) {
	cond := ConditionFieldEquals("status", "active")
	data, err := json.Marshal(cond)
	if err != nil {
		t.Fatalf("Failed to marshal FieldEquals: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if result["type"] != "FieldEquals" {
		t.Errorf("Expected type FieldEquals, got %v", result["type"])
	}

	value, ok := result["value"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected value to be a map")
	}

	if value["field"] != "status" {
		t.Errorf("Expected field 'status', got %v", value["field"])
	}
	if value["value"] != "active" {
		t.Errorf("Expected value 'active', got %v", value["value"])
	}
}

func TestConditionCountEqualsSerialization(t *testing.T) {
	cond := ConditionCountEquals(5)
	data, err := json.Marshal(cond)
	if err != nil {
		t.Fatalf("Failed to marshal CountEquals: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if result["type"] != "CountEquals" {
		t.Errorf("Expected type CountEquals, got %v", result["type"])
	}

	value, ok := result["value"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected value to be a map")
	}

	// JSON numbers are float64
	if value["count"] != float64(5) {
		t.Errorf("Expected count 5, got %v", value["count"])
	}
}

func TestConditionCountGreaterThanSerialization(t *testing.T) {
	cond := ConditionCountGreaterThan(10)
	data, err := json.Marshal(cond)
	if err != nil {
		t.Fatalf("Failed to marshal CountGreaterThan: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if result["type"] != "CountGreaterThan" {
		t.Errorf("Expected type CountGreaterThan, got %v", result["type"])
	}

	value := result["value"].(map[string]interface{})
	if value["count"] != float64(10) {
		t.Errorf("Expected count 10, got %v", value["count"])
	}
}

func TestConditionCountLessThanSerialization(t *testing.T) {
	cond := ConditionCountLessThan(3)
	data, err := json.Marshal(cond)
	if err != nil {
		t.Fatalf("Failed to marshal CountLessThan: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if result["type"] != "CountLessThan" {
		t.Errorf("Expected type CountLessThan, got %v", result["type"])
	}

	value := result["value"].(map[string]interface{})
	if value["count"] != float64(3) {
		t.Errorf("Expected count 3, got %v", value["count"])
	}
}

func TestConditionAndSerialization(t *testing.T) {
	cond := ConditionAnd([]ScriptCondition{
		ConditionHasRecords(),
		ConditionFieldExists("value"),
	})
	data, err := json.Marshal(cond)
	if err != nil {
		t.Fatalf("Failed to marshal And: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if result["type"] != "And" {
		t.Errorf("Expected type And, got %v", result["type"])
	}

	value, ok := result["value"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected value to be a map")
	}

	conditions, ok := value["conditions"].([]interface{})
	if !ok {
		t.Fatal("Expected conditions to be an array")
	}

	if len(conditions) != 2 {
		t.Errorf("Expected 2 conditions, got %d", len(conditions))
	}

	// Verify first condition is HasRecords
	first := conditions[0].(map[string]interface{})
	if first["type"] != "HasRecords" {
		t.Errorf("Expected first condition type HasRecords, got %v", first["type"])
	}
}

func TestConditionOrSerialization(t *testing.T) {
	cond := ConditionOr([]ScriptCondition{
		ConditionFieldEquals("status", "active"),
		ConditionFieldEquals("status", "pending"),
	})
	data, err := json.Marshal(cond)
	if err != nil {
		t.Fatalf("Failed to marshal Or: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if result["type"] != "Or" {
		t.Errorf("Expected type Or, got %v", result["type"])
	}

	value := result["value"].(map[string]interface{})
	conditions := value["conditions"].([]interface{})

	if len(conditions) != 2 {
		t.Errorf("Expected 2 conditions, got %d", len(conditions))
	}
}

func TestConditionNotSerialization(t *testing.T) {
	cond := ConditionNot(ConditionHasRecords())
	data, err := json.Marshal(cond)
	if err != nil {
		t.Fatalf("Failed to marshal Not: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if result["type"] != "Not" {
		t.Errorf("Expected type Not, got %v", result["type"])
	}

	value, ok := result["value"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected value to be a map")
	}

	condition, ok := value["condition"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected condition to be a map")
	}

	if condition["type"] != "HasRecords" {
		t.Errorf("Expected inner condition type HasRecords, got %v", condition["type"])
	}
}

func TestNestedConditionsSerialization(t *testing.T) {
	// Complex nested condition: (HasRecords AND FieldExists("value")) OR CountEquals(0)
	cond := ConditionOr([]ScriptCondition{
		ConditionAnd([]ScriptCondition{
			ConditionHasRecords(),
			ConditionFieldExists("value"),
		}),
		ConditionCountEquals(0),
	})

	data, err := json.Marshal(cond)
	if err != nil {
		t.Fatalf("Failed to marshal nested conditions: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if result["type"] != "Or" {
		t.Errorf("Expected type Or, got %v", result["type"])
	}

	value := result["value"].(map[string]interface{})
	conditions := value["conditions"].([]interface{})

	if len(conditions) != 2 {
		t.Errorf("Expected 2 conditions, got %d", len(conditions))
	}

	// Verify first condition is And
	first := conditions[0].(map[string]interface{})
	if first["type"] != "And" {
		t.Errorf("Expected first condition type And, got %v", first["type"])
	}

	// Verify second condition is CountEquals
	second := conditions[1].(map[string]interface{})
	if second["type"] != "CountEquals" {
		t.Errorf("Expected second condition type CountEquals, got %v", second["type"])
	}
}

// Test that StageIf correctly embeds the condition
func TestStageIfWithCondition(t *testing.T) {
	stage := StageIf(
		ConditionFieldExists("value"),
		[]FunctionStageConfig{StageProject([]string{"value"}, false)},
		[]FunctionStageConfig{StageProject([]string{"error"}, false)},
	)

	if stage.Stage != "If" {
		t.Errorf("Expected stage If, got %s", stage.Stage)
	}

	// Marshal the whole stage to verify serialization
	jsonData, err := json.Marshal(stage)
	if err != nil {
		t.Fatalf("Failed to marshal StageIf: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	// Verify type is If
	if result["type"] != "If" {
		t.Errorf("Expected type If, got %v", result["type"])
	}

	// Verify condition exists and has correct type
	cond, ok := result["condition"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected condition to be a map")
	}
	if cond["type"] != "FieldExists" {
		t.Errorf("Expected condition type FieldExists, got %v", cond["type"])
	}
}
