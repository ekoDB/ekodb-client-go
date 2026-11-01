#!/bin/sh
set -eu

repo_root=$(CDPATH= cd "$(dirname "$0")/.." && pwd)
test_root=$(mktemp -d "${TMPDIR:-/tmp}/ekodb-bump-version-test.XXXXXX")
trap 'rm -rf "$test_root"' EXIT HUP INT TERM

fixture="$test_root/fixture"
mkdir "$fixture"
cp "$repo_root/version.json" "$test_root/real-version.json"
cp "$repo_root/CHANGELOG.md" "$test_root/real-changelog.md"
printf '{\n  "version": "0.29.0"\n}\n' > "$fixture/version.json"
cat > "$fixture/CHANGELOG.md" <<'EOF'
# Synthetic changelog

## [Unreleased]

- Synthetic release entry.

## [0.29.0] - 2040-01-01

- Earlier synthetic entry.
EOF

# The UTC date differs from the simulated local date. A missing -u must fail.
mkdir "$test_root/bin"
cat > "$test_root/bin/date" <<'EOF'
#!/bin/sh
if [ "${1:-}" = "-u" ] && [ "${2:-}" = "+%Y-%m-%d" ]; then
	printf '2040-01-02\n'
else
	printf '2040-01-01\n'
fi
EOF
chmod +x "$test_root/bin/date"

run_bump() {
	PATH="$test_root/bin:$PATH" make --silent -C "$fixture" -f "$repo_root/Makefile" bump-version "$@" > "$test_root/output" 2>&1
}

assert_version() {
	printf '{\n  "version": "%s"\n}\n' "$1" > "$test_root/expected.json"
	cmp "$test_root/expected.json" "$fixture/version.json"
}

run_bump VERSION=0.30.0
assert_version 0.30.0
grep -Fxq '## [0.30.0] - 2040-01-02' "$fixture/CHANGELOG.md"
grep -Fxq -- '- Synthetic release entry.' "$fixture/CHANGELOG.md"
grep -Fxq '## [0.29.0] - 2040-01-01' "$fixture/CHANGELOG.md"
grep -Fq 'Suggested commit subject: chore(release): v0.30.0' "$test_root/output"
if grep -q '^## \[Unreleased\]' "$fixture/CHANGELOG.md"; then
	echo "bump-version did not collapse the unreleased section" >&2
	exit 1
fi

# Even pending notes stay untouched when the version is unchanged or invalid.
{
	printf '# Synthetic changelog\n\n## [Unreleased]\n\n- Next synthetic entry.\n\n'
	cat "$fixture/CHANGELOG.md"
} > "$test_root/next-changelog.md"
cp "$test_root/next-changelog.md" "$fixture/CHANGELOG.md"

cp "$fixture/version.json" "$test_root/before.json"
cp "$fixture/CHANGELOG.md" "$test_root/before-changelog.md"
run_bump VERSION=0.30.0
cmp "$test_root/before.json" "$fixture/version.json"
cmp "$test_root/before-changelog.md" "$fixture/CHANGELOG.md"

if run_bump VERSION=invalid; then
	echo "bump-version accepted an invalid version" >&2
	exit 1
fi
cmp "$test_root/before.json" "$fixture/version.json"
cmp "$test_root/before-changelog.md" "$fixture/CHANGELOG.md"

printf '0.31.0\n' | run_bump VERSION=
assert_version 0.31.0
grep -Fxq '## [0.31.0] - 2040-01-02' "$fixture/CHANGELOG.md"
grep -Fq 'Suggested commit subject: chore(release): v0.31.0' "$test_root/output"

cp "$fixture/version.json" "$test_root/before.json"
cp "$fixture/CHANGELOG.md" "$test_root/before-changelog.md"
if run_bump VERSION= < /dev/null; then
	echo "bump-version accepted EOF as a version" >&2
	exit 1
fi
cmp "$test_root/before.json" "$fixture/version.json"
cmp "$test_root/before-changelog.md" "$fixture/CHANGELOG.md"

rm "$fixture/CHANGELOG.md"
run_bump VERSION=0.32.0
assert_version 0.32.0
test ! -e "$fixture/CHANGELOG.md"
grep -Fq 'Suggested commit subject: chore(release): v0.32.0' "$test_root/output"
cmp "$test_root/real-version.json" "$repo_root/version.json"
cmp "$test_root/real-changelog.md" "$repo_root/CHANGELOG.md"

echo "Version and UTC changelog fixture checks passed."
