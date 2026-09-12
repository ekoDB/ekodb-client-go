package ekodb

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// KVGetLinks retrieves documents linked to a KV key.
func (c *Client) KVGetLinks(key string) ([]map[string]interface{}, error) {
	respBody, err := c.makeRequest("GET", fmt.Sprintf("/api/kv/%s/links", url.PathEscape(key)), nil)
	if err != nil {
		return nil, err
	}
	var result []map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// KVLink creates a link between a KV key and a document.
func (c *Client) KVLink(key, collection, documentID string) (map[string]interface{}, error) {
	path := fmt.Sprintf(
		"/api/kv/%s/links/%s/%s",
		url.PathEscape(key),
		url.PathEscape(collection),
		url.PathEscape(documentID),
	)
	respBody, err := c.makeRequest("POST", path, map[string]interface{}{})
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// KVUnlink removes a link between a KV key and a document.
func (c *Client) KVUnlink(key, collection, documentID string) (map[string]interface{}, error) {
	path := fmt.Sprintf(
		"/api/kv/%s/links/%s/%s",
		url.PathEscape(key),
		url.PathEscape(collection),
		url.PathEscape(documentID),
	)
	respBody, err := c.makeRequest("DELETE", path, nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}
