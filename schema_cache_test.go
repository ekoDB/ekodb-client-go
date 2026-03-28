package ekodb

import (
	"sync"
	"testing"
	"time"
)

func enabledCacheConfig() SchemaCacheConfig {
	return SchemaCacheConfig{
		Enabled:    true,
		MaxEntries: 3,
		TTL:        60 * time.Second,
	}
}

func TestSchemaCacheInsertAndGet(t *testing.T) {
	cache := NewSchemaCache(enabledCacheConfig())
	cache.Insert("users", "id", 1)

	entry := cache.Get("users")
	if entry == nil {
		t.Fatal("expected entry, got nil")
	}
	if entry.PrimaryKeyAlias != "id" {
		t.Errorf("expected alias 'id', got '%s'", entry.PrimaryKeyAlias)
	}
	if entry.Version != 1 {
		t.Errorf("expected version 1, got %d", entry.Version)
	}
}

func TestSchemaCacheGetAlias(t *testing.T) {
	cache := NewSchemaCache(enabledCacheConfig())
	cache.Insert("orders", "order_id", 1)

	if alias := cache.GetAlias("orders"); alias != "order_id" {
		t.Errorf("expected 'order_id', got '%s'", alias)
	}
	if alias := cache.GetAlias("nonexistent"); alias != "" {
		t.Errorf("expected empty, got '%s'", alias)
	}
}

func TestSchemaCacheTTLExpiry(t *testing.T) {
	config := SchemaCacheConfig{
		Enabled:    true,
		MaxEntries: 10,
		TTL:        1 * time.Millisecond,
	}
	cache := NewSchemaCache(config)
	cache.Insert("users", "id", 1)

	time.Sleep(5 * time.Millisecond)

	if entry := cache.Get("users"); entry != nil {
		t.Error("expected nil after TTL expiry")
	}
	if alias := cache.GetAlias("users"); alias != "" {
		t.Errorf("expected empty alias after TTL expiry, got '%s'", alias)
	}
}

func TestSchemaCacheInvalidateSingle(t *testing.T) {
	cache := NewSchemaCache(enabledCacheConfig())
	cache.Insert("users", "id", 1)
	cache.Insert("orders", "id", 1)

	cache.Invalidate("users")

	if cache.Get("users") != nil {
		t.Error("expected nil after invalidation")
	}
	if cache.Get("orders") == nil {
		t.Error("orders should still be cached")
	}
	if cache.Len() != 1 {
		t.Errorf("expected len 1, got %d", cache.Len())
	}
}

func TestSchemaCacheInvalidateAll(t *testing.T) {
	cache := NewSchemaCache(enabledCacheConfig())
	cache.Insert("users", "id", 1)
	cache.Insert("orders", "id", 1)

	cache.InvalidateAll()

	if cache.Len() != 0 {
		t.Errorf("expected len 0 after invalidate all, got %d", cache.Len())
	}
}

func TestSchemaCacheLRUEviction(t *testing.T) {
	config := SchemaCacheConfig{
		Enabled:    true,
		MaxEntries: 2,
		TTL:        60 * time.Second,
	}
	cache := NewSchemaCache(config)

	cache.Insert("a", "id", 1)
	cache.Insert("b", "id", 1)
	// Cache full — inserting "c" should evict "a" (oldest)
	cache.Insert("c", "id", 1)

	if cache.Get("a") != nil {
		t.Error("'a' should have been evicted")
	}
	if cache.Get("b") == nil {
		t.Error("'b' should still be cached")
	}
	if cache.Get("c") == nil {
		t.Error("'c' should still be cached")
	}
	if cache.Len() != 2 {
		t.Errorf("expected len 2, got %d", cache.Len())
	}
}

func TestSchemaCacheLRUTouchPreventsEviction(t *testing.T) {
	config := SchemaCacheConfig{
		Enabled:    true,
		MaxEntries: 2,
		TTL:        60 * time.Second,
	}
	cache := NewSchemaCache(config)

	cache.Insert("a", "id", 1)
	cache.Insert("b", "id", 1)
	// Touch "a" — makes it most recently used
	cache.Get("a")
	// Insert "c" — should evict "b" (now oldest), not "a"
	cache.Insert("c", "id", 1)

	if cache.Get("a") == nil {
		t.Error("'a' was touched, should survive eviction")
	}
	if cache.Get("b") != nil {
		t.Error("'b' should have been evicted")
	}
	if cache.Get("c") == nil {
		t.Error("'c' should still be cached")
	}
}

func TestSchemaCacheHandleSchemaChangedUpdates(t *testing.T) {
	cache := NewSchemaCache(enabledCacheConfig())
	cache.Insert("users", "id", 1)

	// Newer version should update
	cache.HandleSchemaChanged("users", 2, "user_id")

	entry := cache.Get("users")
	if entry == nil {
		t.Fatal("expected entry after schema changed")
	}
	if entry.PrimaryKeyAlias != "user_id" {
		t.Errorf("expected 'user_id', got '%s'", entry.PrimaryKeyAlias)
	}
	if entry.Version != 2 {
		t.Errorf("expected version 2, got %d", entry.Version)
	}
}

func TestSchemaCacheHandleSchemaChangedIgnoresOlder(t *testing.T) {
	cache := NewSchemaCache(enabledCacheConfig())
	cache.Insert("users", "user_id", 5)

	// Older version should be ignored
	cache.HandleSchemaChanged("users", 3, "id")

	entry := cache.Get("users")
	if entry == nil {
		t.Fatal("expected entry")
	}
	if entry.PrimaryKeyAlias != "user_id" {
		t.Errorf("expected 'user_id' unchanged, got '%s'", entry.PrimaryKeyAlias)
	}
	if entry.Version != 5 {
		t.Errorf("expected version 5 unchanged, got %d", entry.Version)
	}
}

func TestSchemaCacheHandleSchemaChangedInsertsNew(t *testing.T) {
	cache := NewSchemaCache(enabledCacheConfig())

	cache.HandleSchemaChanged("new_coll", 1, "_id")

	entry := cache.Get("new_coll")
	if entry == nil {
		t.Fatal("expected new entry from schema changed")
	}
	if entry.PrimaryKeyAlias != "_id" {
		t.Errorf("expected '_id', got '%s'", entry.PrimaryKeyAlias)
	}
}

func TestSchemaCacheDisabledReturnsNil(t *testing.T) {
	cache := NewDisabledSchemaCache()
	cache.Insert("users", "id", 1) // no-op

	if cache.Get("users") != nil {
		t.Error("disabled cache should return nil")
	}
	if cache.GetAlias("users") != "" {
		t.Error("disabled cache should return empty alias")
	}
	if cache.Len() != 0 {
		t.Errorf("disabled cache should have len 0, got %d", cache.Len())
	}
}

func TestSchemaCacheRuntimeEnableDisable(t *testing.T) {
	cache := NewSchemaCache(enabledCacheConfig())
	cache.Insert("users", "id", 1)

	if cache.Get("users") == nil {
		t.Error("expected entry when enabled")
	}

	cache.SetEnabled(false)
	if cache.Get("users") != nil {
		t.Error("disabled cache should return nil")
	}

	cache.SetEnabled(true)
	if cache.Get("users") == nil {
		t.Error("re-enabled cache should return entry")
	}
}

func TestSchemaCacheUpdateExistingEntry(t *testing.T) {
	cache := NewSchemaCache(enabledCacheConfig())
	cache.Insert("users", "id", 1)
	cache.Insert("users", "user_id", 2)

	entry := cache.Get("users")
	if entry == nil {
		t.Fatal("expected entry")
	}
	if entry.PrimaryKeyAlias != "user_id" {
		t.Errorf("expected 'user_id', got '%s'", entry.PrimaryKeyAlias)
	}
	if entry.Version != 2 {
		t.Errorf("expected version 2, got %d", entry.Version)
	}
	if cache.Len() != 1 {
		t.Errorf("expected len 1 (no duplicate), got %d", cache.Len())
	}
}

func TestSchemaCacheExtractRecordIDWithAlias(t *testing.T) {
	cache := NewSchemaCache(enabledCacheConfig())
	cache.Insert("users", "user_id", 1)

	record := map[string]interface{}{
		"user_id": "abc123",
		"name":    "Alice",
	}
	id := cache.ExtractRecordID("users", record)
	if id != "abc123" {
		t.Errorf("expected 'abc123', got '%s'", id)
	}
}

func TestSchemaCacheExtractRecordIDTypedWrapper(t *testing.T) {
	cache := NewSchemaCache(enabledCacheConfig())
	cache.Insert("users", "user_id", 1)

	// Alias path with typed wrapper
	record := map[string]interface{}{
		"user_id": map[string]interface{}{"type": "String", "value": "wrapped-alias"},
	}
	id := cache.ExtractRecordID("users", record)
	if id != "wrapped-alias" {
		t.Errorf("expected 'wrapped-alias', got '%s'", id)
	}

	// Fallback path with typed wrapper (no alias match)
	cache2 := NewSchemaCache(enabledCacheConfig())
	record2 := map[string]interface{}{
		"id": map[string]interface{}{"type": "String", "value": "wrapped-fallback"},
	}
	id2 := cache2.ExtractRecordID("unknown", record2)
	if id2 != "wrapped-fallback" {
		t.Errorf("expected 'wrapped-fallback', got '%s'", id2)
	}
}

func TestSchemaCacheExtractRecordIDFallback(t *testing.T) {
	cache := NewSchemaCache(enabledCacheConfig())
	// No alias cached for "orders"

	record := map[string]interface{}{
		"_id":  "order-456",
		"name": "Order 1",
	}
	id := cache.ExtractRecordID("orders", record)
	if id != "order-456" {
		t.Errorf("expected 'order-456', got '%s'", id)
	}
}

func TestSchemaCacheExtractRecordIDMissing(t *testing.T) {
	cache := NewSchemaCache(enabledCacheConfig())

	record := map[string]interface{}{
		"name": "No ID here",
	}
	id := cache.ExtractRecordID("users", record)
	if id != "" {
		t.Errorf("expected empty string, got '%s'", id)
	}
}

func TestSchemaCacheConcurrentAccess(t *testing.T) {
	config := SchemaCacheConfig{
		Enabled:    true,
		MaxEntries: 100,
		TTL:        60 * time.Second,
	}
	cache := NewSchemaCache(config)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			name := "coll_" + string(rune('a'+n))
			cache.Insert(name, "id", uint64(n))
			cache.Get(name)
			cache.GetAlias(name)
		}(i)
	}
	wg.Wait()

	if cache.Len() != 10 {
		t.Errorf("expected 10 entries, got %d", cache.Len())
	}
}
