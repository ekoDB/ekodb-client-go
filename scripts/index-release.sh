#!/usr/bin/env bash
# Make a pushed release tag visible on pkg.go.dev.
#
# Neither the Go module proxy nor pkg.go.dev watches GitHub. The proxy fetches
# a version the first time something asks for it, and pkg.go.dev indexes a
# version when it is asked to (or, eventually, when its crawler gets to it).
# Left alone, a tag can sit on GitHub for days while pkg.go.dev still shows the
# previous release. This script performs the three requests that close that
# gap, in order, and succeeds only once the version page actually renders:
#
#   1. GET  <proxy>/<escaped module>/@v/<version>.info   -> proxy fetches the tag
#   2. POST <pkgsite>/fetch/<module>@<version>            -> pkg.go.dev indexes it
#   3. GET  <pkgsite>/<module>@<version> until it is 200  -> the proof
#
# Usage: scripts/index-release.sh vX.Y.Z
#
# Environment (overridable, used by the tests to point at local stubs):
#   GOPROXY_URL     default https://proxy.golang.org
#   PKGSITE_URL     default https://pkg.go.dev
#   INDEX_ATTEMPTS  how many times to poll the version page (default 30)
#   INDEX_SLEEP     seconds between polls (default 10)
#
# Exit codes: 0 indexed; 1 a request failed (the URL and status are printed);
# 2 bad usage.
set -euo pipefail

GOPROXY_URL="${GOPROXY_URL:-https://proxy.golang.org}"
PKGSITE_URL="${PKGSITE_URL:-https://pkg.go.dev}"
INDEX_ATTEMPTS="${INDEX_ATTEMPTS:-30}"
INDEX_SLEEP="${INDEX_SLEEP:-10}"

version="${1:-}"
if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "usage: $0 vX.Y.Z (got '${version}')" >&2
  exit 2
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
go_mod="${script_dir}/../go.mod"
module="$(awk '$1 == "module" { print $2; exit }' "$go_mod")"
if [[ -z "$module" ]]; then
  echo "could not read the module path from ${go_mod}" >&2
  exit 1
fi

# The proxy's path encoding: every uppercase letter becomes '!' followed by
# its lowercase form (github.com/ekoDB -> github.com/eko!db). Done one
# character at a time because BSD sed has no case-conversion escape and the
# macOS default bash predates ${var,,}.
escape_module() {
  local path="$1" out="" i c
  for ((i = 0; i < ${#path}; i++)); do
    c="${path:i:1}"
    if [[ "$c" == [A-Z] ]]; then
      # The proxy encodes ASCII A-Z only, so the locale classes would be wrong.
      # shellcheck disable=SC2018,SC2019
      out+="!$(printf '%s' "$c" | tr 'A-Z' 'a-z')"
    else
      out+="$c"
    fi
  done
  printf '%s' "$out"
}

# status <method> <url>: prints the HTTP status, or 000 when the request could
# not be made at all, so a network failure is a distinct reading from a 404.
status() {
  local method="$1" url="$2"
  curl -sS -o /dev/null -w '%{http_code}' -X "$method" "$url" 2>/dev/null || printf '000'
}

proxy_url="${GOPROXY_URL}/$(escape_module "$module")/@v/${version}.info"
echo "1/3 asking the module proxy for ${module}@${version}"
code="$(status GET "$proxy_url")"
if [[ "$code" != "200" ]]; then
  echo "proxy did not serve the version: ${proxy_url} -> HTTP ${code}" >&2
  echo "is the tag pushed to origin, and does it point at a commit with a go.mod?" >&2
  exit 1
fi

fetch_url="${PKGSITE_URL}/fetch/${module}@${version}"
echo "2/3 asking pkg.go.dev to fetch ${module}@${version}"
code="$(status POST "$fetch_url")"
case "$code" in
  200) ;;
  404)
    echo "pkg.go.dev could not find the version: ${fetch_url} -> HTTP ${code}" >&2
    exit 1
    ;;
  *)
    # pkg.go.dev answers with a 5xx while its worker is still processing the
    # fetch; the page rendering below is the real success criterion.
    echo "fetch request returned HTTP ${code}; polling the version page anyway"
    ;;
esac

page_url="${PKGSITE_URL}/${module}@${version}"
echo "3/3 waiting for ${page_url}"
for ((attempt = 1; attempt <= INDEX_ATTEMPTS; attempt++)); do
  code="$(status GET "$page_url")"
  if [[ "$code" == "200" ]]; then
    echo "indexed: ${page_url}"
    exit 0
  fi
  echo "  attempt ${attempt}/${INDEX_ATTEMPTS}: HTTP ${code}"
  if (( attempt < INDEX_ATTEMPTS )); then
    sleep "$INDEX_SLEEP"
  fi
done
echo "pkg.go.dev did not render the version after ${INDEX_ATTEMPTS} attempts: ${page_url} -> HTTP ${code}" >&2
exit 1
