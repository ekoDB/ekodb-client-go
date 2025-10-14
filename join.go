// Package ekodb provides a Go client for ekoDB
package ekodb

// JoinConfig represents configuration for joining collections
type JoinConfig struct {
	Collections  []string `json:"collections"`
	LocalField   string   `json:"local_field"`
	ForeignField string   `json:"foreign_field"`
	AsField      string   `json:"as_field"`
}

// NewJoinConfig creates a new join configuration
func NewJoinConfig(collections []string, localField, foreignField, asField string) JoinConfig {
	return JoinConfig{
		Collections:  collections,
		LocalField:   localField,
		ForeignField: foreignField,
		AsField:      asField,
	}
}

// NewSingleJoin creates a join with a single collection
func NewSingleJoin(collection, localField, foreignField, asField string) JoinConfig {
	return JoinConfig{
		Collections:  []string{collection},
		LocalField:   localField,
		ForeignField: foreignField,
		AsField:      asField,
	}
}

// ToMap converts JoinConfig to a map for use in queries
func (j JoinConfig) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"collections":   j.Collections,
		"local_field":   j.LocalField,
		"foreign_field": j.ForeignField,
		"as_field":      j.AsField,
	}
}
