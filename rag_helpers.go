package ekodb

import (
	"fmt"
	"time"
)

// ========== RAG Helper Methods ==========

// Embed generates embeddings for text using ekoDB's native Functions
//
// This helper simplifies embedding generation by:
// 1. Creating a temporary collection with the text
// 2. Running a Script with FindAll + Embed Functions
// 3. Extracting and returning the embedding vector
// 4. Cleaning up temporary resources
//
// Example:
//
//	embedding, err := client.Embed("Hello world", "text-embedding-3-small")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Generated %d dimensions\n", len(embedding))
func (c *Client) Embed(text, model string) ([]float64, error) {
	tempCollection := fmt.Sprintf("embed_temp_%d", time.Now().UnixNano())

	// Insert temporary record with the text
	record := Record{"text": text}
	if _, err := c.Insert(tempCollection, record); err != nil {
		return nil, fmt.Errorf("failed to insert temp record: %w", err)
	}

	// Create Script with FindAll + Embed Functions
	tempLabel := fmt.Sprintf("embed_script_%d", time.Now().UnixNano())
	script := &Script{
		Label:      tempLabel,
		Name:       "Generate Embedding",
		Version:    "1.0",
		Parameters: map[string]ParameterDefinition{},
		Functions: []FunctionStageConfig{
			StageFindAll(tempCollection),
			{
				Stage: "Embed",
				Data: map[string]interface{}{
					"input_field":  "text",
					"output_field": "embedding",
					"model":        model,
				},
			},
		},
		Tags: []string{},
	}

	// Save and execute the script
	scriptID, err := c.SaveScript(*script)
	if err != nil {
		c.DeleteCollection(tempCollection) // Cleanup on error
		return nil, fmt.Errorf("failed to save script: %w", err)
	}

	result, err := c.CallScript(scriptID, nil)
	if err != nil {
		c.DeleteScript(scriptID)           // Cleanup script
		c.DeleteCollection(tempCollection) // Cleanup collection
		return nil, fmt.Errorf("failed to call script: %w", err)
	}

	// Clean up
	c.DeleteScript(scriptID)
	c.DeleteCollection(tempCollection)

	// Extract embedding from result
	if len(result.Records) > 0 {
		record := result.Records[0]
		if embedding, ok := record["embedding"].([]interface{}); ok {
			// Convert []interface{} to []float64
			vec := make([]float64, len(embedding))
			for i, v := range embedding {
				if f, ok := v.(float64); ok {
					vec[i] = f
				} else {
					return nil, fmt.Errorf("embedding value at index %d is not a float64", i)
				}
			}
			return vec, nil
		}
	}

	return nil, fmt.Errorf("failed to extract embedding from result")
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
