package ekodb

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// newCapturingServerWithBody is like newCapturingServer (client_test.go) but
// lets the caller choose the success-response body, so methods whose response
// unmarshals into a non-object type (e.g. GetChatModel → []string) can be
// exercised while still capturing the escaped request path.
func newCapturingServerWithBody(t *testing.T, got *capturedRequest, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/token" {
			mockTokenHandler(t)(w, r)
			return
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-jwt-token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("Unauthorized"))
			return
		}
		got.method = r.Method
		got.escapedPath = r.URL.EscapedPath()
		got.rawQuery = r.URL.RawQuery
		got.queryValues = r.URL.Query()
		got.body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}

// These tests prove that caller-supplied path segments (function label, chat
// model/provider name, agent name, ids, collection, field, etc.) are
// url.PathEscape'd into the request path so a reserved character (slash,
// space, '#', '?') in a single segment becomes its percent-encoded form and
// cannot break out of its path segment. They reuse the capturing-server
// harness defined in client_test.go (newCapturingServer / capturedRequest /
// createTestClient).

// TestCallerSegmentsEscaped drives one client method per fixed site with a
// segment containing reserved characters and asserts the captured escaped path
// matches url.PathEscape of that segment (so "a/b" appears as "a%2Fb", not a
// raw separator) and round-trips back to the raw value when decoded.
func TestCallerSegmentsEscaped(t *testing.T) {
	// reserved is a single logical segment that MUST stay one segment on the
	// wire: a slash (path separator), a space, a '#' (fragment) and a '?'
	// (query) — all reserved in a path.
	const reserved = "a/b c#d?e"
	esc := url.PathEscape(reserved)

	cases := []struct {
		name       string
		call       func(c *Client) error
		wantPath   string
		wantMethod string
	}{
		// chat.go
		{
			name:       "GetChatSession",
			call:       func(c *Client) error { _, err := c.GetChatSession(reserved); return err },
			wantPath:   "/api/chat/" + esc,
			wantMethod: "GET",
		},
		{
			name:       "DeleteChatSession",
			call:       func(c *Client) error { return c.DeleteChatSession(reserved) },
			wantPath:   "/api/chat/" + esc,
			wantMethod: "DELETE",
		},
		{
			name:       "GetChatMessage",
			call:       func(c *Client) error { _, err := c.GetChatMessage(reserved, reserved); return err },
			wantPath:   "/api/chat/" + esc + "/messages/" + esc,
			wantMethod: "GET",
		},
		// functions.go
		{
			name:       "GetUserFunction",
			call:       func(c *Client) error { _, err := c.GetUserFunction(reserved); return err },
			wantPath:   "/api/functions/" + esc,
			wantMethod: "GET",
		},
		{
			name:       "DeleteUserFunction",
			call:       func(c *Client) error { return c.DeleteUserFunction(reserved) },
			wantPath:   "/api/functions/" + esc,
			wantMethod: "DELETE",
		},
		// goals_tasks_agents.go
		{
			name:       "GoalGet",
			call:       func(c *Client) error { _, err := c.GoalGet(reserved); return err },
			wantPath:   "/api/chat/goals/" + esc,
			wantMethod: "GET",
		},
		{
			name:       "AgentGetByName",
			call:       func(c *Client) error { _, err := c.AgentGetByName(reserved); return err },
			wantPath:   "/api/chat/agents/by-name/" + esc,
			wantMethod: "GET",
		},
		{
			name:       "AgentsByDeployment",
			call:       func(c *Client) error { _, err := c.AgentsByDeployment(reserved); return err },
			wantPath:   "/api/chat/agents/by-deployment/" + esc,
			wantMethod: "GET",
		},
		// schedules.go
		{
			name:       "GetSchedule",
			call:       func(c *Client) error { _, err := c.GetSchedule(reserved); return err },
			wantPath:   "/api/schedules/" + esc,
			wantMethod: "GET",
		},
		{
			name:       "DeleteSchedule",
			call:       func(c *Client) error { return c.DeleteSchedule(reserved) },
			wantPath:   "/api/schedules/" + esc,
			wantMethod: "DELETE",
		},
		// schema.go
		{
			name:       "GetCollection",
			call:       func(c *Client) error { _, err := c.GetCollection(reserved); return err },
			wantPath:   "/api/collections/" + esc,
			wantMethod: "GET",
		},
		// search.go
		{
			name:       "Search",
			call:       func(c *Client) error { _, err := c.Search(reserved, SearchQuery{Query: "x"}); return err },
			wantPath:   "/api/search/" + esc,
			wantMethod: "POST",
		},
		{
			name: "DistinctValues",
			call: func(c *Client) error {
				_, err := c.DistinctValues(reserved, reserved, DistinctValuesQuery{})
				return err
			},
			wantPath:   "/api/distinct/" + esc + "/" + esc,
			wantMethod: "POST",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			server := newCapturingServer(t, &got)
			defer server.Close()

			client := createTestClient(t, server)
			if err := tc.call(client); err != nil {
				t.Fatalf("%s call failed: %v", tc.name, err)
			}

			if got.method != tc.wantMethod {
				t.Errorf("%s method = %q, want %q", tc.name, got.method, tc.wantMethod)
			}
			if got.escapedPath != tc.wantPath {
				t.Errorf("%s escaped path = %q, want %q", tc.name, got.escapedPath, tc.wantPath)
			}
			// The reserved slash must be percent-encoded, never a raw separator.
			if !strings.Contains(got.escapedPath, "a%2Fb") {
				t.Errorf("%s escaped path %q does not contain percent-encoded slash a%%2Fb", tc.name, got.escapedPath)
			}
		})
	}
}

// TestGetChatModelEscaped proves the provider-name segment of
// GET /api/chat_models/{name} is escaped. GetChatModel unmarshals into a
// []string, so it needs a server that replies with a JSON array (the shared
// newCapturingServer replies with {}); we capture the escaped path directly.
func TestGetChatModelEscaped(t *testing.T) {
	const provider = "anthropic/claude"
	esc := url.PathEscape(provider)

	var got capturedRequest
	server := newCapturingServerWithBody(t, &got, "[]")
	defer server.Close()

	client := createTestClient(t, server)
	if _, err := client.GetChatModel(provider); err != nil {
		t.Fatalf("GetChatModel failed: %v", err)
	}

	wantPath := "/api/chat_models/" + esc
	if got.method != "GET" {
		t.Errorf("GetChatModel method = %q, want GET", got.method)
	}
	if got.escapedPath != wantPath {
		t.Errorf("GetChatModel escaped path = %q, want %q", got.escapedPath, wantPath)
	}
	if !strings.Contains(got.escapedPath, "anthropic%2Fclaude") {
		t.Errorf("GetChatModel escaped path %q does not contain percent-encoded slash anthropic%%2Fclaude", got.escapedPath)
	}
}

// TestBatchCollectionEscaped proves the collection segment of the batch CRUD
// endpoints (client.go) is escaped. These used raw string concatenation before
// the parity fix.
func TestBatchCollectionEscaped(t *testing.T) {
	const reserved = "coll/with space"
	esc := url.PathEscape(reserved)

	cases := []struct {
		name     string
		call     func(c *Client) error
		wantPath string
	}{
		{
			name:     "BatchInsert",
			call:     func(c *Client) error { _, err := c.BatchInsert(reserved, []Record{{"k": "v"}}); return err },
			wantPath: "/api/batch/insert/" + esc,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			server := newCapturingServer(t, &got)
			defer server.Close()

			client := createTestClient(t, server)
			if err := tc.call(client); err != nil {
				t.Fatalf("%s call failed: %v", tc.name, err)
			}
			if got.escapedPath != tc.wantPath {
				t.Errorf("%s escaped path = %q, want %q", tc.name, got.escapedPath, tc.wantPath)
			}
			if !strings.Contains(got.escapedPath, "coll%2Fwith") {
				t.Errorf("%s escaped path %q does not contain percent-encoded slash coll%%2Fwith", tc.name, got.escapedPath)
			}
		})
	}
}

// TestGoalStepIndexEscaped proves the numeric step-index segment is rendered
// via strconv.Itoa + url.PathEscape. A number has no reserved characters, so
// escaping is a no-op and the produced path is identical to the previous
// "%d" form — this pins that the conversion stays consistent.
func TestGoalStepIndexEscaped(t *testing.T) {
	const goalID = "goal/x"
	esc := url.PathEscape(goalID)

	var got capturedRequest
	server := newCapturingServer(t, &got)
	defer server.Close()

	client := createTestClient(t, server)
	if _, err := client.GoalStepStart(goalID, 3); err != nil {
		t.Fatalf("GoalStepStart failed: %v", err)
	}

	wantPath := "/api/chat/goals/" + esc + "/steps/3/start"
	if got.escapedPath != wantPath {
		t.Errorf("GoalStepStart escaped path = %q, want %q", got.escapedPath, wantPath)
	}
}
