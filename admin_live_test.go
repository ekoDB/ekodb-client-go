package ekodb

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
)

// TestAdminLive is opt-in and only revokes the uniquely labeled key it creates.
func TestAdminLive(t *testing.T) {
	if os.Getenv("EKODB_ADMIN_LIVE") != "1" {
		t.Skip("requires an authorized disposable environment")
	}
	baseURL, apiKey := os.Getenv("API_BASE_URL"), os.Getenv("API_BASE_KEY")
	if baseURL == "" || apiKey == "" {
		t.Fatal("API_BASE_URL and API_BASE_KEY are required")
	}
	admin, err := NewAdminClient(baseURL, apiKey)
	if err != nil {
		t.Fatal(err)
	}
	health, err := admin.client.HealthStatus()
	if err != nil {
		t.Fatal(err)
	}
	if version, ok := health.Detail["version"].(string); ok {
		t.Logf("server version: %s", version)
	}
	unique := make([]byte, 16)
	if _, err = rand.Read(unique); err != nil {
		t.Fatal("could not generate disposable label")
	}
	label := "sdk-go-admin-" + hex.EncodeToString(unique)
	t.Cleanup(func() {
		if err := cleanupAdminLiveKey(admin, label, apiKey); err != nil {
			t.Errorf("Disposable key cleanup failed; revoke label %s: %v", label, err)
		}
	})
	created, err := admin.CreateAPIKey(CreateAPIKeyRequest{Label: label, CollectionPermissions: map[string][]APIKeyAction{"sdk_admin_probe": {APIKeyRead}}})
	if err != nil {
		t.Fatalf("Disposable creation failed for label %s: %v", label, err)
	}
	modern, err := admin.UsesKeyPermissionRoutes()
	if err != nil {
		t.Fatal(err)
	}
	if modern && (len(created.ID) != 32 || !permissionKeyID.MatchString(created.ID)) {
		t.Fatal("registration omitted a valid key ID")
	}
	if modern {
		t.Log("permission route generation: key-addressed")
	} else {
		t.Log("permission route generation: legacy")
	}
	if created.IsAdmin || created.Key == apiKey {
		t.Fatal("disposable key unexpectedly admin")
	}
	keys, err := admin.ListAPIKeys()
	if err != nil {
		t.Fatal(err)
	}
	matches := 0
	for _, key := range keys {
		if key.Label != nil && *key.Label == label {
			matches++
			if modern && key.ID != created.ID {
				t.Fatal("key listing omitted or changed the registered key ID")
			}
		}
	}
	if matches != 1 {
		t.Fatal("disposable key readback count mismatch")
	}
	scoped, err := NewAdminClient(baseURL, created.Key)
	if err != nil {
		t.Fatal(err)
	}
	before, err := scoped.GetPermissions()
	if err != nil {
		t.Fatal(err)
	}
	if before.IsAdmin || len(before.CollectionPermissions) != 1 || !before.CollectionPermissions["sdk_admin_probe"].Read || before.CollectionPermissions["sdk_admin_probe"].Write {
		t.Fatal("unexpected disposable grants")
	}
	if _, err = scoped.ListAPIKeys(); err == nil {
		t.Fatal("non-admin key unexpectedly listed keys")
	}
	selected, excluded, writable := []string{"public"}, []string{"secret"}, []string{"public"}
	request := UpdateAPIKeyCollectionPermissionsRequest{KeyID: created.ID, Collection: "sdk_admin_probe", Read: true, Write: true, SelectFields: &selected, ExcludeFields: &excluded, WritableFields: &writable}
	if err = updateAdminLiveCollection(admin, modern, created.Key, request); err != nil {
		t.Fatal(err)
	}
	firstClient, err := NewAdminClient(baseURL, created.Key)
	if err != nil {
		t.Fatal(err)
	}
	first, err := firstClient.GetPermissions()
	if err != nil {
		t.Fatal(err)
	}
	initial := first.CollectionPermissions["sdk_admin_probe"]
	if !initial.Read || !initial.Write || !reflect.DeepEqual(initial.SelectFields, selected) || !reflect.DeepEqual(initial.ExcludeFields, excluded) || !reflect.DeepEqual(initial.WritableFields, writable) {
		t.Fatal("initial field restrictions not reflected in fresh token")
	}
	var clear []string
	empty := []string{}
	request.Write = false
	request.SelectFields = nil
	request.ExcludeFields = &clear
	request.WritableFields = &empty
	if err = updateAdminLiveCollection(admin, modern, created.Key, request); err != nil {
		t.Fatal(err)
	}
	refreshed, err := NewAdminClient(baseURL, created.Key)
	if err != nil {
		t.Fatal(err)
	}
	after, err := refreshed.GetPermissions()
	if err != nil {
		t.Fatal(err)
	}
	grant := after.CollectionPermissions["sdk_admin_probe"]
	if !grant.Read || grant.Write || len(grant.SelectFields) != 1 || grant.SelectFields[0] != "public" || grant.ExcludeFields != nil || grant.WritableFields == nil || len(grant.WritableFields) != 0 {
		t.Fatal("permission update not reflected in fresh token")
	}
	if modern {
		verifyAdminLiveFacets(t, admin, scoped, created.ID, "sdk_admin_probe", "sdk-go-admin:"+hex.EncodeToString(unique)+"/")
		verifyAdminLiveNameContract(t, admin, created.ID)
	}
}

func verifyAdminLiveNameContract(t *testing.T, admin *AdminClient, keyID string) {
	t.Helper()
	data, err := os.ReadFile("test-fixtures/admin-permission-names.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Accepted []struct{ Name, Value string }
	}
	if json.Unmarshal(data, &fixture) != nil || len(fixture.Accepted) == 0 {
		t.Fatal("invalid accepted-name fixture")
	}
	for _, name := range fixture.Accepted {
		change, err := admin.PutAPIKeyCollectionPermissions(UpdateAPIKeyCollectionPermissionsRequest{KeyID: keyID, Collection: name.Value, Read: true})
		if err != nil || change.After == nil || !change.After.Read {
			t.Fatalf("accepted collection PUT failed for %s: %v", name.Name, err)
		}
		grant, err := admin.GetAPIKeyCollectionPermissions(keyID, name.Value)
		if err != nil || !grant.Read {
			t.Fatalf("accepted collection GET failed for %s: %v", name.Name, err)
		}
		change, err = admin.DeleteAPIKeyCollectionPermissions(keyID, name.Value)
		if err != nil || change.After != nil {
			t.Fatalf("accepted collection DELETE failed for %s: %v", name.Name, err)
		}
		kv, err := admin.PutAPIKeyKVPermissions(UpdateAPIKeyKVPermissionsRequest{KeyID: keyID, Prefix: name.Value, Read: true})
		if err != nil || kv.After == nil || !kv.After.Read {
			t.Fatalf("accepted KV PUT failed for %s: %v", name.Name, err)
		}
		kvGrant, err := admin.GetAPIKeyKVPermissions(keyID, name.Value)
		if err != nil || !kvGrant.Read {
			t.Fatalf("accepted KV GET failed for %s: %v", name.Name, err)
		}
		kv, err = admin.DeleteAPIKeyKVPermissions(keyID, name.Value)
		if err != nil || kv.After != nil {
			t.Fatalf("accepted KV DELETE failed for %s: %v", name.Name, err)
		}
	}
}

func verifyAdminLiveFacets(t *testing.T, admin, scoped *AdminClient, keyID, collection, prefix string) {
	t.Helper()
	change, err := admin.PutAPIKeyCollectionPermissions(UpdateAPIKeyCollectionPermissionsRequest{KeyID: keyID, Collection: collection, Read: true})
	if err != nil {
		t.Fatal(err)
	}
	if change.Before == nil || change.After == nil || !change.After.Read || change.After.SelectFields != nil || change.After.ExcludeFields != nil || change.After.WritableFields != nil {
		t.Fatal("collection PUT did not replace omitted restrictions")
	}
	write := true
	change, err = admin.PatchAPIKeyCollectionPermissions(PatchAPIKeyCollectionPermissionsRequest{KeyID: keyID, Collection: collection, Write: &write})
	if err != nil || change.Before == nil || change.After == nil || !change.After.Read || !change.After.Write {
		t.Fatal("collection PATCH did not retain omitted rights")
	}
	read, err := admin.GetAPIKeyCollectionPermissions(keyID, collection)
	if err != nil || !read.Read || !read.Write {
		t.Fatal("collection facet readback failed")
	}
	collections, err := admin.ListAPIKeyCollectionPermissions(keyID)
	if err != nil || !collections[collection].Read {
		t.Fatal("collection facet listing failed")
	}
	if _, err = scoped.GetAPIKeyPermissions(keyID); err != nil {
		t.Fatal("non-admin own-ID permission read failed")
	}
	if _, err = scoped.PutAPIKeyCollectionPermissions(UpdateAPIKeyCollectionPermissionsRequest{KeyID: keyID, Collection: collection, Read: true}); err == nil {
		t.Fatal("non-admin changed its own grants")
	}
	change, err = admin.DeleteAPIKeyCollectionPermissions(keyID, collection)
	if err != nil || change.Before == nil || change.After != nil {
		t.Fatal("collection DELETE change is incorrect")
	}
	if _, err = admin.GetAPIKeyCollectionPermissions(keyID, collection); err == nil {
		t.Fatal("deleted collection grant remains")
	}
	kvChange, err := admin.PutAPIKeyKVPermissions(UpdateAPIKeyKVPermissionsRequest{KeyID: keyID, Prefix: prefix, Read: true, Write: true})
	if err != nil || kvChange.Before != nil || kvChange.After == nil || !kvChange.After.Read || !kvChange.After.Write {
		t.Fatal("KV PUT change is incorrect")
	}
	kv, err := admin.GetAPIKeyKVPermissions(keyID, prefix)
	if err != nil || !kv.Read || !kv.Write {
		t.Fatal("KV facet readback failed")
	}
	grants, err := admin.ListAPIKeyKVPermissions(keyID, &prefix)
	if err != nil || len(grants) != 1 || grants[0].Prefix != prefix {
		t.Fatal("KV prefix-filter listing failed")
	}
	remove := false
	kvChange, err = admin.PatchAPIKeyKVPermissions(PatchAPIKeyKVPermissionsRequest{KeyID: keyID, Prefix: prefix, Read: &remove, Write: &remove})
	if err != nil || kvChange.Before == nil || kvChange.After != nil {
		t.Fatal("KV PATCH did not remove empty rights")
	}
	if _, err = admin.GetAPIKeyKVPermissions(keyID, prefix); err == nil {
		t.Fatal("removed KV grant remains")
	}
	kvChange, err = admin.PutAPIKeyKVPermissions(UpdateAPIKeyKVPermissionsRequest{KeyID: keyID, Prefix: "*", Read: true})
	if err != nil || kvChange.After == nil || !kvChange.After.Read {
		t.Fatal("KV wildcard grant failed")
	}
	kvChange, err = admin.DeleteAPIKeyKVPermissions(keyID, "*")
	if err != nil || kvChange.Before == nil || kvChange.After != nil {
		t.Fatal("KV DELETE change is incorrect")
	}
	if _, err = admin.GetAPIKeyKVPermissions(keyID, "*"); err == nil {
		t.Fatal("deleted KV wildcard grant remains")
	}
}

// Legacy requests remain explicit: old servers do not expose stable key IDs.
func updateAdminLiveCollection(admin *AdminClient, modern bool, secret string, request UpdateAPIKeyCollectionPermissionsRequest) error {
	if modern {
		_, err := admin.UpdateAPIKeyCollectionPermissions(request)
		return err
	}
	_, err := admin.LegacyUpdateAPIKeyCollectionPermissions(LegacyUpdateAPIKeyCollectionPermissionsRequest{APIKey: secret, Collection: request.Collection, Read: request.Read, Write: request.Write, SelectFields: request.SelectFields, ExcludeFields: request.ExcludeFields, WritableFields: request.WritableFields})
	return err
}

// Recover by the unique test label even when registration's response is lost.
func cleanupAdminLiveKey(admin *AdminClient, label, ownerKey string) error {
	keys, err := admin.ListAPIKeys()
	if err != nil {
		return err
	}
	var matches []APIKey
	for _, key := range keys {
		if key.Label != nil && *key.Label == label {
			matches = append(matches, key)
		}
	}
	if len(matches) > 1 {
		return fmt.Errorf("multiple keys match the disposable label")
	}
	for _, key := range matches {
		if key.Key == ownerKey {
			return fmt.Errorf("refusing to revoke the owner key")
		}
		target := key.ID
		if target == "" {
			target = key.Key
		}
		if err := admin.RevokeAPIKey(target); err != nil {
			return err
		}
	}
	keys, err = admin.ListAPIKeys()
	if err != nil {
		return err
	}
	for _, key := range keys {
		if key.Label != nil && *key.Label == label {
			return fmt.Errorf("disposable key remains")
		}
	}
	return nil
}
