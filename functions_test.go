package ekodb

// Unit tests for the stored-function builder helpers (Stage* + Parameter).
//
// These cover the pure-data construction side of the library — the shape
// of the JSON that eventually lands on the server. Server-side behavior
// for structural parameter placeholders is covered by integration tests
// in the Rust server repository.

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

// --------------------------------------------------------------------------
// JWT primitives: JwtSign, JwtVerify (ekoDB >= 0.42.0)
// --------------------------------------------------------------------------

func TestStageJwtSign_withClaimsExpiryAndAlgorithm(t *testing.T) {
	exp := int64(3600)
	claims := map[string]interface{}{"sub": "{{user_id}}", "role": "admin"}
	stage := StageJwtSign(claims, "{{env.JWT_SECRET}}", "token", &exp, "HS256")

	if stage.Stage != "JwtSign" {
		t.Fatalf("stage = %q, want JwtSign", stage.Stage)
	}
	if stage.Data["secret"] != "{{env.JWT_SECRET}}" {
		t.Fatalf("secret = %v", stage.Data["secret"])
	}
	if stage.Data["expires_in_secs"] != int64(3600) {
		t.Fatalf("expires_in_secs = %v", stage.Data["expires_in_secs"])
	}
	if stage.Data["algorithm"] != "HS256" {
		t.Fatalf("algorithm = %v", stage.Data["algorithm"])
	}
	if stage.Data["output_field"] != "token" {
		t.Fatalf("output_field = %v", stage.Data["output_field"])
	}
}

func TestStageJwtSign_omitsOptionalFieldsWhenEmpty(t *testing.T) {
	stage := StageJwtSign(
		map[string]interface{}{"sub": "u"},
		"{{env.JWT_SECRET}}",
		"t",
		nil,
		"",
	)
	if _, ok := stage.Data["algorithm"]; ok {
		t.Fatalf("algorithm must be omitted when empty, got %v", stage.Data["algorithm"])
	}
	if _, ok := stage.Data["expires_in_secs"]; ok {
		t.Fatalf("expires_in_secs must be omitted when nil, got %v", stage.Data["expires_in_secs"])
	}
}

func TestStageJwtVerify_wiresAllFields(t *testing.T) {
	stage := StageJwtVerify("auth_token", "{{env.JWT_SECRET}}", "claims", "HS512")
	if stage.Stage != "JwtVerify" {
		t.Fatalf("stage = %q", stage.Stage)
	}
	if stage.Data["token_field"] != "auth_token" {
		t.Fatalf("token_field = %v", stage.Data["token_field"])
	}
	if stage.Data["secret"] != "{{env.JWT_SECRET}}" {
		t.Fatalf("secret = %v", stage.Data["secret"])
	}
	if stage.Data["algorithm"] != "HS512" {
		t.Fatalf("algorithm = %v", stage.Data["algorithm"])
	}
	if stage.Data["output_field"] != "claims" {
		t.Fatalf("output_field = %v", stage.Data["output_field"])
	}
}

func TestJwtStages_jsonRoundTrip(t *testing.T) {
	exp := int64(900)
	cases := []FunctionStageConfig{
		StageJwtSign(
			map[string]interface{}{"sub": "u-1"},
			"{{env.JWT_SECRET}}",
			"token",
			&exp,
			"HS256",
		),
		StageJwtVerify("token", "{{env.JWT_SECRET}}", "claims", ""),
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

// --------------------------------------------------------------------------
// EmailSend (ekoDB >= 0.42.0)
// --------------------------------------------------------------------------

func TestStageEmailSend_withFullPayload(t *testing.T) {
	htmlOn := true
	stage := StageEmailSend(
		"alice@example.com",
		"Welcome",
		"<p>Hi Alice</p>",
		"bot@example.com",
		"{{env.SENDGRID_API_KEY}}",
		&EmailSendOptions{
			ReplyTo:     "support@example.com",
			Provider:    "sendgrid",
			HTML:        &htmlOn,
			OutputField: "send_result",
		},
	)
	if stage.Stage != "EmailSend" {
		t.Fatalf("stage = %q, want EmailSend", stage.Stage)
	}
	if stage.Data["to"] != "alice@example.com" {
		t.Fatalf("to = %v", stage.Data["to"])
	}
	if stage.Data["from"] != "bot@example.com" {
		t.Fatalf("from = %v", stage.Data["from"])
	}
	if stage.Data["reply_to"] != "support@example.com" {
		t.Fatalf("reply_to = %v", stage.Data["reply_to"])
	}
	if stage.Data["api_key"] != "{{env.SENDGRID_API_KEY}}" {
		t.Fatalf("api_key = %v", stage.Data["api_key"])
	}
	if stage.Data["provider"] != "sendgrid" {
		t.Fatalf("provider = %v", stage.Data["provider"])
	}
	if stage.Data["html"] != true {
		t.Fatalf("html = %v", stage.Data["html"])
	}
	if stage.Data["output_field"] != "send_result" {
		t.Fatalf("output_field = %v", stage.Data["output_field"])
	}
}

func TestStageEmailSend_omitsOptionalFieldsWhenEmpty(t *testing.T) {
	stage := StageEmailSend("x@example.com", "s", "b", "f@example.com", "k", nil)
	for _, k := range []string{"reply_to", "provider", "html", "output_field"} {
		if _, ok := stage.Data[k]; ok {
			t.Fatalf("%s must be omitted when nil opts, got %v", k, stage.Data[k])
		}
	}
}

func TestStageEmailSend_jsonRoundTrip(t *testing.T) {
	htmlOn := true
	stage := StageEmailSend(
		"u@example.com",
		"Hi",
		"<p>Hi</p>",
		"f@example.com",
		"{{env.SENDGRID_API_KEY}}",
		&EmailSendOptions{Provider: "sendgrid", HTML: &htmlOn},
	)
	bytes, err := json.Marshal(stage)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(bytes, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["type"] != "EmailSend" {
		t.Fatalf("type = %v", got["type"])
	}
}

func TestStageTryCatch(t *testing.T) {
	stage := StageTryCatch(
		[]FunctionStageConfig{StageFindAll("users")},
		[]FunctionStageConfig{StageInsert("errors", map[string]interface{}{"msg": "failed"}, false, nil)},
		"api_error",
	)
	if stage.Stage != "TryCatch" {
		t.Fatalf("stage = %v, want TryCatch", stage.Stage)
	}
	tryFns := stage.Data["try_functions"].([]FunctionStageConfig)
	catchFns := stage.Data["catch_functions"].([]FunctionStageConfig)
	if len(tryFns) != 1 {
		t.Fatalf("try_functions len = %d, want 1", len(tryFns))
	}
	if len(catchFns) != 1 {
		t.Fatalf("catch_functions len = %d, want 1", len(catchFns))
	}
	if stage.Data["output_error_field"] != "api_error" {
		t.Fatalf("output_error_field = %v, want api_error", stage.Data["output_error_field"])
	}
}

func TestStageTryCatchOmitsOutputErrorField(t *testing.T) {
	stage := StageTryCatch(
		[]FunctionStageConfig{StageFindAll("users")},
		[]FunctionStageConfig{StageFindAll("fallback")},
		"",
	)
	if _, ok := stage.Data["output_error_field"]; ok {
		t.Fatal("output_error_field should be omitted when empty")
	}
}

func TestStageParallel(t *testing.T) {
	stage := StageParallel(
		[]FunctionStageConfig{StageFindAll("a"), StageFindAll("b")},
		true,
	)
	if stage.Stage != "Parallel" {
		t.Fatalf("stage = %v, want Parallel", stage.Stage)
	}
	fns := stage.Data["functions"].([]FunctionStageConfig)
	if len(fns) != 2 {
		t.Fatalf("functions len = %d, want 2", len(fns))
	}
	if stage.Data["wait_for_all"] != true {
		t.Fatalf("wait_for_all = %v, want true", stage.Data["wait_for_all"])
	}
}

func TestStageParallelRaceMode(t *testing.T) {
	stage := StageParallel(
		[]FunctionStageConfig{StageFindAll("a")},
		false,
	)
	if stage.Data["wait_for_all"] != false {
		t.Fatalf("wait_for_all = %v, want false", stage.Data["wait_for_all"])
	}
}

func TestStageSleep(t *testing.T) {
	stage := StageSleep(1000)
	if stage.Stage != "Sleep" {
		t.Fatalf("stage = %v, want Sleep", stage.Stage)
	}
	if stage.Data["duration_ms"] != 1000 {
		t.Fatalf("duration_ms = %v, want 1000", stage.Data["duration_ms"])
	}
}

func TestStageSleepPlaceholder(t *testing.T) {
	stage := StageSleep("{{delay}}")
	if stage.Data["duration_ms"] != "{{delay}}" {
		t.Fatalf("duration_ms = %v, want {{delay}}", stage.Data["duration_ms"])
	}
}

func TestStageReturn(t *testing.T) {
	stage := StageReturn(
		map[string]interface{}{"message": "ok", "user_id": "{{id}}"},
		201,
	)
	if stage.Stage != "Return" {
		t.Fatalf("stage = %v, want Return", stage.Stage)
	}
	fields := stage.Data["fields"].(map[string]interface{})
	if fields["message"] != "ok" {
		t.Fatalf("fields.message = %v, want ok", fields["message"])
	}
	if stage.Data["status_code"] != 201 {
		t.Fatalf("status_code = %v, want 201", stage.Data["status_code"])
	}
}

func TestStageReturnOmitsStatusCode(t *testing.T) {
	stage := StageReturn(map[string]interface{}{"ok": true}, 0)
	if _, ok := stage.Data["status_code"]; ok {
		t.Fatal("status_code should be omitted when 0")
	}
}

func TestStageValidate(t *testing.T) {
	schema := map[string]interface{}{"type": "object", "required": []string{"name"}}
	stage := StageValidate(schema, "{{input}}", []FunctionStageConfig{StageFindAll("errors")})
	if stage.Stage != "Validate" {
		t.Fatalf("stage = %v, want Validate", stage.Stage)
	}
	if stage.Data["data_field"] != "{{input}}" {
		t.Fatalf("data_field = %v, want {{input}}", stage.Data["data_field"])
	}
	onErr := stage.Data["on_error"].([]FunctionStageConfig)
	if len(onErr) != 1 {
		t.Fatalf("on_error len = %d, want 1", len(onErr))
	}
}

func TestStageValidateOmitsOnError(t *testing.T) {
	stage := StageValidate(map[string]interface{}{"type": "object"}, "data", nil)
	if _, ok := stage.Data["on_error"]; ok {
		t.Fatal("on_error should be omitted when nil")
	}
}

func TestNewStagesJSONRoundTrip(t *testing.T) {
	cases := []FunctionStageConfig{
		StageTryCatch(
			[]FunctionStageConfig{StageFindAll("a")},
			[]FunctionStageConfig{StageFindAll("b")},
			"err",
		),
		StageParallel([]FunctionStageConfig{StageFindAll("a")}, true),
		StageSleep(500),
		StageReturn(map[string]interface{}{"ok": true}, 200),
		StageValidate(map[string]interface{}{"type": "object"}, "data", nil),
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

// ===== Crypto + concurrency stages =====

func TestStageHmacSign_withAlgorithmAndEncoding(t *testing.T) {
	s := StageHmacSign("{{p}}", "{{env.K}}", "mac", "sha256", "hex")
	if s.Stage != "HmacSign" {
		t.Fatalf("Stage = %v, want HmacSign", s.Stage)
	}
	if s.Data["algorithm"] != "sha256" || s.Data["encoding"] != "hex" {
		t.Fatalf("Data missing algorithm/encoding: %v", s.Data)
	}
}

func TestStageHmacVerify_omitsOptionalFields(t *testing.T) {
	s := StageHmacVerify("{{p}}", "{{m}}", "{{env.K}}", "ok", "", "")
	if _, ok := s.Data["algorithm"]; ok {
		t.Fatalf("algorithm should be omitted when empty: %v", s.Data)
	}
	if _, ok := s.Data["encoding"]; ok {
		t.Fatalf("encoding should be omitted when empty: %v", s.Data)
	}
}

func TestStageAesAndUuidStages(t *testing.T) {
	enc := StageAesEncrypt("p", "k", "e", "hex")
	if enc.Stage != "AesEncrypt" || enc.Data["key_encoding"] != "hex" {
		t.Fatalf("AesEncrypt malformed: %v", enc)
	}
	dec := StageAesDecrypt("e", "k", "p", "")
	if dec.Stage != "AesDecrypt" {
		t.Fatalf("AesDecrypt stage wrong: %v", dec)
	}
	if _, ok := dec.Data["key_encoding"]; ok {
		t.Fatalf("key_encoding should be omitted when empty")
	}
	uid := StageUuidGenerate("id")
	if uid.Stage != "UuidGenerate" || uid.Data["output_field"] != "id" {
		t.Fatalf("UuidGenerate malformed: %v", uid)
	}
}

func TestStageTotpStages(t *testing.T) {
	digits := 6
	period := uint64(30)
	gen := StageTotpGenerate("{{env.T}}", "code", &TotpOptions{
		Digits:    &digits,
		Period:    &period,
		Algorithm: "sha1",
	})
	if gen.Stage != "TotpGenerate" {
		t.Fatalf("TotpGenerate wrong stage: %v", gen)
	}
	if gen.Data["digits"].(int) != 6 || gen.Data["period"].(uint64) != 30 {
		t.Fatalf("TotpGenerate options not wired: %v", gen.Data)
	}
	skew := uint8(1)
	ver := StageTotpVerify("{{user_code}}", "{{env.T}}", "ok", &TotpOptions{Skew: &skew})
	if ver.Stage != "TotpVerify" || ver.Data["skew"].(uint8) != 1 {
		t.Fatalf("TotpVerify options not wired: %v", ver.Data)
	}
	bare := StageTotpGenerate("s", "c", nil)
	if _, ok := bare.Data["digits"]; ok {
		t.Fatalf("nil opts should leave digits absent: %v", bare.Data)
	}
}

func TestStageBase64HexSlugifyStages(t *testing.T) {
	urlSafe := true
	b := StageBase64Encode("{{x}}", "b", &urlSafe)
	if b.Data["url_safe"].(bool) != true {
		t.Fatalf("Base64Encode url_safe not wired: %v", b.Data)
	}
	bd := StageBase64Decode("{{b}}", "x", nil)
	if _, ok := bd.Data["url_safe"]; ok {
		t.Fatalf("Base64Decode url_safe should be absent when nil: %v", bd.Data)
	}
	h := StageHexEncode("{{x}}", "h")
	if h.Stage != "HexEncode" || h.Data["input"] != "{{x}}" {
		t.Fatalf("HexEncode malformed: %v", h)
	}
	hd := StageHexDecode("{{h}}", "x")
	if hd.Stage != "HexDecode" {
		t.Fatalf("HexDecode wrong stage: %v", hd)
	}
	s := StageSlugify("{{title}}", "slug")
	if s.Stage != "Slugify" || s.Data["output_field"] != "slug" {
		t.Fatalf("Slugify malformed: %v", s)
	}
}

func TestConcurrencyStages(t *testing.T) {
	idem := StageIdempotencyClaim("{{ikey}}", 60, "claim")
	if idem.Data["ttl_secs"].(uint64) != 60 {
		t.Fatalf("IdempotencyClaim ttl_secs wrong: %v", idem.Data)
	}
	rl := StageRateLimit("{{u}}", 100, 60, "rl", "skip")
	if rl.Data["on_exceed"] != "skip" {
		t.Fatalf("RateLimit on_exceed not wired: %v", rl.Data)
	}
	rlBare := StageRateLimit("{{u}}", 100, 60, "rl", "")
	if _, ok := rlBare.Data["on_exceed"]; ok {
		t.Fatalf("on_exceed should be absent when empty: %v", rlBare.Data)
	}
	la := StageLockAcquire("{{r}}", 30, "lock")
	if la.Stage != "LockAcquire" || la.Data["ttl_secs"].(uint64) != 30 {
		t.Fatalf("LockAcquire malformed: %v", la)
	}
	lr := StageLockRelease("{{r}}", "tok", "rel")
	if lr.Data["token"] != "tok" {
		t.Fatalf("LockRelease token not wired: %v", lr.Data)
	}
}

func TestCryptoConcurrencyStages_jsonRoundTrip(t *testing.T) {
	cases := []FunctionStageConfig{
		StageHmacSign("a", "k", "m", "sha256", "hex"),
		StageAesEncrypt("p", "k", "e", "hex"),
		StageUuidGenerate("id"),
		StageTotpGenerate("s", "c", nil),
		StageBase64Encode("x", "b", nil),
		StageHexEncode("x", "h"),
		StageSlugify("t", "s"),
		StageIdempotencyClaim("k", 60, "c"),
		StageRateLimit("k", 5, 60, "r", ""),
		StageLockAcquire("r", 60, "l"),
		StageLockRelease("r", "tok", "rel"),
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

// ===== UserFunction HTTPMethod / HTTPPath (path-routed dispatcher) =====

// TestUserFunction_jsonIncludesHTTPFieldsWhenSet guards the wire shape that
// ekoDB's path-routed dispatcher (`/api/route/{path}`) reads. The fields
// MUST serialize as `http_method` / `http_path`. Renaming or breaking the
// JSON tag on either pointer would silently disable path routing for any
// function defined in this client.
func TestUserFunction_jsonIncludesHTTPFieldsWhenSet(t *testing.T) {
	method := "POST"
	path := "/users/:id"
	fn := UserFunction{
		Label:      "users_create",
		Name:       "users_create",
		Functions:  []FunctionStageConfig{StageReturn(map[string]interface{}{"ok": true}, 200)},
		HTTPMethod: &method,
		HTTPPath:   &path,
	}
	bytes, err := json.Marshal(fn)
	if err != nil {
		t.Fatalf("marshal UserFunction failed: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(bytes, &got); err != nil {
		t.Fatalf("unmarshal UserFunction failed: %v", err)
	}
	if got["http_method"] != method {
		t.Fatalf("http_method = %v, want %q", got["http_method"], method)
	}
	if got["http_path"] != path {
		t.Fatalf("http_path = %v, want %q", got["http_path"], path)
	}
	// Round-trip back into the typed struct: pointers must come back
	// non-nil with the original strings, not lost or re-bound.
	var roundTrip UserFunction
	if err := json.Unmarshal(bytes, &roundTrip); err != nil {
		t.Fatalf("round-trip unmarshal failed: %v", err)
	}
	if roundTrip.HTTPMethod == nil || *roundTrip.HTTPMethod != method {
		t.Fatalf("roundTrip.HTTPMethod = %v, want %q", roundTrip.HTTPMethod, method)
	}
	if roundTrip.HTTPPath == nil || *roundTrip.HTTPPath != path {
		t.Fatalf("roundTrip.HTTPPath = %v, want %q", roundTrip.HTTPPath, path)
	}
}

// TestUserFunction_jsonOmitsHTTPFieldsWhenNil guards the `omitempty` JSON
// tags. A function without routing fields must not emit `http_method` /
// `http_path` keys at all (not even as null) — the server's deserializer
// for older function rows that pre-date the routing schema relies on
// these keys being absent, not null, to skip the routing pathway.
func TestUserFunction_jsonOmitsHTTPFieldsWhenNil(t *testing.T) {
	fn := UserFunction{
		Label:     "no_route",
		Name:      "no_route",
		Functions: []FunctionStageConfig{StageReturn(map[string]interface{}{"ok": true}, 200)},
	}
	bytes, err := json.Marshal(fn)
	if err != nil {
		t.Fatalf("marshal UserFunction failed: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(bytes, &got); err != nil {
		t.Fatalf("unmarshal UserFunction failed: %v", err)
	}
	if _, ok := got["http_method"]; ok {
		t.Fatalf("http_method must be absent when nil (omitempty), got %v", got["http_method"])
	}
	if _, ok := got["http_path"]; ok {
		t.Fatalf("http_path must be absent when nil (omitempty), got %v", got["http_path"])
	}
}
