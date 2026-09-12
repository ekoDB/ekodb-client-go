#!/bin/sh
set -eu

# Git exports repository-local environment variables while invoking hooks.
# Without clearing them, the fixture's nested Git commands can accidentally
# operate on the repository whose hook is under test even when `git -C` points
# at the temporary repository.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_PREFIX GIT_OBJECT_DIRECTORY
unset GIT_ALTERNATE_OBJECT_DIRECTORIES

test_root=$(mktemp -d "${TMPDIR:-/tmp}/ekodb-hooks-test.XXXXXX")
trap 'rm -rf "$test_root"' EXIT HUP INT TERM

main_repo="$test_root/main"
linked_worktree="$test_root/worktree"

git init --quiet "$main_repo"
git -C "$main_repo" config user.email "hooks-test@example.invalid"
git -C "$main_repo" config user.name "Hook Test"
# Keep the fixture independent from any repository-level or global hooks path
# inherited by the process that launched this test.
mkdir -p "$main_repo/.git/hooks"
git -C "$main_repo" config core.hooksPath "$main_repo/.git/hooks"
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
