package ekodb

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

const modernAdminPermissions = `{"admin":false,"collections":{"users":{"read":true,"write":false,"select_fields":["name"],"exclude_fields":null,"writable_fields":[]}},"kv":[{"prefix":"session:","read":true,"write":false}],"future":42}`
const modernAdminID = "0123456789abcdef0123456789abcdef"

func keyPermissionTestClient(t *testing.T, handler http.HandlerFunc) *AdminClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/token" {
			_, _ = io.WriteString(w, `{"token":"modern-jwt"}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer modern-jwt" {
			t.Error("missing authentication")
		}
		if r.URL.Path == "/api/auth/keys/me/permissions" && r.Method != http.MethodGet {
			t.Error("capability probe must be GET")
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	client, err := NewAdminClient(server.URL, "owner-secret")
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestAdminModernRecordedPermissionChanges(t *testing.T) {
	data, err := os.ReadFile("test-fixtures/admin-contract-0.80.0.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Minimum     string          `json:"minimum_server_version"`
		KeyID       string          `json:"key_id"`
		Key         json.RawMessage `json:"key"`
		Permissions json.RawMessage `json:"permissions"`
		Changes     []struct {
			Name, Method, Facet, Path string
			Body, Response            json.RawMessage
		} `json:"permission_changes"`
	}
	if json.Unmarshal(data, &fixture) != nil || fixture.Minimum != MinServerVersionForPermissionEdits || len(fixture.Changes) != 12 {
		t.Fatal("modern contract drift")
	}
	for _, test := range fixture.Changes {
		t.Run(test.Facet+"/"+test.Name+"/"+test.Method, func(t *testing.T) {
			var probes, calls atomic.Int32
			client := keyPermissionTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/auth/keys/me/permissions" {
					probes.Add(1)
					_, _ = w.Write(fixture.Permissions)
					return
				}
				calls.Add(1)
				if r.Method != test.Method || r.URL.EscapedPath() != test.Path || r.URL.RawQuery != "" {
					t.Errorf("unexpected method/path: %s %s", r.Method, r.URL.EscapedPath())
				}
				body, _ := io.ReadAll(r.Body)
				for _, secret := range []string{"owner-secret", "synthetic-target-key-0123456789-abcdef", "modern-jwt"} {
					if strings.Contains(r.RequestURI, secret) || strings.Contains(string(body), secret) {
						t.Error("secret entered permission request")
					}
				}
				if strings.Contains(string(body), fixture.KeyID) {
					t.Error("routing ID was included in permission body")
				}
				if test.Method == http.MethodDelete {
					if len(body) != 0 {
						t.Error("DELETE unexpectedly has a body")
					}
				} else {
					var got, want any
					if json.Unmarshal(body, &got) != nil || json.Unmarshal(test.Body, &want) != nil || !reflect.DeepEqual(got, want) {
						t.Errorf("permission body differs: %s", body)
					}
				}
				_, _ = w.Write(test.Response)
			})
			var result any
			var err error
			if test.Facet == "collections" {
				switch test.Method {
				case http.MethodPut:
					var request UpdateAPIKeyCollectionPermissionsRequest
					if json.Unmarshal(test.Body, &request) != nil {
						t.Fatal("invalid request fixture")
					}
					request.KeyID, request.Collection = fixture.KeyID, test.Name
					result, err = client.PutAPIKeyCollectionPermissions(request)
				case http.MethodPatch:
					var request PatchAPIKeyCollectionPermissionsRequest
					if json.Unmarshal(test.Body, &request) != nil {
						t.Fatal("invalid request fixture")
					}
					request.KeyID, request.Collection = fixture.KeyID, test.Name
					result, err = client.PatchAPIKeyCollectionPermissions(request)
				case http.MethodDelete:
					result, err = client.DeleteAPIKeyCollectionPermissions(fixture.KeyID, test.Name)
				}
			} else {
				switch test.Method {
				case http.MethodPut:
					var request UpdateAPIKeyKVPermissionsRequest
					if json.Unmarshal(test.Body, &request) != nil {
						t.Fatal("invalid request fixture")
					}
					request.KeyID, request.Prefix = fixture.KeyID, test.Name
					result, err = client.PutAPIKeyKVPermissions(request)
				case http.MethodPatch:
					var request PatchAPIKeyKVPermissionsRequest
					if json.Unmarshal(test.Body, &request) != nil {
						t.Fatal("invalid request fixture")
					}
					request.KeyID, request.Prefix = fixture.KeyID, test.Name
					result, err = client.PatchAPIKeyKVPermissions(request)
				case http.MethodDelete:
					result, err = client.DeleteAPIKeyKVPermissions(fixture.KeyID, test.Name)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var got, want any
			if json.Unmarshal(encoded, &got) != nil || json.Unmarshal(test.Response, &want) != nil {
				t.Fatal("invalid change response")
			}
			// Explicit null restriction fields normalize identically to absent fields;
			// the wire request, above, must preserve all three edit states.
			var expected PermissionChange[CollectionPermissions]
			if test.Facet == "collections" {
				if json.Unmarshal(test.Response, &expected) != nil {
					t.Fatal("invalid fixture change")
				}
				bytes, _ := json.Marshal(expected)
				_ = json.Unmarshal(bytes, &want)
			} else {
				var expected PermissionChange[KVPermissions]
				if json.Unmarshal(test.Response, &expected) != nil {
					t.Fatal("invalid fixture change")
				}
				bytes, _ := json.Marshal(expected)
				_ = json.Unmarshal(bytes, &want)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatal("before/after response lost")
			}
			if probes.Load() != 1 || calls.Load() != 1 {
				t.Fatal("generation probe or facet operation repeated")
			}
		})
	}
}

func TestAdminModernAggregateAndIDs(t *testing.T) {
	keyJSON := strings.Replace(adminKeyJSON, `{"key":`, `{"id":"`+modernAdminID+`","key":`, 1)
	var permissionReads atomic.Int32
	client := keyPermissionTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/keys/me/permissions":
			permissionReads.Add(1)
			_, _ = io.WriteString(w, modernAdminPermissions)
		case "/api/auth/register":
			_, _ = io.WriteString(w, keyJSON)
		case "/api/auth/keys":
			_, _ = io.WriteString(w, "["+keyJSON+"]")
		default:
			t.Error("unexpected route")
		}
	})
	permissions, err := client.GetPermissions()
	if err != nil {
		t.Fatal(err)
	}
	if permissions.IsAdmin || !permissions.CollectionPermissions["users"].Read || permissions.CollectionPermissions["users"].WritableFields == nil || len(permissions.KVPermissions) != 1 || string(permissions.Metadata["future"]) != "42" {
		t.Fatal("aggregate permission fields lost")
	}
	if permissionReads.Load() != 1 {
		t.Fatal("initial probe was not reused for aggregate read")
	}
	created, err := client.CreateAPIKey(CreateAPIKeyRequest{})
	if err != nil || created.ID != modernAdminID {
		t.Fatal("registered ID lost")
	}
	listed, err := client.ListAPIKeys()
	if err != nil || len(listed) != 1 || listed[0].ID != modernAdminID {
		t.Fatal("listed ID lost")
	}
	if _, exists := listed[0].Metadata["id"]; exists {
		t.Fatal("ID left as untyped metadata")
	}
}

func TestAdminRegistrationAndListingNullKV(t *testing.T) {
	keyJSON := strings.Replace(adminKeyJSON, `{"key":`, `{"id":"`+modernAdminID+`","key":`, 1)
	keyJSON = strings.Replace(keyJSON, `"kv_permissions":[]`, `"kv_permissions":null`, 1)
	client := keyPermissionTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/register" {
			_, _ = io.WriteString(w, keyJSON)
			return
		}
		if r.URL.Path == "/api/auth/keys" {
			_, _ = io.WriteString(w, "["+keyJSON+"]")
			return
		}
		t.Error("unexpected key operation")
	})
	created, err := client.CreateAPIKey(CreateAPIKeyRequest{})
	if err != nil || created.ID != modernAdminID || created.KVPermissions != nil {
		t.Fatal("registration null KV or ID lost")
	}
	listed, err := client.ListAPIKeys()
	if err != nil || len(listed) != 1 || listed[0].ID != modernAdminID || listed[0].KVPermissions != nil {
		t.Fatal("listing null KV or ID lost")
	}
}

func TestAdminModernPermissionFacetReads(t *testing.T) {
	client := keyPermissionTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/api/auth/keys/me/permissions", "/api/auth/keys/" + modernAdminID + "/permissions":
			_, _ = io.WriteString(w, modernAdminPermissions)
		case "/api/auth/keys/me/permissions/collections":
			_, _ = io.WriteString(w, `{"users":{"read":true,"write":false}}`)
		case "/api/auth/keys/me/permissions/collections/tenant%3Aa%2F":
			_, _ = io.WriteString(w, `{"read":true,"write":false}`)
		case "/api/auth/keys/me/permissions/kv":
			if r.URL.Query().Get("prefix") != "tenant:a/+ " {
				t.Error("prefix filter encoding lost")
			}
			_, _ = io.WriteString(w, `[{"prefix":"tenant:a/+ ","read":true,"write":false}]`)
		case "/api/auth/keys/me/permissions/kv/%2A":
			_, _ = io.WriteString(w, `{"read":true,"write":false}`)
		default:
			t.Errorf("unexpected facet route: %s", r.URL.EscapedPath())
		}
	})
	if _, err := client.GetAPIKeyPermissions(modernAdminID); err != nil {
		t.Fatal(err)
	}
	if grants, err := client.ListAPIKeyCollectionPermissions("me"); err != nil || !grants["users"].Read {
		t.Fatal("collection list failed")
	}
	if grant, err := client.GetAPIKeyCollectionPermissions("me", "tenant:a/"); err != nil || !grant.Read {
		t.Fatal("collection read failed")
	}
	prefix := "tenant:a/+ "
	if grants, err := client.ListAPIKeyKVPermissions("me", &prefix); err != nil || len(grants) != 1 || grants[0].Prefix != prefix {
		t.Fatal("KV list filter failed")
	}
	if grant, err := client.GetAPIKeyKVPermissions("me", "*"); err != nil || !grant.Read {
		t.Fatal("KV read failed")
	}
}

func TestAdminPermissionProbeFallbackAndFailures(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 405, 429, 500, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var oldRequests atomic.Int32
			client := keyPermissionTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/auth/keys/me/permissions" {
					w.WriteHeader(status)
					_, _ = io.WriteString(w, "owner-secret modern-jwt")
					return
				}
				if r.URL.Path == "/api/auth/permissions" {
					oldRequests.Add(1)
					_, _ = io.WriteString(w, adminKeyJSON)
					return
				}
				t.Error("unexpected fallback request")
			})
			_, err := client.GetPermissions()
			fallback := status == 404 || status == 405
			if (err == nil) != fallback || (oldRequests.Load() == 1) != fallback {
				t.Fatal("probe downgraded on a non-capability error")
			}
			if err != nil && (strings.Contains(err.Error(), "owner-secret") || strings.Contains(err.Error(), "modern-jwt")) {
				t.Fatal("probe failure leaked credentials")
			}
		})
	}
}

func TestAdminPermissionProbeSharedAcrossConcurrentEdits(t *testing.T) {
	var probes, changes atomic.Int32
	client := keyPermissionTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/keys/me/permissions" {
			probes.Add(1)
			_, _ = io.WriteString(w, modernAdminPermissions)
			return
		}
		if r.Method != "PATCH" {
			t.Error("update must preserve omission using PATCH")
		}
		changes.Add(1)
		_, _ = io.WriteString(w, `{"before":null,"after":{"read":true,"write":false}}`)
	})
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_, err := client.UpdateAPIKeyCollectionPermissions(UpdateAPIKeyCollectionPermissionsRequest{KeyID: modernAdminID, Collection: "users", Read: true})
			if err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if probes.Load() != 1 || changes.Load() != 8 {
		t.Fatal("capability probe not shared")
	}
}

func TestAdminPermissionGenerationBoundaries(t *testing.T) {
	for _, modern := range []bool{false, true} {
		t.Run(fmt.Sprint(modern), func(t *testing.T) {
			var writes atomic.Int32
			client := keyPermissionTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/auth/keys/me/permissions" {
					if !modern {
						w.WriteHeader(404)
						return
					}
					_, _ = io.WriteString(w, modernAdminPermissions)
					return
				}
				writes.Add(1)
			})
			if modern {
				_, err := client.LegacyUpdateAPIKeyCollectionPermissions(LegacyUpdateAPIKeyCollectionPermissionsRequest{APIKey: "legacy-secret", Collection: "users"})
				if err == nil {
					t.Fatal("legacy secret-valued edit accepted on modern server")
				}
			} else {
				_, err := client.PutAPIKeyCollectionPermissions(UpdateAPIKeyCollectionPermissionsRequest{KeyID: modernAdminID, Collection: "users"})
				if err == nil {
					t.Fatal("modern facet silently translated on legacy server")
				}
			}
			if writes.Load() != 0 {
				t.Fatal("unsupported generation sent a write")
			}
		})
	}
}

func TestAdminModernPermissionValidationBeforeHTTP(t *testing.T) {
	client := keyPermissionTestClient(t, func(http.ResponseWriter, *http.Request) { t.Error("invalid input reached HTTP") })
	for _, target := range []string{"", "owner-secret", "ME", strings.ToUpper(modernAdminID), modernAdminID + "/permissions"} {
		if _, err := client.GetAPIKeyPermissions(target); err == nil {
			t.Fatal("invalid target accepted")
		}
	}
	for _, facet := range []string{"", "bad\nname", "hidden\u200bname", ".", ".."} {
		if _, err := client.DeleteAPIKeyCollectionPermissions("me", facet); err == nil {
			t.Fatal("invalid collection accepted")
		}
		if _, err := client.DeleteAPIKeyKVPermissions("me", facet); err == nil {
			t.Fatal("invalid prefix accepted")
		}
	}
	if _, err := client.PutAPIKeyKVPermissions(UpdateAPIKeyKVPermissionsRequest{KeyID: "me", Prefix: "*"}); err == nil {
		t.Fatal("empty KV rights accepted")
	}
}

func TestAdminModernChangeRequiresBeforeAndAfter(t *testing.T) {
	for _, body := range []string{`{}`, `null`, `{"before":null}`, `{"after":null}`, `{"before":null,"after":{"read":true}}`} {
		client := keyPermissionTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/auth/keys/me/permissions" {
				_, _ = io.WriteString(w, modernAdminPermissions)
				return
			}
			_, _ = io.WriteString(w, body)
		})
		if _, err := client.PutAPIKeyCollectionPermissions(UpdateAPIKeyCollectionPermissionsRequest{KeyID: "me", Collection: "users", Read: true}); err == nil {
			t.Fatalf("invalid change accepted: %s", body)
		}
	}
}

func TestAdminModernPermissionRefreshAfter401(t *testing.T) {
	var tokens, probes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/token" {
			tokens.Add(1)
			_, _ = fmt.Fprintf(w, `{"token":"generation-%d"}`, tokens.Load())
			return
		}
		if r.URL.Path != "/api/auth/keys/me/permissions" {
			t.Error("unexpected route during refresh")
		}
		if r.Method != http.MethodGet {
			t.Error("permission probe must be GET")
		}
		if probes.Add(1) == 1 {
			w.WriteHeader(401)
			return
		}
		if r.Header.Get("Authorization") != "Bearer generation-2" {
			t.Error("old token used after 401")
		}
		_, _ = io.WriteString(w, modernAdminPermissions)
	}))
	defer server.Close()
	client, err := NewAdminClient(server.URL, "owner-secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.GetPermissions(); err != nil {
		t.Fatal(err)
	}
	if tokens.Load() != 2 || probes.Load() != 2 {
		t.Fatal("upgrade 401 did not cause exactly one re-exchange")
	}
}

func TestAdminPermissionChangeMetadata(t *testing.T) {
	client := keyPermissionTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/keys/me/permissions" {
			_, _ = io.WriteString(w, modernAdminPermissions)
			return
		}
		_, _ = io.WriteString(w, `{"before":null,"after":{"read":true,"write":false,"future_grant":7},"future_change":{"ok":true}}`)
	})
	change, err := client.PutAPIKeyCollectionPermissions(UpdateAPIKeyCollectionPermissionsRequest{KeyID: "me", Collection: "users", Read: true})
	if err != nil {
		t.Fatal(err)
	}
	if change.After == nil || string(change.After.Metadata["future_grant"]) != "7" || len(change.Metadata["future_change"]) == 0 {
		t.Fatal("change metadata dropped")
	}
	encoded, err := json.Marshal(change)
	if err != nil || !strings.Contains(string(encoded), "future_change") || !strings.Contains(string(encoded), "future_grant") {
		t.Fatal("change metadata lost on round-trip")
	}
}

func TestAdminMalformedPermissionProbeCanRecover(t *testing.T) {
	for _, invalid := range []string{`null`, `{}`, `{"admin":false,"collections":{},"kv":null}`, `{"admin":false,"collections":{},"kv":[{"prefix":"x","read":true}]}`, `{"admin":false,"collections":{},"kv":[{"read":true,"write":false}]}`} {
		var probes atomic.Int32
		client := keyPermissionTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/auth/keys/me/permissions" {
				t.Error("malformed success triggered a downgrade")
			}
			if probes.Add(1) == 1 {
				_, _ = io.WriteString(w, invalid)
				return
			}
			_, _ = io.WriteString(w, modernAdminPermissions)
		})
		if _, err := client.UsesKeyPermissionRoutes(); err == nil {
			t.Fatal("malformed capability success accepted")
		}
		if modern, err := client.UsesKeyPermissionRoutes(); err != nil || !modern {
			t.Fatal("transient probe failure poisoned capability cache")
		}
		if probes.Load() != 2 {
			t.Fatal("failed capability probe was cached")
		}
	}
}

func TestAdminModernPutFixtureClearsOmittedLimits(t *testing.T) {
	data, err := os.ReadFile("test-fixtures/admin-contract-0.80.0.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Changes []struct {
			Method, Facet string
			Body          map[string]json.RawMessage
			Response      struct{ After map[string]json.RawMessage }
		} `json:"permission_changes"`
	}
	if json.Unmarshal(data, &fixture) != nil {
		t.Fatal("invalid fixture")
	}
	for _, change := range fixture.Changes {
		if change.Method != "PUT" || change.Facet != "collections" {
			continue
		}
		for _, field := range []string{"select_fields", "exclude_fields", "writable_fields"} {
			if _, present := change.Body[field]; present {
				continue
			}
			if after, present := change.Response.After[field]; present && string(after) != "null" {
				t.Errorf("PUT fixture retains omitted %s restriction", field)
			}
		}
	}
}

func TestAdminKVListRequiresPrefixes(t *testing.T) {
	for _, body := range []string{`[{"read":true,"write":false}]`, `[{"prefix":"","read":true,"write":false}]`} {
		client := keyPermissionTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/auth/keys/me/permissions" {
				_, _ = io.WriteString(w, modernAdminPermissions)
				return
			}
			_, _ = io.WriteString(w, body)
		})
		if _, err := client.ListAPIKeyKVPermissions("me", nil); err == nil {
			t.Fatal("KV list accepted a grant without its required prefix")
		}
	}
}
