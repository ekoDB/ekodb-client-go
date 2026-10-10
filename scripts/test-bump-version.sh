#!/bin/sh
set -eu

repo_root=$(CDPATH= cd "$(dirname "$0")/.." && pwd)
test_root=$(mktemp -d "${TMPDIR:-/tmp}/ekodb-bump-version-test.XXXXXX")
trap 'rm -rf "$test_root"' EXIT HUP INT TERM

fixture="$test_root/fixture"
mkdir "$fixture"
cp "$repo_root/version.json" "$test_root/real-version.json"
printf '{\n  "version": "0.29.0"\n}\n' > "$fixture/version.json"

run_bump() {
	make --silent -C "$fixture" -f "$repo_root/Makefile" bump-version "$@" > "$test_root/output" 2>&1
}

assert_version() {
	printf '{\n  "version": "%s"\n}\n' "$1" > "$test_root/expected.json"
	cmp "$test_root/expected.json" "$fixture/version.json"
}

run_bump VERSION=0.30.0
assert_version 0.30.0

cp "$fixture/version.json" "$test_root/before.json"
run_bump VERSION=0.30.0
cmp "$test_root/before.json" "$fixture/version.json"

if run_bump VERSION=invalid; then
	echo "bump-version accepted an invalid version" >&2
	exit 1
fi
cmp "$test_root/before.json" "$fixture/version.json"

printf '0.31.0\n' | run_bump VERSION=
assert_version 0.31.0

cp "$fixture/version.json" "$test_root/before.json"
if run_bump VERSION= < /dev/null; then
	echo "bump-version accepted EOF as a version" >&2
	exit 1
fi
cmp "$test_root/before.json" "$fixture/version.json"
cmp "$test_root/real-version.json" "$repo_root/version.json"

echo "Version bump fixture checks passed."
