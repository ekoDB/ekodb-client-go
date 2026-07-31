package ekodb

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// healthTestServer stands up a mock ekoDB whose /api/health returns the given body.
func healthTestServer(t *testing.T, body map[string]interface{}) (*Client, func()) {
	t.Helper()
	handlers := map[string]http.HandlerFunc{
		"GET /api/health": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(body)
		},
	}
	server := createTestServer(t, handlers)
	client := createTestClient(t, server)
	return client, server.Close
}

// A healthy server: status "ok" -> reachable, ok, no error.
func TestHealthStatus_Ok(t *testing.T) {
	client, closeFn := healthTestServer(t, map[string]interface{}{
		"status": "ok", "integrity_ok": true,
	})
	defer closeFn()

	hs, err := client.HealthStatus()
	if err != nil {
		t.Fatalf("HealthStatus on a healthy server should not error, got: %v", err)
	}
	if !hs.Reachable {
		t.Errorf("expected Reachable=true")
	}
	if hs.Status != "ok" {
		t.Errorf("expected Status=ok, got %q", hs.Status)
	}
	if !hs.IntegrityOK {
		t.Errorf("expected IntegrityOK=true")
	}
}

// The crux: a degraded (HTTP 200) server is REACHABLE, not an error.
func TestHealthStatus_DegradedIsNotAnError(t *testing.T) {
	client, closeFn := healthTestServer(t, map[string]interface{}{
		"status": "degraded", "integrity_ok": false,
	})
	defer closeFn()

	hs, err := client.HealthStatus()
	if err != nil {
		t.Fatalf("degraded is reachable-but-unhealthy, must NOT error, got: %v", err)
	}
	if !hs.Reachable {
		t.Errorf("expected Reachable=true for a 200 degraded response")
	}
	if hs.Status != "degraded" {
		t.Errorf("expected Status=degraded, got %q", hs.Status)
	}
	if hs.IntegrityOK {
		t.Errorf("expected IntegrityOK=false")
	}
}

// An unreachable server -> error, Reachable=false.
func TestHealthStatus_UnreachableErrors(t *testing.T) {
	client, closeFn := healthTestServer(t, map[string]interface{}{"status": "ok"})
	closeFn() // kill the server before probing

	hs, err := client.HealthStatus()
	if err == nil {
		t.Fatalf("HealthStatus against a down server must error")
	}
	if hs == nil || hs.Reachable {
		t.Errorf("expected a non-nil, unreachable snapshot, got %+v", hs)
	}
}

// A reachable server returning a non-2xx status is unreachable for the contract:
// error, nil HealthStatus (guards the "non-2xx -> error" branch that downstream
// liveness gates rely on).
func TestHealthStatus_Non2xxErrors(t *testing.T) {
	handlers := map[string]http.HandlerFunc{
		"GET /api/health": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"degraded"}`))
		},
	}
	server := createTestServer(t, handlers)
	defer server.Close()
	client := createTestClient(t, server)

	hs, err := client.HealthStatus()
	if err == nil {
		t.Fatalf("a 503 from /api/health must error (non-2xx is not reachable)")
	}
	if hs == nil || hs.Reachable {
		t.Errorf("expected a non-nil, unreachable snapshot on a non-2xx response, got %+v", hs)
	}
}

// Health() is now reachable-only: degraded must NOT error (regression guard: treating
// degraded as a hard failure previously blocked consumer startup).
func TestHealth_ReachableOnlyToleratesDegraded(t *testing.T) {
	client, closeFn := healthTestServer(t, map[string]interface{}{
		"status": "degraded", "integrity_ok": false,
	})
	defer closeFn()

	if err := client.Health(); err != nil {
		t.Fatalf("Health() must tolerate degraded (reachable-only), got: %v", err)
	}
}

// ParseHealthStatus is the shared contract parser reused by non-client probes
// (e.g. circuit-breaker health checks in downstream services) so every consumer
// interprets /api/health identically.
func TestParseHealthStatus_DegradedIsReachable(t *testing.T) {
	hs, err := ParseHealthStatus([]byte(`{"status":"degraded","integrity_ok":false}`))
	if err != nil {
		t.Fatalf("degraded body must parse without error, got: %v", err)
	}
	if !hs.Reachable || hs.Status != "degraded" || hs.IntegrityOK {
		t.Errorf("got %+v", hs)
	}
}

func TestParseHealthStatus_MissingStatusFailsSafeToDegraded(t *testing.T) {
	hs, err := ParseHealthStatus([]byte(`{"integrity_ok":true}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hs.Status != "degraded" {
		t.Errorf("a missing status must fail safe to degraded, got %q", hs.Status)
	}
}

func TestParseHealthStatus_GarbageIsUnreachable(t *testing.T) {
	hs, err := ParseHealthStatus([]byte(`not json`))
	if err == nil {
		t.Fatal("unparseable body must error")
	}
	if hs == nil || hs.Reachable {
		t.Errorf("expected a non-nil, unreachable snapshot on an unparseable body, got %+v", hs)
	}
}

// The admin /api/health response nests integrity as integrity.healthy (there is
// NO top-level integrity_ok); the clients authenticate as admin, so IntegrityOK
// must read that nested shape. A healthy admin body is the discriminating case:
// the old code read the absent top-level key and defaulted IntegrityOK to false.
func TestParseHealthStatus_AdminShapeHealthy(t *testing.T) {
	hs, err := ParseHealthStatus([]byte(`{"status":"ok","integrity":{"healthy":true,"manifest_load_failed":[]}}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hs.Status != "ok" {
		t.Errorf("expected Status=ok, got %q", hs.Status)
	}
	if !hs.IntegrityOK {
		t.Errorf("admin shape: IntegrityOK must come from integrity.healthy (got false)")
	}
}

func TestParseHealthStatus_AdminShapeDegraded(t *testing.T) {
	hs, err := ParseHealthStatus([]byte(`{"status":"degraded","integrity":{"healthy":false,"manifest_load_failed":["deployment_operations"]}}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hs.Status != "degraded" {
		t.Errorf("expected Status=degraded, got %q", hs.Status)
	}
	if hs.IntegrityOK {
		t.Errorf("admin shape: IntegrityOK must be false when integrity.healthy is false")
	}
}

// Constants exist for the Status values so consumers compare against a symbol.
func TestHealthStatusConstants(t *testing.T) {
	if HealthOK != "ok" || HealthDegraded != "degraded" {
		t.Errorf("unexpected status constants: %q / %q", HealthOK, HealthDegraded)
	}
}

// The snapshot serializes to a safe summary (reachable/status/integrity_ok) that
// a consumer can surface directly. Detail carries the full, possibly sensitive
// admin body and MUST NOT be serialized.
func TestHealthStatus_JSONShape(t *testing.T) {
	hs := &HealthStatus{
		Reachable:   true,
		Status:      HealthDegraded,
		IntegrityOK: false,
		Detail:      map[string]interface{}{"manifest_load_failed": []string{"secret_collection"}},
	}
	b, err := json.Marshal(hs)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	for _, k := range []string{"reachable", "status", "integrity_ok"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing key %q in %s", k, b)
		}
	}
	if m["status"] != "degraded" {
		t.Errorf("status = %v, want degraded", m["status"])
	}
	if m["reachable"] != true {
		t.Errorf("reachable = %v, want true", m["reachable"])
	}
	// Detail must not be serialized (even when populated), so the internal admin
	// body is never leaked when the snapshot is surfaced.
	if _, ok := m["detail"]; ok {
		t.Errorf("detail must not be serialized, got %s", b)
	}
	if strings.Contains(string(b), "secret_collection") {
		t.Errorf("internal detail leaked into the serialized snapshot: %s", b)
	}
}

// An unreachable/unparseable snapshot reports Status HealthUnknown (not an empty
// string) so the serialized DTO is coherent.
func TestHealthStatus_UnreachableSnapshotIsUnknown(t *testing.T) {
	client, closeFn := healthTestServer(t, map[string]interface{}{"status": "ok"})
	closeFn() // unreachable

	hs, err := client.HealthStatus()
	if err == nil {
		t.Fatalf("expected an error for an unreachable server")
	}
	if hs == nil || hs.Reachable || hs.Status != HealthUnknown {
		t.Errorf("unreachable snapshot should be {Reachable:false, Status:HealthUnknown}, got %+v", hs)
	}

	hs2, err2 := ParseHealthStatus([]byte("not json"))
	if err2 == nil {
		t.Fatal("expected an error for an unparseable body")
	}
	if hs2 == nil || hs2.Status != HealthUnknown {
		t.Errorf("unparseable snapshot should have Status HealthUnknown, got %+v", hs2)
	}
}
