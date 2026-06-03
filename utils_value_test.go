package ekodb

import (
	"reflect"
	"testing"
)

// TestGetValueUnwrapsTypedWrapper confirms a genuine {type,value} wrapper is unwrapped.
func TestGetValueUnwrapsTypedWrapper(t *testing.T) {
	in := map[string]interface{}{"type": "String", "value": "hi"}
	if got := GetValue(in); got != "hi" {
		t.Fatalf("expected unwrapped \"hi\", got %v", got)
	}
}

// TestGetValuePassesThroughUserObjectWithValueKey is the regression for #35: a user object
// that merely has a "value" key (but no "type" discriminator) must NOT be unwrapped.
func TestGetValuePassesThroughUserObjectWithValueKey(t *testing.T) {
	in := map[string]interface{}{"value": float64(1), "currency": "USD"}
	got := GetValue(in)
	m, ok := got.(map[string]interface{})
	if !ok || !reflect.DeepEqual(m, in) {
		t.Fatalf("expected the user object to pass through unchanged, got %v", got)
	}
}

// TestGetValuePassThroughNonMapAndNil covers scalars and nil.
func TestGetValuePassThroughNonMapAndNil(t *testing.T) {
	if got := GetValue("raw"); got != "raw" {
		t.Fatalf("expected raw scalar passthrough, got %v", got)
	}
	if got := GetValue(nil); got != nil {
		t.Fatalf("expected nil passthrough, got %v", got)
	}
}

// TestGetStringValueOnUserObjectIsEmpty confirms the typed extractor does not mis-read
// a user object with a "value" key as that inner value.
func TestGetStringValueOnUserObjectIsEmpty(t *testing.T) {
	in := map[string]interface{}{"value": "inner", "currency": "USD"}
	if got := GetStringValue(in); got != "" {
		t.Fatalf("expected empty string (object is not a typed wrapper), got %q", got)
	}
}
