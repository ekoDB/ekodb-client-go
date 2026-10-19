package ekodb

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
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
	t.Cleanup(func() {
		if err := cleanupAdminLiveKey(admin, label, apiKey); err != nil {
			t.Errorf("Disposable key cleanup failed; revoke label %s: %v", label, err)
		}
	})
	created, err := admin.CreateAPIKey(CreateAPIKeyRequest{Label: label, CollectionPermissions: map[string][]APIKeyAction{"sdk_admin_probe": {APIKeyRead}}})
	if err != nil {
		t.Fatalf("Disposable creation failed for label %s: %v", label, err)
	}
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
		if err := admin.RevokeAPIKey(key.Key); err != nil {
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
