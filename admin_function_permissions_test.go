package ekodb

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type functionGrantFixture struct {
	KeyID                    string          `json:"key_id"`
	RegistrationRequest      json.RawMessage `json:"registration_request"`
	RegistrationResponse     json.RawMessage `json:"registration_response"`
	RegistrationWithoutGrant json.RawMessage `json:"registration_response_without_function_permissions"`
	ExpectedPatterns         []string        `json:"expected_registered_patterns"`
	KeyListRecord            json.RawMessage `json:"key_list_record"`
	Permissions              json.RawMessage `json:"permissions"`
	PermissionsWithout       json.RawMessage `json:"permissions_without_functions"`
	FunctionGrants           json.RawMessage `json:"function_grants"`
	Changes                  []struct {
		Name, Method, Pattern, Path string
		Body, Response              json.RawMessage
		Status                      int
	} `json:"function_grant_changes"`
	Rejected []struct {
		Name   string
		Value  *string
		Length int
	} `json:"rejected_patterns"`
	AcceptedLength int `json:"accepted_pattern_length"`
	Refusals       []struct {
		Name, Method, Pattern string
		Status                int
	} `json:"refusals"`
	CallRefusal struct {
		Status int
		Body   json.RawMessage
	} `json:"function_call_refusal"`
	Definition   json.RawMessage `json:"function_definition"`
	Unrecorded   json.RawMessage `json:"function_definition_unrecorded"`
	SaveRequest  json.RawMessage `json:"function_save_request"`
	ScheduleItem json.RawMessage `json:"schedule_record"`
}

func loadFunctionGrantFixture(t *testing.T) functionGrantFixture {
	t.Helper()
	data, err := os.ReadFile("test-fixtures/admin-function-permissions-contract.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture functionGrantFixture
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

type recordedRequest struct {
	Method, Path, Body string
}

// functionGrantClient answers key-addressed permission routes and records every other request.
func functionGrantClient(t *testing.T, probe string, handler http.HandlerFunc) (*AdminClient, *[]recordedRequest) {
	t.Helper()
	var mu sync.Mutex
	recorded := &[]recordedRequest{}
	client := keyPermissionTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/keys/me/permissions" {
			if probe == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = io.WriteString(w, probe)
			return
		}
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		*recorded = append(*recorded, recordedRequest{r.Method, r.URL.EscapedPath(), string(body)})
		mu.Unlock()
		handler(w, r)
	})
	return client, recorded
}

const emptyModernPermissions = `{"admin":true,"collections":{},"kv":[],"functions":[]}`

func patternsOf(grants []FunctionPermissions) []string {
	patterns := []string{}
	for _, grant := range grants {
		patterns = append(patterns, grant.Pattern)
	}
	return patterns
}

func TestFunctionGrantRegistrationSendsGrantsAndReadsTheEchoedMap(t *testing.T) {
	fixture := loadFunctionGrantFixture(t)
	client, recorded := functionGrantClient(t, emptyModernPermissions, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(fixture.RegistrationResponse)
	})
	key, err := client.CreateAPIKey(CreateAPIKeyRequest{
		Label: "report-runner",
		FunctionPermissions: map[string][]FunctionAction{
			"report_*":      {FunctionCall},
			"nightly_close": {FunctionCall},
			"audit_only":    {},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var sent, want map[string]any
	if json.Unmarshal([]byte((*recorded)[0].Body), &sent) != nil || json.Unmarshal(fixture.RegistrationRequest, &want) != nil {
		t.Fatal("invalid registration body")
	}
	// The recorded request carries the fixture's admin key; the transport sends this test's.
	sent["api_key"], want["api_key"] = nil, nil
	if !reflect.DeepEqual(sent, want) {
		t.Fatalf("registration body differs: %v", sent)
	}
	if !reflect.DeepEqual(patternsOf(key.FunctionPermissions), fixture.ExpectedPatterns) {
		t.Fatalf("empty rights must grant nothing: %v", key.FunctionPermissions)
	}
}

func TestFunctionGrantRegistrationWithoutGrantsKeepsTheOldBodyAndReadsNil(t *testing.T) {
	fixture := loadFunctionGrantFixture(t)
	client, recorded := functionGrantClient(t, emptyModernPermissions, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(fixture.RegistrationWithoutGrant)
	})
	key, err := client.CreateAPIKey(CreateAPIKeyRequest{Label: "report-runner"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains((*recorded)[0].Body, "function_permissions") {
		t.Fatal("an empty grant map must be omitted")
	}
	if key.FunctionPermissions != nil {
		t.Fatalf("a null response reads as nil: %v", key.FunctionPermissions)
	}
}

func TestFunctionGrantRegistrationRefusesInvalidInputBeforeAnyRequest(t *testing.T) {
	client, recorded := functionGrantClient(t, emptyModernPermissions, func(w http.ResponseWriter, r *http.Request) {
		t.Error("request reached transport")
	})
	for _, pattern := range []string{"", "re*port", "report_**", "*report"} {
		if _, err := client.CreateAPIKey(CreateAPIKeyRequest{FunctionPermissions: map[string][]FunctionAction{pattern: {FunctionCall}}}); err == nil {
			t.Errorf("pattern %q was accepted", pattern)
		}
	}
	for _, rights := range [][]FunctionAction{nil, {"run"}} {
		if _, err := client.CreateAPIKey(CreateAPIKeyRequest{FunctionPermissions: map[string][]FunctionAction{"report_*": rights}}); err == nil {
			t.Errorf("rights %v were accepted", rights)
		}
	}
	if len(*recorded) != 0 {
		t.Fatal("invalid registration reached the network")
	}
}

func TestFunctionGrantsOnKeyListingsAndPermissionReads(t *testing.T) {
	fixture := loadFunctionGrantFixture(t)
	client, _ := functionGrantClient(t, string(fixture.Permissions), func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "["+string(fixture.KeyListRecord)+"]")
	})
	keys, err := client.ListAPIKeys()
	if err != nil {
		t.Fatal(err)
	}
	var want []FunctionPermissions
	if json.Unmarshal(fixture.FunctionGrants, &want) != nil {
		t.Fatal("invalid fixture")
	}
	if !reflect.DeepEqual(keys[0].FunctionPermissions, want) {
		t.Fatalf("listing grants: %v", keys[0].FunctionPermissions)
	}
	permissions, err := client.GetPermissions()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(permissions.FunctionPermissions, want) {
		t.Fatalf("permission grants: %v", permissions.FunctionPermissions)
	}
	if _, leaked := permissions.Metadata["functions"]; leaked {
		t.Fatal("the grants must not also appear as unknown metadata")
	}
	round, err := json.Marshal(keys[0])
	if err != nil || !strings.Contains(string(round), `"function_permissions":[{"pattern":"nightly_close"},{"pattern":"report_*"}]`) {
		t.Fatalf("grants did not round-trip: %s %v", round, err)
	}

	older, _ := functionGrantClient(t, string(fixture.PermissionsWithout), nil)
	permissions, err = older.GetPermissions()
	if err != nil || len(permissions.FunctionPermissions) != 0 {
		t.Fatalf("a server that predates grants reads none: %v %v", permissions, err)
	}
}

func TestFunctionGrantListAndGetUseTheRecordedRoutes(t *testing.T) {
	fixture := loadFunctionGrantFixture(t)
	client, recorded := functionGrantClient(t, emptyModernPermissions, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/api/auth/keys/" + fixture.KeyID + "/permissions/functions":
			_, _ = w.Write(fixture.FunctionGrants)
		case "/api/auth/keys/" + fixture.KeyID + "/permissions/functions/report_%2A":
			_, _ = io.WriteString(w, `{"pattern":"report_*"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"synthetic-secret"}`)
		}
	})
	grants, err := client.ListAPIKeyFunctionPermissions(fixture.KeyID)
	if err != nil || !reflect.DeepEqual(patternsOf(grants), []string{"nightly_close", "report_*"}) {
		t.Fatalf("list: %v %v", grants, err)
	}
	grant, err := client.GetAPIKeyFunctionPermissions(fixture.KeyID, "report_*")
	if err != nil || grant.Pattern != "report_*" {
		t.Fatalf("get: %v %v", grant, err)
	}
	_, err = client.GetAPIKeyFunctionPermissions(fixture.KeyID, "nope")
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusNotFound || strings.Contains(err.Error(), "synthetic-secret") {
		t.Fatalf("a missing grant is a redacted 404: %v", err)
	}
	if len(*recorded) != 3 {
		t.Fatalf("unexpected requests: %v", *recorded)
	}
}

func TestFunctionGrantChangesFollowTheRecordedRoutesBodiesAndReplies(t *testing.T) {
	fixture := loadFunctionGrantFixture(t)
	for _, test := range fixture.Changes {
		t.Run(test.Name, func(t *testing.T) {
			client, recorded := functionGrantClient(t, emptyModernPermissions, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.Status)
				_, _ = w.Write(test.Response)
			})
			var change *FunctionPermissionChange
			var err error
			if test.Method == http.MethodPut {
				change, err = client.PutAPIKeyFunctionPermissions(fixture.KeyID, test.Pattern)
			} else {
				change, err = client.DeleteAPIKeyFunctionPermissions(fixture.KeyID, test.Pattern)
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(*recorded) != 1 || (*recorded)[0].Method != test.Method || (*recorded)[0].Path != test.Path {
				t.Fatalf("unexpected route: %v", *recorded)
			}
			var gotBody, wantBody any
			if (*recorded)[0].Body != "" {
				_ = json.Unmarshal([]byte((*recorded)[0].Body), &gotBody)
			}
			_ = json.Unmarshal(test.Body, &wantBody)
			if !reflect.DeepEqual(gotBody, wantBody) {
				t.Fatalf("body %v, want %v", gotBody, wantBody)
			}
			encoded, err := json.Marshal(change)
			if err != nil {
				t.Fatal(err)
			}
			var got, want any
			if json.Unmarshal(encoded, &got) != nil || json.Unmarshal(test.Response, &want) != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("reply %s, want %s", encoded, test.Response)
			}
		})
	}
}

func TestFunctionGrantRefusalsSurfaceTheStatusWithoutTheBody(t *testing.T) {
	fixture := loadFunctionGrantFixture(t)
	for _, test := range fixture.Refusals {
		t.Run(test.Name, func(t *testing.T) {
			client, _ := functionGrantClient(t, emptyModernPermissions, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.Status)
				_, _ = io.WriteString(w, `{"error":"synthetic-secret"}`)
			})
			var err error
			if test.Method == http.MethodPut {
				_, err = client.PutAPIKeyFunctionPermissions(fixture.KeyID, test.Pattern)
			} else {
				_, err = client.DeleteAPIKeyFunctionPermissions(fixture.KeyID, test.Pattern)
			}
			var httpErr *HTTPError
			if !errors.As(err, &httpErr) || httpErr.StatusCode != test.Status || strings.Contains(err.Error(), "synthetic-secret") {
				t.Fatalf("expected a redacted %d: %v", test.Status, err)
			}
		})
	}
}

func TestFunctionGrantRejectedPatternsFailBeforeAnyRequest(t *testing.T) {
	fixture := loadFunctionGrantFixture(t)
	client, recorded := functionGrantClient(t, emptyModernPermissions, func(w http.ResponseWriter, r *http.Request) {
		t.Error("request reached transport")
	})
	for _, test := range fixture.Rejected {
		pattern := strings.Repeat("a", test.Length)
		if test.Value != nil {
			pattern = *test.Value
		}
		if _, err := client.GetAPIKeyFunctionPermissions(fixture.KeyID, pattern); err == nil {
			t.Errorf("%s: get accepted", test.Name)
		}
		if _, err := client.PutAPIKeyFunctionPermissions(fixture.KeyID, pattern); err == nil {
			t.Errorf("%s: put accepted", test.Name)
		}
		if _, err := client.DeleteAPIKeyFunctionPermissions(fixture.KeyID, pattern); err == nil {
			t.Errorf("%s: delete accepted", test.Name)
		}
	}
	if _, err := client.PutAPIKeyFunctionPermissions("synthetic-target-key-0123456789-abcdef", "report_*"); err == nil {
		t.Error("a key value was accepted as a target")
	}
	if len(*recorded) != 0 {
		t.Fatalf("rejected input reached the network: %v", *recorded)
	}
}

func TestFunctionGrantLongestPatternIsAcceptedAndBytesAreCounted(t *testing.T) {
	fixture := loadFunctionGrantFixture(t)
	if fixture.AcceptedLength != MaxFunctionPatternBytes {
		t.Fatal("pattern limit drift")
	}
	pattern := strings.Repeat("a", MaxFunctionPatternBytes-1) + "*"
	client, recorded := functionGrantClient(t, emptyModernPermissions, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"before":null,"after":{"pattern":"`+pattern+`"}}`)
	})
	change, err := client.PutAPIKeyFunctionPermissions(fixture.KeyID, pattern)
	if err != nil || change.After == nil || change.After.Pattern != pattern {
		t.Fatalf("longest pattern: %v %v", change, err)
	}
	if !strings.HasSuffix((*recorded)[0].Path, strings.Repeat("a", 255)+"%2A") {
		t.Fatalf("unexpected path %s", (*recorded)[0].Path)
	}
	if _, err = client.PutAPIKeyFunctionPermissions(fixture.KeyID, strings.Repeat("é", MaxFunctionPatternBytes/2+1)); err == nil {
		t.Fatal("limit is in bytes, not characters")
	}
}

func TestFunctionGrantRoutesNeedKeyAddressedServers(t *testing.T) {
	client, _ := functionGrantClient(t, "", func(w http.ResponseWriter, r *http.Request) {
		t.Error("request reached transport")
	})
	if _, err := client.PutAPIKeyFunctionPermissions("me", "report_*"); err == nil {
		t.Fatal("legacy servers have no function grant routes")
	}
}

func TestFunctionGrantMalformedRepliesAreRejected(t *testing.T) {
	for _, reply := range []string{`{"before":null}`, `{"before":null,"after":{"pattern":""}}`, `{"before":null,"after":{}}`, `[]`} {
		client, _ := functionGrantClient(t, emptyModernPermissions, func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, reply)
		})
		if _, err := client.PutAPIKeyFunctionPermissions("me", "report_*"); err == nil {
			t.Errorf("reply %s was accepted", reply)
		}
	}
}

func functionClientAgainst(t *testing.T, status int, body []byte, record *recordedRequest) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/token" {
			_, _ = io.WriteString(w, `{"token":"test-jwt-token"}`)
			return
		}
		payload, _ := io.ReadAll(r.Body)
		if record != nil {
			*record = recordedRequest{r.Method, r.URL.EscapedPath(), string(payload)}
		}
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return createTestClient(t, server)
}

func TestFunctionDefinitionsCarryRunAsAndCreatedBy(t *testing.T) {
	fixture := loadFunctionGrantFixture(t)
	read, err := functionClientAgainst(t, 200, fixture.Definition, nil).GetUserFunction("nightly_close")
	if err != nil {
		t.Fatal(err)
	}
	if read.RunAs == nil || *read.RunAs != RunAsAuthor {
		t.Fatalf("run_as: %v", read.RunAs)
	}
	label, subject := "report-runner", "alice"
	if read.CreatedBy == nil || read.CreatedBy.KeyID != fixture.KeyID || !reflect.DeepEqual(read.CreatedBy.KeyLabel, &label) || !reflect.DeepEqual(read.CreatedBy.Subject, &subject) {
		t.Fatalf("created_by: %+v", read.CreatedBy)
	}
	unrecorded, err := functionClientAgainst(t, 200, fixture.Unrecorded, nil).GetUserFunction("legacy_close")
	if err != nil || unrecorded.RunAs != nil || unrecorded.CreatedBy != nil {
		t.Fatalf("an unrecorded function reads nil: %+v %v", unrecorded, err)
	}
	list, err := functionClientAgainst(t, 200, []byte("["+string(fixture.Definition)+"]"), nil).ListFunctions(nil)
	if err != nil || len(list) != 1 || list[0].RunAs == nil {
		t.Fatalf("list: %v %v", list, err)
	}

	var request UserFunction
	if err = json.Unmarshal(fixture.SaveRequest, &request); err != nil || request.RunAs == nil || *request.RunAs != RunAsCaller {
		t.Fatalf("save request: %+v %v", request, err)
	}
	var recorded recordedRequest
	if _, err = functionClientAgainst(t, 200, []byte(`{"status":"success","id":"f1"}`), &recorded).SaveUserFunction(request); err != nil {
		t.Fatal(err)
	}
	var sent map[string]any
	if json.Unmarshal([]byte(recorded.Body), &sent) != nil || sent["run_as"] != "caller" {
		t.Fatalf("run_as was not sent: %s", recorded.Body)
	}
	if _, present := sent["created_by"]; present {
		t.Fatal("an unset author must not be sent")
	}
	var invalid UserFunction
	if json.Unmarshal([]byte(`{"label":"f","name":"F","parameters":{},"functions":[],"run_as":"root"}`), &invalid) == nil {
		t.Fatal("only caller and author are run_as values")
	}
}

func TestEveryFunctionWriteRefusesAnUnknownRunAsBeforeSending(t *testing.T) {
	root := RunAs("root")
	empty := RunAs("")
	for _, value := range []*RunAs{&root, &empty} {
		definition := UserFunction{Label: "f", Name: "F", Parameters: map[string]ParameterDefinition{}, Functions: []FunctionStageConfig{}, RunAs: value}
		writes := map[string]func(*Client) error{
			"SaveFunction":       func(c *Client) error { _, err := c.SaveFunction(definition); return err },
			"UpdateFunction":     func(c *Client) error { return c.UpdateFunction("f", definition) },
			"SaveUserFunction":   func(c *Client) error { _, err := c.SaveUserFunction(definition); return err },
			"UpdateUserFunction": func(c *Client) error { return c.UpdateUserFunction("f", definition) },
		}
		for name, write := range writes {
			var recorded recordedRequest
			err := write(functionClientAgainst(t, 200, []byte(`{"status":"success","id":"f1"}`), &recorded))
			if err == nil || !strings.Contains(err.Error(), "run_as") {
				t.Fatalf("%s with run_as %q: expected a run_as error, got %v", name, *value, err)
			}
			if recorded.Method != "" {
				t.Fatalf("%s with run_as %q reached the server: %+v", name, *value, recorded)
			}
		}
	}
	for _, valid := range []RunAs{RunAsCaller, RunAsAuthor} {
		v := valid
		var recorded recordedRequest
		definition := UserFunction{Label: "f", Name: "F", Parameters: map[string]ParameterDefinition{}, Functions: []FunctionStageConfig{}, RunAs: &v}
		if err := functionClientAgainst(t, 200, []byte(`{"status":"success","id":"f1"}`), &recorded).UpdateUserFunction("f", definition); err != nil || recorded.Method != "PUT" {
			t.Fatalf("run_as %q must be sent: %v %+v", v, err, recorded)
		}
	}
}

func TestRefusedFunctionCallIsTheClientsNormalHTTPError(t *testing.T) {
	fixture := loadFunctionGrantFixture(t)
	_, err := functionClientAgainst(t, fixture.CallRefusal.Status, fixture.CallRefusal.Body, nil).CallFunction("nightly_close", nil)
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusForbidden {
		t.Fatalf("expected an HTTPError 403: %v", err)
	}
}

func TestSchedulesReturnTheCreatorTheServerRecorded(t *testing.T) {
	fixture := loadFunctionGrantFixture(t)
	var record map[string]any
	if json.Unmarshal(fixture.ScheduleItem, &record) != nil {
		t.Fatal("invalid fixture")
	}
	schedule, err := functionClientAgainst(t, 200, fixture.ScheduleItem, nil).GetSchedule(record["id"].(string))
	if err != nil || !reflect.DeepEqual(schedule["created_by"], record["created_by"]) {
		t.Fatalf("schedule: %v %v", schedule, err)
	}
}
