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

// ClientConfig contains configuration options for the client
type ClientConfig struct {
	BaseURL     string        // Base URL of the ekoDB server
	APIKey      string        // API key for authentication
	ShouldRetry bool          // Enable automatic retries (default: true)
	MaxRetries  int           // Maximum number of retry attempts (default: 3)
	Timeout     time.Duration // Request timeout (default: 30s)
}

// Client represents an ekoDB client
type Client struct {
	baseURL       string
	apiKey        string
	token         string
	httpClient    *http.Client
	shouldRetry   bool
	maxRetries    int
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

	client := &Client{
		baseURL:     config.BaseURL,
		apiKey:      config.APIKey,
		shouldRetry: config.ShouldRetry,
		maxRetries:  config.MaxRetries,
		httpClient: &http.Client{
			Timeout: config.Timeout,
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
		return fmt.Errorf("auth failed with status: %d", resp.StatusCode)
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
	if data != nil {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		body = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

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

	// Handle service unavailable (503)
	if resp.StatusCode == http.StatusServiceUnavailable && c.shouldRetry && attempt < c.maxRetries {
		retryDelay := 10 * time.Second
		log.Printf("Service unavailable, retrying after %v...", retryDelay)
		time.Sleep(retryDelay)
		return c.makeRequestWithRetry(method, path, data, attempt+1)
	}

	// Handle other errors
	return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(responseBody))
}

// Insert inserts a document into a collection
func (c *Client) Insert(collection string, record Record, ttl ...string) (Record, error) {
	// Add TTL if provided
	if len(ttl) > 0 && ttl[0] != "" {
		record["ttl_duration"] = ttl[0]
	}

	respBody, err := c.makeRequest("POST", "/api/insert/"+collection, record)
	if err != nil {
		return nil, err
	}

	var result Record
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// Find finds documents in a collection
func (c *Client) Find(collection string, query interface{}) ([]Record, error) {
	respBody, err := c.makeRequest("POST", "/api/find/"+collection, query)
	if err != nil {
		return nil, err
	}

	var results []Record
	if err := json.Unmarshal(respBody, &results); err != nil {
		return nil, err
	}

	return results, nil
}

// FindByID finds a document by ID
func (c *Client) FindByID(collection, id string) (Record, error) {
	respBody, err := c.makeRequest("GET", fmt.Sprintf("/api/find/%s/%s", collection, id), nil)
	if err != nil {
		return nil, err
	}

	var result Record
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// Update updates a document
func (c *Client) Update(collection, id string, record Record) (Record, error) {
	respBody, err := c.makeRequest("PUT", fmt.Sprintf("/api/update/%s/%s", collection, id), record)
	if err != nil {
		return nil, err
	}

	var result Record
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// Delete deletes a document
func (c *Client) Delete(collection, id string) error {
	_, err := c.makeRequest("DELETE", fmt.Sprintf("/api/delete/%s/%s", collection, id), nil)
	return err
}

// BatchInsert inserts multiple documents
func (c *Client) BatchInsert(collection string, records []Record) ([]Record, error) {
	// Convert to server format
	type batchInsertItem struct {
		Data Record `json:"data"`
	}
	type batchInsertQuery struct {
		Inserts []batchInsertItem `json:"inserts"`
	}

	inserts := make([]batchInsertItem, len(records))
	for i, r := range records {
		inserts[i] = batchInsertItem{Data: r}
	}

	query := batchInsertQuery{Inserts: inserts}

	respBody, err := c.makeRequest("POST", "/api/batch/insert/"+collection, query)
	if err != nil {
		return nil, err
	}

	type batchResult struct {
		Successful []string      `json:"successful"`
		Failed     []interface{} `json:"failed"`
	}

	var result batchResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	// Convert IDs to Records
	results := make([]Record, len(result.Successful))
	for i, id := range result.Successful {
		results[i] = Record{"id": id}
	}

	return results, nil
}

// BatchUpdate updates multiple documents
func (c *Client) BatchUpdate(collection string, updates map[string]Record) ([]Record, error) {
	// Convert to server format
	type batchUpdateItem struct {
		ID   string `json:"id"`
		Data Record `json:"data"`
	}
	type batchUpdateQuery struct {
		Updates []batchUpdateItem `json:"updates"`
	}

	items := make([]batchUpdateItem, 0, len(updates))
	for id, data := range updates {
		items = append(items, batchUpdateItem{ID: id, Data: data})
	}

	query := batchUpdateQuery{Updates: items}

	respBody, err := c.makeRequest("PUT", "/api/batch/update/"+collection, query)
	if err != nil {
		return nil, err
	}

	type batchResult struct {
		Successful []string      `json:"successful"`
		Failed     []interface{} `json:"failed"`
	}

	var result batchResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	// Convert IDs to Records
	results := make([]Record, len(result.Successful))
	for i, id := range result.Successful {
		results[i] = Record{"id": id}
	}

	return results, nil
}

// BatchDelete deletes multiple documents
func (c *Client) BatchDelete(collection string, ids []string) (int, error) {
	// Convert to server format
	type batchDeleteItem struct {
		ID string `json:"id"`
	}
	type batchDeleteQuery struct {
		Deletes []batchDeleteItem `json:"deletes"`
	}

	deletes := make([]batchDeleteItem, len(ids))
	for i, id := range ids {
		deletes[i] = batchDeleteItem{ID: id}
	}

	query := batchDeleteQuery{Deletes: deletes}

	respBody, err := c.makeRequest("DELETE", "/api/batch/delete/"+collection, query)
	if err != nil {
		return 0, err
	}

	type batchResult struct {
		Successful []string      `json:"successful"`
		Failed     []interface{} `json:"failed"`
	}

	var result batchResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return 0, err
	}

	return len(result.Successful), nil
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
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return result["value"], nil
}

// KVDelete deletes a key
func (c *Client) KVDelete(key string) error {
	_, err := c.makeRequest("DELETE", "/api/kv/delete/"+url.PathEscape(key), nil)
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
