#!/bin/sh
set -eu

test_root=$(mktemp -d "${TMPDIR:-/tmp}/ekodb-hooks-test.XXXXXX")
trap 'rm -rf "$test_root"' EXIT HUP INT TERM

main_repo="$test_root/main"
linked_worktree="$test_root/worktree"

git init --quiet "$main_repo"
git -C "$main_repo" config user.email "hooks-test@example.invalid"
git -C "$main_repo" config user.name "Hook Test"
mkdir -p "$main_repo/scripts"
cp Makefile "$main_repo/Makefile"
cp scripts/pre-commit "$main_repo/scripts/pre-commit"
chmod +x "$main_repo/scripts/pre-commit"
git -C "$main_repo" add Makefile scripts/pre-commit
git -C "$main_repo" commit --quiet -m "test fixture"
git -C "$main_repo" worktree add --quiet -b hook-test "$linked_worktree"

make --silent -C "$linked_worktree" ensure-hooks
hook_path=$(git -C "$linked_worktree" rev-parse --git-path hooks/pre-commit)
test -L "$hook_path"
test -e "$hook_path"

rm "$hook_path"
ln -s "$test_root/missing-hook" "$hook_path"
if make --silent -C "$linked_worktree" ensure-hooks >/dev/null 2>&1; then
	echo "ensure-hooks accepted a dangling hook symlink" >&2
	exit 1
fi

echo "Hook installer worktree checks passed."
