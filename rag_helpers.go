package ekodb

import (
	"encoding/json"
	"fmt"
)

// ========== RAG Helper Methods ==========

// Embed generates embeddings for a single text via the /api/embed endpoint
//
// Example:
//
//	embedding, err := client.Embed("Hello world", "text-embedding-3-small")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Generated %d dimensions\n", len(embedding))
func (c *Client) Embed(text, model string) ([]float64, error) {
	request := EmbedRequest{
		Text:  &text,
		Model: &model,
	}

	respBody, err := c.makeRequest("POST", "/api/embed", request)
	if err != nil {
		return nil, fmt.Errorf("embed request failed: %w", err)
	}

	var response EmbedResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse embed response: %w", err)
	}

	if len(response.Embeddings) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}

	return response.Embeddings[0], nil
}

// EmbedBatch generates embeddings for multiple texts in a single request
//
// Example:
//
//	embeddings, err := client.EmbedBatch([]string{"Hello", "World"}, "text-embedding-3-small")
//	if err != nil {
//	    log.Fatal(err)
//	}
func (c *Client) EmbedBatch(texts []string, model string) ([][]float64, error) {
	request := EmbedRequest{
		Texts: texts,
		Model: &model,
	}

	respBody, err := c.makeRequest("POST", "/api/embed", request)
	if err != nil {
		return nil, fmt.Errorf("embed batch request failed: %w", err)
	}

	var response EmbedResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse embed response: %w", err)
	}

	return response.Embeddings, nil
}

// TextSearch performs text search without embeddings
//
// Simplified text search with full-text matching, fuzzy search, and stemming.
//
// Example:
//
//	results, err := client.TextSearch("documents", "ownership system", 10)
//	if err != nil {
//	    log.Fatal(err)
//	}
func (c *Client) TextSearch(collection, queryText string, limit int) ([]Record, error) {
	searchQuery := SearchQuery{
		Query: queryText,
		Limit: &limit,
	}

	response, err := c.Search(collection, searchQuery)
	if err != nil {
		return nil, err
	}

	// Extract records from results
	records := make([]Record, len(response.Results))
	for i, result := range response.Results {
		records[i] = result.Record
	}

	return records, nil
}

// HybridSearch performs hybrid search combining text and vector search
//
// Combines semantic similarity (vector search) with keyword matching (text search)
// for more accurate and relevant results.
//
// Example:
//
//	embedding, _ := client.Embed(query, "text-embedding-3-small")
//	results, err := client.HybridSearch("documents", query, embedding, 5)
//	if err != nil {
//	    log.Fatal(err)
//	}
func (c *Client) HybridSearch(collection, queryText string, queryVector []float64, limit int) ([]Record, error) {
	searchQuery := SearchQuery{
		Query:  queryText,
		Vector: queryVector,
		Limit:  &limit,
	}

	response, err := c.Search(collection, searchQuery)
	if err != nil {
		return nil, err
	}

	// Extract records from results
	records := make([]Record, len(response.Results))
	for i, result := range response.Results {
		records[i] = result.Record
	}

	return records, nil
}

// FindAll finds all records in a collection with a limit
//
// Simplified method to query all documents in a collection.
//
// Example:
//
//	allMessages, err := client.FindAll("messages", 1000)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Found %d messages\n", len(allMessages))
func (c *Client) FindAll(collection string, limit int) ([]Record, error) {
	query := NewQueryBuilder().Limit(limit).Build()
	return c.Find(collection, query)
}
