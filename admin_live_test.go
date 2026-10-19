package ekodb

import (
	"crypto/rand"
	"encoding/hex"
	"os"
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
	unique := make([]byte, 16)
	if _, err = rand.Read(unique); err != nil {
		t.Fatal("could not generate disposable label")
	}
	label := "sdk-go-admin-" + hex.EncodeToString(unique)
	created, err := admin.CreateAPIKey(CreateAPIKeyRequest{Label: label, CollectionPermissions: map[string][]APIKeyAction{"sdk_admin_probe": {APIKeyRead}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.RevokeAPIKey(created.Key); err != nil {
			t.Errorf("Disposable key cleanup failed; revoke label %s: %v", label, err)
			return
		}
		keys, err := admin.ListAPIKeys()
		if err != nil {
			t.Errorf("Could not verify disposable cleanup for label %s: %v", label, err)
			return
		}
		for _, key := range keys {
			if key.Label != nil && *key.Label == label {
				t.Errorf("Disposable key remains for label %s", label)
			}
		}
	})
	if created.IsAdmin {
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
	_, err = admin.UpdateAPIKeyCollectionPermissions(UpdateAPIKeyCollectionPermissionsRequest{APIKey: created.Key, Collection: "sdk_admin_probe", Read: true, Write: true})
	if err != nil {
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
	if !after.CollectionPermissions["sdk_admin_probe"].Write {
		t.Fatal("permission update not reflected in fresh token")
	}
}
