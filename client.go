// Package ekodb provides a Go client for ekoDB
package ekodb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

// SerializationFormat represents the serialization format for client-server communication
type SerializationFormat int

const (
	// MessagePack format (binary, 2-3x faster) - DEFAULT for best performance
	MessagePack SerializationFormat = iota
	// JSON format (human-readable, opt-in for debugging)
	JSON
)

// RateLimitInfo contains rate limit information from the server
type RateLimitInfo struct {
	Limit     int   // Maximum requests allowed per window
	Remaining int   // Requests remaining in current window
	Reset     int64 // Unix timestamp when the rate limit resets
}

// IsNearLimit checks if approaching rate limit (less than 10% remaining)
func (r *RateLimitInfo) IsNearLimit() bool {
	threshold := float64(r.Limit) * 0.1
	return float64(r.Remaining) <= threshold
}

// IsExceeded checks if the rate limit has been exceeded
func (r *RateLimitInfo) IsExceeded() bool {
	return r.Remaining == 0
}

// RemainingPercentage returns the percentage of requests remaining
func (r *RateLimitInfo) RemainingPercentage() float64 {
	return (float64(r.Remaining) / float64(r.Limit)) * 100.0
}

// RateLimitError represents a rate limit error
type RateLimitError struct {
	RetryAfterSecs int
	Message        string
}

func (e *RateLimitError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("rate limit exceeded, retry after %d seconds", e.RetryAfterSecs)
}

// HTTPError represents an HTTP error with status code
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("request failed with status %d: %s", e.StatusCode, e.Message)
}

// IsNotFound checks if the error is a 404 Not Found error
func (e *HTTPError) IsNotFound() bool {
	return e.StatusCode == 404
}

// ClientConfig contains configuration options for the client
type ClientConfig struct {
	BaseURL     string              // Base URL of the ekoDB server
	APIKey      string              // API key for authentication
	ShouldRetry bool                // Enable automatic retries (default: true)
	MaxRetries  int                 // Maximum number of retry attempts (default: 3)
	Timeout     time.Duration       // Request timeout (default: 30s)
	Format      SerializationFormat // Serialization format (default: MessagePack for best performance, use JSON for debugging)
}

// Client represents an ekoDB client
type Client struct {
	baseURL       string
	apiKey        string
	token         string
	httpClient    *http.Client
	shouldRetry   bool
	maxRetries    int
	format        SerializationFormat
	rateLimitInfo *RateLimitInfo
}

// Record represents a document in ekoDB
type Record map[string]interface{}

// Query represents a query for finding records
type Query struct {
	Limit  *int                   `json:"limit,omitempty"`
	Offset *int                   `json:"offset,omitempty"`
	Filter map[string]interface{} `json:"filter,omitempty"`
}

// NewClient creates a new ekoDB client (legacy signature for backward compatibility)
func NewClient(baseURL, apiKey string) (*Client, error) {
	return NewClientWithConfig(ClientConfig{
		BaseURL:     baseURL,
		APIKey:      apiKey,
		ShouldRetry: true,
		MaxRetries:  3,
		Timeout:     30 * time.Second,
	})
}

// NewClientWithConfig creates a new ekoDB client with configuration
func NewClientWithConfig(config ClientConfig) (*Client, error) {
	// Set defaults
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}

	// Create HTTP client with automatic gzip compression support
	// The default transport handles Accept-Encoding and decompression automatically
	client := &Client{
		baseURL:     config.BaseURL,
		apiKey:      config.APIKey,
		shouldRetry: config.ShouldRetry,
		maxRetries:  config.MaxRetries,
		format:      config.Format, // Default is JSON (0 value)
		httpClient: &http.Client{
			Timeout: config.Timeout,
			// Using nil transport enables default transport with automatic compression
			Transport: nil,
		},
	}

	// Automatically get token
	if err := client.refreshToken(); err != nil {
		return nil, fmt.Errorf("failed to get auth token: %w", err)
	}

	return client, nil
}

// GetRateLimitInfo returns the current rate limit information
func (c *Client) GetRateLimitInfo() *RateLimitInfo {
	return c.rateLimitInfo
}

// IsNearRateLimit checks if approaching rate limit
func (c *Client) IsNearRateLimit() bool {
	if c.rateLimitInfo == nil {
		return false
	}
	return c.rateLimitInfo.IsNearLimit()
}

// refreshToken gets a new authentication token
func (c *Client) refreshToken() error {
	authReq := map[string]string{"api_key": c.apiKey}
	body, err := json.Marshal(authReq)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Post(c.baseURL+"/api/auth/token", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("auth failed with status: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	token, ok := result["token"].(string)
	if !ok {
		return fmt.Errorf("invalid token response")
	}

	c.token = token
	return nil
}

// extractRateLimitInfo extracts rate limit information from response headers
func (c *Client) extractRateLimitInfo(resp *http.Response) {
	limitStr := resp.Header.Get("X-RateLimit-Limit")
	remainingStr := resp.Header.Get("X-RateLimit-Remaining")
	resetStr := resp.Header.Get("X-RateLimit-Reset")

	if limitStr != "" && remainingStr != "" && resetStr != "" {
		limit, _ := strconv.Atoi(limitStr)
		remaining, _ := strconv.Atoi(remainingStr)
		reset, _ := strconv.ParseInt(resetStr, 10, 64)

		c.rateLimitInfo = &RateLimitInfo{
			Limit:     limit,
			Remaining: remaining,
			Reset:     reset,
		}

		// Log warning if approaching rate limit
		if c.rateLimitInfo.IsNearLimit() {
			log.Printf("Warning: Approaching rate limit: %d/%d remaining (%.1f%%)",
				c.rateLimitInfo.Remaining, c.rateLimitInfo.Limit, c.rateLimitInfo.RemainingPercentage())
		}
	}
}

// makeRequest makes an HTTP request to the ekoDB API with retry logic
func (c *Client) makeRequest(method, path string, data interface{}) ([]byte, error) {
	return c.makeRequestWithRetry(method, path, data, 0)
}

// makeRequestWithRetry makes an HTTP request with retry logic
func (c *Client) makeRequestWithRetry(method, path string, data interface{}, attempt int) ([]byte, error) {
	var body io.Reader
	var contentType string

	// Check if this path should always use JSON (metadata endpoints)
	forceJSON := shouldUseJSON(path)

	// Set content type based on client format (unless forced to JSON)
	if !forceJSON && c.format == MessagePack {
		contentType = "application/msgpack"
	} else {
		contentType = "application/json"
	}

	if data != nil {
		var serializedData []byte
		var err error

		if !forceJSON && c.format == MessagePack {
			// Serialize to MessagePack
			serializedData, err = msgpack.Marshal(data)
		} else {
			// Serialize to JSON (default)
			serializedData, err = json.Marshal(data)
		}

		if err != nil {
			return nil, err
		}
		body = bytes.NewBuffer(serializedData)
	}

	req, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", contentType)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Handle network errors with retry
		if c.shouldRetry && attempt < c.maxRetries {
			retryDelay := 3 * time.Second
			log.Printf("Network error, retrying after %v...", retryDelay)
			time.Sleep(retryDelay)
			return c.makeRequestWithRetry(method, path, data, attempt+1)
		}
		return nil, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Extract rate limit info from successful responses
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		c.extractRateLimitInfo(resp)
		return responseBody, nil
	}

	// Handle rate limiting (429)
	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfterStr := resp.Header.Get("Retry-After")
		retryAfter := 60 // default
		if retryAfterStr != "" {
			if val, err := strconv.Atoi(retryAfterStr); err == nil {
				retryAfter = val
			}
		}

		if c.shouldRetry && attempt < c.maxRetries {
			retryDelay := time.Duration(retryAfter) * time.Second
			log.Printf("Rate limited, retrying after %v...", retryDelay)
			time.Sleep(retryDelay)
			return c.makeRequestWithRetry(method, path, data, attempt+1)
		}

		return nil, &RateLimitError{
			RetryAfterSecs: retryAfter,
			Message:        string(responseBody),
		}
	}

	// Handle unauthorized (401) or token errors - try refreshing token
	if resp.StatusCode == http.StatusUnauthorized ||
		(resp.StatusCode == http.StatusInternalServerError && bytes.Contains(responseBody, []byte("Invalid token"))) {
		if attempt == 0 { // Only try token refresh once
			log.Printf("Authentication failed, refreshing token...")
			if err := c.refreshToken(); err != nil {
				return nil, fmt.Errorf("failed to refresh token: %w", err)
			}
			// Retry with new token
			return c.makeRequestWithRetry(method, path, data, attempt+1)
		}
		// Authentication is still failing after a token refresh attempt; return a clear auth error.
		return nil, fmt.Errorf("authentication failed after token refresh (status %d): %s", resp.StatusCode, string(responseBody))
	}

	// Handle service unavailable (503)
	if resp.StatusCode == http.StatusServiceUnavailable && c.shouldRetry && attempt < c.maxRetries {
		retryDelay := 10 * time.Second
		log.Printf("Service unavailable, retrying after %v...", retryDelay)
		time.Sleep(retryDelay)
		return c.makeRequestWithRetry(method, path, data, attempt+1)
	}

	// Handle other errors
	return nil, &HTTPError{
		StatusCode: resp.StatusCode,
		Message:    string(responseBody),
	}
}

// unmarshal deserializes data based on the client's format and path
func (c *Client) unmarshal(path string, data []byte, v interface{}) error {
	// Use JSON if the path requires it or if client is set to JSON
	if shouldUseJSON(path) || c.format == JSON {
		return json.Unmarshal(data, v)
	}
	return msgpack.Unmarshal(data, v)
}

// shouldUseJSON determines if a path should use JSON
// Only CRUD operations (insert/update/delete/find/batch) use MessagePack
// Everything else uses JSON for compatibility
func shouldUseJSON(path string) bool {
	// ONLY these operations support MessagePack
	msgpackPaths := []string{
		"/api/insert/",
		"/api/batch_insert/",
		"/api/update/",
		"/api/batch_update/",
		"/api/delete/",
		"/api/batch_delete/",
		"/api/find/",
	}

	// Check if path starts with any MessagePack-supported operation
	for _, prefix := range msgpackPaths {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			return false // Use MessagePack
		}
	}

	// Everything else uses JSON
	return true
}

// InsertOptions contains optional parameters for Insert
type InsertOptions struct {
	TTL           string
	BypassRipple  *bool
	TransactionId *string
	BypassCache   *bool
}

// Insert inserts a document into a collection
// Usage:
//
//	Insert(collection, record)                                    // basic insert
//	Insert(collection, record, InsertOptions{TTL: "1h"})          // with TTL
//	Insert(collection, record, InsertOptions{BypassRipple: &t})   // bypass ripple
func (c *Client) Insert(collection string, record Record, opts ...InsertOptions) (Record, error) {
	// Add TTL if provided
	if len(opts) > 0 && opts[0].TTL != "" {
		record["ttl"] = opts[0].TTL
	}

	// Build query parameters
	path := "/api/insert/" + collection
	if len(opts) > 0 {
		params := url.Values{}
		if opts[0].BypassRipple != nil {
			params.Add("bypass_ripple", fmt.Sprintf("%t", *opts[0].BypassRipple))
		}
		if opts[0].TransactionId != nil {
			params.Add("transaction_id", *opts[0].TransactionId)
		}
		if opts[0].BypassCache != nil {
			params.Add("bypass_cache", fmt.Sprintf("%t", *opts[0].BypassCache))
		}
		if len(params) > 0 {
			path = fmt.Sprintf("%s?%s", path, params.Encode())
		}
	}
	respBody, err := c.makeRequest("POST", path, record)
	if err != nil {
		return nil, err
	}

	var result Record
	if err := c.unmarshal(path, respBody, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// FindOptions contains optional parameters for Find
type FindOptions struct {
	Filter        interface{}
	Sort          interface{}
	Limit         *int
	Skip          *int
	Join          interface{}
	BypassCache   *bool
	BypassRipple  *bool
	SelectFields  []string
	ExcludeFields []string
}

// Find finds documents in a collection
func (c *Client) Find(collection string, query interface{}, opts ...FindOptions) ([]Record, error) {
	path := "/api/find/" + collection
	respBody, err := c.makeRequest("POST", path, query)
	if err != nil {
		return nil, err
	}

	var results []Record
	if err := c.unmarshal(path, respBody, &results); err != nil {
		return nil, err
	}

	return results, nil
}

// FindByID finds a document by ID
func (c *Client) FindByID(collection, id string) (Record, error) {
	path := fmt.Sprintf("/api/find/%s/%s", collection, id)
	respBody, err := c.makeRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var result Record
	if err := c.unmarshal(path, respBody, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// FindByIDWithProjection finds a document by ID with field projection
// selectFields: only return these fields (plus 'id')
// excludeFields: exclude these fields from results
func (c *Client) FindByIDWithProjection(collection, id string, selectFields, excludeFields []string) (Record, error) {
	// Build query with projection using Find endpoint
	query := NewQueryBuilder().Eq("id", id).Limit(1)

	if len(selectFields) > 0 {
		query.SelectFields(selectFields...)
	}

	if len(excludeFields) > 0 {
		query.ExcludeFields(excludeFields...)
	}

	results, err := c.Find(collection, query.Build())
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, &HTTPError{
			StatusCode: http.StatusNotFound,
			Message:    "document not found",
		}
	}

	return results[0], nil
}

// UpdateOptions contains optional parameters for Update
type UpdateOptions struct {
	BypassRipple  *bool
	TransactionId *string
	BypassCache   *bool
	SelectFields  []string
	ExcludeFields []string
}

// Update updates a document
func (c *Client) Update(collection, id string, record Record, opts ...UpdateOptions) (Record, error) {
	// Build query parameters
	path := fmt.Sprintf("/api/update/%s/%s", collection, id)
	if len(opts) > 0 {
		params := url.Values{}
		if opts[0].BypassRipple != nil {
			params.Add("bypass_ripple", fmt.Sprintf("%t", *opts[0].BypassRipple))
		}
		if opts[0].TransactionId != nil {
			params.Add("transaction_id", *opts[0].TransactionId)
		}
		if opts[0].BypassCache != nil {
			params.Add("bypass_cache", fmt.Sprintf("%t", *opts[0].BypassCache))
		}
		if len(opts[0].SelectFields) > 0 {
			for _, field := range opts[0].SelectFields {
				params.Add("select_fields", field)
			}
		}
		if len(opts[0].ExcludeFields) > 0 {
			for _, field := range opts[0].ExcludeFields {
				params.Add("exclude_fields", field)
			}
		}
		if len(params) > 0 {
			path = fmt.Sprintf("%s?%s", path, params.Encode())
		}
	}
	respBody, err := c.makeRequest("PUT", path, record)
	if err != nil {
		return nil, err
	}

	var result Record
	if err := c.unmarshal(path, respBody, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// DeleteOptions contains optional parameters for Delete
type DeleteOptions struct {
	BypassRipple  *bool
	TransactionId *string
}

// Delete deletes a document
func (c *Client) Delete(collection, id string, opts ...DeleteOptions) error {
	// Build query parameters
	path := fmt.Sprintf("/api/delete/%s/%s", collection, id)
	if len(opts) > 0 {
		params := url.Values{}
		if opts[0].BypassRipple != nil {
			params.Add("bypass_ripple", fmt.Sprintf("%t", *opts[0].BypassRipple))
		}
		if opts[0].TransactionId != nil {
			params.Add("transaction_id", *opts[0].TransactionId)
		}
		if len(params) > 0 {
			path = fmt.Sprintf("%s?%s", path, params.Encode())
		}
	}
	respBody, err := c.makeRequest("DELETE", path, nil)
	if err != nil {
		return err
	}

	var result map[string]interface{}
	if err := c.unmarshal(path, respBody, &result); err != nil {
		return err
	}

	return nil
}

// BatchInsertOptions contains optional parameters for BatchInsert
type BatchInsertOptions struct {
	BypassRipple  *bool
	TransactionId *string
}

// BatchInsert inserts multiple documents
func (c *Client) BatchInsert(collection string, records []Record, opts ...BatchInsertOptions) ([]Record, error) {
	var bypassRipple *bool
	if len(opts) > 0 {
		bypassRipple = opts[0].BypassRipple
	}
	// Convert to server format
	type batchInsertItem struct {
		Data         Record `json:"data" msgpack:"data"`
		BypassRipple *bool  `json:"bypass_ripple,omitempty" msgpack:"bypass_ripple,omitempty"`
	}
	type batchInsertQuery struct {
		Inserts []batchInsertItem `json:"inserts" msgpack:"inserts"`
	}

	inserts := make([]batchInsertItem, len(records))
	for i, r := range records {
		inserts[i] = batchInsertItem{Data: r, BypassRipple: bypassRipple}
	}

	query := batchInsertQuery{Inserts: inserts}

	path := "/api/batch/insert/" + collection
	respBody, err := c.makeRequest("POST", path, query)
	if err != nil {
		return nil, err
	}

	type batchResult struct {
		Successful []string      `json:"successful" msgpack:"successful"`
		Failed     []interface{} `json:"failed" msgpack:"failed"`
	}

	var result batchResult
	if err := c.unmarshal(path, respBody, &result); err != nil {
		return nil, err
	}

	// Convert IDs to Records
	results := make([]Record, len(result.Successful))
	for i, id := range result.Successful {
		results[i] = Record{"id": id}
	}

	return results, nil
}

// BatchUpdateOptions contains optional parameters for BatchUpdate
type BatchUpdateOptions struct {
	BypassRipple  *bool
	TransactionId *string
}

// BatchUpdate updates multiple documents
func (c *Client) BatchUpdate(collection string, updates map[string]Record, opts ...BatchUpdateOptions) ([]Record, error) {
	var bypassRipple *bool
	if len(opts) > 0 {
		bypassRipple = opts[0].BypassRipple
	}
	// Convert to server format
	type batchUpdateItem struct {
		ID           string `json:"id" msgpack:"id"`
		Data         Record `json:"data" msgpack:"data"`
		BypassRipple *bool  `json:"bypass_ripple,omitempty" msgpack:"bypass_ripple,omitempty"`
	}
	type batchUpdateQuery struct {
		Updates []batchUpdateItem `json:"updates" msgpack:"updates"`
	}

	items := make([]batchUpdateItem, 0, len(updates))
	for id, data := range updates {
		items = append(items, batchUpdateItem{ID: id, Data: data, BypassRipple: bypassRipple})
	}

	query := batchUpdateQuery{Updates: items}

	path := "/api/batch/update/" + collection
	respBody, err := c.makeRequest("PUT", path, query)
	if err != nil {
		return nil, err
	}

	type batchResult struct {
		Successful []string      `json:"successful" msgpack:"successful"`
		Failed     []interface{} `json:"failed" msgpack:"failed"`
	}

	var result batchResult
	if err := c.unmarshal(path, respBody, &result); err != nil {
		return nil, err
	}

	// Convert IDs to Records
	results := make([]Record, len(result.Successful))
	for i, id := range result.Successful {
		results[i] = Record{"id": id}
	}

	return results, nil
}

// BatchDeleteOptions contains optional parameters for BatchDelete
type BatchDeleteOptions struct {
	BypassRipple  *bool
	TransactionId *string
}

// BatchDelete deletes multiple documents
func (c *Client) BatchDelete(collection string, ids []string, opts ...BatchDeleteOptions) (int, error) {
	var bypassRipple *bool
	if len(opts) > 0 {
		bypassRipple = opts[0].BypassRipple
	}
	// Convert to server format
	type batchDeleteItem struct {
		ID           string `json:"id" msgpack:"id"`
		BypassRipple *bool  `json:"bypass_ripple,omitempty" msgpack:"bypass_ripple,omitempty"`
	}
	type batchDeleteQuery struct {
		Deletes []batchDeleteItem `json:"deletes" msgpack:"deletes"`
	}

	deletes := make([]batchDeleteItem, len(ids))
	for i, id := range ids {
		deletes[i] = batchDeleteItem{ID: id, BypassRipple: bypassRipple}
	}

	query := batchDeleteQuery{Deletes: deletes}

	path := "/api/batch/delete/" + collection
	respBody, err := c.makeRequest("DELETE", path, query)
	if err != nil {
		return 0, err
	}

	type batchResult struct {
		Successful []string      `json:"successful" msgpack:"successful"`
		Failed     []interface{} `json:"failed" msgpack:"failed"`
	}

	var result batchResult
	if err := c.unmarshal(path, respBody, &result); err != nil {
		return 0, err
	}

	return len(result.Successful), nil
}

// ========== Convenience Methods ==========

// UpsertOptions contains optional parameters for Upsert
type UpsertOptions struct {
	TTL           string
	BypassRipple  *bool
	TransactionId *string
	BypassCache   *bool
}

// Upsert inserts or updates a document (atomic insert-or-update)
// Attempts to update first. If the record doesn't exist (404), it will be inserted.
func (c *Client) Upsert(collection, id string, record Record, opts ...UpsertOptions) (Record, error) {
	var bypassRipple *bool
	var transactionId *string
	var bypassCache *bool
	var ttl string
	if len(opts) > 0 {
		bypassRipple = opts[0].BypassRipple
		transactionId = opts[0].TransactionId
		bypassCache = opts[0].BypassCache
		ttl = opts[0].TTL
	}

	// Try update first
	updateOpts := UpdateOptions{
		BypassRipple:  bypassRipple,
		TransactionId: transactionId,
		BypassCache:   bypassCache,
	}
	result, err := c.Update(collection, id, record, updateOpts)
	if err != nil {
		// Check if it's a 404 Not Found error
		if httpErr, ok := err.(*HTTPError); ok && httpErr.IsNotFound() {
			// Record doesn't exist, insert it with the intended id
			record["id"] = id
			insertOpts := InsertOptions{
				TTL:           ttl,
				BypassRipple:  bypassRipple,
				TransactionId: transactionId,
				BypassCache:   bypassCache,
			}
			return c.Insert(collection, record, insertOpts)
		}
		// Other error, propagate it
		return nil, err
	}
	return result, nil
}

// FindOne finds a single record by field value
// Returns nil if no record matches, or the first matching record.
func (c *Client) FindOne(collection, field string, value interface{}) (Record, error) {
	query := NewQueryBuilder().Eq(field, value).Limit(1).Build()

	results, err := c.Find(collection, query)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, nil
	}

	return results[0], nil
}

// Exists checks if a record exists by ID
// Returns true if the record exists, false if it doesn't.
func (c *Client) Exists(collection, id string) (bool, error) {
	_, err := c.FindByID(collection, id)
	if err != nil {
		// Check if it's a 404 Not Found error
		if httpErr, ok := err.(*HTTPError); ok && httpErr.IsNotFound() {
			return false, nil
		}
		// Other error, propagate it
		return false, err
	}
	return true, nil
}

// Paginate retrieves records with pagination (1-indexed page numbers)
// Page 1 = first page, Page 2 = second page, etc.
// Returns an error if page < 1 or pageSize < 1.
func (c *Client) Paginate(collection string, page, pageSize int) ([]Record, error) {
	// Validate input parameters
	if page < 1 {
		return nil, fmt.Errorf("page must be >= 1, got %d", page)
	}
	if pageSize < 1 {
		return nil, fmt.Errorf("pageSize must be >= 1, got %d", pageSize)
	}

	// Page 1 = offset 0, Page 2 = offset pageSize, etc.
	offset := (page - 1) * pageSize

	query := NewQueryBuilder().Limit(pageSize).Skip(offset).Build()

	return c.Find(collection, query)
}

// KVSet sets a key-value pair
func (c *Client) KVSet(key string, value interface{}) error {
	data := map[string]interface{}{"value": value}
	_, err := c.makeRequest("POST", "/api/kv/set/"+url.PathEscape(key), data)
	return err
}

// KVGet gets a value by key
func (c *Client) KVGet(key string) (interface{}, error) {
	respBody, err := c.makeRequest("GET", "/api/kv/get/"+url.PathEscape(key), nil)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := c.unmarshal("/api/kv/get/"+url.PathEscape(key), respBody, &result); err != nil {
		return nil, err
	}

	return result["value"], nil
}

// KVDelete deletes a key
func (c *Client) KVDelete(key string) error {
	_, err := c.makeRequest("DELETE", "/api/kv/delete/"+url.PathEscape(key), nil)
	return err
}

// KVBatchGet retrieves multiple keys in a single request
func (c *Client) KVBatchGet(keys []string) ([]map[string]interface{}, error) {
	data := map[string]interface{}{
		"keys": keys,
	}

	respBody, err := c.makeRequest("POST", "/api/kv/batch/get", data)
	if err != nil {
		return nil, err
	}

	var result []map[string]interface{}
	if err := c.unmarshal("/api/kv/batch/get", respBody, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// KVBatchSet sets multiple key-value pairs in a single request.
// TTL from the first entry with a valid TTL is applied to all entries (server limitation).
func (c *Client) KVBatchSet(entries []map[string]interface{}) ([][]interface{}, error) {
	keys := make([]string, len(entries))
	values := make([]map[string]interface{}, len(entries))
	var ttl *int64

	for i, entry := range entries {
		// Safe type assertion for key
		key, ok := entry["key"].(string)
		if !ok {
			return nil, fmt.Errorf("KVBatchSet: entry %d has non-string or missing key", i)
		}
		keys[i] = key
		value, ok := entry["value"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("KVBatchSet: entry %d has non-map or missing value", i)
		}
		values[i] = value
		// Use TTL from first entry if provided (supports both int and int64)
		if ttl == nil {
			if entryTTL, ok := entry["ttl"].(int64); ok {
				ttl = &entryTTL
			} else if entryTTLInt, ok := entry["ttl"].(int); ok {
				ttlVal := int64(entryTTLInt)
				ttl = &ttlVal
			}
		}
	}

	data := map[string]interface{}{
		"keys":   keys,
		"values": values,
	}
	if ttl != nil {
		data["ttl"] = *ttl
	}

	respBody, err := c.makeRequest("POST", "/api/kv/batch/set", data)
	if err != nil {
		return nil, err
	}

	var result [][]interface{}
	if err := c.unmarshal("/api/kv/batch/set", respBody, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// KVBatchDelete deletes multiple keys in a single request
func (c *Client) KVBatchDelete(keys []string) ([][]interface{}, error) {
	data := map[string]interface{}{
		"keys": keys,
	}

	respBody, err := c.makeRequest("DELETE", "/api/kv/batch/delete", data)
	if err != nil {
		return nil, err
	}

	var result [][]interface{}
	if err := c.unmarshal("/api/kv/batch/delete", respBody, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// KVExists checks if a key exists
func (c *Client) KVExists(key string) (bool, error) {
	_, err := c.KVGet(key)
	if err != nil {
		// Check if it's a "not found" error using structured error type
		if httpErr, ok := err.(*HTTPError); ok && httpErr.IsNotFound() {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// KVFind queries/finds KV entries with pattern matching. The pattern supports
// simple wildcards where '*' matches any sequence of characters in a key.
// If includeExpired is true, results will also include entries that are past
// their configured TTL but may still be present in the store.
func (c *Client) KVFind(pattern string, includeExpired bool) ([]map[string]interface{}, error) {
	data := map[string]interface{}{
		"include_expired": includeExpired,
	}
	if pattern != "" {
		data["pattern"] = pattern
	}

	respBody, err := c.makeRequest("POST", "/api/kv/find", data)
	if err != nil {
		return nil, err
	}

	var result []map[string]interface{}
	if err := c.unmarshal("/api/kv/find", respBody, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// KVQuery is an alias for KVFind - queries KV store with pattern
func (c *Client) KVQuery(pattern string, includeExpired bool) ([]map[string]interface{}, error) {
	return c.KVFind(pattern, includeExpired)
}

// ============================================================================
// Transaction Operations
// ============================================================================

// BeginTransaction starts a new transaction on the server with the given isolation level.
// The isolationLevel parameter must be one of: "READ_UNCOMMITTED", "READ_COMMITTED",
// "REPEATABLE_READ", or "SERIALIZABLE". It returns the server-assigned transaction ID
// as a string, or an error if the transaction could not be created.
func (c *Client) BeginTransaction(isolationLevel string) (string, error) {
	// Map user-friendly uppercase format to server's PascalCase format
	isolationMap := map[string]string{
		"READ_UNCOMMITTED": "ReadUncommitted",
		"READ_COMMITTED":   "ReadCommitted",
		"REPEATABLE_READ":  "RepeatableRead",
		"SERIALIZABLE":     "Serializable",
	}

	serverIsolation, valid := isolationMap[isolationLevel]
	if !valid {
		return "", fmt.Errorf("invalid isolation level: %s (must be one of: READ_UNCOMMITTED, READ_COMMITTED, REPEATABLE_READ, SERIALIZABLE)", isolationLevel)
	}

	data := map[string]interface{}{
		"isolation_level": serverIsolation,
	}
	respBody, err := c.makeRequest("POST", "/api/transactions", data)
	if err != nil {
		return "", err
	}

	var result struct {
		TransactionID string `json:"transaction_id"`
	}
	if err := c.unmarshal("/api/transactions", respBody, &result); err != nil {
		return "", err
	}

	return result.TransactionID, nil
}

// GetTransactionStatus gets the status of a transaction
func (c *Client) GetTransactionStatus(transactionID string) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("GET", "/api/transactions/"+transactionID, nil)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// CommitTransaction commits a transaction
func (c *Client) CommitTransaction(transactionID string) error {
	_, err := c.makeRequest("POST", "/api/transactions/"+transactionID+"/commit", nil)
	return err
}

// RollbackTransaction rolls back a transaction
func (c *Client) RollbackTransaction(transactionID string) error {
	_, err := c.makeRequest("POST", "/api/transactions/"+transactionID+"/rollback", nil)
	return err
}

// ListCollections lists all collections
func (c *Client) ListCollections() ([]string, error) {
	respBody, err := c.makeRequest("GET", "/api/collections", nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Collections []string `json:"collections"`
	}
	// Always use JSON for metadata endpoints
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return result.Collections, nil
}

// DeleteCollection deletes a collection
func (c *Client) DeleteCollection(collection string) error {
	_, err := c.makeRequest("DELETE", "/api/collections/"+collection, nil)
	return err
}

// RestoreRecord restores a deleted record from trash
// Records remain in trash for 30 days before permanent deletion
func (c *Client) RestoreRecord(collection, id string) error {
	path := fmt.Sprintf("/api/trash/%s/%s", collection, id)
	_, err := c.makeRequest("POST", path, nil)
	return err
}

// RestoreCollection restores all deleted records in a collection from trash
// Records remain in trash for 30 days before permanent deletion
func (c *Client) RestoreCollection(collection string) (int, error) {
	path := fmt.Sprintf("/api/trash/%s", collection)
	respBody, err := c.makeRequest("POST", path, nil)
	if err != nil {
		return 0, err
	}

	var result struct {
		Status          string `json:"status"`
		Collection      string `json:"collection"`
		RecordsRestored int    `json:"records_restored"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return 0, err
	}

	return result.RecordsRestored, nil
}

// Health checks if the ekoDB server is healthy
func (c *Client) Health() error {
	respBody, err := c.makeRequest("GET", "/api/health", nil)
	if err != nil {
		return err
	}

	var result map[string]interface{}
	// Always use JSON for health endpoint
	if err := json.Unmarshal(respBody, &result); err != nil {
		return err
	}

	// Check if status is "ok"
	if status, ok := result["status"].(string); ok && status == "ok" {
		return nil
	}

	return fmt.Errorf("health check failed: unexpected response")
}
