# Publishing ekoDB Go Client

The client is distributed as a Go module from this repository. A release is a
semantic-version tag on `main`; there is no registry upload.

## Release

```bash
make publish
```

`make publish` runs `check-ready` (format check, `go vet`, the test suite, and a
clean working tree) and then `publish.sh`, which:

1. runs the tests and `go mod tidy`
2. prompts for the new version (`vX.Y.Z`) and checks that the head commit is the
   cap `chore(<scope>): vX.Y.Z`
3. pushes `main`

The cap merging to `main` is the release: `.github/workflows/release.yml` tags
itself, publishes the GitHub Release from the `CHANGELOG.md` block, and runs
`scripts/index-release.sh` to make the tag visible on pkg.go.dev. `publish.sh`
neither creates nor pushes a tag — CI does both. Watch it with:

```bash
gh run list --repo ekoDB/ekodb-client-go --workflow release.yml --limit 1
```

## Why the pkg.go.dev index step exists

Neither the Go module proxy nor pkg.go.dev watches GitHub. The proxy fetches a
version the first time something asks for it, and pkg.go.dev indexes a version
when it is asked to. Left alone, a pushed tag can sit for days while
<https://pkg.go.dev/github.com/ekoDB/ekodb-client-go> still shows the previous
release, and `@latest` on the proxy disagrees with the page.

`scripts/index-release.sh vX.Y.Z` performs the three requests in order and
succeeds only once the version page renders:

1. `GET https://proxy.golang.org/github.com/eko!d!b/ekodb-client-go/@v/vX.Y.Z.info`
   (the proxy's case-encoded module path) until it returns 200, so the proxy
   fetches the tag; the first fetch from origin is not instant after a push
2. `POST https://pkg.go.dev/fetch/github.com/ekoDB/ekodb-client-go@vX.Y.Z` so
   pkg.go.dev indexes it
3. `GET https://pkg.go.dev/github.com/ekoDB/ekodb-client-go@vX.Y.Z` until it
   returns 200

Every request has a connect timeout of 10 seconds and an overall timeout of 30
(`CONNECT_TIMEOUT` and `REQUEST_TIMEOUT`), so an unresponsive server ends the
run instead of hanging it. The bound is per request, not per run: each polled
step can take up to `INDEX_ATTEMPTS x (REQUEST_TIMEOUT + INDEX_SLEEP)`, about 20
minutes at the defaults against a server that never answers. A tag that is not
on origin costs `(INDEX_ATTEMPTS - 1) x INDEX_SLEEP`, just under five minutes at
the defaults, before step 1 gives up, because the proxy's 404 for a tag it has
not fetched yet is the same 404 it gives for one that does not exist.

It can be run on its own for a tag that was pushed some other way:

```bash
make index-release VERSION=vX.Y.Z
```

It exits 1, naming the URL and HTTP status (with curl's own message when the
request did not complete), when the proxy has not served the version after
`INDEX_ATTEMPTS` polls `INDEX_SLEEP` seconds apart (the tag is not on origin, or
points at a commit without a `go.mod`), when pkg.go.dev reports the version as
not found, or when the page has not rendered after the same number of polls. The
defaults are 30 attempts and 10 seconds. `INDEX_ATTEMPTS`, `CONNECT_TIMEOUT` and
`REQUEST_TIMEOUT` must be integers from 1 to 99999 and `INDEX_SLEEP` an integer
from 0 to 99999, written as plain decimals; the script exits 2 before any
request when they are not, or when the version is not of the form `vX.Y.Z`. Its
tests live in `scripts/index_release_test.go` and run as part of
`go test ./...`.

## Checking a version before capping

`make bump-version` runs the tests against a candidate `vX.Y.Z` and prints the
next steps (push `main`, watch `release.yml`, the install command) without
tagging or pushing anything — the tag is CI's, cut once the cap commit lands.

## Installation

```bash
go get github.com/ekoDB/ekodb-client-go@vX.Y.Z
```
