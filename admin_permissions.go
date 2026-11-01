package ekodb

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

// PermissionChange is the before/after result of a key-addressed grant edit.
// A nil Before means creation; a nil After means removal. It contains no token.
type PermissionChange[T any] struct {
	Before   *T                         `json:"before"`
	After    *T                         `json:"after"`
	Metadata map[string]json.RawMessage `json:"-"`
}

type CollectionPermissionChange = PermissionChange[CollectionPermissions]
type KVPermissionChange = PermissionChange[KVPermissions]

func (c *PermissionChange[T]) UnmarshalJSON(data []byte) error {
	type plain PermissionChange[T]
	var fields map[string]json.RawMessage
	var value plain
	if json.Unmarshal(data, &fields) != nil || fields["before"] == nil || fields["after"] == nil || json.Unmarshal(data, &value) != nil {
		return invalidAdminResponse()
	}
	delete(fields, "before")
	delete(fields, "after")
	value.Metadata = fields
	*c = PermissionChange[T](value)
	return nil
}

func (c PermissionChange[T]) MarshalJSON() ([]byte, error) {
	type plain PermissionChange[T]
	return adminJSONWithMetadata(plain(c), c.Metadata)
}

// UpdateAPIKeyCollectionPermissionsRequest targets a key ID (or "me"), never its secret.
// PUT treats omitted restrictions as unlimited. Update/PATCH preserves omissions.
type UpdateAPIKeyCollectionPermissionsRequest struct {
	KeyID          string    `json:"-"`
	Collection     string    `json:"-"`
	Read           bool      `json:"read"`
	Write          bool      `json:"write"`
	SelectFields   *[]string `json:"select_fields,omitempty"`
	ExcludeFields  *[]string `json:"exclude_fields,omitempty"`
	WritableFields *[]string `json:"writable_fields,omitempty"`
}

// PatchAPIKeyCollectionPermissionsRequest preserves omitted flags and restrictions.
// A pointer to a nil slice clears a restriction; a pointer to [] sets an empty list.
type PatchAPIKeyCollectionPermissionsRequest struct {
	KeyID          string    `json:"-"`
	Collection     string    `json:"-"`
	Read           *bool     `json:"read,omitempty"`
	Write          *bool     `json:"write,omitempty"`
	SelectFields   *[]string `json:"select_fields,omitempty"`
	ExcludeFields  *[]string `json:"exclude_fields,omitempty"`
	WritableFields *[]string `json:"writable_fields,omitempty"`
}

func preserveNullRestrictions(data []byte, selected, excluded, writable **[]string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for name, target := range map[string]**[]string{"select_fields": selected, "exclude_fields": excluded, "writable_fields": writable} {
		if raw, present := fields[name]; present && strings.TrimSpace(string(raw)) == "null" {
			var clear []string
			*target = &clear
		}
	}
	return nil
}

func (r *UpdateAPIKeyCollectionPermissionsRequest) UnmarshalJSON(data []byte) error {
	type plain UpdateAPIKeyCollectionPermissionsRequest
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if err := preserveNullRestrictions(data, &value.SelectFields, &value.ExcludeFields, &value.WritableFields); err != nil {
		return err
	}
	*r = UpdateAPIKeyCollectionPermissionsRequest(value)
	return nil
}

func (r *PatchAPIKeyCollectionPermissionsRequest) UnmarshalJSON(data []byte) error {
	type plain PatchAPIKeyCollectionPermissionsRequest
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if err := preserveNullRestrictions(data, &value.SelectFields, &value.ExcludeFields, &value.WritableFields); err != nil {
		return err
	}
	*r = PatchAPIKeyCollectionPermissionsRequest(value)
	return nil
}

// UpdateAPIKeyKVPermissionsRequest replaces one prefix grant.
type UpdateAPIKeyKVPermissionsRequest struct {
	KeyID  string `json:"-"`
	Prefix string `json:"-"`
	Read   bool   `json:"read"`
	Write  bool   `json:"write"`
}

// PatchAPIKeyKVPermissionsRequest changes only the supplied rights.
type PatchAPIKeyKVPermissionsRequest struct {
	KeyID  string `json:"-"`
	Prefix string `json:"-"`
	Read   *bool  `json:"read,omitempty"`
	Write  *bool  `json:"write,omitempty"`
}

var permissionKeyID = regexp.MustCompile(`^(?:me|[0-9a-f]{32})$`)

func validatePermissionTarget(keyID string) error {
	if !permissionKeyID.MatchString(keyID) {
		return fmt.Errorf("permission target must be a key ID or me")
	}
	return nil
}

func validatePermissionFacet(facet string) error {
	if strings.TrimSpace(facet) == "" || facet == "." || facet == ".." {
		return fmt.Errorf("a valid permission facet is required")
	}
	for _, r := range facet {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return fmt.Errorf("a valid permission facet is required")
		}
	}
	return nil
}

// encodePermissionFacet encodes a single path segment using RFC 3986's
// unreserved set. QueryEscape's plus-for-space and PathEscape's literal colon
// differ from the permission route contract.
func encodePermissionFacet(facet string) string {
	return strings.ReplaceAll(url.QueryEscape(facet), "+", "%20")
}

// permissionRoutes caches only a successful generation decision. A transient
// error or forbidden response must not turn into a downgrade to legacy routes.
// The first successful probe body can also satisfy the caller's aggregate read.
func (a *AdminClient) permissionRoutes() (bool, []byte, error) {
	a.permissionMu.Lock()
	defer a.permissionMu.Unlock()
	if a.permissionKnown {
		return a.keyPermissionRoutes, nil, nil
	}
	data, err := a.client.makeRequest(http.MethodGet, "/api/auth/keys/me/permissions", nil)
	if err == nil {
		var permissions APIKeyPermissions
		var fields map[string]json.RawMessage
		if json.Unmarshal(data, &fields) != nil || fields["admin"] == nil || fields["collections"] == nil || fields["kv"] == nil || json.Unmarshal(data, &permissions) != nil {
			return false, nil, invalidAdminResponse()
		}
		a.permissionKnown, a.keyPermissionRoutes = true, true
		return true, data, nil
	}
	var httpError *HTTPError
	if errors.As(err, &httpError) && (httpError.StatusCode == http.StatusNotFound || httpError.StatusCode == http.StatusMethodNotAllowed) {
		a.permissionKnown = true
		return false, nil, nil
	}
	return false, nil, err
}

// UsesKeyPermissionRoutes reports the detected generation, probing once if needed.
// Prefer this capability result over a server's version string.
func (a *AdminClient) UsesKeyPermissionRoutes() (bool, error) {
	modern, _, err := a.permissionRoutes()
	return modern, err
}

func (a *AdminClient) permissionPath(keyID, resource, facet string) (string, error) {
	if err := validatePermissionTarget(keyID); err != nil {
		return "", err
	}
	if facet != "" {
		if err := validatePermissionFacet(facet); err != nil {
			return "", err
		}
	}
	modern, _, err := a.permissionRoutes()
	if err != nil {
		return "", err
	}
	if !modern {
		return "", fmt.Errorf("this permission operation requires key-addressed routes; use the explicit legacy API on 0.79.2")
	}
	path := "/api/auth/keys/" + keyID + "/permissions"
	if resource != "" {
		path += "/" + resource
	}
	if facet != "" {
		path += "/" + encodePermissionFacet(facet)
	}
	return path, nil
}

// GetAPIKeyPermissions reads a key's grants. Only "me" is supported by legacy servers.
// The public response keeps IsAdmin/CollectionPermissions and includes KV grants.
func (a *AdminClient) GetAPIKeyPermissions(keyID string) (*APIKeyPermissions, error) {
	if err := validatePermissionTarget(keyID); err != nil {
		return nil, err
	}
	modern, data, err := a.permissionRoutes()
	if err != nil {
		return nil, err
	}
	path := "/api/auth/keys/" + keyID + "/permissions"
	if !modern {
		if keyID != "me" {
			return nil, fmt.Errorf("legacy permission reads support only me")
		}
		path = "/api/auth/permissions"
	}
	if data == nil || keyID != "me" {
		data, err = a.client.makeRequest(http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}
	}
	var permissions APIKeyPermissions
	if err := json.Unmarshal(data, &permissions); err != nil {
		return nil, invalidAdminResponse()
	}
	return &permissions, nil
}

func (a *AdminClient) ListAPIKeyCollectionPermissions(keyID string) (map[string]CollectionPermissions, error) {
	path, err := a.permissionPath(keyID, "collections", "")
	if err != nil {
		return nil, err
	}
	data, err := a.client.makeRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var permissions map[string]CollectionPermissions
	if json.Unmarshal(data, &permissions) != nil || permissions == nil {
		return nil, invalidAdminResponse()
	}
	return permissions, nil
}

func (a *AdminClient) GetAPIKeyCollectionPermissions(keyID, collection string) (*CollectionPermissions, error) {
	if err := validatePermissionFacet(collection); err != nil {
		return nil, err
	}
	path, err := a.permissionPath(keyID, "collections", collection)
	if err != nil {
		return nil, err
	}
	data, err := a.client.makeRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var permissions CollectionPermissions
	if json.Unmarshal(data, &permissions) != nil {
		return nil, invalidAdminResponse()
	}
	return &permissions, nil
}

func (a *AdminClient) ListAPIKeyKVPermissions(keyID string, prefix *string) ([]KVPermissions, error) {
	path, err := a.permissionPath(keyID, "kv", "")
	if err != nil {
		return nil, err
	}
	if prefix != nil {
		path += "?prefix=" + url.QueryEscape(*prefix)
	}
	data, err := a.client.makeRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var permissions []KVPermissions
	if json.Unmarshal(data, &permissions) != nil || permissions == nil {
		return nil, invalidAdminResponse()
	}
	return permissions, nil
}

func (a *AdminClient) GetAPIKeyKVPermissions(keyID, prefix string) (*KVPermissions, error) {
	if err := validatePermissionFacet(prefix); err != nil {
		return nil, err
	}
	path, err := a.permissionPath(keyID, "kv", prefix)
	if err != nil {
		return nil, err
	}
	data, err := a.client.makeRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var permissions KVPermissions
	if json.Unmarshal(data, &permissions) != nil {
		return nil, invalidAdminResponse()
	}
	return &permissions, nil
}

func decodePermissionChange[T any](data []byte) (*PermissionChange[T], error) {
	var change PermissionChange[T]
	if json.Unmarshal(data, &change) != nil {
		return nil, invalidAdminResponse()
	}
	return &change, nil
}

func validateRestrictionEdits(edits ...*[]string) error {
	for _, edit := range edits {
		if edit != nil {
			for _, field := range *edit {
				if strings.TrimSpace(field) == "" {
					return fmt.Errorf("invalid collection permission update")
				}
			}
		}
	}
	return nil
}

func (a *AdminClient) collectionPermissionEdit(method, keyID, collection string, body any) (*CollectionPermissionChange, error) {
	if err := validatePermissionFacet(collection); err != nil {
		return nil, err
	}
	path, err := a.permissionPath(keyID, "collections", collection)
	if err != nil {
		return nil, err
	}
	data, err := a.client.makeRequest(method, path, body)
	if err != nil {
		return nil, err
	}
	return decodePermissionChange[CollectionPermissions](data)
}

// PutAPIKeyCollectionPermissions replaces flags and restrictions for one collection.
func (a *AdminClient) PutAPIKeyCollectionPermissions(request UpdateAPIKeyCollectionPermissionsRequest) (*CollectionPermissionChange, error) {
	if err := validateRestrictionEdits(request.SelectFields, request.ExcludeFields, request.WritableFields); err != nil {
		return nil, err
	}
	return a.collectionPermissionEdit(http.MethodPut, request.KeyID, request.Collection, request)
}

// PatchAPIKeyCollectionPermissions preserves omitted fields and clears explicit nulls.
func (a *AdminClient) PatchAPIKeyCollectionPermissions(request PatchAPIKeyCollectionPermissionsRequest) (*CollectionPermissionChange, error) {
	if err := validateRestrictionEdits(request.SelectFields, request.ExcludeFields, request.WritableFields); err != nil {
		return nil, err
	}
	return a.collectionPermissionEdit(http.MethodPatch, request.KeyID, request.Collection, request)
}

// UpdateAPIKeyCollectionPermissions retains the previous keep/clear/set edit semantics.
// Its target is now KeyID, and its result is a before/after change, not a key record.
func (a *AdminClient) UpdateAPIKeyCollectionPermissions(request UpdateAPIKeyCollectionPermissionsRequest) (*CollectionPermissionChange, error) {
	return a.PatchAPIKeyCollectionPermissions(PatchAPIKeyCollectionPermissionsRequest{KeyID: request.KeyID, Collection: request.Collection, Read: &request.Read, Write: &request.Write, SelectFields: request.SelectFields, ExcludeFields: request.ExcludeFields, WritableFields: request.WritableFields})
}

func (a *AdminClient) DeleteAPIKeyCollectionPermissions(keyID, collection string) (*CollectionPermissionChange, error) {
	return a.collectionPermissionEdit(http.MethodDelete, keyID, collection, nil)
}

func (a *AdminClient) kvPermissionEdit(method, keyID, prefix string, body any) (*KVPermissionChange, error) {
	if err := validatePermissionFacet(prefix); err != nil {
		return nil, err
	}
	path, err := a.permissionPath(keyID, "kv", prefix)
	if err != nil {
		return nil, err
	}
	data, err := a.client.makeRequest(method, path, body)
	if err != nil {
		return nil, err
	}
	return decodePermissionChange[KVPermissions](data)
}

func (a *AdminClient) PutAPIKeyKVPermissions(request UpdateAPIKeyKVPermissionsRequest) (*KVPermissionChange, error) {
	if !request.Read && !request.Write {
		return nil, fmt.Errorf("a KV grant requires read or write")
	}
	return a.kvPermissionEdit(http.MethodPut, request.KeyID, request.Prefix, request)
}

func (a *AdminClient) PatchAPIKeyKVPermissions(request PatchAPIKeyKVPermissionsRequest) (*KVPermissionChange, error) {
	return a.kvPermissionEdit(http.MethodPatch, request.KeyID, request.Prefix, request)
}

func (a *AdminClient) DeleteAPIKeyKVPermissions(keyID, prefix string) (*KVPermissionChange, error) {
	return a.kvPermissionEdit(http.MethodDelete, keyID, prefix, nil)
}
