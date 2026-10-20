package ekodb

import (
	"encoding/json"
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

func TestAdminRecordedPermissionEdits(t *testing.T) {
	data, err := os.ReadFile("test-fixtures/admin-contract-0.79.2.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Minimum string `json:"minimum_server_version"`
		Cases   []struct {
			Name     string          `json:"name"`
			Request  json.RawMessage `json:"request"`
			Response json.RawMessage `json:"response"`
		} `json:"permission_edits"`
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Minimum != MinServerVersionForPermissionEdits || len(fixture.Cases) != 10 {
		t.Fatal("contract release or coverage drift")
	}
	for _, test := range fixture.Cases {
		t.Run(test.Name, func(t *testing.T) {
			var request UpdateAPIKeyCollectionPermissionsRequest
			if err := json.Unmarshal(test.Request, &request); err != nil {
				t.Fatal(err)
			}
			var want any
			if err := json.Unmarshal(test.Request, &want); err != nil {
				t.Fatal(err)
			}
			client := adminTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "PUT" || r.URL.Path != "/api/auth/permissions" {
					t.Error("unexpected route")
				}
				var got any
				if json.NewDecoder(r.Body).Decode(&got) != nil || !reflect.DeepEqual(got, want) {
					t.Error("three-state body changed")
				}
				_, _ = w.Write(test.Response)
			})
			for range 2 {
				key, err := client.UpdateAPIKeyCollectionPermissions(request)
				if err != nil {
					t.Fatal(err)
				}
				encoded, err := json.Marshal(key.CollectionPermissions["users"])
				if err != nil {
					t.Fatal(err)
				}
				var record struct {
					Grants map[string]json.RawMessage `json:"collection_permissions"`
				}
				if json.Unmarshal(test.Response, &record) != nil {
					t.Fatal("bad fixture")
				}
				var got, want any
				if json.Unmarshal(encoded, &got) != nil || json.Unmarshal(record.Grants["users"], &want) != nil || !reflect.DeepEqual(got, want) {
					t.Error("response restrictions lost")
				}
			}
		})
	}
}
func TestAdminPermissionVersionFloor(t *testing.T) {
	for _, version := range []string{"", "0.79.0", "0.79.1", "0.79.2-rc.1", "0.79.2", "0.79.2+build.1", "0.79.3", "0.80.0", "1.0.0", "09.79.2", "999999999999999999999999.1.1"} {
		t.Run(version, func(t *testing.T) {
			var health, puts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/auth/token":
					_, _ = io.WriteString(w, `{"token":"jwt-secret"}`)
				case "/api/health":
					health.Add(1)
					_ = json.NewEncoder(w).Encode(map[string]string{"version": version})
				case "/api/auth/permissions":
					puts.Add(1)
					_, _ = io.WriteString(w, adminKeyJSON)
				default:
					t.Error("unexpected request")
				}
			}))
			defer server.Close()
			client, err := NewAdminClient(server.URL, "admin-secret")
			if err != nil {
				t.Fatal(err)
			}
			allowed := version == "0.79.2" || version == "0.79.2+build.1" || version == "0.79.3" || version == "0.80.0" || version == "1.0.0"
			for range 2 {
				_, err := client.UpdateAPIKeyCollectionPermissions(UpdateAPIKeyCollectionPermissionsRequest{APIKey: "target-secret", Collection: "users", Read: true})
				if (err == nil) != allowed {
					t.Fatal("version gate outcome mismatch")
				}
				if err != nil && (!strings.Contains(err.Error(), "earlier servers remove") || strings.Contains(err.Error(), "secret")) {
					t.Error("unsafe gate error")
				}
			}
			if health.Load() != 1 || (allowed && puts.Load() != 2) || (!allowed && puts.Load() != 0) {
				t.Error("gate request count mismatch")
			}
		})
	}
}
func TestAdminPermissionConcurrentVersionCache(t *testing.T) {
	var health atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/token":
			_, _ = io.WriteString(w, `{"token":"jwt-secret"}`)
		case "/api/health":
			health.Add(1)
			_, _ = io.WriteString(w, `{"version":"0.79.2"}`)
		default:
			_, _ = io.WriteString(w, adminKeyJSON)
		}
	}))
	defer server.Close()
	client, err := NewAdminClient(server.URL, "admin-secret")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_, err := client.UpdateAPIKeyCollectionPermissions(UpdateAPIKeyCollectionPermissionsRequest{APIKey: "target-secret", Collection: "users"})
			if err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if health.Load() != 1 {
		t.Error("version read was not shared")
	}
}
func TestAdminInvalidFieldEditsBeforeHealth(t *testing.T) {
	client := adminTestClient(t, func(http.ResponseWriter, *http.Request) { t.Error("invalid input reached HTTP") })
	empty := []string{" "}
	for _, request := range []UpdateAPIKeyCollectionPermissionsRequest{{APIKey: "target", Collection: "users", SelectFields: &empty}, {APIKey: " ", Collection: "users"}, {APIKey: "target", Collection: " "}} {
		if _, err := client.UpdateAPIKeyCollectionPermissions(request); err == nil {
			t.Error("invalid edit accepted")
		}
	}
}

func TestAdminUnknownMetadataSurvives(t *testing.T) {
	raw := `{"key":"new-secret","is_admin":false,"collection_permissions":{"users":{"read":true,"write":false,"select_fields":null,"exclude_fields":[],"writable_fields":["name"],"future_field":{"flag":true}}},"future_key":{"flag":true},"kv_permissions":[{"prefix":"jobs:","read":true,"write":false,"future_kv":{"flag":true}}]}`
	client := adminTestClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, raw) })
	keyResult, err := client.CreateAPIKey(CreateAPIKeyRequest{})
	if err != nil {
		t.Fatal(err)
	}
	key := *keyResult
	if len(key.KVPermissions) != 1 || len(key.KVPermissions[0].Metadata["future_kv"]) == 0 {
		t.Fatal("nested KV metadata lost")
	}

	if len(key.Metadata["future_key"]) == 0 || len(key.CollectionPermissions["users"].Metadata["future_field"]) == 0 {
		t.Fatal("unknown metadata dropped")
	}
	output, err := json.Marshal(key)
	if err != nil {
		t.Fatal(err)
	}
	var read map[string]any
	if json.Unmarshal(output, &read) != nil || read["future_key"] == nil || !strings.Contains(string(output), "future_kv") {
		t.Fatal("metadata roundtrip lost")
	}
	var permissions APIKeyPermissions
	if json.Unmarshal([]byte(raw), &permissions) != nil || len(permissions.Metadata["future_key"]) == 0 {
		t.Fatal("permission metadata lost")
	}
}
