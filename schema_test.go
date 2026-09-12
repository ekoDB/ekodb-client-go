package ekodb

import (
	"encoding/json"
	"testing"
)

// TestUpdateSchemaConstraintsRequestShape proves UpdateSchemaConstraints PUTs
// to /api/schemas/{collection} with a top-level "constraints" key — matching
// the server's schema-constraints-update request contract exactly.
// docs.ekodb.io previously documented a client.UpdateSchema method
// using a "fields" envelope that never existed in this client; this test pins
// the real wire shape so that mistake cannot recur.
func TestUpdateSchemaConstraintsRequestShape(t *testing.T) {
	var got capturedRequest
	server := newCapturingServer(t, &got)
	defer server.Close()

	client := createTestClient(t, server)

	fieldType := "string"
	required := true
	unique := false
	minVal := 1.0
	maxVal := 100.0
	regex := "^[a-z]+$"

	constraints := map[string]SchemaConstraintUpdate{
		"email": {
			FieldType: &fieldType,
			Required:  &required,
			Unique:    &unique,
			Regex:     &regex,
		},
		"age": {
			Min: &minVal,
			Max: &maxVal,
			// Enums intentionally left unset to prove omitempty drops it.
		},
	}

	if err := client.UpdateSchemaConstraints("users", constraints); err != nil {
		t.Fatalf("UpdateSchemaConstraints failed: %v", err)
	}

	if got.method != "PUT" {
		t.Errorf("method = %q, want PUT", got.method)
	}
	wantPath := "/api/schemas/users"
	if got.escapedPath != wantPath {
		t.Errorf("escaped path = %q, want %q", got.escapedPath, wantPath)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(got.body, &body); err != nil {
		t.Fatalf("request body is not JSON: %v (body=%s)", err, got.body)
	}

	// The top-level key MUST be "constraints" — never "fields" (the wrong
	// envelope docs.ekodb.io previously documented for a method that never
	// existed in this client).
	if _, present := body["fields"]; present {
		t.Error(`request body carries a top-level "fields" key — the server expects "constraints"`)
	}
	rawConstraints, present := body["constraints"]
	if !present {
		t.Fatalf(`request body missing top-level "constraints" key; body=%s`, got.body)
	}
	if len(body) != 1 {
		t.Errorf("request body has %d top-level keys, want exactly 1 (\"constraints\"); body=%s", len(body), got.body)
	}

	constraintsMap, ok := rawConstraints.(map[string]interface{})
	if !ok {
		t.Fatalf("constraints is not a JSON object: %T (%v)", rawConstraints, rawConstraints)
	}

	emailRaw, ok := constraintsMap["email"].(map[string]interface{})
	if !ok {
		t.Fatalf("constraints.email is not a JSON object: %v", constraintsMap["email"])
	}
	if emailRaw["field_type"] != "string" {
		t.Errorf("constraints.email.field_type = %v, want %q", emailRaw["field_type"], "string")
	}
	if emailRaw["required"] != true {
		t.Errorf("constraints.email.required = %v, want true", emailRaw["required"])
	}
	if emailRaw["unique"] != false {
		t.Errorf("constraints.email.unique = %v, want false", emailRaw["unique"])
	}
	if emailRaw["regex"] != regex {
		t.Errorf("constraints.email.regex = %v, want %q", emailRaw["regex"], regex)
	}
	// default/enums/max/min were never set on "email" — omitempty must drop
	// them from the wire body entirely (partial-update semantics).
	for _, key := range []string{"default", "enums", "max", "min"} {
		if _, present := emailRaw[key]; present {
			t.Errorf("constraints.email carries unset key %q on the wire; omitempty should have dropped it", key)
		}
	}

	ageRaw, ok := constraintsMap["age"].(map[string]interface{})
	if !ok {
		t.Fatalf("constraints.age is not a JSON object: %v", constraintsMap["age"])
	}
	if ageRaw["min"] != minVal {
		t.Errorf("constraints.age.min = %v, want %v", ageRaw["min"], minVal)
	}
	if ageRaw["max"] != maxVal {
		t.Errorf("constraints.age.max = %v, want %v", ageRaw["max"], maxVal)
	}
	for _, key := range []string{"field_type", "default", "unique", "required", "enums", "regex"} {
		if _, present := ageRaw[key]; present {
			t.Errorf("constraints.age carries unset key %q on the wire; omitempty should have dropped it", key)
		}
	}
}

// TestUpdateSchemaConstraintsCollectionEscaped proves the collection segment
// of PUT /api/schemas/{collection} is url.PathEscape'd, following the same
// convention as every other collection-scoped path in this client.
func TestUpdateSchemaConstraintsCollectionEscaped(t *testing.T) {
	var got capturedRequest
	server := newCapturingServer(t, &got)
	defer server.Close()

	client := createTestClient(t, server)

	const reserved = "a/b c"
	if err := client.UpdateSchemaConstraints(reserved, map[string]SchemaConstraintUpdate{}); err != nil {
		t.Fatalf("UpdateSchemaConstraints failed: %v", err)
	}

	wantPath := "/api/schemas/a%2Fb%20c"
	if got.escapedPath != wantPath {
		t.Errorf("escaped path = %q, want %q", got.escapedPath, wantPath)
	}
}
