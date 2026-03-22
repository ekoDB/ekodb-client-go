package ekodb

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// Token Refresh & Thread Safety Tests
// ============================================================================

// TestTokenRefreshOnUnauthorized verifies that a 401 triggers a token refresh
// and the retried request succeeds with the new token.
func TestTokenRefreshOnUnauthorized(t *testing.T) {
	var requestCount atomic.Int32
	newToken := "refreshed-token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/token" {
			w.Header().Set("Content-Type", "application/json")
			// First call returns the initial token, subsequent calls return refreshed token
			if requestCount.Load() == 0 {
				json.NewEncoder(w).Encode(map[string]string{"token": "initial-token"})
			} else {
				json.NewEncoder(w).Encode(map[string]string{"token": newToken})
			}
			return
		}

		auth := r.Header.Get("Authorization")
		if auth == "Bearer "+newToken {
			// New token is accepted
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]interface{}{})
			return
		}

		// Old token rejected with 401
		requestCount.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"code":401,"message":"Invalid token: wrong instance"}`))
	}))
	defer server.Close()

	client, err := NewClientWithConfig(ClientConfig{
		BaseURL:     server.URL,
		APIKey:      "test-key",
		ShouldRetry: true,
		MaxRetries:  3,
		Timeout:     5 * time.Second,
		Format:      JSON,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// This request should fail with 401, refresh token, and retry successfully
	_, err = client.Find("test_collection", map[string]interface{}{})
	if err != nil {
		t.Fatalf("Expected request to succeed after token refresh, got: %v", err)
	}

	// Verify the client now holds the new token
	got := client.getToken()
	if got != newToken {
		t.Errorf("Expected token %q, got %q", newToken, got)
	}
}

// TestRefreshTokenIfStaleSkipsDuplicate verifies that concurrent goroutines
// don't redundantly refresh when one goroutine has already done it.
func TestRefreshTokenIfStaleSkipsDuplicate(t *testing.T) {
	var refreshCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/token" {
			refreshCount.Add(1)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"token": "new-token"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, err := NewClientWithConfig(ClientConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Timeout: 5 * time.Second,
		Format:  JSON,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Reset counter after initial token fetch
	refreshCount.Store(0)

	// Simulate: set a stale token, then have multiple goroutines try to refresh
	staleToken := "stale-token-abc"
	client.tokenMu.Lock()
	client.token = staleToken
	client.tokenMu.Unlock()

	// Launch multiple goroutines all trying to refresh with the same stale token
	var wg sync.WaitGroup
	goroutines := 10
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			client.refreshTokenIfStale(staleToken)
		}()
	}
	wg.Wait()

	// Only ONE actual HTTP refresh should have happened
	count := refreshCount.Load()
	if count != 1 {
		t.Errorf("Expected exactly 1 refresh call, got %d", count)
	}

	// Token should be the new one
	got := client.getToken()
	if got != "new-token" {
		t.Errorf("Expected token %q, got %q", "new-token", got)
	}
}

// TestConcurrentRequestsAfterTokenRefresh verifies that once a token is
// refreshed, all subsequent concurrent requests use the new token without
// hitting 401.
func TestConcurrentRequestsAfterTokenRefresh(t *testing.T) {
	var tokenRefreshCount atomic.Int32
	var unauthorizedCount atomic.Int32
	currentToken := "old-token"
	newToken := "fresh-token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/token" {
			tokenRefreshCount.Add(1)
			// Small delay to simulate real network latency
			time.Sleep(10 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"token": newToken})
			return
		}

		auth := r.Header.Get("Authorization")
		if auth == "Bearer "+newToken {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]interface{}{})
			return
		}

		// Old token gets 401
		unauthorizedCount.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"code":401,"message":"Invalid token: wrong instance"}`))
	}))
	defer server.Close()

	client, err := NewClientWithConfig(ClientConfig{
		BaseURL:     server.URL,
		APIKey:      "test-key",
		ShouldRetry: true,
		MaxRetries:  3,
		Timeout:     5 * time.Second,
		Format:      JSON,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Reset counter after initial token fetch
	tokenRefreshCount.Store(0)

	// Set stale token to simulate post-restart scenario
	client.tokenMu.Lock()
	client.token = currentToken
	client.tokenMu.Unlock()

	// Fire concurrent requests that will all initially fail with stale token
	goroutines := 20
	var wg sync.WaitGroup
	errors := make([]error, goroutines)
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			_, err := client.Find("test_collection", map[string]interface{}{})
			errors[idx] = err
		}(i)
	}
	wg.Wait()

	// All requests should eventually succeed
	for i, err := range errors {
		if err != nil {
			t.Errorf("Goroutine %d failed: %v", i, err)
		}
	}

	// Only 1 actual refresh HTTP call should have happened (double-check prevents duplicates)
	refreshes := tokenRefreshCount.Load()
	if refreshes != 1 {
		t.Errorf("Expected 1 token refresh call, got %d (thundering herd not prevented)", refreshes)
	}

	// Verify final token
	got := client.getToken()
	if got != newToken {
		t.Errorf("Expected token %q, got %q", newToken, got)
	}
}

// TestGetTokenIsThreadSafe verifies concurrent reads don't race with writes.
func TestGetTokenIsThreadSafe(t *testing.T) {
	var refreshCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/token" {
			refreshCount.Add(1)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"token": "token-v" + string(rune('0'+refreshCount.Load()))})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, err := NewClientWithConfig(ClientConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Timeout: 5 * time.Second,
		Format:  JSON,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Concurrent reads and writes should not panic or produce empty strings
	var wg sync.WaitGroup
	wg.Add(20)

	// 10 readers
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				token := client.getToken()
				if token == "" {
					t.Error("getToken() returned empty string during concurrent access")
					return
				}
			}
		}()
	}

	// 10 writers
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				client.refreshToken()
			}
		}()
	}

	wg.Wait()
}

// ============================================================================
// JWT Expiry & Token Caching Tests
// ============================================================================

// makeTestJWT constructs a minimal JWT (header.payload.signature) with the given
// claims in the payload. The signature is fake — these are only for testing
// extractJWTExpiry, not for cryptographic verification.
func makeTestJWT(claims map[string]interface{}) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT","alg":"HS256"}`))
	payloadBytes, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	return fmt.Sprintf("%s.%s.fakesignature", header, payload)
}

// TestExtractJWTExpiry verifies that extractJWTExpiry correctly decodes the exp
// claim from a well-formed JWT.
func TestExtractJWTExpiry(t *testing.T) {
	expected := int64(1700000000)
	token := makeTestJWT(map[string]interface{}{
		"sub": "test-user",
		"exp": expected,
		"iat": 1699996400,
	})

	exp, ok := extractJWTExpiry(token)
	if !ok {
		t.Fatal("extractJWTExpiry returned false for a valid JWT")
	}
	if exp != expected {
		t.Errorf("Expected expiry %d, got %d", expected, exp)
	}
}

// TestExtractJWTExpiryInvalid verifies that extractJWTExpiry gracefully returns
// (0, false) for malformed inputs.
func TestExtractJWTExpiryInvalid(t *testing.T) {
	cases := []struct {
		name  string
		token string
	}{
		{"not a JWT", "not-a-jwt"},
		{"two segments", "header.payload"},
		{"bad base64 payload", "header.!!!invalid!!!.signature"},
		{"empty string", ""},
		{"four segments", "a.b.c.d"},
		{"valid base64 but not JSON", fmt.Sprintf("x.%s.y", base64.RawURLEncoding.EncodeToString([]byte("not json")))},
		{"valid JSON but no exp claim", makeTestJWT(map[string]interface{}{"sub": "no-exp"})},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exp, ok := extractJWTExpiry(tc.token)
			if ok {
				t.Errorf("Expected (0, false) for %q, got (%d, true)", tc.name, exp)
			}
			if exp != 0 {
				t.Errorf("Expected exp=0 for %q, got %d", tc.name, exp)
			}
		})
	}
}

// TestTokenExpiryProactiveRefresh verifies that getToken() proactively refreshes
// when the cached token is about to expire (within 60 seconds).
func TestTokenExpiryProactiveRefresh(t *testing.T) {
	var refreshCount atomic.Int32
	newToken := makeTestJWT(map[string]interface{}{
		"exp": time.Now().Unix() + 7200, // 2 hours from now
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/token" {
			refreshCount.Add(1)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"token": newToken})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, err := NewClientWithConfig(ClientConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Timeout: 5 * time.Second,
		Format:  JSON,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Reset counter after initial token fetch during NewClientWithConfig
	refreshCount.Store(0)

	// Manually set a token that expires in 30 seconds (within the 60s threshold)
	soonExpiry := time.Now().Unix() + 30
	client.tokenMu.Lock()
	client.token = makeTestJWT(map[string]interface{}{"exp": soonExpiry})
	client.tokenExpiry = soonExpiry
	client.tokenMu.Unlock()

	// getToken() should detect the near-expiry and proactively refresh
	got := client.getToken()

	// Verify a refresh happened
	count := refreshCount.Load()
	if count != 1 {
		t.Errorf("Expected 1 proactive refresh, got %d", count)
	}

	// Verify the returned token is the new one from the server
	if got != newToken {
		t.Errorf("Expected new token from server, got %q", got)
	}

	// Verify the expiry was updated to the new token's expiry
	client.tokenMu.RLock()
	newExpiry := client.tokenExpiry
	client.tokenMu.RUnlock()
	if newExpiry <= soonExpiry {
		t.Errorf("Expected updated expiry > %d, got %d", soonExpiry, newExpiry)
	}
}

// TestTokenExpiryStillValid verifies that getToken() returns the cached token
// without making any HTTP call when the token is far from expiry.
func TestTokenExpiryStillValid(t *testing.T) {
	var refreshCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/token" {
			refreshCount.Add(1)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"token": "should-not-be-fetched"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, err := NewClientWithConfig(ClientConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Timeout: 5 * time.Second,
		Format:  JSON,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Reset counter after initial token fetch
	refreshCount.Store(0)

	// Set a token with expiry far in the future (2 hours)
	farExpiry := time.Now().Unix() + 7200
	cachedToken := makeTestJWT(map[string]interface{}{"exp": farExpiry})
	client.tokenMu.Lock()
	client.token = cachedToken
	client.tokenExpiry = farExpiry
	client.tokenMu.Unlock()

	// getToken() should return the cached token without any HTTP call
	got := client.getToken()

	if got != cachedToken {
		t.Errorf("Expected cached token, got different token")
	}

	count := refreshCount.Load()
	if count != 0 {
		t.Errorf("Expected 0 refresh calls (token still valid), got %d", count)
	}
}

// TestClearTokenCache verifies that ClearTokenCache resets both the token and expiry.
func TestClearTokenCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/token" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"token": "some-token"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, err := NewClientWithConfig(ClientConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Timeout: 5 * time.Second,
		Format:  JSON,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Verify token is populated after init
	client.tokenMu.RLock()
	if client.token == "" {
		t.Fatal("Expected non-empty token after init")
	}
	if client.tokenExpiry == 0 {
		t.Fatal("Expected non-zero tokenExpiry after init")
	}
	client.tokenMu.RUnlock()

	// Clear the cache
	client.ClearTokenCache()

	// Verify both fields are zeroed
	client.tokenMu.RLock()
	defer client.tokenMu.RUnlock()
	if client.token != "" {
		t.Errorf("Expected empty token after ClearTokenCache, got %q", client.token)
	}
	if client.tokenExpiry != 0 {
		t.Errorf("Expected tokenExpiry=0 after ClearTokenCache, got %d", client.tokenExpiry)
	}
}
