package ekodb

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const adminKeyJSON = `{"key":"new-secret","label":"disposable","description":null,"is_admin":false,"collection_permissions":{"users":{"read":true,"write":false,"select_fields":["name"],"exclude_fields":["secret"],"writable_fields":[]}},"kv_permissions":[],"created_at":1,"expires_at":null}`

func adminTestClient(t *testing.T, handler http.HandlerFunc) *AdminClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/token" {
			if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" {
				t.Error("invalid authentication request")
			}
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !reflect.DeepEqual(body, map[string]string{"api_key": "admin-secret"}) {
				t.Error("invalid authentication body")
			}
			_, _ = io.WriteString(w, `{"token":"jwt-secret"}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer jwt-secret" || r.Header.Get("Accept") != "application/json" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("invalid admin headers")
		}
		if r.URL.Path == "/api/health" {
			_, _ = io.WriteString(w, `{"version":"0.79.2"}`)
			return
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	client, err := NewAdminClient(server.URL+"/", "admin-secret")
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestAdminWireContract(t *testing.T) {
	tests := []struct {
		name, method, path, body, response string
		status                             int
		run                                func(*AdminClient) error
	}{
		{"create-default", "POST", "/api/auth/register", `{"api_key":"admin-secret","is_admin":false,"collection_permissions":{}}`, adminKeyJSON, 200, func(a *AdminClient) error { _, err := a.CreateAPIKey(CreateAPIKeyRequest{}); return err }},
		{"create-admin", "POST", "/api/auth/register", `{"api_key":"admin-secret","is_admin":true,"label":"disposable","description":"test","collection_permissions":{"users":["read","write"]}}`, strings.Replace(adminKeyJSON, `"is_admin":false`, `"is_admin":true`, 1), 201, func(a *AdminClient) error {
			k, err := a.CreateAPIKey(CreateAPIKeyRequest{IsAdmin: true, Label: "disposable", Description: "test", CollectionPermissions: map[string][]APIKeyAction{"users": {APIKeyRead, APIKeyWrite}}})
			if err == nil && !k.IsAdmin {
				t.Error("admin flag lost")
			}
			return err
		}},
		{"list", "GET", "/api/auth/keys", "", "[" + adminKeyJSON + "]", 200, func(a *AdminClient) error {
			keys, err := a.ListAPIKeys()
			if err == nil && (len(keys) != 1 || keys[0].CollectionPermissions["users"].WritableFields == nil || len(keys[0].CollectionPermissions["users"].SelectFields) != 1 || keys[0].CreatedAt == nil) {
				t.Error("metadata or field restrictions lost")
			}
			return err
		}},
		{"permissions", "GET", "/api/auth/permissions", "", adminKeyJSON, 200, func(a *AdminClient) error {
			p, err := a.GetPermissions()
			if err == nil && (p.CollectionPermissions["users"].WritableFields == nil || !reflect.DeepEqual(p.CollectionPermissions["users"].ExcludeFields, []string{"secret"})) {
				t.Error("field restrictions lost")
			}
			return err
		}},
		{"update", "PUT", "/api/auth/permissions", `{"api_key":"target-secret","collection":"users","read":false,"write":true}`, adminKeyJSON, 200, func(a *AdminClient) error {
			_, err := a.UpdateAPIKeyCollectionPermissions(UpdateAPIKeyCollectionPermissionsRequest{APIKey: "target-secret", Collection: "users", Write: true})
			return err
		}},
		{"revoke-200", "DELETE", "/api/auth/keys/secret%2F%20%3F%23%25+&=%E9%9B%AA", "", `{"status":"success"}`, 200, func(a *AdminClient) error { return a.RevokeAPIKey("secret/ ?#%+&=雪") }},
		{"revoke-204", "DELETE", "/api/auth/keys/secret", "", "", 204, func(a *AdminClient) error { return a.RevokeAPIKey("secret") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			client := adminTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != tt.method || r.URL.EscapedPath() != tt.path || r.URL.RawQuery != "" {
					t.Errorf("unexpected method/path: %s %s", r.Method, r.URL.EscapedPath())
				}
				data, _ := io.ReadAll(r.Body)
				if tt.body == "" {
					if len(data) != 0 {
						t.Error("unexpected body")
					}
				} else {
					var got, want any
					if json.Unmarshal(data, &got) != nil || json.Unmarshal([]byte(tt.body), &want) != nil || !reflect.DeepEqual(got, want) {
						t.Error("JSON body mismatch")
					}
				}
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.response)
			})
			if err := tt.run(client); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Errorf("calls: %d", calls)
			}
		})
	}
}

func TestAdminLegacyArrayPermissions(t *testing.T) {
	client := adminTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"key":"new-secret","is_admin":false,"collection_permissions":{"users":["read","write"]}}`)
	})
	key, err := client.CreateAPIKey(CreateAPIKeyRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !key.CollectionPermissions["users"].Read || !key.CollectionPermissions["users"].Write {
		t.Fatal("legacy grants lost")
	}
}

func TestAdminMalformedResponses(t *testing.T) {
	bodies := []string{`null`, `{}`, `{"is_admin":"new-secret","collection_permissions":{}}`, `{"is_admin":false,"collection_permissions":{"users":{"read":true}}}`, `{"is_admin":false,"collection_permissions":{"users":{"read":true,"write":false,"select_fields":"new-secret"}}}`, `{"is_admin":false,"collection_permissions":{"users":["new-secret"]}}`, `{"is_admin":false,"collection_permissions":null}`, `{"is_admin":false,"collection_permissions":{},"key":null}`, `{"is_admin":false,"collection_permissions":{},"key":"new-secret","created_at":"new-secret"}`}
	for i, body := range bodies {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			client := adminTestClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) })
			_, err := client.CreateAPIKey(CreateAPIKeyRequest{})
			if err == nil || err.Error() != "invalid admin response" {
				t.Fatalf("expected redacted malformed response error, got %v", err)
			}
		})
	}
}

func TestAdminErrorsNeverLeakOrRetryAmbiguousCreation(t *testing.T) {
	for _, status := range []int{403, 429, 500, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var logs bytes.Buffer
			previous := log.Writer()
			log.SetOutput(&logs)
			defer log.SetOutput(previous)
			calls := 0
			client := adminTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "admin-secret jwt-secret new-secret Invalid token")
			})
			_, err := client.CreateAPIKey(CreateAPIKeyRequest{})
			if err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(logs.String(), "secret") {
				t.Fatal("expected safe failure")
			}
			if calls != 1 {
				t.Fatalf("ambiguous creation retried %d times", calls)
			}
		})
	}
}

func TestAdminAuthRefreshIsBounded(t *testing.T) {
	var tokens, calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/token" {
			n := tokens.Add(1)
			_, _ = fmt.Fprintf(w, `{"token":"jwt-%d"}`, n)
			return
		}
		n := calls.Add(1)
		if r.Header.Get("Authorization") != fmt.Sprintf("Bearer jwt-%d", n) {
			t.Error("stale token")
		}
		w.WriteHeader(401)
		_, _ = io.WriteString(w, "jwt-secret")
	}))
	defer server.Close()
	client, err := NewAdminClient(server.URL, "admin-secret")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CreateAPIKey(CreateAPIKeyRequest{})
	if err == nil || strings.Contains(err.Error(), "secret") || calls.Load() != 2 || tokens.Load() != 2 {
		t.Fatalf("unexpected bounded refresh: calls=%d tokens=%d err=%v", calls.Load(), tokens.Load(), err)
	}
}

func TestAdminProactiveRefresh(t *testing.T) {
	var tokens atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/token" {
			tokens.Add(1)
			_, _ = fmt.Fprintf(w, `{"token":"header.%s.signature"}`, base64.RawURLEncoding.EncodeToString([]byte(`{"exp":1}`)))
			return
		}
		_, _ = io.WriteString(w, `[]`)
	}))
	defer server.Close()
	client, err := NewAdminClient(server.URL, "admin-secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.ListAPIKeys(); err != nil {
		t.Fatal(err)
	}
	if tokens.Load() != 2 {
		t.Fatal("expired token was not refreshed")
	}
}

func TestAdminSuccessfulAuthRefresh(t *testing.T) {
	var tokens, calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/token" {
			tokens.Add(1)
			_, _ = io.WriteString(w, `{"token":"jwt-secret"}`)
			return
		}
		if calls.Add(1) == 1 {
			w.WriteHeader(401)
			return
		}
		_, _ = io.WriteString(w, adminKeyJSON)
	}))
	defer server.Close()
	client, err := NewAdminClient(server.URL, "admin-secret")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CreateAPIKey(CreateAPIKeyRequest{})
	if err != nil || tokens.Load() != 2 || calls.Load() != 2 {
		t.Fatal("definite 401 did not refresh successfully")
	}
}

func TestAdminRejectsRedirectsWithoutForwardingCredentials(t *testing.T) {
	var forwarded atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded.Add(1) }))
	defer destination.Close()
	client := adminTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	})
	_, err := client.CreateAPIKey(CreateAPIKeyRequest{})
	if err == nil || forwarded.Load() != 0 || strings.Contains(err.Error(), destination.URL) {
		t.Fatal("unsafe admin redirect")
	}
}

func TestAdminTimeoutNoRetry(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	defer close(release)
	client := adminTestClient(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); <-release })
	client.client.httpClient.Timeout = 10 * time.Millisecond
	_, err := client.CreateAPIKey(CreateAPIKeyRequest{})
	if err == nil || strings.Contains(err.Error(), "secret") || calls.Load() != 1 {
		t.Fatal("unsafe timeout handling")
	}
}

func TestAdminAuthErrorsAreRedacted(t *testing.T) {
	for _, body := range []string{`{"token":""}`, `{"token":123}`, `{"token":"new-secret" broken}`, `{"error":"admin-secret"}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) }))
		client, err := NewAdminClient(server.URL, "admin-secret")
		server.Close()
		if err == nil || client != nil || strings.Contains(err.Error(), "secret") {
			t.Fatal("invalid authentication accepted or leaked")
		}
	}
}

func TestAdminValidationBeforeHTTP(t *testing.T) {
	client := adminTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid request reached HTTP") })
	_, err := client.CreateAPIKey(CreateAPIKeyRequest{CollectionPermissions: map[string][]APIKeyAction{"users": {"admin-secret"}}})
	if err == nil {
		t.Fatal("unknown action accepted")
	}
	if err = client.RevokeAPIKey(""); err == nil {
		t.Fatal("empty key accepted")
	}
	_, err = client.UpdateAPIKeyCollectionPermissions(UpdateAPIKeyCollectionPermissionsRequest{})
	if err == nil {
		t.Fatal("empty target accepted")
	}
}

func TestAdminRegistrationPreservesPropagationWarning(t *testing.T) {
	client := adminTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, strings.Replace(adminKeyJSON, `"expires_at":null}`, `"expires_at":null,"ripple_warning":"Propagation incomplete"}`, 1))
	})
	key, err := client.CreateAPIKey(CreateAPIKeyRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if key.RippleWarning == nil || *key.RippleWarning != "Propagation incomplete" {
		t.Fatal("registration warning lost")
	}
}

func TestAdminDisposableCleanupAfterAmbiguousCreation(t *testing.T) {
	present := false
	deletions := 0
	const label = "sdk-go-admin-cleanup-test"
	client := adminTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/auth/register":
			present = true
			_, _ = io.WriteString(w, `malformed response after creating key`)
		case r.Method == "GET" && r.URL.Path == "/api/auth/keys":
			unrelated := strings.Replace(strings.Replace(adminKeyJSON, `"label":"disposable"`, `"label":"unrelated"`, 1), `"key":"new-secret"`, `"key":"unrelated-secret"`, 1)
			if present {
				_, _ = io.WriteString(w, "["+unrelated+","+strings.Replace(adminKeyJSON, `"label":"disposable"`, `"label":"`+label+`"`, 1)+"]")
			} else {
				_, _ = io.WriteString(w, "["+unrelated+"]")
			}
		case r.Method == "DELETE" && r.URL.Path == "/api/auth/keys/new-secret":
			present = false
			deletions++
			w.WriteHeader(204)
		default:
			t.Error("unexpected cleanup request")
		}
	})
	if _, err := client.CreateAPIKey(CreateAPIKeyRequest{Label: label}); err == nil {
		t.Fatal("malformed create response accepted")
	}
	if err := cleanupAdminLiveKey(client, label, "admin-secret"); err != nil {
		t.Fatal(err)
	}
	if present || deletions != 1 {
		t.Fatal("ambiguous key creation was not cleaned up exactly once")
	}
}
