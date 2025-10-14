// Package ekodb provides a Go client for ekoDB
package ekodb

import (
	"encoding/json"
	"fmt"
)

// VectorIndexAlgorithm represents the vector index algorithm
type VectorIndexAlgorithm string

const (
	// VectorIndexFlat represents simple flat index (brute force)
	VectorIndexFlat VectorIndexAlgorithm = "flat"
	// VectorIndexHNSW represents Hierarchical Navigable Small World
	VectorIndexHNSW VectorIndexAlgorithm = "hnsw"
	// VectorIndexIVF represents Inverted File Index
	VectorIndexIVF VectorIndexAlgorithm = "ivf"
)

// DistanceMetric represents the distance metric for vector similarity
type DistanceMetric string

const (
	// DistanceMetricCosine represents cosine similarity
	DistanceMetricCosine DistanceMetric = "cosine"
	// DistanceMetricEuclidean represents euclidean distance
	DistanceMetricEuclidean DistanceMetric = "euclidean"
	// DistanceMetricDotProduct represents dot product
	DistanceMetricDotProduct DistanceMetric = "dotproduct"
)

// IndexConfig represents index configuration for a field
type IndexConfig struct {
	Type           string                `json:"type"`
	Language       *string               `json:"language,omitempty"`
	Analyzer       *string               `json:"analyzer,omitempty"`
	Algorithm      *VectorIndexAlgorithm `json:"algorithm,omitempty"`
	Metric         *DistanceMetric       `json:"metric,omitempty"`
	M              *int                  `json:"m,omitempty"`
	EfConstruction *int                  `json:"ef_construction,omitempty"`
}

// FieldTypeSchema represents field type schema with constraints
type FieldTypeSchema struct {
	FieldType string        `json:"field_type"`
	Default   interface{}   `json:"default,omitempty"`
	Unique    bool          `json:"unique,omitempty"`
	Required  bool          `json:"required,omitempty"`
	Enums     []interface{} `json:"enums,omitempty"`
	Max       interface{}   `json:"max,omitempty"`
	Min       interface{}   `json:"min,omitempty"`
	Regex     *string       `json:"regex,omitempty"`
	Index     *IndexConfig  `json:"index,omitempty"`
}

// Schema represents a collection schema
type Schema struct {
	Fields       map[string]FieldTypeSchema `json:"fields"`
	Version      *int                       `json:"version,omitempty"`
	CreatedAt    *string                    `json:"created_at,omitempty"`
	LastModified *string                    `json:"last_modified,omitempty"`
	BypassRipple *bool                      `json:"bypass_ripple,omitempty"`
}

// CollectionMetadata represents collection metadata with analytics
type CollectionMetadata struct {
	Collection Schema      `json:"collection"`
	Analytics  interface{} `json:"analytics,omitempty"`
}

// FieldTypeSchemaBuilder provides a fluent API for building field type schemas
type FieldTypeSchemaBuilder struct {
	schema FieldTypeSchema
}

// NewFieldTypeSchemaBuilder creates a new FieldTypeSchemaBuilder
func NewFieldTypeSchemaBuilder(fieldType string) *FieldTypeSchemaBuilder {
	return &FieldTypeSchemaBuilder{
		schema: FieldTypeSchema{
			FieldType: fieldType,
		},
	}
}

// Required sets the field as required
func (fb *FieldTypeSchemaBuilder) Required() *FieldTypeSchemaBuilder {
	fb.schema.Required = true
	return fb
}

// Unique sets the field as unique
func (fb *FieldTypeSchemaBuilder) Unique() *FieldTypeSchemaBuilder {
	fb.schema.Unique = true
	return fb
}

// DefaultValue sets a default value
func (fb *FieldTypeSchemaBuilder) DefaultValue(value interface{}) *FieldTypeSchemaBuilder {
	fb.schema.Default = value
	return fb
}

// Enums sets enum values
func (fb *FieldTypeSchemaBuilder) Enums(values []interface{}) *FieldTypeSchemaBuilder {
	fb.schema.Enums = values
	return fb
}

// Range sets min/max range
func (fb *FieldTypeSchemaBuilder) Range(min, max interface{}) *FieldTypeSchemaBuilder {
	fb.schema.Min = min
	fb.schema.Max = max
	return fb
}

// Pattern sets regex pattern
func (fb *FieldTypeSchemaBuilder) Pattern(regex string) *FieldTypeSchemaBuilder {
	fb.schema.Regex = &regex
	return fb
}

// TextIndex adds a text index
func (fb *FieldTypeSchemaBuilder) TextIndex(language string) *FieldTypeSchemaBuilder {
	fb.schema.Index = &IndexConfig{
		Type:     "text",
		Language: &language,
	}
	return fb
}

// VectorIndex adds a vector index
func (fb *FieldTypeSchemaBuilder) VectorIndex(algorithm VectorIndexAlgorithm, metric DistanceMetric, m, efConstruction int) *FieldTypeSchemaBuilder {
	fb.schema.Index = &IndexConfig{
		Type:           "vector",
		Algorithm:      &algorithm,
		Metric:         &metric,
		M:              &m,
		EfConstruction: &efConstruction,
	}
	return fb
}

// BTreeIndex adds a B-tree index
func (fb *FieldTypeSchemaBuilder) BTreeIndex() *FieldTypeSchemaBuilder {
	fb.schema.Index = &IndexConfig{
		Type: "btree",
	}
	return fb
}

// HashIndex adds a hash index
func (fb *FieldTypeSchemaBuilder) HashIndex() *FieldTypeSchemaBuilder {
	fb.schema.Index = &IndexConfig{
		Type: "hash",
	}
	return fb
}

// Build builds the final FieldTypeSchema
func (fb *FieldTypeSchemaBuilder) Build() FieldTypeSchema {
	return fb.schema
}

// SchemaBuilder provides a fluent API for building collection schemas
type SchemaBuilder struct {
	schema Schema
}

// NewSchemaBuilder creates a new SchemaBuilder
func NewSchemaBuilder() *SchemaBuilder {
	version := 1
	bypassRipple := true
	return &SchemaBuilder{
		schema: Schema{
			Fields:       make(map[string]FieldTypeSchema),
			Version:      &version,
			BypassRipple: &bypassRipple,
		},
	}
}

// AddField adds a field to the schema
func (sb *SchemaBuilder) AddField(name string, field FieldTypeSchema) *SchemaBuilder {
	sb.schema.Fields[name] = field
	return sb
}

// BypassRipple sets bypass_ripple flag
func (sb *SchemaBuilder) BypassRipple(bypass bool) *SchemaBuilder {
	sb.schema.BypassRipple = &bypass
	return sb
}

// Version sets schema version
func (sb *SchemaBuilder) Version(version int) *SchemaBuilder {
	sb.schema.Version = &version
	return sb
}

// Build builds the final Schema
func (sb *SchemaBuilder) Build() Schema {
	return sb.schema
}

// CreateCollection creates a collection with schema
func (c *Client) CreateCollection(collection string, schema Schema) error {
	endpoint := fmt.Sprintf("/api/collections/%s", collection)
	_, err := c.makeRequest("POST", endpoint, schema)
	return err
}

// GetCollection gets collection metadata and schema
func (c *Client) GetCollection(collection string) (*CollectionMetadata, error) {
	endpoint := fmt.Sprintf("/api/collections/%s", collection)

	data, err := c.makeRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var metadata CollectionMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}

	return &metadata, nil
}

// GetSchema gets collection schema
func (c *Client) GetSchema(collection string) (*Schema, error) {
	metadata, err := c.GetCollection(collection)
	if err != nil {
		return nil, err
	}

	return &metadata.Collection, nil
}
