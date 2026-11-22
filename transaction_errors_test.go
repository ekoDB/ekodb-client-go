package ekodb

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// A call on an existing transaction (commit, rollback, or a savepoint call)
// answers a failure with {"error", "transaction_id", "retryable"}. A failed
// commit has already discarded its transaction, so the client never re-sends
// those calls, even with retries enabled: the caller sees the server's own
// message and flag, and Retryable means begin a new transaction
// (ekoDB/ekodb-client#406). Begin and status keep the normal retry policy, and
// a 401 is sent once more after the token is refreshed. Every test client
// below has retries enabled, so a single recorded request proves the client
// chose not to re-send.

// transactionAnswer is one canned response to a transaction request.
type transactionAnswer struct {
	status      int
	contentType string
	headers     map[string]string
	body        string
}

// transactionFailureServer answers every transaction route with status and
// body, and records the path of each transaction request it receives.
func transactionFailureServer(t *testing.T, status int, contentType, body string) (*httptest.Server, func() []string) {
	t.Helper()
	server, paths, _ := transactionAnswerServer(t, transactionAnswer{status: status, contentType: contentType, body: body})
	return server, paths
}

// transactionAnswerServer answers transaction requests with answers in order,
// repeating the last one, and records the path of each transaction request and
// the number of token exchanges.
func transactionAnswerServer(t *testing.T, answers ...transactionAnswer) (*httptest.Server, func() []string, func() int) {
	t.Helper()
	var mu sync.Mutex
	var paths []string
	tokenRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/token" {
			mu.Lock()
			tokenRequests++
			mu.Unlock()
			mockTokenHandler(t)(w, r)
			return
		}
		mu.Lock()
		paths = append(paths, r.URL.Path)
		answer := answers[min(len(paths), len(answers))-1]
		mu.Unlock()
		for name, value := range answer.headers {
			w.Header().Set(name, value)
		}
		w.Header().Set("Content-Type", answer.contentType)
		w.WriteHeader(answer.status)
		_, _ = w.Write([]byte(answer.body))
	}))
	recordedPaths := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), paths...)
	}
	recordedTokenRequests := func() int {
		mu.Lock()
		defer mu.Unlock()
		return tokenRequests
	}
	return server, recordedPaths, recordedTokenRequests
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

func TestEveryCallOnAnExistingTransactionReturnsTheTypedError(t *testing.T) {
	server, paths := transactionFailureServer(t, http.StatusNotFound, "application/json",
		`{"error":"transaction tx-9 not found","transaction_id":"tx-9","retryable":false}`)
	defer server.Close()
	client := retryingTestClient(t, server)

	calls := map[string]func() error{
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

// A commit answered with a status the retry policy would re-send, or with the
// 500 the policy reads as an expired token, is still sent once.
func TestCommitIsSentOnceOnStatusesTheRetryPolicyWouldResend(t *testing.T) {
	for _, tt := range []struct {
		name          string
		answer        transactionAnswer
		wantMessage   string
		wantRetryable bool
	}{
		{
			name: "429",
			answer: transactionAnswer{
				status:      http.StatusTooManyRequests,
				contentType: "application/json",
				headers:     map[string]string{"Retry-After": "0"},
				body:        `{"error":"too many requests","transaction_id":"tx-7","retryable":true}`,
			},
			wantMessage:   "too many requests",
			wantRetryable: true,
		},
		{
			name: "500 mentioning an invalid token",
			answer: transactionAnswer{
				status:      http.StatusInternalServerError,
				contentType: "application/json",
				body:        `{"error":"Invalid token","transaction_id":"tx-7","retryable":false}`,
			},
			wantMessage:   "Invalid token",
			wantRetryable: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server, paths, tokenRequests := transactionAnswerServer(t, tt.answer)
			defer server.Close()
			client := retryingTestClient(t, server)

			httpErr := requireHTTPError(t, client.CommitTransaction("tx-7"))
			if httpErr.StatusCode != tt.answer.status ||
				httpErr.Message != tt.wantMessage ||
				httpErr.Retryable != tt.wantRetryable {
				t.Errorf("got %+v, want status %d, message %q, Retryable %v",
					httpErr, tt.answer.status, tt.wantMessage, tt.wantRetryable)
			}
			if got := paths(); len(got) != 1 {
				t.Errorf("transaction requests = %v, want exactly one commit", got)
			}
			// Only the token exchange made when the client was built.
			if got := tokenRequests(); got != 1 {
				t.Errorf("token requests = %d, want 1 (no refresh)", got)
			}
		})
	}
}

// Begin keeps the normal retry policy: a begin refused with 429 started no
// transaction, so it is sent again and the second answer succeeds.
func TestBeginIsRetriedAfterA429(t *testing.T) {
	server, paths, _ := transactionAnswerServer(t,
		transactionAnswer{status: http.StatusTooManyRequests, contentType: "text/plain", headers: map[string]string{"Retry-After": "0"}},
		transactionAnswer{status: http.StatusOK, contentType: "application/json", body: `{"transaction_id":"tx-new"}`},
	)
	defer server.Close()
	client := retryingTestClient(t, server)

	transactionID, err := client.BeginTransaction()
	if err != nil {
		t.Fatalf("BeginTransaction: %v", err)
	}
	if transactionID != "tx-new" {
		t.Errorf("transaction ID = %q, want tx-new", transactionID)
	}
	if got := paths(); len(got) != 2 {
		t.Errorf("transaction requests = %v, want the begin sent twice", got)
	}
}

// A begin still throttled after every retry returns the normal RateLimitError,
// not a transaction HTTPError.
func TestBeginThrottledPastItsRetriesReturnsRateLimitError(t *testing.T) {
	server, paths, _ := transactionAnswerServer(t,
		transactionAnswer{status: http.StatusTooManyRequests, contentType: "text/plain", headers: map[string]string{"Retry-After": "0"}, body: "rate limited"},
	)
	defer server.Close()
	client := retryingTestClient(t, server)

	_, err := client.BeginTransaction()
	var rateLimitErr *RateLimitError
	if !errors.As(err, &rateLimitErr) {
		t.Fatalf("expected *RateLimitError, got %T: %v", err, err)
	}
	if rateLimitErr.RetryAfterSecs != 0 {
		t.Errorf("RetryAfterSecs = %d, want 0", rateLimitErr.RetryAfterSecs)
	}
	// The first send plus the client's three retries.
	if got := paths(); len(got) != 4 {
		t.Errorf("transaction requests = %v, want four sends", got)
	}
}

// Status is a read, so it keeps the normal retry policy too.
func TestStatusIsRetriedAfterA429(t *testing.T) {
	server, paths, _ := transactionAnswerServer(t,
		transactionAnswer{status: http.StatusTooManyRequests, contentType: "text/plain", headers: map[string]string{"Retry-After": "0"}},
		transactionAnswer{status: http.StatusOK, contentType: "application/json", body: `{"state":"Active","operations_count":2}`},
	)
	defer server.Close()
	client := retryingTestClient(t, server)

	status, err := client.GetTransactionStatus("tx-10")
	if err != nil {
		t.Fatalf("GetTransactionStatus: %v", err)
	}
	if status["state"] != "Active" {
		t.Errorf("state = %v, want Active", status["state"])
	}
	if got := paths(); len(got) != 2 {
		t.Errorf("transaction requests = %v, want the status read sent twice", got)
	}
}

// A commit answered 401 never reached the transaction, so the client refreshes
// its token once and sends the commit once more, which succeeds.
func TestCommit401RefreshesTheTokenOnceAndResends(t *testing.T) {
	server, paths, tokenRequests := transactionAnswerServer(t,
		transactionAnswer{status: http.StatusUnauthorized, contentType: "application/json", body: `{"error":"token expired"}`},
		transactionAnswer{status: http.StatusOK, contentType: "application/json", body: `{"status":"committed"}`},
	)
	defer server.Close()
	client := retryingTestClient(t, server)

	if err := client.CommitTransaction("tx-11"); err != nil {
		t.Fatalf("CommitTransaction: %v", err)
	}
	if got := paths(); len(got) != 2 {
		t.Errorf("transaction requests = %v, want the commit sent twice", got)
	}
	// The token exchange made when the client was built, and the one refresh.
	if got := tokenRequests(); got != 2 {
		t.Errorf("token requests = %d, want 2", got)
	}
}

// A commit answered 401 twice is sent exactly twice and then returns the
// authentication error, without looping.
func TestCommit401TwiceReturnsAnAuthErrorAfterTwoSends(t *testing.T) {
	server, paths, tokenRequests := transactionAnswerServer(t,
		transactionAnswer{status: http.StatusUnauthorized, contentType: "application/json", body: `{"error":"token expired"}`},
	)
	defer server.Close()
	client := retryingTestClient(t, server)

	err := client.CommitTransaction("tx-12")
	if err == nil || !strings.Contains(err.Error(), "authentication failed after token refresh (status 401)") {
		t.Fatalf("err = %v, want the authentication failure after a refresh", err)
	}
	if got := paths(); len(got) != 2 {
		t.Errorf("transaction requests = %v, want the commit sent twice", got)
	}
	if got := tokenRequests(); got != 2 {
		t.Errorf("token requests = %d, want 2", got)
	}
}

// On a client that redacts errors (the one behind an AdminClient), a failed
// commit keeps the status and flag but never the server's text.
func TestRedactedClientCommitErrorCarriesTheRedactedMessage(t *testing.T) {
	server, paths := transactionFailureServer(t, http.StatusConflict, "application/json",
		`{"error":"write conflict on secret-users/1","transaction_id":"tx-13","retryable":true}`)
	defer server.Close()
	admin, err := NewAdminClientWithConfig(AdminClientConfig{BaseURL: server.URL, APIKey: "test-api-key", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("NewAdminClientWithConfig: %v", err)
	}

	httpErr := requireHTTPError(t, admin.client.CommitTransaction("tx-13"))
	if httpErr.StatusCode != http.StatusConflict {
		t.Errorf("StatusCode = %d, want 409", httpErr.StatusCode)
	}
	if httpErr.Message != "admin request failed" {
		t.Errorf("Message = %q, want the redacted text", httpErr.Message)
	}
	if !httpErr.Retryable {
		t.Error("Retryable = false, want the server's flag")
	}
	if got := paths(); len(got) != 1 {
		t.Errorf("transaction requests = %v, want exactly one commit", got)
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

func TestActsOnExistingTransaction(t *testing.T) {
	for _, tt := range []struct {
		method string
		path   string
		want   bool
	}{
		{http.MethodPost, "/api/transactions/tx-1/commit", true},
		{http.MethodPost, "/api/transactions/tx-1/rollback", true},
		{http.MethodPost, "/api/transactions/tx-1/savepoints", true},
		{http.MethodPost, "/api/transactions/tx-1/savepoints/sp1/rollback", true},
		{http.MethodDelete, "/api/transactions/tx-1/savepoints/sp1", true},
		{http.MethodPost, "/api/transactions/tx%2F1/commit", true},
		// Begin and status keep the normal retry policy.
		{http.MethodPost, "/api/transactions", false},
		{http.MethodGet, "/api/transactions/tx-1", false},
		{http.MethodGet, "/api/transactions/tx-1/commit", false},
		{http.MethodPost, "/api/transactions/tx-1/savepoints/sp1", false},
		{http.MethodPost, "/api/transactions//commit", false},
		{http.MethodPost, "/api/transactionsx/tx-1/commit", false},
		{http.MethodPost, "/api/insert/transactions", false},
	} {
		if got := actsOnExistingTransaction(tt.method, tt.path); got != tt.want {
			t.Errorf("actsOnExistingTransaction(%s, %q) = %v, want %v", tt.method, tt.path, got, tt.want)
		}
	}
}
