package ekodb

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
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
	client       *Client
	versionOnce  sync.Once
	versionError error
}

// MinServerVersionForPermissionEdits rejects releases that erase field restrictions.
const MinServerVersionForPermissionEdits = "0.79.2"

func (a *AdminClient) requirePermissionEdits() error {
	a.versionOnce.Do(func() {
		a.versionError = fmt.Errorf("permission updates need ekoDB 0.79.2 or later because earlier servers remove a grant's field restrictions when it is updated")
		data, err := a.client.makeRequest(http.MethodGet, "/api/health", nil)
		if err != nil {
			return
		}
		var health struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(data, &health) != nil {
			return
		}
		match := regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`).FindStringSubmatch(health.Version)
		if match == nil {
			return
		}
		minimum := make([]uint64, 3)
		for i, part := range strings.Split(MinServerVersionForPermissionEdits, ".") {
			n, err := strconv.ParseUint(part, 10, 64)
			if err != nil || i >= len(minimum) {
				return
			}
			minimum[i] = n
		}
		parts := make([]uint64, 3)
		for i := range parts {
			n, err := strconv.ParseUint(match[i+1], 10, 64)
			if err != nil {
				return
			}
			parts[i] = n
		}
		for i := range parts {
			if parts[i] > minimum[i] {
				a.versionError = nil
				return
			}
			if parts[i] < minimum[i] {
				return
			}
		}
		a.versionError = nil
	})
	return a.versionError
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
	Prefix   string                     `json:"prefix"`
	Read     bool                       `json:"read"`
	Write    bool                       `json:"write"`
}

// APIKeyPermissions describes the calling token's grants.
type APIKeyPermissions struct {
	Metadata              map[string]json.RawMessage       `json:"-"`
	IsAdmin               bool                             `json:"is_admin"`
	CollectionPermissions map[string]CollectionPermissions `json:"collection_permissions"`
	KVPermissions         []KVPermissions                  `json:"kv_permissions,omitempty"`
}

// APIKey contains a secret. Never log this value.
type APIKey struct {
	Metadata              map[string]json.RawMessage       `json:"-"`
	Key                   string                           `json:"key"`
	Label                 *string                          `json:"label,omitempty"`
	Description           *string                          `json:"description,omitempty"`
	IsAdmin               bool                             `json:"is_admin"`
	CollectionPermissions map[string]CollectionPermissions `json:"collection_permissions"`
	KVPermissions         []KVPermissions                  `json:"kv_permissions,omitempty"`
	CreatedAt             *int64                           `json:"created_at,omitempty"`
	ExpiresAt             *int64                           `json:"expires_at,omitempty"`
	RippleWarning         *string                          `json:"ripple_warning,omitempty"`
}

// CreateAPIKeyRequest defaults to a non-admin key with no collection grants.
type CreateAPIKeyRequest struct {
	Label                 string                    `json:"label,omitempty"`
	Description           string                    `json:"description,omitempty"`
	IsAdmin               bool                      `json:"is_admin"`
	CollectionPermissions map[string][]APIKeyAction `json:"collection_permissions"`
}

// UpdateAPIKeyCollectionPermissionsRequest replaces one collection grant.
type UpdateAPIKeyCollectionPermissionsRequest struct {
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
func (r *UpdateAPIKeyCollectionPermissionsRequest) UnmarshalJSON(data []byte) error {
	type plain UpdateAPIKeyCollectionPermissionsRequest
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
	*r = UpdateAPIKeyCollectionPermissionsRequest(value)
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

// RevokeAPIKey revokes by secret value, not a separate ID. Accepts 200 or 204.
func (a *AdminClient) RevokeAPIKey(apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("an API key is required")
	}
	_, err := a.client.makeRequest(http.MethodDelete, "/api/auth/keys/"+url.PathEscape(apiKey), nil)
	return err
}

// GetPermissions reads permissions for the current token, including field ACLs.
func (a *AdminClient) GetPermissions() (*APIKeyPermissions, error) {
	data, err := a.client.makeRequest(http.MethodGet, "/api/auth/permissions", nil)
	if err != nil {
		return nil, err
	}
	if err := validateAdminPermissions(data); err != nil {
		return nil, err
	}
	var result APIKeyPermissions
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, invalidAdminResponse()
	}
	return &result, nil
}

// UpdateAPIKeyCollectionPermissions replaces one target key's collection grant.
// Returns the server's key record; verify effective grants separately.
func (a *AdminClient) UpdateAPIKeyCollectionPermissions(request UpdateAPIKeyCollectionPermissionsRequest) (*APIKey, error) {
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
	var value plain
	if json.Unmarshal(data, &value) != nil {
		return invalidAdminResponse()
	}
	extra, err := extraAdminFields(data, "key", "label", "description", "is_admin", "collection_permissions", "kv_permissions", "created_at", "expires_at", "ripple_warning")
	if err != nil {
		return err
	}
	value.Metadata = extra
	*k = APIKey(value)
	return nil
}
func (k *APIKeyPermissions) UnmarshalJSON(data []byte) error {
	type plain APIKeyPermissions
	var value plain
	if json.Unmarshal(data, &value) != nil {
		return invalidAdminResponse()
	}
	extra, err := extraAdminFields(data, "is_admin", "collection_permissions", "kv_permissions")
	if err != nil {
		return err
	}
	value.Metadata = extra
	*k = APIKeyPermissions(value)
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
	var value plain
	if json.Unmarshal(data, &value) != nil {
		return invalidAdminResponse()
	}
	extra, err := extraAdminFields(data, "prefix", "read", "write")
	if err != nil {
		return err
	}
	value.Metadata = extra
	*k = KVPermissions(value)
	return nil
}
func (k KVPermissions) MarshalJSON() ([]byte, error) {
	type plain KVPermissions
	return adminJSONWithMetadata(plain(k), k.Metadata)
}
