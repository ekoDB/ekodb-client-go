package ekodb

import (
	"sync"
	"time"
)

// CachedSchema holds cached schema metadata for a collection.
type CachedSchema struct {
	PrimaryKeyAlias string
	Version         uint64
	CachedAt        time.Time
}

// SchemaCacheConfig configures the schema cache.
type SchemaCacheConfig struct {
	Enabled    bool
	MaxEntries int
	TTL        time.Duration
}

// SchemaCache is an opt-in in-memory LRU cache for collection schema metadata.
type SchemaCache struct {
	mu       sync.Mutex
	entries  map[string]*CachedSchema
	lruOrder []string
	config   SchemaCacheConfig
	enabled  bool
}

// NewSchemaCache creates a new schema cache with the given config.
func NewSchemaCache(config SchemaCacheConfig) *SchemaCache {
	if config.MaxEntries <= 0 {
		config.MaxEntries = 100
	}
	if config.TTL <= 0 {
		config.TTL = 5 * time.Minute
	}
	return &SchemaCache{
		entries:  make(map[string]*CachedSchema, config.MaxEntries),
		lruOrder: make([]string, 0, config.MaxEntries),
		config:   config,
		enabled:  config.Enabled,
	}
}

// NewDisabledSchemaCache creates a cache instance that is disabled by default.
func NewDisabledSchemaCache() *SchemaCache {
	return NewSchemaCache(SchemaCacheConfig{})
}

// IsEnabled returns whether the cache is enabled.
func (sc *SchemaCache) IsEnabled() bool {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.enabled
}

// SetEnabled enables or disables the cache at runtime.
func (sc *SchemaCache) SetEnabled(enabled bool) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.enabled = enabled
}

// GetAlias returns the cached primary_key_alias for a collection, or "" if not cached/stale.
func (sc *SchemaCache) GetAlias(collection string) string {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if !sc.enabled {
		return ""
	}
	entry, ok := sc.entries[collection]
	if !ok {
		return ""
	}
	if time.Since(entry.CachedAt) > sc.config.TTL {
		delete(sc.entries, collection)
		sc.removeLRU(collection)
		return ""
	}
	sc.touchLRU(collection)
	return entry.PrimaryKeyAlias
}

// Get returns the full cached schema entry, or nil if not cached/stale.
func (sc *SchemaCache) Get(collection string) *CachedSchema {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if !sc.enabled {
		return nil
	}
	entry, ok := sc.entries[collection]
	if !ok {
		return nil
	}
	if time.Since(entry.CachedAt) > sc.config.TTL {
		delete(sc.entries, collection)
		sc.removeLRU(collection)
		return nil
	}
	sc.touchLRU(collection)
	// Return a copy
	copied := *entry
	return &copied
}

// Insert adds or updates a cached schema entry.
func (sc *SchemaCache) Insert(collection, primaryKeyAlias string, version uint64) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if !sc.enabled {
		return
	}
	_, exists := sc.entries[collection]
	sc.entries[collection] = &CachedSchema{
		PrimaryKeyAlias: primaryKeyAlias,
		Version:         version,
		CachedAt:        time.Now(),
	}
	if !exists {
		sc.lruOrder = append(sc.lruOrder, collection)
		for len(sc.lruOrder) > sc.config.MaxEntries {
			oldest := sc.lruOrder[0]
			sc.lruOrder = sc.lruOrder[1:]
			delete(sc.entries, oldest)
		}
	} else {
		sc.touchLRU(collection)
	}
}

// Invalidate removes the cached schema for a collection.
func (sc *SchemaCache) Invalidate(collection string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	delete(sc.entries, collection)
	sc.removeLRU(collection)
}

// InvalidateAll clears the entire cache.
func (sc *SchemaCache) InvalidateAll() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.entries = make(map[string]*CachedSchema)
	sc.lruOrder = sc.lruOrder[:0]
}

// HandleSchemaChanged processes a SchemaChanged event from the server.
// Updates the cache if the event version is newer.
func (sc *SchemaCache) HandleSchemaChanged(collection string, version uint64, primaryKeyAlias string) {
	sc.mu.Lock()
	if !sc.enabled {
		sc.mu.Unlock()
		return
	}
	if entry, ok := sc.entries[collection]; ok && version <= entry.Version {
		sc.mu.Unlock()
		return // ignore older versions
	}
	// Release lock before calling Insert (which re-acquires)
	sc.mu.Unlock()
	sc.Insert(collection, primaryKeyAlias, version)
}

// Len returns the number of cached entries.
func (sc *SchemaCache) Len() int {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return len(sc.entries)
}

// touchLRU moves a collection to the end of the LRU order (most recent).
// Must be called with mu held.
func (sc *SchemaCache) touchLRU(collection string) {
	sc.removeLRU(collection)
	sc.lruOrder = append(sc.lruOrder, collection)
}

// removeLRU removes a collection from the LRU order.
// Must be called with mu held.
func (sc *SchemaCache) removeLRU(collection string) {
	for i, c := range sc.lruOrder {
		if c == collection {
			sc.lruOrder = append(sc.lruOrder[:i], sc.lruOrder[i+1:]...)
			return
		}
	}
}

// ExtractRecordID extracts the record ID from a map, trying the schema cache alias
// first, then common fallbacks ("id", "_id").
func (sc *SchemaCache) ExtractRecordID(collection string, record map[string]interface{}) string {
	// Try cached alias first
	if alias := sc.GetAlias(collection); alias != "" {
		if id, ok := record[alias]; ok {
			if s, ok := id.(string); ok {
				return s
			}
			// Handle typed wrapper {"type": "String", "value": "..."}
			if m, ok := id.(map[string]interface{}); ok {
				if v, ok := m["value"]; ok {
					if s, ok := v.(string); ok {
						return s
					}
				}
			}
		}
	}
	// Fallbacks
	for _, key := range []string{"id", "_id"} {
		if id, ok := record[key]; ok {
			if s, ok := id.(string); ok {
				return s
			}
			// Handle typed wrapper {"type": "String", "value": "..."}
			if m, ok := id.(map[string]interface{}); ok {
				if v, ok := m["value"]; ok {
					if s, ok := v.(string); ok {
						return s
					}
				}
			}
		}
	}
	return ""
}
