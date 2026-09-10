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
2. prompts for the new version (`vX.Y.Z`) and creates an annotated tag
3. pushes the tag
4. runs `scripts/index-release.sh`, which makes the tag visible on pkg.go.dev
5. offers to push `main`

`publish.sh` runs under `set -e`, so a failure in step 4 stops the run there:
the tag is already on origin and `main` has not been pushed. Nothing needs
undoing. Fix whatever the printed URL and status point at, then finish by hand:

```bash
make index-release VERSION=vX.Y.Z
git push origin main
```

## Why step 4 exists

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

Every request has a connect timeout of 10 seconds and an overall timeout of 30,
so an unresponsive server ends the run instead of hanging it.

It can be run on its own for a tag that was pushed some other way:

```bash
make index-release VERSION=vX.Y.Z
```

It exits 1, naming the URL and HTTP status (with curl's own message when no
status was obtained), when the proxy has not served the version after
`INDEX_ATTEMPTS` polls `INDEX_SLEEP` seconds apart (the tag is not on origin, or
points at a commit without a `go.mod`), when pkg.go.dev reports the version as
not found, or when the page has not rendered after the same number of polls. The
defaults are 30 attempts and 10 seconds; both must be integers, and the script
exits 2 before any request when they are not, or when the version is not of the
form `vX.Y.Z`. Its tests live in `scripts/index_release_test.go` and run as part
of `go test ./...`.

## Tagging without publishing

`make bump-version` runs the tests and creates the tag locally without pushing
anything; it prints the push and `make index-release` commands to run next.

## Installation

```bash
go get github.com/ekoDB/ekodb-client-go@vX.Y.Z
```
