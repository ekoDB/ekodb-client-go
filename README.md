# ekoDB Go Client 🔷

[![Go Reference](https://pkg.go.dev/badge/github.com/ekoDB/ekodb-client-go.svg)](https://pkg.go.dev/github.com/ekoDB/ekodb-client-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/ekoDB/ekodb-client-go)](https://goreportcard.com/report/github.com/ekoDB/ekodb-client-go)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Official Go client library for ekoDB - a high-performance document database.

## 📦 Installation

```bash
go get github.com/ekoDB/ekodb-client-go
```

## 🚀 Quick Start

### Basic Usage

```go
package main

import (
    "fmt"
    "log"

    "github.com/ekoDB/ekodb-client-go"
)

func main() {
    // Create client with default configuration
    client, err := ekodb.NewClient("http://localhost:8080", "your-api-key")
    if err != nil {
        log.Fatal(err)
    }

    // Insert a document
    record := ekodb.Record{
        "name":  "John Doe",
        "email": "john@example.com",
        "age":   30,
    }

    inserted, err := client.Insert("users", record)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Inserted: %+v\n", inserted)

    // Check rate limit status
    if rateLimitInfo := client.GetRateLimitInfo(); rateLimitInfo != nil {
        fmt.Printf("Rate limit: %d/%d remaining (%.1f%%)\n",
            rateLimitInfo.Remaining, rateLimitInfo.Limit, rateLimitInfo.RemainingPercentage())
    }

    // Find documents
    query := ekodb.Query{
        Limit: intPtr(10),
    }
    results, err := client.Find("users", query)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Found %d documents\n", len(results))
}

func intPtr(i int) *int {
    return &i
}
```

### Advanced Configuration

```go
package main

import (
    "errors"
    "fmt"
    "log"
    "time"

    "github.com/ekoDB/ekodb-client-go"
)

func main() {
    // Create client with custom configuration
    client, err := ekodb.NewClientWithConfig(ekodb.ClientConfig{
        BaseURL:    "http://localhost:8080",
        APIKey:     "your-api-key",
        ShouldRetry: true,              // Enable automatic retries (default: true)
        MaxRetries: 3,                  // Maximum retry attempts (default: 3)
        Timeout:    30 * time.Second,   // Request timeout (default: 30s)
    })
    if err != nil {
        log.Fatal(err)
    }

    // Insert with error handling
    record := ekodb.Record{
        "name":  "John Doe",
        "email": "john@example.com",
    }

    inserted, err := client.Insert("users", record)
    if err != nil {
        // Check if it's a rate limit error
        var rateLimitErr *ekodb.RateLimitError
        if errors.As(err, &rateLimitErr) {
            fmt.Printf("Rate limited! Retry after %d seconds\n", rateLimitErr.RetryAfterSecs)
            // Handle rate limiting manually if needed
            return
        }
        log.Fatal(err)
    }
    fmt.Printf("Inserted: %+v\n", inserted)

    // Check if approaching rate limit
    if client.IsNearRateLimit() {
        fmt.Println("Warning: Approaching rate limit!")
    }
}
```

## ✨ Features

- ✅ CRUD operations
- ✅ Batch operations
- ✅ Key-value operations
- ✅ Collection management
- ✅ WebSocket support
- ✅ TTL support
- ✅ Automatic token management
- ✅ **Query Builder** - Fluent API for complex queries with operators, sorting,
  and pagination
- ✅ **Search** - Full-text search, fuzzy search, and field-specific search with
  scoring
- ✅ **Schema Management** - Define and enforce data schemas with validation
- ✅ **Join Operations** - Single and multi-collection joins with queries
- ✅ **Rate limiting with automatic retry** (429, 503, network errors)
- ✅ **Rate limit tracking** (`X-RateLimit-*` headers)
- ✅ **Configurable retry behavior**
- ✅ **Retry-After header support**

## 📖 Usage Examples

### Query Builder

```go
// Simple query with operators
query := ekodb.NewQueryBuilder().
    Eq("status", "active").
    Gte("age", 18).
    Lt("age", 65).
    Limit(10).
    Build()

results, err := client.Find("users", query)

// Complex query with sorting and pagination
query := ekodb.NewQueryBuilder().
    In("status", []interface{}{"active", "pending"}).
    Contains("email", "@example.com").
    SortDescending("created_at").
    Skip(20).
    Limit(10).
    Build()

results, err := client.Find("users", query)
```

### Search Operations

```go
// Basic text search
searchQuery := ekodb.SearchQuery{
    Query:    "programming",
    MinScore: 0.1,
    Limit:    10,
}

results, err := client.Search("articles", searchQuery)
for _, result := range results.Results {
    fmt.Printf("Score: %.4f - %v\n", result.Score, result.Record["title"])
}

// Search with field weights
searchQuery := ekodb.SearchQuery{
    Query:   "rust database",
    Fields:  []string{"title", "description"},
    Weights: map[string]float64{"title": 2.0},
    Limit:   5,
}

results, err := client.Search("articles", searchQuery)
```

### Schema Management

```go
// Create a collection with schema
schema := ekodb.NewSchemaBuilder().
    AddField("name", ekodb.NewFieldTypeSchemaBuilder("String").
        Required().
        Pattern("^[a-zA-Z ]+$").
        Build()).
    AddField("email", ekodb.NewFieldTypeSchemaBuilder("String").
        Required().
        Unique().
        Build()).
    AddField("age", ekodb.NewFieldTypeSchemaBuilder("Integer").
        Range(0, 150).
        Build()).
    Build()

err := client.CreateCollection("users", schema)

// Get collection schema
schema, err := client.GetSchema("users")
```

### Join Operations

```go
// Single collection join
join := ekodb.NewSingleJoin("departments", "department_id", "id", "department")

query := ekodb.NewQueryBuilder().
    Join(join.ToMap()).
    Limit(10).
    Build()

results, err := client.Find("users", query)

// Multi-collection join
join := ekodb.NewJoinConfig(
    []string{"departments", "profiles"},
    "department_id",
    "id",
    "related_data",
)

query := ekodb.NewQueryBuilder().
    Join(join.ToMap()).
    Build()

results, err := client.Find("users", query)
```

## 📚 API Reference

### Client Creation

- `NewClient(baseURL, apiKey string) (*Client, error)` - Create client with
  default configuration
- `NewClientWithConfig(config ClientConfig) (*Client, error)` - Create client
  with custom configuration

### Rate Limit Methods

- `GetRateLimitInfo() *RateLimitInfo` - Get current rate limit information
- `IsNearRateLimit() bool` - Check if approaching rate limit (<10% remaining)

### CRUD Methods

- `Insert(collection string, record Record, ttl ...string) (Record, error)`
- `Find(collection string, query Query) ([]Record, error)`
- `FindByID(collection, id string) (Record, error)`
- `Update(collection, id string, record Record) (Record, error)`
- `Delete(collection, id string) error`
- `BatchInsert(collection string, records []Record) ([]Record, error)`
- `BatchUpdate(collection string, updates map[string]Record) ([]Record, error)`
- `BatchDelete(collection string, ids []string) (int, error)`

### Query Builder Methods

- `NewQueryBuilder() *QueryBuilder` - Create a new query builder
- `Eq(field, value)` - Equal to
- `Ne(field, value)` - Not equal to
- `Gt(field, value)` - Greater than
- `Gte(field, value)` - Greater than or equal
- `Lt(field, value)` - Less than
- `Lte(field, value)` - Less than or equal
- `In(field, values)` - In array
- `Nin(field, values)` - Not in array
- `Contains(field, value)` - String contains
- `StartsWith(field, value)` - String starts with
- `EndsWith(field, value)` - String ends with
- `Regex(field, pattern)` - Regex match
- `SortAscending(field)` / `SortDescending(field)` - Sorting
- `Limit(n)` / `Skip(n)` - Pagination
- `Build()` - Build the query

### Search Methods

- `Search(collection string, query SearchQuery) (*SearchResults, error)` -
  Full-text search

### Schema Methods

- `CreateCollection(collection string, schema Schema) error` - Create collection
  with schema
- `GetSchema(collection string) (*Schema, error)` - Get collection schema
- `GetCollection(collection string) (*CollectionMetadata, error)` - Get
  collection metadata

### Join Methods

- `NewSingleJoin(collection, localField, foreignField, as string) *JoinConfig` -
  Single collection join
- `NewJoinConfig(collections []string, localField, foreignField, as string) *JoinConfig` -
  Multi-collection join

### Key-Value Methods

- `KVSet(key string, value interface{}) error`
- `KVGet(key string) (interface{}, error)`
- `KVDelete(key string) error`

### Collection Methods

- `ListCollections() ([]string, error)`
- `DeleteCollection(collection string) error`

### WebSocket Methods

- `WebSocket(wsURL string) (*WebSocketClient, error)`
- `FindAll(collection string) ([]Record, error)`
- `Close() error`

## 💡 Examples

For complete, runnable examples, visit the [ekoDB Examples Repository](https://github.com/ekoDB/ekodb-client/tree/main/examples/go).

The repository contains **12 working Go examples**:

### Basic Operations
- **[client_simple_crud.go](https://github.com/ekoDB/ekodb-client/blob/main/examples/go/client_simple_crud.go)** - Basic CRUD operations
- **[client_collection_management.go](https://github.com/ekoDB/ekodb-client/blob/main/examples/go/client_collection_management.go)** - Collection management
- **[client_kv_operations.go](https://github.com/ekoDB/ekodb-client/blob/main/examples/go/client_kv_operations.go)** - Key-value operations

### Advanced Queries
- **[client_query_builder.go](https://github.com/ekoDB/ekodb-client/blob/main/examples/go/client_query_builder.go)** - Complex queries with QueryBuilder
- **[client_search.go](https://github.com/ekoDB/ekodb-client/blob/main/examples/go/client_search.go)** - Full-text search operations
- **[client_joins.go](https://github.com/ekoDB/ekodb-client/blob/main/examples/go/client_joins.go)** - Join operations

### Schema & Batch
- **[client_schema.go](https://github.com/ekoDB/ekodb-client/blob/main/examples/go/client_schema.go)** - Schema management
- **[client_batch_operations.go](https://github.com/ekoDB/ekodb-client/blob/main/examples/go/client_batch_operations.go)** - Batch operations

### Real-time & TTL
- **[client_simple_websocket.go](https://github.com/ekoDB/ekodb-client/blob/main/examples/go/client_simple_websocket.go)** - WebSocket queries
- **[client_websocket_ttl.go](https://github.com/ekoDB/ekodb-client/blob/main/examples/go/client_websocket_ttl.go)** - WebSocket with TTL
- **[client_document_ttl.go](https://github.com/ekoDB/ekodb-client/blob/main/examples/go/client_document_ttl.go)** - Document TTL

### Chat Examples
- **[client_chat_basic.go](https://github.com/ekoDB/ekodb-client/blob/main/examples/go/client_chat_basic.go)** - Basic chat application
- **[client_chat_sessions.go](https://github.com/ekoDB/ekodb-client/blob/main/examples/go/client_chat_sessions.go)** - Chat with sessions
- **[client_chat_advanced.go](https://github.com/ekoDB/ekodb-client/blob/main/examples/go/client_chat_advanced.go)** - Advanced chat features

### Running Examples

```bash
# Clone the examples repository
git clone https://github.com/ekoDB/ekodb-client.git
cd ekodb-client/examples/go

# Set up environment
cp ../.env.example ../.env
# Edit ../.env with your API URL and key

# Run an example
go run client_simple_crud.go
```

## 📚 Documentation

- **[Official Documentation](https://docs.ekodb.io)** - Complete guides and tutorials
- **[API Reference](https://pkg.go.dev/github.com/ekoDB/ekodb-client-go)** - Go package documentation
- **[Examples Repository](https://github.com/ekoDB/ekodb-client/tree/main/examples)** - 78 examples across all languages

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## 📄 License

MIT License - see the [LICENSE](LICENSE) file for details.

## 💬 Support

- 📧 Email: [support@ekodb.io](mailto:support@ekodb.io)
- 🐛 Issues: [GitHub Issues](https://github.com/ekoDB/ekodb-client-go/issues)
- 📖 Documentation: [docs.ekodb.io](https://docs.ekodb.io)
