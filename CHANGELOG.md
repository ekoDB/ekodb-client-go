# Changelog

All notable changes to ekodb-client-go will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

#### Core Features

- **Convenience methods** for ergonomic API usage:
  - `Upsert()` - Insert or update with automatic fallback (atomic
    insert-or-update semantics)
  - `FindOne()` - Find single record by field value
  - `Exists()` - Efficient existence check without fetching full record
  - `Paginate()` - Simplified pagination with page/pageSize parameters
  - `TextSearch()` - Full-text search helper

#### API Improvements

- **Options structs** for cleaner method signatures:
  - `InsertOptions` - TTL, BypassRipple, TransactionID, BypassCache
  - `UpdateOptions` - BypassRipple, TransactionID, BypassCache, SelectFields,
    ExcludeFields
  - `UpsertOptions` - TTL, BypassRipple, TransactionID, BypassCache
  - `DeleteOptions` - BypassRipple, TransactionID
  - `FindOptions` - BypassCache, TransactionID
  - `BatchInsertOptions`, `BatchUpdateOptions`, `BatchDeleteOptions` - options
    for batch operations
- Variadic options pattern for idiomatic Go API

#### Field Projection

- **QueryBuilder projection methods**:
  - `SelectFields()` - Specify which fields to return (whitelist)
  - `ExcludeFields()` - Specify which fields to exclude (blacklist)
- `FindByIDWithProjection()` - Find by ID with field projection support

#### KV Batch Operations

- **Batch KV operations** for efficient multi-key access:
  - `KVBatchGet()` - Retrieve multiple keys in a single request
  - `KVBatchSet()` - Set multiple key-value pairs atomically
  - `KVBatchDelete()` - Delete multiple keys in a single request

#### Function Stages

- **StageSWR** - Stale-While-Revalidate pattern for external API caching
  - Automatic cache check → HTTP request → cache set workflow
  - Optional audit trail storage
  - Supports duration strings, integers, or ISO timestamps for TTL

#### Testing & Quality

- Comprehensive unit tests for all convenience methods (50+ new tests in
  `convenience_test.go`)
- Test coverage for options and edge cases
- Integration with existing test suite

#### Script Conditions

- **ScriptCondition types** for function If/control flow:
  - `ConditionHasRecords()` - Check if working data has records
  - `ConditionFieldExists(field)` - Check if field exists in records
  - `ConditionFieldEquals(field, value)` - Check if field equals value
  - `ConditionCountEquals(count)` - Check if record count equals value
  - `ConditionCountGreaterThan(count)` - Check if record count is greater
  - `ConditionCountLessThan(count)` - Check if record count is less
  - `ConditionAnd(conditions)` - Logical AND of multiple conditions
  - `ConditionOr(conditions)` - Logical OR of multiple conditions
  - `ConditionNot(condition)` - Logical NOT of a condition
- Comprehensive unit tests for ScriptCondition serialization
  (`condition_test.go`)

### Fixed

### Changed

- **Breaking**: `ScriptCondition` JSON serialization now uses adjacently-tagged
  format
  - Old format: `{"type": "FieldEquals", "field": "x", "value": "y"}`
  - New format: `{"type": "FieldEquals", "value": {"field": "x", "value": "y"}}`
  - This matches the Rust server's serde adjacently-tagged enum format
  - `HasRecords` remains simple: `{"type": "HasRecords"}`
- Updated examples to use new convenience methods where appropriate
- Improved error messages and documentation
- Enhanced type safety with stricter option types

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
