#!/bin/bash
set -e

echo "🐹 Publishing Go Client"
echo "======================="

# Check if we're in the right directory
if [ ! -f "go.mod" ]; then
    echo "❌ Error: go.mod not found"
    exit 1
fi

# Get current version from go.mod or use git tags
MODULE=$(grep '^module ' go.mod | awk '{print $2}')
echo "📦 Module: $MODULE"

# Check if git is initialized
if [ ! -d ".git" ]; then
    echo ""
    echo "⚠️  This directory is not a git repository."
    echo "For Go modules, you need to:"
    echo "  1. Create a separate repository at github.com/ekoDB/ekodb-client-go"
    echo "  2. Copy the Go client files to that repository"
    echo "  3. Release by merging a version cap (chore(*): vX.Y.Z) to main; CI cuts the tag"
    echo ""
    echo "Steps to publish:"
    echo "  1. git init"
    echo "  2. git add ."
    echo "  3. git commit -m 'Initial commit'"
    echo "  4. git remote add origin git@github.com:ekoDB/ekodb-client-go.git"
    echo "  5. git push -u origin main"
    echo "  6. make bump-version VERSION=X.Y.Z, commit CHANGELOG.md and version.json as chore(*): vX.Y.Z, and push main; CI tags and publishes the Release"
    echo ""
    echo "After that, users can install with:"
    echo "  go get github.com/ekoDB/ekodb-client-go@v0.1.0"
    exit 0
fi

# Run tests
echo ""
echo "🧪 Running tests..."
go test -v ./...

# Run go mod tidy
echo ""
echo "🧹 Running go mod tidy..."
go mod tidy

# Check for uncommitted changes
if [[ -n $(git status -s) ]]; then
    echo ""
    echo "⚠️  You have uncommitted changes:"
    git status -s
    echo ""
    read -p "Do you want to commit these changes? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        git add .
        read -p "Enter commit message: " commit_msg
        git commit -m "$commit_msg"
    fi
fi

# The version is the manifest's: version.json, stamped by `make bump-version`.
NEW_VERSION="v$(sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' version.json)"
if [[ ! $NEW_VERSION =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "❌ Error: version.json does not carry a plain X.Y.Z version (read '${NEW_VERSION#v}'); run make bump-version VERSION=X.Y.Z first"
    exit 1
fi
echo ""
echo "📦 Version (version.json): $NEW_VERSION"

# The tag is cut by CI. A cap commit `chore(*): vX.Y.Z` on main triggers
# .github/workflows/release.yml, which tags, publishes the Release and runs
# `make index-release`. This script only pushes main.
# The same shape release-cap-detect.sh accepts: any scope, or none.
subject="$(git log -1 --format=%s)"
cap_re="^chore(\\([^)]*\\))?: ${NEW_VERSION//./\\.}\$"
if ! [[ "$subject" =~ $cap_re ]]; then
    echo "❌ The head commit '$subject' is not the cap 'chore(<scope>): $NEW_VERSION' that CI tags. Cut the cap first (make bump-version VERSION=${NEW_VERSION#v}, then commit CHANGELOG.md and version.json as chore(*): $NEW_VERSION), then run this from main."
    exit 1
fi
branch="$(git rev-parse --abbrev-ref HEAD)"
if [[ "$branch" != "main" ]]; then
    echo "❌ On '$branch', not main. CI tags a cap only when it reaches main; merge it there first, then run this from main."
    exit 1
fi
echo ""
echo "🚀 Pushing main; CI tags $NEW_VERSION and publishes the Release..."
git push origin main
echo "📚 Watch: gh run list --repo ekoDB/ekodb-client-go --workflow release.yml --limit 1"

echo ""
echo "✅ main carries the cap; CI cuts $NEW_VERSION, publishes the Release and indexes it. Nothing is published until that run is green."
echo "📦 Then users can install with: go get $MODULE@$NEW_VERSION"
echo "📚 And the docs render at: https://pkg.go.dev/$MODULE@$NEW_VERSION"
