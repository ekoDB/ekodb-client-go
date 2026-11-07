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
// err = admin.RevokeAPIKey(oldKeyID)
```

`CreateAPIKey` defaults to non-admin and no collection grants. Set
`IsAdmin: true` explicitly to create an admin key. Key records contain secrets:
never log a registration or listing response. Server 0.80.0 adds `APIKey.ID`, a
server-generated key identifier; 0.79.2 responses can leave it empty. Prefer
`RevokeAPIKey(key.ID)` on 0.80.0. Legacy key values remain accepted for
revocation, but an ID keeps the secret out of the URL. Revocation sends no body.

`GetPermissions()` reads the caller's permissions on either supported route
generation. It preserves `IsAdmin`, `CollectionPermissions`, all three field
restrictions, `KVPermissions`, and unknown response metadata. Legacy action
arrays normalize to read/write objects. The new wire shape
`{admin, collections, kv}` maps to these same public Go fields.

Check `key.RippleWarning` before installing a new key: a non-nil warning means
registration succeeded locally but replication to peers failed.

## Permission routes and versions

Version 0.80.0 moves permission routes under `/api/auth/keys/{id}/permissions`.
`UsesKeyPermissionRoutes()` probes `GET /api/auth/keys/me/permissions` and
caches a successful capability decision per client. Only 404 or 405 selects the
legacy generation. Forbidden responses, transient failures, malformed successes,
and authentication failures never silently downgrade to the older routes. A
definite 401 permits one token re-exchange, including a token minted before a
server upgrade. A 404/405 from that token refresh propagates as an
authentication failure; it cannot select or cache the legacy generation.

The capability probe takes precedence over the version string: a server
reporting `0.79.2` or `0.80.0-rc.N` that serves the new routes is supported.
`MinServerVersionForPermissionEdits` documents the new release as `0.80.0`;
`MinLegacyServerVersionForPermissionEdits` is `0.79.2`. The legacy edit API
still checks authenticated health and refuses releases that can erase
restrictions. Only successfully read, parseable stable versions are cached.
Failed or unreadable health reads remain retryable and preserve sanitized
transport/JSON errors. A valid older stable version remains cached and refused.

Modern permission methods accept a server-generated key ID (32 lowercase hex
characters) or `me`. A secret key value is rejected before HTTP. Collection
names and KV prefixes are encoded as one path segment, including colon, slash,
space, plus, and the `*` prefix granting the whole KV namespace.

Facet names reject empty strings, `.` and `..`, Unicode category Cc controls,
and exactly U+00AD, U+061C, U+180E, U+200B–U+200F, U+2028–U+202E, U+2060–U+2064,
U+2066–U+2069 and U+FEFF. Other valid Unicode characters remain accepted,
including format characters outside this list and supplementary-plane
characters. The canonical name cases are vendored as
`test-fixtures/admin-permission-names.json` so this module can test standalone.
KV list filters remain query values, separate from named facet validation.

- `GetAPIKeyPermissions(keyID)` reads aggregate permissions.
- `ListAPIKeyCollectionPermissions(keyID)` and
  `GetAPIKeyCollectionPermissions(keyID, collection)` read collection grants.
- `ListAPIKeyKVPermissions(keyID, prefix)` optionally filters by prefix;
  `GetAPIKeyKVPermissions(keyID, prefix)` reads one grant.
- Collection and KV grants each have `Put`, `Patch`, and `Delete` methods.

Every modern edit returns a `CollectionPermissionChange` or `KVPermissionChange`
containing `Before` and `After`. A nil `Before` denotes a new grant; a nil
`After` denotes removal. No token or secret key is returned.

```go
change, err := admin.PutAPIKeyCollectionPermissions(ekodb.UpdateAPIKeyCollectionPermissionsRequest{
    KeyID: key.ID, Collection: "jobs", Read: true, Write: false,
})
if err != nil {
    return err
}
_ = change.After

var clear []string
empty := []string{}
_, err = admin.PatchAPIKeyCollectionPermissions(ekodb.PatchAPIKeyCollectionPermissionsRequest{
    KeyID: key.ID, Collection: "jobs",
    // Read, Write and SelectFields omitted: preserve them.
    ExcludeFields: &clear, // null: clear the restriction.
    WritableFields: &empty, // []: exact empty replacement.
})
```

PUT replaces the entry: omitted field restrictions mean no limit. PATCH merges:
nil pointers omit a field and preserve its old value; a pointer to a nil slice
sends null to clear a restriction; a pointer to a slice sets the exact list.
Optional `*bool` flags in PATCH distinguish omission from false.
`UpdateAPIKeyCollectionPermissions` is a PATCH convenience method retaining
required read/write flags, with the new `KeyID` target and before/after result.

```go
_, err = admin.PutAPIKeyKVPermissions(ekodb.UpdateAPIKeyKVPermissionsRequest{
    KeyID: key.ID, Prefix: "session:", Read: true, Write: true,
})
_, err = admin.DeleteAPIKeyKVPermissions(key.ID, "session:")
```

KV PUT needs at least one true right. PATCH can remove a grant by leaving both
rights false. DELETE sends no body. A change takes effect in the key's next
token; an already-minted token can retain its old grants until expiry.

## Stored-function call grants

A non-admin key can be allowed to call stored functions. A grant is a `Pattern`
with one right, `FunctionCall`: an exact function label, a label prefix ending
in one `*` (`report_*`), or `*` for every function. `*` is valid only as the
last character; a pattern is non-empty, at most `MaxFunctionPatternBytes` (256)
bytes, and holds no control or invisible formatting character. The bare segments
`.` and `..` are also rejected (an SDK-side rule: a bare dot segment is not
safely routable, and the client does not percent-encode around it). Every method
checks these rules before sending a request and encodes the pattern as one path
segment (`*` is `%2A`). A key holds no empty grant: put gives one and delete
removes it.

```go
key, err := admin.CreateAPIKey(ekodb.CreateAPIKeyRequest{
    Label: "report-runner",
    FunctionPermissions: map[string][]ekodb.FunctionAction{
        "report_*": {ekodb.FunctionCall}, // an empty slice grants nothing
    },
})
grants, err := admin.ListAPIKeyFunctionPermissions(key.ID)
change, err := admin.PutAPIKeyFunctionPermissions(key.ID, "nightly_close")
_ = change.After // &FunctionPermissions{Pattern: "nightly_close"}
grant, err := admin.GetAPIKeyFunctionPermissions(key.ID, "nightly_close")
_, err = admin.DeleteAPIKeyFunctionPermissions(key.ID, "nightly_close")
```

`CreateAPIKeyRequest.FunctionPermissions` is omitted from the request when
empty. A registration response echoes the request map, while key listings and
`GetPermissions()` return a list of grants; all read as `FunctionPermissions`
sorted by pattern. `APIKey.FunctionPermissions` is nil when the response carries
none, and `APIKeyPermissions.FunctionPermissions` is empty for a server that
predates function grants. Changes return `FunctionPermissionChange` with
`Before` and `After` (nil for none) and preserve extra metadata such as
`ripple_warning`. Deleting a grant the key does not hold is a 404; a non-admin
caller is refused with 403, reported as an `*HTTPError`. A removed grant stays
usable through a token already issued until it expires. These methods need
key-addressed permission routes and never accept a key value as the target.

Stored-function definitions gain `UserFunction.RunAs` (`RunAsCaller` or
`RunAsAuthor`; any other value fails to decode, and the four save and update
methods refuse it before sending a definition) and the read-only,
server-recorded `UserFunction.CreatedBy` (`Principal`: `KeyID`, optional
`KeyLabel` and `Subject`). A function saved before the server recorded these
reads as nil. Schedules are untyped maps, so the server-recorded `created_by` is
returned unchanged. A function call refused for want of a grant is an
`*HTTPError` with status 403. The shared contract is
[`admin-function-permissions-contract.json`](../test-fixtures/admin-function-permissions-contract.json),
vendored unchanged from the public client contract.

## Explicit 0.79.2 compatibility

The older generation has no equivalent for every modern facet operation. Modern
facet methods fail explicitly on that generation. For an existing 0.79.2
deployment, use the separately named legacy request and method:

```go
modern, err := admin.UsesKeyPermissionRoutes()
if err != nil {
    return err
}
if !modern {
    _, err = admin.LegacyUpdateAPIKeyCollectionPermissions(ekodb.LegacyUpdateAPIKeyCollectionPermissionsRequest{
        APIKey: key.Key, Collection: "jobs", Read: true, Write: false,
    })
}
```

This legacy-only API retains the old secret-bearing body and keep/clear/set
field semantics. It refuses to send a write when the capability probe detects
modern routes. `GetPermissions()` works on both generations; aggregate reads of
another key and facet reads require the modern generation.

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
states. The synthetic 0.80.0 fixture adds both facet families, three edit verbs,
path encoding, null-clears and before/after results. These are offline
contracts, not live-server captures. Its vendored copy permits offline tests
without a sibling checkout. The matching `examples/go/client_admin.go` in the
public client repository is read-only by default and can run the authorized
disposable lifecycle with `EKODB_ADMIN_LIVE=1`. Creation uses both an admin
Bearer JWT and the admin key in the body for deployed-server compatibility;
registration never retries alternative formats.
