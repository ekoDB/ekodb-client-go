package ekodb

import (
	"encoding/json"
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
