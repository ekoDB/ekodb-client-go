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
# Steps 1 and 3 are polled, up to INDEX_ATTEMPTS times INDEX_SLEEP seconds
# apart: step 1 because it runs seconds after the tag push and the proxy's
# first fetch from origin is not instant, step 3 because pkg.go.dev processes
# the fetch asynchronously. Every request carries a timeout, so a server that
# accepts the connection and never answers cannot hang a release. The worst
# case per polled step is INDEX_ATTEMPTS x (REQUEST_TIMEOUT + INDEX_SLEEP),
# about 20 minutes at the defaults against a server that never answers, and a
# tag that is not on origin costs the full INDEX_ATTEMPTS x INDEX_SLEEP (five
# minutes at the defaults) before step 1 gives up: the proxy's 404 for a tag it
# has not fetched yet is the same 404 it gives for one that does not exist.
#
# Usage: scripts/index-release.sh vX.Y.Z
#
# Environment (overridable, used by the tests to point at local stubs):
#   GOPROXY_URL     default https://proxy.golang.org
#   PKGSITE_URL     default https://pkg.go.dev
#   INDEX_ATTEMPTS  polls per polled step, an integer >= 1 (default 30)
#   INDEX_SLEEP     seconds between polls, an integer >= 0 (default 10)
#   CONNECT_TIMEOUT seconds to establish each connection, >= 1 (default 10)
#   REQUEST_TIMEOUT seconds for each whole request, >= 1 (default 30)
#
# Exit codes: 0 indexed; 1 a request failed (the URL and status are printed,
# with curl's own message when the request could not be made at all); 2 bad
# usage.
set -euo pipefail

GOPROXY_URL="${GOPROXY_URL:-https://proxy.golang.org}"
PKGSITE_URL="${PKGSITE_URL:-https://pkg.go.dev}"
INDEX_ATTEMPTS="${INDEX_ATTEMPTS:-30}"
INDEX_SLEEP="${INDEX_SLEEP:-10}"
CONNECT_TIMEOUT="${CONNECT_TIMEOUT:-10}"
REQUEST_TIMEOUT="${REQUEST_TIMEOUT:-30}"

version="${1:-}"
if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "usage: $0 vX.Y.Z (got '${version}')" >&2
  exit 2
fi
# The settings are matched as decimal digit strings with no leading zero, and
# never evaluated arithmetically here: bash reads "08" as octal and errors,
# and an error inside an `||` chain would skip the check instead of failing it.
for setting in INDEX_ATTEMPTS CONNECT_TIMEOUT REQUEST_TIMEOUT; do
  if [[ ! "${!setting}" =~ ^[1-9][0-9]*$ ]]; then
    echo "${setting} must be an integer >= 1 (got '${!setting}')" >&2
    exit 2
  fi
done
if [[ ! "$INDEX_SLEEP" =~ ^(0|[1-9][0-9]*)$ ]]; then
  echo "INDEX_SLEEP must be an integer >= 0 (got '${INDEX_SLEEP}')" >&2
  exit 2
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
go_mod="${script_dir}/../go.mod"
module=""
if [[ -f "$go_mod" ]]; then
  module="$(awk '$1 == "module" { print $2; exit }' "$go_mod")"
fi
if [[ -z "$module" ]]; then
  echo "could not read the module path from ${go_mod}" >&2
  exit 1
fi

# The proxy's path encoding: every uppercase ASCII letter becomes '!' followed
# by its lowercase form (github.com/ekoDB -> github.com/eko!d!b). Done one
# character at a time because BSD sed has no case-conversion escape and the
# macOS default bash predates ${var,,}. The letters are listed rather than
# written as a range so the match cannot depend on the locale's collation.
escape_module() {
  local path="$1" out="" i c
  for ((i = 0; i < ${#path}; i++)); do
    c="${path:i:1}"
    case "$c" in
      [ABCDEFGHIJKLMNOPQRSTUVWXYZ])
        # The proxy encodes ASCII A-Z only, so the locale classes would be wrong.
        # shellcheck disable=SC2018,SC2019
        out+="!$(printf '%s' "$c" | tr 'A-Z' 'a-z')"
        ;;
      *) out+="$c" ;;
    esac
  done
  printf '%s' "$out"
}

curl_stderr="$(mktemp)"
trap 'rm -f "$curl_stderr"' EXIT

# request <method> <url>: sets `code` to the HTTP status, or to 000 with
# `detail` carrying curl's own message when no status was obtained (refused
# connection, DNS failure, timeout). The two causes are kept apart so a network
# failure never reads as a 404 and vice versa.
code=""
detail=""
request() {
  local method="$1" url="$2" out rc
  detail=""
  if out="$(curl -sS -o /dev/null -w '%{http_code}' \
      --connect-timeout "$CONNECT_TIMEOUT" --max-time "$REQUEST_TIMEOUT" \
      -X "$method" "$url" 2>"$curl_stderr")"; then
    rc=0
  else
    rc=$?
  fi
  if (( rc != 0 )); then
    code="000"
    detail="$(tr -d '\n' <"$curl_stderr")"
  else
    code="$out"
  fi
}

# describe: the status for a message, with curl's detail when there is one.
describe() {
  if [[ -n "$detail" ]]; then
    printf 'HTTP %s (%s)' "$code" "$detail"
  else
    printf 'HTTP %s' "$code"
  fi
}

# wait_for <method> <url>: polls until the status is 200, INDEX_ATTEMPTS times
# INDEX_SLEEP seconds apart, with no sleep after the last attempt. Returns 0 on
# a 200; otherwise returns 1 with `code`/`detail` holding the last reading.
wait_for() {
  local method="$1" url="$2" attempt
  for ((attempt = 1; attempt <= INDEX_ATTEMPTS; attempt++)); do
    request "$method" "$url"
    if [[ "$code" == "200" ]]; then
      return 0
    fi
    echo "  attempt ${attempt}/${INDEX_ATTEMPTS}: $(describe)"
    if (( attempt < INDEX_ATTEMPTS )); then
      sleep "$INDEX_SLEEP"
    fi
  done
  return 1
}

proxy_url="${GOPROXY_URL}/$(escape_module "$module")/@v/${version}.info"
echo "1/3 asking the module proxy for ${module}@${version}"
if ! wait_for GET "$proxy_url"; then
  echo "proxy did not serve the version after ${INDEX_ATTEMPTS} attempts: ${proxy_url} -> $(describe)" >&2
  echo "is the tag pushed to origin, and does it point at a commit with a go.mod?" >&2
  exit 1
fi

fetch_url="${PKGSITE_URL}/fetch/${module}@${version}"
echo "2/3 asking pkg.go.dev to fetch ${module}@${version}"
request POST "$fetch_url"
case "$code" in
  200) ;;
  404)
    echo "pkg.go.dev could not find the version: ${fetch_url} -> $(describe)" >&2
    exit 1
    ;;
  *)
    # pkg.go.dev answers with a 5xx while its worker is still processing the
    # fetch; the page rendering below is the real success criterion.
    echo "fetch request returned $(describe); polling the version page anyway"
    ;;
esac

page_url="${PKGSITE_URL}/${module}@${version}"
echo "3/3 waiting for ${page_url}"
if wait_for GET "$page_url"; then
  echo "indexed: ${page_url}"
  exit 0
fi
echo "pkg.go.dev did not render the version after ${INDEX_ATTEMPTS} attempts: ${page_url} -> $(describe)" >&2
exit 1
