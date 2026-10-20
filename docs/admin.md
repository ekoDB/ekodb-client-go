# API-key administration

`AdminClient` uses the existing Go module and keeps key administration separate
from the data client:

```go
admin, err := ekodb.NewAdminClient(os.Getenv("API_BASE_URL"), os.Getenv("API_BASE_KEY"))
if err != nil {
    return err
}
key, err := admin.CreateAPIKey(ekodb.CreateAPIKeyRequest{
    Label: "worker-replacement",
    CollectionPermissions: map[string][]ekodb.APIKeyAction{
        "jobs": {ekodb.APIKeyRead, ekodb.APIKeyWrite},
    },
})
if err != nil {
    return err
}
// Key records contain secrets. Never log them.
scoped, err := ekodb.NewAdminClient(os.Getenv("API_BASE_URL"), key.Key)
if err != nil {
    return err
}
permissions, err := scoped.GetPermissions()
if err != nil {
    return err
}
if permissions.IsAdmin || !permissions.CollectionPermissions["jobs"].Read {
    return fmt.Errorf("unexpected worker permissions")
}
// Install and verify the replacement in the application before revoking an old key.
// err = admin.RevokeAPIKey(oldKey)
```

`CreateAPIKey` defaults to non-admin and no collection grants. Set
`IsAdmin: true` explicitly to create an admin key. `ListAPIKeys()` returns
secret-bearing records. `RevokeAPIKey(secret)` encodes the actual key value as a
single path segment. `GetPermissions()` reads the calling token's grants and
works for non-admin keys. It preserves `SelectFields`, `ExcludeFields`, and
`WritableFields`, including nil versus empty slices. Legacy action arrays
normalize to read/write objects.

Check `key.RippleWarning` before installing a new key: a non-nil warning means
registration succeeded locally but replication to peers failed.

```go
_, err = admin.UpdateAPIKeyCollectionPermissions(ekodb.UpdateAPIKeyCollectionPermissionsRequest{
    APIKey: key.Key,
    Collection: "jobs",
    Read: true,
    Write: false,
})
```

Updates replace the read/write flags for one collection grant. Restriction edits
require server **0.79.2 or later**, verified through authenticated `/api/health`
and cached per admin client. Old, prerelease, malformed or unreadable versions
fail before PUT. Earlier servers remove restrictions and can leave grants stale.

For `SelectFields`, `ExcludeFields` and `WritableFields`, a nil pointer omits
the field and preserves its previous value; a pointer to a nil slice sends null
and clears it; a pointer to a slice replaces it, including an empty slice:

```go
var clear []string
empty := []string{}
_, err = admin.UpdateAPIKeyCollectionPermissions(ekodb.UpdateAPIKeyCollectionPermissionsRequest{
    APIKey: key.Key, Collection: "jobs", Read: true, Write: false,
    // SelectFields omitted: preserve it.
    ExcludeFields: &clear, // null: clear the restriction.
    WritableFields: &empty, // []: exact empty replacement.
})
```

The response `Metadata` maps preserve unknown fields. Freshly authenticate the
scoped key to verify a changed grant; an existing JWT can retain its old claims.

`NewAdminClientWithConfig(AdminClientConfig{...})` accepts `BaseURL`, `APIKey`,
and `Timeout` (default 30 seconds). The facade reuses the data SDK's
authentication, token cache, proactive refresh and JSON transport. It permits
one refresh after a definite 401; it never retries network failures, timeouts,
rate limits or server failures, and rejects redirects. After an ambiguous
creation failure, inspect uniquely labeled keys before manually retrying. Errors
retain useful status codes but omit server bodies, request URLs and underlying
transport/parser errors that might contain credentials.

Keep identity labels, rotation locks, installation and replacement verification
in the application. This API provides no delegation, inherited quotas,
permission ceilings, tenant isolation or cascading revocation. Collection grants
do not constrain stored-function internals; see the
[function permission model](https://docs.ekodb.io/reference/functions-architecture#permission-model).

## Verification

Offline HTTP-boundary tests run as part of `make test`. An opt-in live test
creates a unique disposable key, exercises all five operations, verifies
non-admin denial, then revokes its key and checks cleanup:

```sh
# Supply credentials for an authorized disposable environment through the environment.
EKODB_ADMIN_LIVE=1 go test -v -run '^TestAdminLive$' -count=1 ./...
```

The recorded 0.79.2 fixture covers omitted, null, empty and replacement field
states, including fresh-token grants. Its vendored copy permits offline tests
without a sibling checkout. The matching `examples/go/client_admin.go` in the
public client repository is read-only by default and can run the authorized
disposable lifecycle with `EKODB_ADMIN_LIVE=1`. Creation uses both an admin
Bearer JWT and the admin key in the body for deployed-server compatibility;
registration never retries alternative formats.
