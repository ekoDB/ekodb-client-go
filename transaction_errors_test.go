package ekodb

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// A transaction route answers a failure with
// {"error", "transaction_id", "retryable"}. A failed commit has already
// discarded its transaction, so the client never re-sends a transaction call,
// even with retries enabled: the caller sees the server's own message and
// flag, and Retryable means begin a new transaction (#406). Every test client
// below has retries enabled, so a single recorded request proves the client
// chose not to re-send.

// transactionFailureServer answers every transaction route with status and
// body, and records the path of each transaction request it receives.
func transactionFailureServer(t *testing.T, status int, contentType, body string) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/token" {
			mockTokenHandler(t)(w, r)
			return
		}
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	return server, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), paths...)
	}
}

// retryingTestClient has retries enabled, so a single recorded request proves
// the client chose not to re-send.
func retryingTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client, err := NewClientWithConfig(ClientConfig{
		BaseURL:     server.URL,
		APIKey:      "test-api-key",
		ShouldRetry: true,
		MaxRetries:  3,
		Timeout:     5 * time.Second,
		Format:      JSON,
	})
	if err != nil {
		t.Fatalf("Failed to create test client: %v", err)
	}
	return client
}

func requireHTTPError(t *testing.T, err error) *HTTPError {
	t.Helper()
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *HTTPError, got %T: %v", err, err)
	}
	return httpErr
}

func TestCommitConflictIsRetryableAndSentOnce(t *testing.T) {
	server, paths := transactionFailureServer(t, http.StatusConflict, "application/json",
		`{"error":"write conflict on users/1","transaction_id":"tx-1","retryable":true}`)
	defer server.Close()
	client := retryingTestClient(t, server)

	httpErr := requireHTTPError(t, client.CommitTransaction("tx-1"))
	if httpErr.StatusCode != http.StatusConflict {
		t.Errorf("StatusCode = %d, want 409", httpErr.StatusCode)
	}
	if httpErr.Message != "write conflict on users/1" {
		t.Errorf("Message = %q, want the server's error text", httpErr.Message)
	}
	if !httpErr.Retryable {
		t.Error("Retryable = false, want true")
	}
	if got := paths(); len(got) != 1 {
		t.Errorf("transaction requests = %v, want exactly one commit", got)
	}
}

func TestCommitConstraintFailureIsNotRetried(t *testing.T) {
	server, paths := transactionFailureServer(t, http.StatusBadRequest, "application/json",
		`{"error":"unique constraint violated","transaction_id":"tx-2","retryable":false}`)
	defer server.Close()
	client := retryingTestClient(t, server)

	httpErr := requireHTTPError(t, client.CommitTransaction("tx-2"))
	if httpErr.StatusCode != http.StatusBadRequest {
		t.Errorf("StatusCode = %d, want 400", httpErr.StatusCode)
	}
	if httpErr.Message != "unique constraint violated" {
		t.Errorf("Message = %q, want the server's error text", httpErr.Message)
	}
	if httpErr.Retryable {
		t.Error("Retryable = true, want false")
	}
	if got := paths(); len(got) != 1 {
		t.Errorf("transaction requests = %v, want exactly one commit", got)
	}
}

func TestCommit503IsSentOnceWhateverItsFlag(t *testing.T) {
	for _, tt := range []struct {
		body          string
		wantRetryable bool
	}{
		{`{"error":"at capacity","transaction_id":"tx-3","retryable":false}`, false},
		{`{"error":"shutting down","transaction_id":"tx-3","retryable":true}`, true},
	} {
		server, paths := transactionFailureServer(t, http.StatusServiceUnavailable, "application/json", tt.body)
		client := retryingTestClient(t, server)

		httpErr := requireHTTPError(t, client.CommitTransaction("tx-3"))
		if httpErr.StatusCode != http.StatusServiceUnavailable || httpErr.Retryable != tt.wantRetryable {
			t.Errorf("got %+v, want a 503 with Retryable %v", httpErr, tt.wantRetryable)
		}
		if got := paths(); len(got) != 1 {
			t.Errorf("transaction requests = %v, want exactly one commit", got)
		}
		server.Close()
	}
}

func TestCommitIsNotResentAfterANetworkError(t *testing.T) {
	var mu sync.Mutex
	commits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/token" {
			mockTokenHandler(t)(w, r)
			return
		}
		mu.Lock()
		commits++
		mu.Unlock()
		// Drop the connection without an answer: the client sees a network
		// error after the request reached the server.
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack failed: %v", err)
			return
		}
		_ = conn.Close()
	}))
	defer server.Close()
	client := retryingTestClient(t, server)

	err := client.CommitTransaction("tx-5")
	if err == nil {
		t.Fatal("CommitTransaction succeeded, want a network error")
	}
	mu.Lock()
	defer mu.Unlock()
	if commits != 1 {
		t.Errorf("commit requests = %d, want exactly one", commits)
	}
}

func TestCommitNonJSONErrorBodyFallsBackToRawText(t *testing.T) {
	server, paths := transactionFailureServer(t, http.StatusBadGateway, "text/plain", "upstream connect error")
	defer server.Close()
	client := retryingTestClient(t, server)

	httpErr := requireHTTPError(t, client.CommitTransaction("tx-4"))
	if httpErr.StatusCode != http.StatusBadGateway {
		t.Errorf("StatusCode = %d, want 502", httpErr.StatusCode)
	}
	if httpErr.Message != "upstream connect error" {
		t.Errorf("Message = %q, want the raw body", httpErr.Message)
	}
	if httpErr.Retryable {
		t.Error("Retryable = true, want false")
	}
	if got := paths(); len(got) != 1 {
		t.Errorf("transaction requests = %v, want exactly one commit", got)
	}
}

func TestEveryTransactionRouteReturnsTheTypedError(t *testing.T) {
	server, paths := transactionFailureServer(t, http.StatusNotFound, "application/json",
		`{"error":"transaction tx-9 not found","transaction_id":"tx-9","retryable":false}`)
	defer server.Close()
	client := retryingTestClient(t, server)

	calls := map[string]func() error{
		"BeginTransaction": func() error {
			_, err := client.BeginTransaction()
			return err
		},
		"GetTransactionStatus": func() error {
			_, err := client.GetTransactionStatus("tx-9")
			return err
		},
		"RollbackTransaction": func() error { return client.RollbackTransaction("tx-9") },
		"CreateSavepoint":     func() error { return client.CreateSavepoint("tx-9", "sp1") },
		"RollbackToSavepoint": func() error { return client.RollbackToSavepoint("tx-9", "sp1") },
		"ReleaseSavepoint":    func() error { return client.ReleaseSavepoint("tx-9", "sp1") },
	}
	for name, call := range calls {
		httpErr := requireHTTPError(t, call())
		if httpErr.StatusCode != http.StatusNotFound ||
			httpErr.Message != "transaction tx-9 not found" ||
			httpErr.Retryable {
			t.Errorf("%s: got %+v, want a non-retryable 404 with the server's message", name, httpErr)
		}
	}
	if got := paths(); len(got) != len(calls) {
		t.Errorf("transaction requests = %v, want one per call", got)
	}
}

func TestTransactionHTTPErrorParsing(t *testing.T) {
	tests := []struct {
		name          string
		status        int
		body          string
		wantMessage   string
		wantRetryable bool
	}{
		{"server envelope", 409, `{"error":"conflict","retryable":true}`, "conflict", true},
		{"absent flag", 400, `{"error":"bad"}`, "bad", false},
		{"non-boolean flag", 409, `{"error":"conflict","retryable":"true"}`, "conflict", false},
		{"non-string error", 500, `{"error":7,"retryable":false}`, `{"error":7,"retryable":false}`, false},
		{"empty body", 500, "", "500 Internal Server Error", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := transactionHTTPError(tt.status, []byte(tt.body))
			if got.StatusCode != tt.status || got.Message != tt.wantMessage || got.Retryable != tt.wantRetryable {
				t.Errorf("transactionHTTPError(%d, %q) = %+v, want message %q retryable %v",
					tt.status, tt.body, got, tt.wantMessage, tt.wantRetryable)
			}
		})
	}
}

func TestIsTransactionPath(t *testing.T) {
	for path, want := range map[string]bool{
		"/api/transactions":              true,
		"/api/transactions/tx-1/commit":  true,
		"/api/transactionsx":             false,
		"/api/insert/transactions":       false,
		"/api/find/users?transaction_id": false,
	} {
		if got := isTransactionPath(path); got != want {
			t.Errorf("isTransactionPath(%q) = %v, want %v", path, got, want)
		}
	}
}
