# Changelog

All notable changes to ekodb-client-go will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.11.1] - 2026-02-14

### Fixed

- **Thread-safe token management** — Added `sync.RWMutex` to protect the `token`
  field on `Client`, eliminating a data race where concurrent goroutines could
  read stale tokens indefinitely after a server restart
- **Double-check token refresh** — `refreshTokenIfStale()` skips redundant HTTP
  refresh calls when another goroutine has already refreshed the token,
  preventing thundering herd 401 errors on the server after instance restarts
- **WebSocket token read** — `WebSocket()` now reads the token via the
  thread-safe `getToken()` accessor instead of directly accessing the field

## [0.11.0] - 2026-02-08

### Added

- **Chat Models API** — Query available AI models across providers:
  - `GetChatModels()` — Retrieve all available chat models from all providers
    (OpenAI, Anthropic, Perplexity)
  - `GetChatModel(providerName string)` — Retrieve available models for a
    specific provider
  - `GetChatMessage(sessionID, messageID string)` — Get a specific chat message
    by ID
  - `ChatModels` struct — Contains lists of available models by provider
- **User Functions API** — Reusable function sequences with lifecycle
  management:
  - `SaveUserFunction(userFunction UserFunction)` — Create a new reusable user
    function
  - `GetUserFunction(label string)` — Retrieve a user function by label
  - `ListUserFunctions(tags []string)` — List all user functions, optionally
    filtered by tags
  - `UpdateUserFunction(label string, userFunction UserFunction)` — Update an
    existing user function
  - `DeleteUserFunction(label string)` — Delete a user function
  - `UserFunction` struct — Label, Name, Description, Version, Parameters,
    Functions, Tags, ID, CreatedAt, UpdatedAt
- **Collection utilities**:
  - `CollectionExists(collection string)` — Check if a collection exists
    (returns bool)
  - `CountDocuments(collection string)` — Count all documents in a collection

### Changed

- Updated README with `CountDocuments` return type and `GetChatModels` signature

## [0.10.0] - 2026-01-27

### Changed

- **Breaking**: `StageKvGet` signature simplified — removed `outputField`
  parameter
  - Old: `StageKvGet(key string, outputField *string)`
  - New: `StageKvGet(key string)`
  - Returns `{value: <data>}` on hit, `{value: null}` on miss

### Fixed

- **KVBatchSet value handling** — Fixed value wrapping: now directly uses the
  entry value map instead of double-wrapping in `{"value": ...}`. Added
  validation that value is a `map[string]interface{}` and not nil

## [0.9.0] - 2026-01-27

### Added

- **Field Projection** — Control which fields are returned in query results:
  - `FindByIDWithProjection(collection, id string, selectFields, excludeFields []string)`
    — Find by ID with field whitelist/blacklist
  - `SelectFields()` / `ExcludeFields()` on `QueryBuilder` — Projection methods
    for query builder
- **KV Batch Operations** — Efficient multi-key access in single requests:
  - `KVBatchGet(keys []string)` — Retrieve multiple keys
  - `KVBatchSet(entries []map[string]interface{})` — Set multiple key-value
    pairs with optional TTL
  - `KVBatchDelete(keys []string)` — Delete multiple keys
- **StageSWR** — Stale-While-Revalidate function stage for external API caching:
  - Automatic workflow: KV cache check → HTTP request → KV cache set → optional
    audit storage
  - Supports parameter substitution (e.g., `"user:{{user_id}}"`)
  - TTL accepts duration strings (`"15m"`, `"1h"`), integers (seconds), or ISO
    timestamps
- **ScriptCondition types** — Recursive condition system for function If/control
  flow:
  - `ConditionHasRecords()`, `ConditionFieldExists(field)`,
    `ConditionFieldEquals(field, value)`
  - `ConditionCountEquals(count)`, `ConditionCountGreaterThan(count)`,
    `ConditionCountLessThan(count)`
  - `ConditionAnd(conditions)`, `ConditionOr(conditions)`,
    `ConditionNot(condition)`
  - Custom `MarshalJSON()` for adjacently-tagged serialization matching Rust
    server's serde format

### Changed

- **Breaking**: `ScriptCondition` JSON serialization now uses adjacently-tagged
  format
  - Old: `{"type": "FieldEquals", "field": "x", "value": "y"}`
  - New: `{"type": "FieldEquals", "value": {"field": "x", "value": "y"}}`
  - `HasRecords` remains simple: `{"type": "HasRecords"}`

### Testing

- Added `projection_test.go` — QueryBuilder projection and
  FindByIDWithProjection tests (294 lines)
- Added `client_kv_batch_test.go` — KV batch operation tests (205 lines)
- Added `condition_test.go` — ScriptCondition serialization tests (330 lines)
- Added `swr_test.go` — StageSWR serialization and format tests (243 lines)

## [0.8.0] - 2026-01-06

### Added

- **Options structs** — Variadic options pattern for cleaner, extensible method
  signatures:
  - `InsertOptions` — TTL, BypassRipple, TransactionId, BypassCache
  - `UpdateOptions` — BypassRipple, TransactionId, BypassCache, SelectFields,
    ExcludeFields
  - `DeleteOptions` — BypassRipple, TransactionId
  - `FindOptions` — Filter, Sort, Limit, Skip, Join, BypassCache, BypassRipple,
    SelectFields, ExcludeFields
  - `UpsertOptions` — TTL, BypassRipple, TransactionId, BypassCache
  - `BatchInsertOptions`, `BatchUpdateOptions`, `BatchDeleteOptions`
- **Convenience methods** for ergonomic API usage:
  - `Upsert(collection, id string, record Record, opts ...UpsertOptions)` —
    Atomic insert-or-update (tries update first, falls back to insert on 404)
  - `FindOne(collection, field string, value interface{})` — Find single record
    by field value
  - `Exists(collection, id string)` — Check if record exists by ID (returns
    bool)
  - `Paginate(collection string, page, pageSize int)` — Paginated retrieval
    (1-indexed pages)
  - `RestoreRecord(collection, id string)` — Restore a deleted record from trash
  - `RestoreCollection(collection string)` — Restore all deleted records in a
    collection
- **Search projection** — Added `SelectFields` and `ExcludeFields` to
  `SearchQuery` and `SearchQueryBuilder`

### Changed

- **Breaking**: All CRUD method signatures now accept variadic options structs
  instead of positional parameters:
  - `Insert(collection, record, opts ...InsertOptions)` (was `...string` for
    TTL)
  - `Update(collection, id, record, opts ...UpdateOptions)`
  - `Delete(collection, id, opts ...DeleteOptions)`
  - `Find(collection, query, opts ...FindOptions)`
  - `BatchInsert(collection, records, opts ...BatchInsertOptions)`
  - `BatchUpdate(collection, updates, opts ...BatchUpdateOptions)`
  - `BatchDelete(collection, ids, opts ...BatchDeleteOptions)`

### Testing

- Added `convenience_test.go` — Tests for Upsert, FindOne, Exists, Paginate (184
  lines)
- Comprehensive client tests for all new option structs and convenience methods
  (800+ lines added to `client_test.go`)

## [0.7.1] - 2026-01-03

### Added

- Comprehensive unit tests across all client methods
- Test coverage for CRUD operations, batch operations, collections, KV store,
  transactions, search, functions, and chat operations
- Unit test suite in `client_test.go` and `query_builder_test.go`

### Fixed

- Standardized isolation level constants and validation
- Error handling improvements across all operations
- Transaction isolation level type consistency

## [0.7.0] - 2026-01-03

### Added

- Transaction support with full CRUD operations
- Transaction isolation levels (ReadUncommitted, ReadCommitted, RepeatableRead,
  Serializable)
- Savepoint support for nested transactions
- KV store utilities:
  - `KVExists()` - Check if key exists
  - `KVIncrement()` - Atomic counter increment
  - `KVDecrement()` - Atomic counter decrement
  - `KVAppend()` - Append to list values
- Dependabot configuration for automated dependency updates

### Changed

- Enhanced transaction API with better error handling
- Improved documentation for transaction methods

## [0.6.1] - 2026-01-02

### Added

- Type-specific getValue helpers for extracting values from ekoDB responses
- `getStringValue()`, `getIntValue()`, `getBoolValue()`, `getFloat64Value()`,
  `getMapValue()`, `getSliceValue()` utility functions
- Simplified value extraction from nested field structures

### Changed

- Updated examples to use new type utility functions
- Improved type safety in example code

## [0.6.0] - 2025-12-31

### Added

- **Functions and Scripts** support
  - `CreateFunction()` - Register server-side functions
  - `ExecuteFunction()` - Execute registered functions
  - `ListFunctions()` - List all available functions
  - `GetFunction()` - Get function details
  - `UpdateFunction()` - Update existing functions
  - `DeleteFunction()` - Remove functions
- Function versioning support (optional tags)
- Dynamic function examples with runtime variables
- Standardized inter-stage function composition

### Changed

- Enhanced function execution with better variable handling
- Improved function stage configuration

## [0.5.0] - 2025-12-21

### Added

- Self-improving RAG (Retrieval-Augmented Generation) helper functions
- RAG utilities in `rag_helpers.go`:
  - `CreateRAGPipeline()` - Set up RAG workflows
  - `QueryRAG()` - Execute RAG queries
  - `OptimizeRAGEmbeddings()` - Improve embeddings over time
- Enhanced documentation for RAG patterns

### Changed

- Removed example files (consolidated in main ekodb-client repository)
- Updated dependencies
- Improved README formatting

## [0.4.0] - 2025-12-20

### Added

- **Functions and Scripts** - Initial implementation
- Server-side function execution support
- Script management operations
- Function examples and documentation
- Example count tracking in README

## [0.2.0] - 2025-11-05

### Added

- **MessagePack serialization** support for binary data transfer
- **Gzip compression** for reduced bandwidth usage
- Configurable serialization format (JSON or MessagePack)
- Compression toggle for all requests
- Performance improvements with binary protocol

### Changed

- Updated client to support MessagePack + Gzip
- Enhanced test suite with compression benchmarks
- Updated dependencies:
  - Added `github.com/vmihailenco/msgpack/v5`

### Fixed

- Code formatting and linting improvements

## [0.1.4] - 2025-10-14

### Changed

- Updated Makefile with improved commands
- Enhanced README with better formatting and examples
- Documentation improvements

## [0.1.3] - 2025-10-14

### Fixed

- Removed incorrect repository references
- Corrected package documentation

## [0.1.2] - 2025-10-14

### Added

- `Regex()` query method for pattern matching
- Enhanced README with more examples

### Changed

- Improved query builder documentation

## [0.1.1] - 2025-10-14

### Fixed

- Updated `publish.sh` script for standalone repository
- Corrected publishing workflow

## [0.1.0] - 2025-10-14

### Added

- Initial Go client library release
- Core CRUD operations:
  - `Insert()`, `Find()`, `FindByID()`, `Update()`, `Delete()`
- Batch operations:
  - `BatchInsert()`, `BatchUpdate()`, `BatchDelete()`
- Collection management:
  - `CreateCollection()`, `ListCollections()`, `DeleteCollection()`
- Query builder with fluent API:
  - Filters: `Eq()`, `Ne()`, `Gt()`, `Gte()`, `Lt()`, `Lte()`, `In()`,
    `Contains()`, `StartsWith()`, `EndsWith()`
  - Sorting: `SortBy()`, `SortDesc()`
  - Pagination: `Skip()`, `Limit()`
  - Projection: `Fields()`
  - Logical operators: `And()`, `Or()`, `Not()`
- Search operations with BM25 scoring
- Schema management and validation
- WebSocket support for real-time queries
- KV store operations:
  - `KVGet()`, `KVSet()`, `KVDelete()`, `KVList()`
- TTL support for automatic document expiration
- Comprehensive error handling
- Rate limit tracking
- Retry logic with exponential backoff
- API key authentication
- Full documentation and examples

### Dependencies

- `github.com/gorilla/websocket` v1.5.3
- Go 1.24.0+
