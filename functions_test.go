package ekodb

// Unit tests for the stored-function builder helpers (Stage* + Parameter).
//
// These cover the pure-data construction side of the library — the shape
// of the JSON that eventually lands on the server. Server-side behavior
// for structural parameter placeholders is covered by the Rust
// integration tests in
// `ekodb/ekodb_server/tests/function_parameters_tests.rs`.

import (
	"encoding/json"
	"reflect"
	"testing"
)

// --------------------------------------------------------------------------
// Parameter()
// --------------------------------------------------------------------------

func TestParameter_shape(t *testing.T) {
	got := Parameter("record")
	want := map[string]interface{}{
		"type": "Parameter",
		"name": "record",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parameter(\"record\") = %v, want %v", got, want)
	}
}

func TestParameter_preservesArbitraryName(t *testing.T) {
	if got := Parameter("user_id")["name"]; got != "user_id" {
		t.Fatalf("Parameter name not preserved: got %v", got)
	}
}

// --------------------------------------------------------------------------
// StageInsert with a structural parameter placeholder
// --------------------------------------------------------------------------

func TestStageInsert_acceptsWholeRecordParameter(t *testing.T) {
	stage := StageInsert("users", Parameter("record"), false, nil)

	if stage.Stage != "Insert" {
		t.Fatalf("stage type = %q, want Insert", stage.Stage)
	}
	if stage.Data["collection"] != "users" {
		t.Fatalf("collection = %v", stage.Data["collection"])
	}
	record, ok := stage.Data["record"].(map[string]interface{})
	if !ok {
		t.Fatalf("record not a map: %T", stage.Data["record"])
	}
	if record["type"] != "Parameter" || record["name"] != "record" {
		t.Fatalf("record is not a Parameter placeholder: %v", record)
	}
}

func TestStageInsert_acceptsPerFieldPlaceholders(t *testing.T) {
	stage := StageInsert("items", map[string]interface{}{
		"label":     "{{label}}",
		"parent_id": Parameter("parent_id"),
		"kind":      "item",
		"tags":      Parameter("tags"),
	}, false, nil)

	record := stage.Data["record"].(map[string]interface{})
	if record["label"] != "{{label}}" {
		t.Fatalf("label placeholder not preserved: %v", record["label"])
	}
	if record["kind"] != "item" {
		t.Fatalf("kind literal not preserved: %v", record["kind"])
	}
	parent := record["parent_id"].(map[string]interface{})
	if parent["type"] != "Parameter" || parent["name"] != "parent_id" {
		t.Fatalf("parent_id Parameter placeholder not preserved: %v", parent)
	}
}

// --------------------------------------------------------------------------
// StageUpdateById with a structural parameter placeholder
// --------------------------------------------------------------------------

func TestStageUpdateById_acceptsWholeUpdatesParameter(t *testing.T) {
	stage := StageUpdateById("items", "{{id}}", Parameter("updates"), false, nil)

	if stage.Stage != "UpdateById" {
		t.Fatalf("stage type = %q, want UpdateById", stage.Stage)
	}
	if stage.Data["record_id"] != "{{id}}" {
		t.Fatalf("record_id = %v", stage.Data["record_id"])
	}
	updates := stage.Data["updates"].(map[string]interface{})
	if updates["type"] != "Parameter" || updates["name"] != "updates" {
		t.Fatalf("updates is not a Parameter placeholder: %v", updates)
	}
}

// --------------------------------------------------------------------------
// StageUpdate (filter-based) with structural filter values + updates
// --------------------------------------------------------------------------

func TestStageUpdate_acceptsParameterInFilterAndUpdates(t *testing.T) {
	filter := map[string]interface{}{
		"type": "Condition",
		"content": map[string]interface{}{
			"field":    "id",
			"operator": "Eq",
			"value":    Parameter("id"),
		},
	}
	stage := StageUpdate("items", filter, Parameter("updates"), false, nil)

	if stage.Stage != "Update" {
		t.Fatalf("stage type = %q", stage.Stage)
	}

	gotFilter := stage.Data["filter"].(map[string]interface{})
	content := gotFilter["content"].(map[string]interface{})
	value := content["value"].(map[string]interface{})
	if value["type"] != "Parameter" || value["name"] != "id" {
		t.Fatalf("filter value is not a Parameter placeholder: %v", value)
	}

	updates := stage.Data["updates"].(map[string]interface{})
	if updates["type"] != "Parameter" || updates["name"] != "updates" {
		t.Fatalf("updates is not a Parameter placeholder: %v", updates)
	}
}

// --------------------------------------------------------------------------
// StageBatchInsert with Parameter placeholders in each record
// --------------------------------------------------------------------------

func TestStageBatchInsert_acceptsParameterInEachRecord(t *testing.T) {
	stage := StageBatchInsert("audit_log", []map[string]interface{}{
		{"actor": Parameter("user_id"), "at": "{{now}}", "message": "created"},
		{"actor": Parameter("user_id"), "at": "{{now}}", "message": "initialized"},
	}, false)

	if stage.Stage != "BatchInsert" {
		t.Fatalf("stage type = %q", stage.Stage)
	}
	records := stage.Data["records"].([]map[string]interface{})
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	first := records[0]["actor"].(map[string]interface{})
	if first["name"] != "user_id" {
		t.Fatalf("actor Parameter placeholder missing: %v", first)
	}
	if records[1]["message"] != "initialized" {
		t.Fatalf("second record message not preserved: %v", records[1]["message"])
	}
}

// --------------------------------------------------------------------------
// JSON serialization — what actually goes on the wire
// --------------------------------------------------------------------------

func TestStageInsert_jsonShapeMatchesEkodbServer(t *testing.T) {
	stage := StageInsert("users", Parameter("record"), false, nil)

	bytes, err := json.Marshal(stage)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Round-trip through json.Unmarshal so map ordering doesn't matter.
	var got map[string]interface{}
	if err := json.Unmarshal(bytes, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got["type"] != "Insert" {
		t.Fatalf("type = %v, want Insert", got["type"])
	}
	if got["collection"] != "users" {
		t.Fatalf("collection = %v", got["collection"])
	}
	record := got["record"].(map[string]interface{})
	if record["type"] != "Parameter" || record["name"] != "record" {
		t.Fatalf("record is not a Parameter placeholder after JSON round-trip: %v", record)
	}
}

func TestStageUpdateById_jsonShapeMatchesEkodbServer(t *testing.T) {
	stage := StageUpdateById("items", "{{id}}", Parameter("updates"), false, nil)

	bytes, err := json.Marshal(stage)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(bytes, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got["type"] != "UpdateById" {
		t.Fatalf("type = %v", got["type"])
	}
	if got["record_id"] != "{{id}}" {
		t.Fatalf("record_id = %v", got["record_id"])
	}
	updates := got["updates"].(map[string]interface{})
	if updates["type"] != "Parameter" || updates["name"] != "updates" {
		t.Fatalf("updates Parameter placeholder lost in JSON: %v", updates)
	}
}

// --------------------------------------------------------------------------
// Crypto primitives: BcryptHash, BcryptVerify, RandomToken (ekoDB >= 0.41.0)
// --------------------------------------------------------------------------

func TestStageBcryptHash_withExplicitCost(t *testing.T) {
	cost := 12
	stage := StageBcryptHash("{{password}}", "password_hash", &cost)

	if stage.Stage != "BcryptHash" {
		t.Fatalf("stage type = %q, want BcryptHash", stage.Stage)
	}
	if stage.Data["plain"] != "{{password}}" {
		t.Fatalf("plain = %v", stage.Data["plain"])
	}
	if stage.Data["cost"] != 12 {
		t.Fatalf("cost = %v, want 12", stage.Data["cost"])
	}
	if stage.Data["output_field"] != "password_hash" {
		t.Fatalf("output_field = %v", stage.Data["output_field"])
	}
}

func TestStageBcryptHash_omitsCostWhenNil(t *testing.T) {
	stage := StageBcryptHash("{{password}}", "pw_hash", nil)
	if _, ok := stage.Data["cost"]; ok {
		t.Fatalf("cost must be omitted when nil, got %v", stage.Data["cost"])
	}
}

func TestStageBcryptVerify_wiresAllFields(t *testing.T) {
	stage := StageBcryptVerify("{{password}}", "password_hash", "valid")

	if stage.Stage != "BcryptVerify" {
		t.Fatalf("stage type = %q", stage.Stage)
	}
	if stage.Data["plain"] != "{{password}}" {
		t.Fatalf("plain = %v", stage.Data["plain"])
	}
	if stage.Data["hash_field"] != "password_hash" {
		t.Fatalf("hash_field = %v", stage.Data["hash_field"])
	}
	if stage.Data["output_field"] != "valid" {
		t.Fatalf("output_field = %v", stage.Data["output_field"])
	}
}

func TestStageRandomToken_withExplicitEncoding(t *testing.T) {
	stage := StageRandomToken(32, "hex", "session_token")

	if stage.Stage != "RandomToken" {
		t.Fatalf("stage type = %q", stage.Stage)
	}
	if stage.Data["bytes"] != 32 {
		t.Fatalf("bytes = %v", stage.Data["bytes"])
	}
	if stage.Data["encoding"] != "hex" {
		t.Fatalf("encoding = %v", stage.Data["encoding"])
	}
	if stage.Data["output_field"] != "session_token" {
		t.Fatalf("output_field = %v", stage.Data["output_field"])
	}
}

func TestStageRandomToken_omitsEncodingWhenEmpty(t *testing.T) {
	stage := StageRandomToken(16, "", "token")
	if _, ok := stage.Data["encoding"]; ok {
		t.Fatalf("encoding must be omitted when empty, got %v", stage.Data["encoding"])
	}
}

func TestCryptoStages_jsonRoundTrip(t *testing.T) {
	cost := 12
	cases := []FunctionStageConfig{
		StageBcryptHash("{{password}}", "password_hash", &cost),
		StageBcryptVerify("{{password}}", "password_hash", "valid"),
		StageRandomToken(32, "base64url", "token"),
	}

	for _, stage := range cases {
		bytes, err := json.Marshal(stage)
		if err != nil {
			t.Fatalf("marshal %s failed: %v", stage.Stage, err)
		}
		var got map[string]interface{}
		if err := json.Unmarshal(bytes, &got); err != nil {
			t.Fatalf("unmarshal %s failed: %v", stage.Stage, err)
		}
		if got["type"] != stage.Stage {
			t.Fatalf("%s: type = %v, want %v", stage.Stage, got["type"], stage.Stage)
		}
	}
}
