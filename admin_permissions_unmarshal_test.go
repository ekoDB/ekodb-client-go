package ekodb

import (
	"encoding/json"
	"testing"
)

func TestUpdateAPIKeyCollectionPermissionsRequestUnmarshalPreservesRouting(t *testing.T) {
	for _, populated := range []bool{true, false} {
		for _, explicitNull := range []bool{true, false} {
			name, body := permissionRequestDecodeCase(populated, explicitNull)
			t.Run(name, func(t *testing.T) {
				var request UpdateAPIKeyCollectionPermissionsRequest
				if populated {
					old := []string{"old"}
					request = UpdateAPIKeyCollectionPermissionsRequest{KeyID: modernAdminID, Collection: "users", Write: true, SelectFields: &old}
				}
				keyID, collection := request.KeyID, request.Collection
				if err := json.Unmarshal([]byte(body), &request); err != nil {
					t.Fatal(err)
				}
				if request.KeyID != keyID || request.Collection != collection {
					t.Errorf("routing fields changed: got (%q, %q), want (%q, %q)", request.KeyID, request.Collection, keyID, collection)
				}
				if !request.Read || request.Write {
					t.Errorf("body rights not decoded: read=%v write=%v", request.Read, request.Write)
				}
				assertDecodedPermissionRestrictions(t, request.SelectFields, request.ExcludeFields, request.WritableFields, explicitNull)
			})
		}
	}
}

func TestPatchAPIKeyCollectionPermissionsRequestUnmarshalPreservesRouting(t *testing.T) {
	for _, populated := range []bool{true, false} {
		for _, explicitNull := range []bool{true, false} {
			name, body := permissionRequestDecodeCase(populated, explicitNull)
			t.Run(name, func(t *testing.T) {
				var request PatchAPIKeyCollectionPermissionsRequest
				if populated {
					read, write, old := false, true, []string{"old"}
					request = PatchAPIKeyCollectionPermissionsRequest{KeyID: modernAdminID, Collection: "users", Read: &read, Write: &write, SelectFields: &old}
				}
				keyID, collection := request.KeyID, request.Collection
				if err := json.Unmarshal([]byte(body), &request); err != nil {
					t.Fatal(err)
				}
				if request.KeyID != keyID || request.Collection != collection {
					t.Errorf("routing fields changed: got (%q, %q), want (%q, %q)", request.KeyID, request.Collection, keyID, collection)
				}
				if request.Read == nil || !*request.Read || request.Write == nil || *request.Write {
					t.Error("body rights not decoded as read=true and write=false")
				}
				assertDecodedPermissionRestrictions(t, request.SelectFields, request.ExcludeFields, request.WritableFields, explicitNull)
			})
		}
	}
}

func permissionRequestDecodeCase(populated, explicitNull bool) (string, string) {
	name := "zero_receiver"
	if populated {
		name = "populated_receiver"
	}
	if explicitNull {
		return name + "/null_restriction", `{"read":true,"write":false,"select_fields":null}`
	}
	return name + "/omitted_restrictions", `{"read":true,"write":false}`
}

func assertDecodedPermissionRestrictions(t *testing.T, selected, excluded, writable *[]string, explicitNull bool) {
	t.Helper()
	if explicitNull {
		if selected == nil || *selected != nil {
			t.Error("explicit null must decode to a non-nil pointer to a nil slice")
		}
	} else if selected != nil {
		t.Error("omitted select_fields must remain omitted")
	}
	if excluded != nil || writable != nil {
		t.Error("omitted restrictions must remain omitted")
	}
}
