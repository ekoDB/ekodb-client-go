// Package ekodb provides a Go client for ekoDB
package ekodb

import (
	"encoding/json"
	"fmt"
)

// SearchQuery represents a search query for full-text and vector search
type SearchQuery struct {
	Query string `json:"query"`

	// Full-text search parameters
	Language        *string  `json:"language,omitempty"`
	CaseSensitive   *bool    `json:"case_sensitive,omitempty"`
	Fuzzy           *bool    `json:"fuzzy,omitempty"`
	MinScore        *float64 `json:"min_score,omitempty"`
	Fields          *string  `json:"fields,omitempty"`
	Weights         *string  `json:"weights,omitempty"`
	EnableStemming  *bool    `json:"enable_stemming,omitempty"`
	BoostExact      *bool    `json:"boost_exact,omitempty"`
	MaxEditDistance *int     `json:"max_edit_distance,omitempty"`

	// Vector search parameters
	Vector          []float64 `json:"vector,omitempty"`
	VectorField     *string   `json:"vector_field,omitempty"`
	VectorMetric    *string   `json:"vector_metric,omitempty"`
	VectorK         *int      `json:"vector_k,omitempty"`
	VectorThreshold *float64  `json:"vector_threshold,omitempty"`

	// Hybrid search parameters
	TextWeight   *float64 `json:"text_weight,omitempty"`
	VectorWeight *float64 `json:"vector_weight,omitempty"`

	// Performance flags
	BypassRipple *bool `json:"bypass_ripple,omitempty"`
	BypassCache  *bool `json:"bypass_cache,omitempty"`
	Limit        *int  `json:"limit,omitempty"`
}

// SearchResult represents a single search result
type SearchResult struct {
	Record        map[string]interface{} `json:"record"`
	Score         float64                `json:"score"`
	MatchedFields []string               `json:"matched_fields"`
}

// SearchResponse represents the response from a search query
type SearchResponse struct {
	Results []SearchResult `json:"results"`
	Total   int            `json:"total"`
	TookMs  *int           `json:"took_ms,omitempty"`
}

// SearchQueryBuilder provides a fluent API for building search queries
type SearchQueryBuilder struct {
	query SearchQuery
}

// NewSearchQueryBuilder creates a new SearchQueryBuilder
func NewSearchQueryBuilder(queryString string) *SearchQueryBuilder {
	return &SearchQueryBuilder{
		query: SearchQuery{
			Query: queryString,
		},
	}
}

// Language sets the language for stemming
func (sb *SearchQueryBuilder) Language(language string) *SearchQueryBuilder {
	sb.query.Language = &language
	return sb
}

// CaseSensitive enables case-sensitive search
func (sb *SearchQueryBuilder) CaseSensitive(enabled bool) *SearchQueryBuilder {
	sb.query.CaseSensitive = &enabled
	return sb
}

// Fuzzy enables fuzzy matching
func (sb *SearchQueryBuilder) Fuzzy(enabled bool) *SearchQueryBuilder {
	sb.query.Fuzzy = &enabled
	return sb
}

// MinScore sets minimum score threshold
func (sb *SearchQueryBuilder) MinScore(score float64) *SearchQueryBuilder {
	sb.query.MinScore = &score
	return sb
}

// Fields sets fields to search in (comma-separated)
func (sb *SearchQueryBuilder) Fields(fields string) *SearchQueryBuilder {
	sb.query.Fields = &fields
	return sb
}

// Weights sets field weights (format: "field1:2.0,field2:1.5")
func (sb *SearchQueryBuilder) Weights(weights string) *SearchQueryBuilder {
	sb.query.Weights = &weights
	return sb
}

// EnableStemming enables stemming
func (sb *SearchQueryBuilder) EnableStemming(enabled bool) *SearchQueryBuilder {
	sb.query.EnableStemming = &enabled
	return sb
}

// BoostExact boosts exact matches
func (sb *SearchQueryBuilder) BoostExact(enabled bool) *SearchQueryBuilder {
	sb.query.BoostExact = &enabled
	return sb
}

// MaxEditDistance sets maximum edit distance for fuzzy matching
func (sb *SearchQueryBuilder) MaxEditDistance(distance int) *SearchQueryBuilder {
	sb.query.MaxEditDistance = &distance
	return sb
}

// Vector sets query vector for semantic search
func (sb *SearchQueryBuilder) Vector(vector []float64) *SearchQueryBuilder {
	sb.query.Vector = vector
	return sb
}

// VectorField sets vector field name
func (sb *SearchQueryBuilder) VectorField(field string) *SearchQueryBuilder {
	sb.query.VectorField = &field
	return sb
}

// VectorMetric sets vector similarity metric
func (sb *SearchQueryBuilder) VectorMetric(metric string) *SearchQueryBuilder {
	sb.query.VectorMetric = &metric
	return sb
}

// VectorK sets number of vector results (k-nearest neighbors)
func (sb *SearchQueryBuilder) VectorK(k int) *SearchQueryBuilder {
	sb.query.VectorK = &k
	return sb
}

// VectorThreshold sets minimum similarity threshold
func (sb *SearchQueryBuilder) VectorThreshold(threshold float64) *SearchQueryBuilder {
	sb.query.VectorThreshold = &threshold
	return sb
}

// TextWeight sets text search weight for hybrid search
func (sb *SearchQueryBuilder) TextWeight(weight float64) *SearchQueryBuilder {
	sb.query.TextWeight = &weight
	return sb
}

// VectorWeight sets vector search weight for hybrid search
func (sb *SearchQueryBuilder) VectorWeight(weight float64) *SearchQueryBuilder {
	sb.query.VectorWeight = &weight
	return sb
}

// BypassRipple bypasses ripple cache
func (sb *SearchQueryBuilder) BypassRipple(bypass bool) *SearchQueryBuilder {
	sb.query.BypassRipple = &bypass
	return sb
}

// BypassCache bypasses cache
func (sb *SearchQueryBuilder) BypassCache(bypass bool) *SearchQueryBuilder {
	sb.query.BypassCache = &bypass
	return sb
}

// Limit sets maximum number of results to return
func (sb *SearchQueryBuilder) Limit(limit int) *SearchQueryBuilder {
	sb.query.Limit = &limit
	return sb
}

// Build builds the final SearchQuery
func (sb *SearchQueryBuilder) Build() SearchQuery {
	return sb.query
}

// Search performs a search query on a collection
func (c *Client) Search(collection string, searchQuery SearchQuery) (*SearchResponse, error) {
	endpoint := fmt.Sprintf("/api/search/%s", collection)

	data, err := c.makeRequest("POST", endpoint, searchQuery)
	if err != nil {
		return nil, err
	}

	var response SearchResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}

	return &response, nil
}
