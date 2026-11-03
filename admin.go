package ekodb

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// AdminClientConfig configures API-key administration using JSON and no
// automatic transient retries. A definite 401 permits one token refresh.
type AdminClientConfig struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
}

// AdminClient provides API-key administration separately from data operations.
type AdminClient struct {
	client              *Client
	versionMu           sync.Mutex
	versionKnown        bool
	versionError        error
	permissionMu        sync.Mutex
	permissionKnown     bool
	keyPermissionRoutes bool
}

// MinServerVersionForPermissionEdits documents the key-addressed permission API release.
// A successful route probe is authoritative for prerelease and pre-cap builds.
const MinServerVersionForPermissionEdits = "0.80.0"

// MinLegacyServerVersionForPermissionEdits rejects legacy releases that erase field restrictions.
const MinLegacyServerVersionForPermissionEdits = "0.79.2"

func (a *AdminClient) requirePermissionEdits() error {
	a.versionMu.Lock()
	defer a.versionMu.Unlock()
	if a.versionKnown {
		return a.versionError
	}
	requirement := fmt.Errorf("permission updates need ekoDB 0.79.2 or later because earlier servers remove a grant's field restrictions when it is updated")
	data, err := a.client.makeRequest(http.MethodGet, "/api/health", nil)
	if err != nil {
		return err
	}
	var health map[string]json.RawMessage
	if json.Unmarshal(data, &health) != nil {
		return invalidAdminResponse()
	}
	var version string
	if json.Unmarshal(health["version"], &version) != nil {
		return requirement
	}
	match := regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`).FindStringSubmatch(version)
	if match == nil {
		return requirement
	}
	minimum := make([]uint64, 3)
	for i, part := range strings.Split(MinLegacyServerVersionForPermissionEdits, ".") {
		n, err := strconv.ParseUint(part, 10, 64)
		if err != nil || i >= len(minimum) {
			return requirement
		}
		minimum[i] = n
	}
	parts := make([]uint64, 3)
	for i := range parts {
		n, err := strconv.ParseUint(match[i+1], 10, 64)
		if err != nil {
			return requirement
		}
		parts[i] = n
	}
	a.versionKnown, a.versionError = true, requirement
	for i := range parts {
		if parts[i] > minimum[i] {
			a.versionError = nil
			return nil
		}
		if parts[i] < minimum[i] {
			return a.versionError
		}
	}
	a.versionError = nil
	return nil
}

// NewAdminClient authenticates an admin facade. Non-admin keys may use GetPermissions.
func NewAdminClient(baseURL, apiKey string) (*AdminClient, error) {
	return NewAdminClientWithConfig(AdminClientConfig{BaseURL: baseURL, APIKey: apiKey})
}

// NewAdminClientWithConfig authenticates with the same token cache and timeout
// behavior as Client. Errors never retain response bodies or secret-bearing URLs.
func NewAdminClientWithConfig(config AdminClientConfig) (*AdminClient, error) {
	client := newUninitializedClient(ClientConfig{BaseURL: strings.TrimRight(config.BaseURL, "/"), APIKey: config.APIKey, Timeout: config.Timeout, Format: JSON})
	client.redactErrors = true
	client.httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	if err := client.refreshToken(); err != nil {
		return nil, err
	}
	return &AdminClient{client: client}, nil
}

// APIKeyAction is a registration grant. Registration accepts action arrays.
type APIKeyAction string

const (
	APIKeyRead  APIKeyAction = "read"
	APIKeyWrite APIKeyAction = "write"
)

// FunctionAction is the one right a function grant carries.
type FunctionAction string

// FunctionCall is the right to call the stored functions a grant covers.
const FunctionCall FunctionAction = "call"

// MaxFunctionPatternBytes is the longest function grant pattern the server accepts.
const MaxFunctionPatternBytes = 256

// CollectionPermissions preserves field restrictions, including empty lists.
// A nil slice represents an absent/null restriction. Empty slices remain
// distinct from nil, preserving the server's field-permission semantics.
type CollectionPermissions struct {
	Read           bool                       `json:"read"`
	Write          bool                       `json:"write"`
	SelectFields   []string                   `json:"select_fields"`
	ExcludeFields  []string                   `json:"exclude_fields"`
	WritableFields []string                   `json:"writable_fields"`
	Metadata       map[string]json.RawMessage `json:"-"`
}

// UnmarshalJSON accepts legacy action arrays and modern permission objects.
func (p *CollectionPermissions) UnmarshalJSON(data []byte) error {
	var actions []APIKeyAction
	if len(data) > 0 && data[0] == '[' {
		if err := json.Unmarshal(data, &actions); err != nil {
			return invalidAdminResponse()
		}
		*p = CollectionPermissions{}
		for _, action := range actions {
			switch action {
			case APIKeyRead:
				p.Read = true
			case APIKeyWrite:
				p.Write = true
			default:
				return invalidAdminResponse()
			}
		}
		return nil
	}
	type plain CollectionPermissions
	var value struct {
		*plain
		Read  *bool `json:"read"`
		Write *bool `json:"write"`
	}
	value.plain = new(plain)
	if err := json.Unmarshal(data, &value); err != nil || value.Read == nil || value.Write == nil {
		return invalidAdminResponse()
	}
	value.plain.Read, value.plain.Write = *value.Read, *value.Write
	*p = CollectionPermissions(*value.plain)
	var extra map[string]json.RawMessage
	if json.Unmarshal(data, &extra) != nil {
		return invalidAdminResponse()
	}
	for _, field := range []string{"read", "write", "select_fields", "exclude_fields", "writable_fields"} {
		delete(extra, field)
	}
	p.Metadata = extra
	return nil
}

// KVPermissions is a server-returned key-prefix grant.
type KVPermissions struct {
	Metadata map[string]json.RawMessage `json:"-"`
	Prefix   string                     `json:"prefix,omitempty"`
	Read     bool                       `json:"read"`
	Write    bool                       `json:"write"`
}

// FunctionPermissions is a server-returned stored-function call grant. Pattern is a
// function label, a label prefix ending in one "*", or "*". Holding the grant is the
// right to call.
type FunctionPermissions struct {
	Metadata map[string]json.RawMessage `json:"-"`
	Pattern  string                     `json:"pattern"`
}

// APIKeyPermissions describes the calling token's grants.
type APIKeyPermissions struct {
	Metadata              map[string]json.RawMessage       `json:"-"`
	IsAdmin               bool                             `json:"is_admin"`
	CollectionPermissions map[string]CollectionPermissions `json:"collection_permissions"`
	KVPermissions         []KVPermissions                  `json:"kv_permissions,omitempty"`
	// FunctionPermissions is empty when the key holds no function grant, and when
	// the server predates function grants.
	FunctionPermissions []FunctionPermissions `json:"function_permissions,omitempty"`
}

// APIKey contains a secret. Never log this value.
type APIKey struct {
	ID                    string                           `json:"id,omitempty"`
	Metadata              map[string]json.RawMessage       `json:"-"`
	Key                   string                           `json:"key"`
	Label                 *string                          `json:"label,omitempty"`
	Description           *string                          `json:"description,omitempty"`
	IsAdmin               bool                             `json:"is_admin"`
	CollectionPermissions map[string]CollectionPermissions `json:"collection_permissions"`
	KVPermissions         []KVPermissions                  `json:"kv_permissions,omitempty"`
	// FunctionPermissions is nil when the response carries none. Registration echoes
	// the request map and key listings return grants; both read as grants sorted by
	// pattern, and a pattern registered with no rights grants nothing.
	FunctionPermissions []FunctionPermissions `json:"function_permissions,omitempty"`
	CreatedAt           *int64                `json:"created_at,omitempty"`
	ExpiresAt           *int64                `json:"expires_at,omitempty"`
	RippleWarning       *string               `json:"ripple_warning,omitempty"`
}

// CreateAPIKeyRequest defaults to a non-admin key with no collection grants.
type CreateAPIKeyRequest struct {
	Label                 string                    `json:"label,omitempty"`
	Description           string                    `json:"description,omitempty"`
	IsAdmin               bool                      `json:"is_admin"`
	CollectionPermissions map[string][]APIKeyAction `json:"collection_permissions"`
	// FunctionPermissions grants calls to stored functions: a function label, a label
	// prefix ending in one "*", or "*", each with []FunctionAction{FunctionCall}. A
	// pattern given an empty slice grants nothing. Omitted from the request when empty.
	FunctionPermissions map[string][]FunctionAction `json:"function_permissions,omitempty"`
}

// LegacyUpdateAPIKeyCollectionPermissionsRequest replaces one collection grant.
type LegacyUpdateAPIKeyCollectionPermissionsRequest struct {
	APIKey     string `json:"api_key"`
	Collection string `json:"collection"`
	Read       bool   `json:"read"`
	Write      bool   `json:"write"`
	// nil pointer keeps, pointer to nil slice clears, pointer to a slice replaces.
	SelectFields   *[]string `json:"select_fields,omitempty"`
	ExcludeFields  *[]string `json:"exclude_fields,omitempty"`
	WritableFields *[]string `json:"writable_fields,omitempty"`
}

// UnmarshalJSON preserves explicit null rather than collapsing it to omission.
func (r *LegacyUpdateAPIKeyCollectionPermissionsRequest) UnmarshalJSON(data []byte) error {
	type plain LegacyUpdateAPIKeyCollectionPermissionsRequest
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for name, target := range map[string]**[]string{"select_fields": &value.SelectFields, "exclude_fields": &value.ExcludeFields, "writable_fields": &value.WritableFields} {
		if raw, present := fields[name]; present && strings.TrimSpace(string(raw)) == "null" {
			var clear []string
			*target = &clear
		}
	}
	*r = LegacyUpdateAPIKeyCollectionPermissionsRequest(value)
	return nil
}

func invalidAdminResponse() error { return fmt.Errorf("invalid admin response") }

func validateAdminPermissions(data []byte) error {
	var value struct {
		IsAdmin               *bool                            `json:"is_admin"`
		CollectionPermissions map[string]CollectionPermissions `json:"collection_permissions"`
	}
	if err := json.Unmarshal(data, &value); err != nil || value.IsAdmin == nil || value.CollectionPermissions == nil {
		return invalidAdminResponse()
	}
	return nil
}

func decodeAdminKey(data []byte) (*APIKey, error) {
	if err := validateAdminPermissions(data); err != nil {
		return nil, err
	}
	var key APIKey
	if err := json.Unmarshal(data, &key); err != nil || key.Key == "" {
		return nil, invalidAdminResponse()
	}
	return &key, nil
}

// CreateAPIKey registers a key once. A timeout or transport error may mean it
// was created; inspect uniquely labeled keys before deciding to retry manually.
func (a *AdminClient) CreateAPIKey(request CreateAPIKeyRequest) (*APIKey, error) {
	if request.CollectionPermissions == nil {
		request.CollectionPermissions = map[string][]APIKeyAction{}
	}
	for _, actions := range request.CollectionPermissions {
		for _, action := range actions {
			if action != APIKeyRead && action != APIKeyWrite {
				return nil, fmt.Errorf("registration requires read/write action arrays")
			}
		}
		if actions == nil {
			return nil, fmt.Errorf("registration requires read/write action arrays")
		}
	}
	for pattern, rights := range request.FunctionPermissions {
		if err := validateFunctionPattern(pattern); err != nil {
			return nil, err
		}
		if rights == nil {
			return nil, fmt.Errorf("registration requires call action arrays")
		}
		for _, right := range rights {
			if right != FunctionCall {
				return nil, fmt.Errorf("registration requires call action arrays")
			}
		}
	}
	body := struct {
		CreateAPIKeyRequest
		APIKey string `json:"api_key"`
	}{request, a.client.apiKey}
	data, err := a.client.makeRequest(http.MethodPost, "/api/auth/register", body)
	if err != nil {
		return nil, err
	}
	return decodeAdminKey(data)
}

// ListAPIKeys returns secret-bearing key records. Store securely; never log.
func (a *AdminClient) ListAPIKeys() ([]APIKey, error) {
	data, err := a.client.makeRequest(http.MethodGet, "/api/auth/keys", nil)
	if err != nil {
		return nil, err
	}
	var records []json.RawMessage
	if err := json.Unmarshal(data, &records); err != nil || records == nil {
		return nil, invalidAdminResponse()
	}
	keys := make([]APIKey, 0, len(records))
	for _, record := range records {
		key, err := decodeAdminKey(record)
		if err != nil {
			return nil, err
		}
		keys = append(keys, *key)
	}
	return keys, nil
}

// RevokeAPIKey prefers the server-generated ID; legacy secret values remain accepted.
// It accepts 200 or 204 and sends no request body.
func (a *AdminClient) RevokeAPIKey(apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("an API key is required")
	}
	_, err := a.client.makeRequest(http.MethodDelete, "/api/auth/keys/"+url.PathEscape(apiKey), nil)
	return err
}

// GetPermissions reads the calling key's permissions across both route generations.
func (a *AdminClient) GetPermissions() (*APIKeyPermissions, error) {
	return a.GetAPIKeyPermissions("me")
}

// LegacyUpdateAPIKeyCollectionPermissions uses the secret-valued 0.79.2 request.
// It is rejected on a server serving key-addressed permission routes.
func (a *AdminClient) LegacyUpdateAPIKeyCollectionPermissions(request LegacyUpdateAPIKeyCollectionPermissionsRequest) (*APIKey, error) {
	if strings.TrimSpace(request.APIKey) == "" || strings.TrimSpace(request.Collection) == "" {
		return nil, fmt.Errorf("an API key and collection are required")
	}
	for _, edit := range []*[]string{request.SelectFields, request.ExcludeFields, request.WritableFields} {
		if edit != nil {
			for _, field := range *edit {
				if strings.TrimSpace(field) == "" {
					return nil, fmt.Errorf("invalid collection permission update")
				}
			}
		}
	}
	modern, _, err := a.permissionRoutes()
	if err != nil {
		return nil, err
	}
	if modern {
		return nil, fmt.Errorf("legacy permission updates are unavailable on key-addressed servers")
	}
	if err := a.requirePermissionEdits(); err != nil {
		return nil, err
	}
	data, err := a.client.makeRequest(http.MethodPut, "/api/auth/permissions", request)
	if err != nil {
		return nil, err
	}
	return decodeAdminKey(data)
}

func extraAdminFields(data []byte, known ...string) (map[string]json.RawMessage, error) {
	var extra map[string]json.RawMessage
	if json.Unmarshal(data, &extra) != nil {
		return nil, invalidAdminResponse()
	}
	for _, field := range known {
		delete(extra, field)
	}
	return extra, nil
}
func (k *APIKey) UnmarshalJSON(data []byte) error {
	type plain APIKey
	// The outer field shadows the embedded one so the grants can arrive as a list or
	// as the registration map.
	var value struct {
		plain
		FunctionPermissions json.RawMessage `json:"function_permissions"`
	}
	if json.Unmarshal(data, &value) != nil {
		return invalidAdminResponse()
	}
	functions, err := decodeFunctionGrants(value.FunctionPermissions)
	if err != nil {
		return err
	}
	extra, err := extraAdminFields(data, "id", "key", "label", "description", "is_admin", "collection_permissions", "kv_permissions", "function_permissions", "created_at", "expires_at", "ripple_warning")
	if err != nil {
		return err
	}
	value.Metadata, value.plain.FunctionPermissions = extra, functions
	*k = APIKey(value.plain)
	return nil
}

// decodeFunctionGrants reads the three shapes a key's function grants arrive in:
// absent or null (nil), a list of grants, or the registration map pattern -> ["call"]
// (sorted by pattern; a pattern with no rights grants nothing).
func decodeFunctionGrants(raw json.RawMessage) ([]FunctionPermissions, error) {
	text := strings.TrimSpace(string(raw))
	switch {
	case text == "" || text == "null":
		return nil, nil
	case strings.HasPrefix(text, "["):
		grants := []FunctionPermissions{}
		if json.Unmarshal(raw, &grants) != nil {
			return nil, invalidAdminResponse()
		}
		return grants, nil
	case strings.HasPrefix(text, "{"):
		var registered map[string][]FunctionAction
		if json.Unmarshal(raw, &registered) != nil {
			return nil, invalidAdminResponse()
		}
		patterns := make([]string, 0, len(registered))
		for pattern, rights := range registered {
			if pattern == "" {
				return nil, invalidAdminResponse()
			}
			for _, right := range rights {
				if right != FunctionCall {
					return nil, invalidAdminResponse()
				}
			}
			if len(rights) > 0 {
				patterns = append(patterns, pattern)
			}
		}
		sort.Strings(patterns)
		grants := make([]FunctionPermissions, 0, len(patterns))
		for _, pattern := range patterns {
			grants = append(grants, FunctionPermissions{Pattern: pattern})
		}
		return grants, nil
	}
	return nil, invalidAdminResponse()
}
func (k *APIKeyPermissions) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || fields == nil {
		return invalidAdminResponse()
	}
	adminField, collectionField, kvField, functionField := "is_admin", "collection_permissions", "kv_permissions", "function_permissions"
	if _, modern := fields["admin"]; modern {
		adminField, collectionField, kvField, functionField = "admin", "collections", "kv", "functions"
	}
	var admin *bool
	var collections map[string]CollectionPermissions
	if json.Unmarshal(fields[adminField], &admin) != nil || admin == nil || json.Unmarshal(fields[collectionField], &collections) != nil || collections == nil {
		return invalidAdminResponse()
	}
	var kv []KVPermissions
	if raw, ok := fields[kvField]; ok && json.Unmarshal(raw, &kv) != nil {
		return invalidAdminResponse()
	}
	if adminField == "admin" && kv == nil {
		return invalidAdminResponse()
	}
	if adminField == "admin" {
		for _, grant := range kv {
			if grant.Prefix == "" {
				return invalidAdminResponse()
			}
		}
	}
	functions, err := decodeFunctionGrants(fields[functionField])
	if err != nil {
		return err
	}
	extra, err := extraAdminFields(data, adminField, collectionField, kvField, functionField)
	if err != nil {
		return err
	}
	*k = APIKeyPermissions{IsAdmin: *admin, CollectionPermissions: collections, KVPermissions: kv, FunctionPermissions: functions, Metadata: extra}
	return nil
}
func (p CollectionPermissions) MarshalJSON() ([]byte, error) {
	type plain CollectionPermissions
	base, err := json.Marshal(plain(p))
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(base, &fields); err != nil {
		return nil, err
	}
	for name, value := range p.Metadata {
		if _, known := fields[name]; !known {
			fields[name] = value
		}
	}
	return json.Marshal(fields)
}

func adminJSONWithMetadata(value any, metadata map[string]json.RawMessage) ([]byte, error) {
	base, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(base, &fields); err != nil {
		return nil, err
	}
	for name, extra := range metadata {
		if _, known := fields[name]; !known {
			fields[name] = extra
		}
	}
	return json.Marshal(fields)
}
func (k APIKey) MarshalJSON() ([]byte, error) {
	type plain APIKey
	return adminJSONWithMetadata(plain(k), k.Metadata)
}
func (k APIKeyPermissions) MarshalJSON() ([]byte, error) {
	type plain APIKeyPermissions
	return adminJSONWithMetadata(plain(k), k.Metadata)
}

func (k *KVPermissions) UnmarshalJSON(data []byte) error {
	type plain KVPermissions
	var value struct {
		*plain
		Read  *bool `json:"read"`
		Write *bool `json:"write"`
	}
	value.plain = new(plain)
	if json.Unmarshal(data, &value) != nil || value.Read == nil || value.Write == nil {
		return invalidAdminResponse()
	}
	value.plain.Read, value.plain.Write = *value.Read, *value.Write
	extra, err := extraAdminFields(data, "prefix", "read", "write")
	if err != nil {
		return err
	}
	value.Metadata = extra
	*k = KVPermissions(*value.plain)
	return nil
}
func (k KVPermissions) MarshalJSON() ([]byte, error) {
	type plain KVPermissions
	return adminJSONWithMetadata(plain(k), k.Metadata)
}

func (f *FunctionPermissions) UnmarshalJSON(data []byte) error {
	type plain FunctionPermissions
	var value plain
	if json.Unmarshal(data, &value) != nil || value.Pattern == "" {
		return invalidAdminResponse()
	}
	extra, err := extraAdminFields(data, "pattern")
	if err != nil {
		return err
	}
	value.Metadata = extra
	*f = FunctionPermissions(value)
	return nil
}
func (f FunctionPermissions) MarshalJSON() ([]byte, error) {
	type plain FunctionPermissions
	return adminJSONWithMetadata(plain(f), f.Metadata)
}
