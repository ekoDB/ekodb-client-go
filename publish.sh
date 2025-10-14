#!/bin/bash
set -e

echo "🐹 Publishing Go Client"
echo "======================="

# Get paths
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

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
    echo "  3. Tag releases with semantic versions (e.g., v0.1.0)"
    echo ""
    echo "Steps to publish:"
    echo "  1. git init"
    echo "  2. git add ."
    echo "  3. git commit -m 'Initial commit'"
    echo "  4. git remote add origin git@github.com:ekoDB/ekodb-client-go.git"
    echo "  5. git push -u origin main"
    echo "  6. git tag v0.1.0"
    echo "  7. git push origin v0.1.0"
    echo ""
    echo "After that, users can install with:"
    echo "  go get github.com/ekoDB/ekodb-client-go@v0.1.0"
    exit 0
fi

# Get latest git tag
LATEST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "none")
echo "📌 Latest tag: $LATEST_TAG"

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

# Prompt for new version
echo ""
read -p "Enter new version (e.g., v0.1.0): " NEW_VERSION

# Validate version format
if [[ ! $NEW_VERSION =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "❌ Error: Version must be in format vX.Y.Z (e.g., v0.1.0)"
    exit 1
fi

# Create and push tag
echo ""
echo "🏷️  Creating tag $NEW_VERSION..."
git tag -a "$NEW_VERSION" -m "Release $NEW_VERSION"

echo ""
echo "⚠️  Ready to push tag $NEW_VERSION to remote"
read -p "Do you want to continue? (y/N): " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "❌ Publication cancelled"
    git tag -d "$NEW_VERSION"
    exit 1
fi

# Push tag
echo ""
echo "🚀 Pushing tag to remote..."
git push origin "$NEW_VERSION"

# Also push main branch if needed
echo ""
read -p "Push main branch too? (y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    git push origin main
fi

echo ""
echo "✅ Successfully published ekodb-client-go $NEW_VERSION!"
echo "📦 Users can install with: go get $MODULE@$NEW_VERSION"
echo "📚 Documentation available at: https://pkg.go.dev/$MODULE@$NEW_VERSION"
echo ""
echo "Note: It may take a few minutes for pkg.go.dev to index the new version."
