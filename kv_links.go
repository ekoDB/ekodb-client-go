package ekodb

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// KVGetLinks retrieves documents linked to a KV key.
func (c *Client) KVGetLinks(key string) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("GET", fmt.Sprintf("/api/kv/links/%s", url.PathEscape(key)), nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// KVLink creates a link between a KV key and a document.
func (c *Client) KVLink(key, collection, documentId string) (map[string]interface{}, error) {
	body := map[string]interface{}{
		"key":         key,
		"collection":  collection,
		"document_id": documentId,
	}
	respBody, err := c.makeRequest("POST", "/api/kv/link", body)
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
func (c *Client) KVUnlink(key, collection, documentId string) (map[string]interface{}, error) {
	body := map[string]interface{}{
		"key":         key,
		"collection":  collection,
		"document_id": documentId,
	}
	respBody, err := c.makeRequest("POST", "/api/kv/unlink", body)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}
