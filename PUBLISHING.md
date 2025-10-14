# Publishing ekoDB Go Client

This Go client will be moved to a separate public repository for distribution
via Go modules.

## Current Status

🚧 **In Development** - Currently part of the ekoDB monorepo

## Future Publishing Strategy

### When Moving to Separate Repository

1. **Create Public Repository**

   ```bash
   # Create new repo at: github.com/ekoDB/ekodb-client-go
   ```

2. **Copy Files**

   ```bash
   # Copy all Go client files to the new repository
   cp -r ekodb-client-go/* /path/to/new/ekodb-client-go/
   ```

3. **Initialize and Push**

   ```bash
   cd /path/to/new/ekodb-client-go
   git init
   git add .
   git commit -m "Initial commit: ekoDB Go client"
   git remote add origin git@github.com:ekoDB/ekodb-client-go.git
   git push -u origin main
   ```

4. **Tag and Publish**

   ```bash
   # Use the publish.sh script
   ./publish.sh

   # Or manually:
   git tag v0.1.0
   git push origin v0.1.0
   ```

## Publishing from Separate Repository

Once in its own repository, use the `publish.sh` script:

```bash
./publish.sh
```

This script will:

- Run tests
- Create a semantic version tag
- Push the tag to GitHub
- Automatically indexed by pkg.go.dev

## Installation (After Publishing)

Users will install with:

```bash
go get github.com/ekoDB/ekodb-client-go@v0.1.0
```

## Why Separate Repository?

Go modules work best with dedicated repositories because:

- Go pulls the entire repository by default
- Keeping it separate prevents users from downloading the server code
- Standard practice in the Go ecosystem
- Cleaner dependency management

## Current Development

For now, the Go client is developed in the monorepo alongside other clients. The
`publish.sh` script is ready for when we move to a separate repository.
