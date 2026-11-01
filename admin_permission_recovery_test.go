package ekodb

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func callNamedPermission(client *AdminClient, facet, method, name string) error {
	read := true
	var err error
	if facet == "collections" {
		switch method {
		case http.MethodGet:
			_, err = client.GetAPIKeyCollectionPermissions("me", name)
		case http.MethodPut:
			_, err = client.PutAPIKeyCollectionPermissions(UpdateAPIKeyCollectionPermissionsRequest{KeyID: "me", Collection: name, Read: true})
		case http.MethodPatch:
			_, err = client.PatchAPIKeyCollectionPermissions(PatchAPIKeyCollectionPermissionsRequest{KeyID: "me", Collection: name, Read: &read})
		case http.MethodDelete:
			_, err = client.DeleteAPIKeyCollectionPermissions("me", name)
		}
	} else {
		switch method {
		case http.MethodGet:
			_, err = client.GetAPIKeyKVPermissions("me", name)
		case http.MethodPut:
			_, err = client.PutAPIKeyKVPermissions(UpdateAPIKeyKVPermissionsRequest{KeyID: "me", Prefix: name, Read: true})
		case http.MethodPatch:
			_, err = client.PatchAPIKeyKVPermissions(PatchAPIKeyKVPermissionsRequest{KeyID: "me", Prefix: name, Read: &read})
		case http.MethodDelete:
			_, err = client.DeleteAPIKeyKVPermissions("me", name)
		}
	}
	return err
}

func TestAdminPermissionNamesMatchSharedContract(t *testing.T) {
	var fixture struct {
		Accepted, Rejected []struct{ Name, Value string }
	}
	data, err := os.ReadFile("test-fixtures/admin-permission-names.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Accepted) == 0 || len(fixture.Rejected) == 0 {
		t.Fatal("name contract must include accepted and rejected cases")
	}
	for _, name := range fixture.Accepted {
		for _, facet := range []string{"collections", "kv"} {
			for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete} {
				t.Run("accepted/"+name.Name+"/"+facet+"/"+method, func(t *testing.T) {
					var calls atomic.Int32
					client := keyPermissionTestClient(t, func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path == "/api/auth/keys/me/permissions" {
							_, _ = io.WriteString(w, modernAdminPermissions)
							return
						}
						calls.Add(1)
						if r.Method != method || r.URL.Path != "/api/auth/keys/me/permissions/"+facet+"/"+name.Value || r.URL.RawQuery != "" {
							t.Error("name did not survive the HTTP boundary")
						}
						grant := `{"read":true,"write":false}`
						if method == http.MethodGet {
							_, _ = io.WriteString(w, grant)
						} else if method == http.MethodDelete {
							_, _ = fmt.Fprintf(w, `{"before":%s,"after":null}`, grant)
						} else {
							_, _ = fmt.Fprintf(w, `{"before":null,"after":%s}`, grant)
						}
					})
					if err := callNamedPermission(client, facet, method, name.Value); err != nil {
						t.Fatal(err)
					}
					if calls.Load() != 1 {
						t.Fatal("accepted facet was not requested exactly once")
					}
				})
			}
		}
	}
	var requests atomic.Int32
	client := keyPermissionTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/api/auth/keys/me/permissions" {
			_, _ = io.WriteString(w, modernAdminPermissions)
		} else if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, `{"read":true,"write":false}`)
		} else {
			_, _ = io.WriteString(w, `{"before":null,"after":null}`)
		}
	})
	for _, name := range fixture.Rejected {
		for _, facet := range []string{"collections", "kv"} {
			for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete} {
				t.Run("rejected/"+name.Name+"/"+facet+"/"+method, func(t *testing.T) {
					if err := callNamedPermission(client, facet, method, name.Value); err == nil {
						t.Error("rejected name was accepted")
					}
				})
			}
		}
	}
	if requests.Load() != 0 {
		t.Error("rejected names reached HTTP")
	}
}

func TestAdminLegacyHealthFailureRecovery(t *testing.T) {
	for _, failure := range []struct {
		name, body, fragment string
		status               int
	}{
		{"unavailable", `{"error":"synthetic-secret"}`, "503", 503},
		{"malformed_json", "synthetic-secret invalid JSON", "invalid admin response", 200},
		{"missing_version", `{}`, "earlier servers remove", 200},
		{"non_string_version", `{"version":9}`, "earlier servers remove", 200},
		{"invalid_version", `{"version":"invalid"}`, "earlier servers remove", 200},
		{"prerelease", `{"version":"0.79.2-rc.1"}`, "earlier servers remove", 200},
	} {
		t.Run(failure.name, func(t *testing.T) {
			var health, updates atomic.Int32
			client := keyPermissionTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/auth/keys/me/permissions":
					w.WriteHeader(http.StatusNotFound)
				case "/api/health":
					if health.Add(1) == 1 {
						w.WriteHeader(failure.status)
						_, _ = io.WriteString(w, failure.body)
					} else {
						_, _ = io.WriteString(w, `{"version":"0.79.2"}`)
					}
				case "/api/auth/permissions":
					updates.Add(1)
					_, _ = io.WriteString(w, adminKeyJSON)
				default:
					t.Error("unexpected request")
				}
			})
			request := LegacyUpdateAPIKeyCollectionPermissionsRequest{APIKey: "synthetic-target", Collection: "users", Read: true}
			_, first := client.LegacyUpdateAPIKeyCollectionPermissions(request)
			if first == nil || strings.Contains(first.Error(), "synthetic-secret") {
				t.Fatal("first failure was absent or unsafe")
			}
			for range 2 {
				if _, err := client.LegacyUpdateAPIKeyCollectionPermissions(request); err != nil {
					t.Error("health recovery failed:", err)
				}
			}
			if !strings.Contains(first.Error(), failure.fragment) {
				t.Error("health failure lost its safe classification")
			}
			if health.Load() != 2 || updates.Load() != 2 {
				t.Error("failed read was cached or successful read was not cached")
			}
		})
	}
}

func TestAdminProbeRefreshFailureDoesNotSelectLegacy(t *testing.T) {
	for _, status := range []int{404, 405} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var auth, probes, legacy atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/auth/token":
					if auth.Add(1) == 2 {
						w.WriteHeader(status)
						_, _ = io.WriteString(w, "synthetic-refresh-secret")
					} else {
						_, _ = fmt.Fprintf(w, `{"token":"jwt-%d"}`, auth.Load())
					}
				case "/api/auth/keys/me/permissions":
					if probes.Add(1) < 3 {
						w.WriteHeader(http.StatusUnauthorized)
					} else {
						_, _ = io.WriteString(w, modernAdminPermissions)
					}
				case "/api/auth/permissions":
					legacy.Add(1)
					_, _ = io.WriteString(w, `{"is_admin":false,"collection_permissions":{},"kv_permissions":[]}`)
				default:
					t.Error("unexpected request")
				}
			}))
			t.Cleanup(server.Close)
			client, err := NewAdminClient(server.URL, "synthetic-owner")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.GetPermissions(); err == nil || strings.Contains(err.Error(), "synthetic-refresh-secret") {
				t.Error("refresh failure was swallowed or leaked its body")
			}
			permissions, err := client.GetPermissions()
			if err != nil || permissions == nil || len(permissions.KVPermissions) != 1 {
				t.Error("probe did not recover on the same client")
			}
			if legacy.Load() != 0 || probes.Load() != 3 || auth.Load() != 3 {
				t.Error("refresh status selected/cached legacy or prevented recovery")
			}
		})
	}
}
