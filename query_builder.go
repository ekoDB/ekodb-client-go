// Package ekodb provides a Go client for ekoDB
package ekodb

import "encoding/json"

// SortOrder represents the sort direction
type SortOrder string

const (
	// SortAsc represents ascending sort order
	SortAsc SortOrder = "asc"
	// SortDesc represents descending sort order
	SortDesc SortOrder = "desc"
)

// QueryBuilder provides a fluent API for building complex queries
type QueryBuilder struct {
	filters      []map[string]interface{}
	sortFields   []map[string]interface{}
	limit        *int
	skip         *int
	join         map[string]interface{}
	bypassCache  bool
	bypassRipple bool
}

// NewQueryBuilder creates a new QueryBuilder
func NewQueryBuilder() *QueryBuilder {
	return &QueryBuilder{
		filters:    make([]map[string]interface{}, 0),
		sortFields: make([]map[string]interface{}, 0),
	}
}

// Eq adds an equality filter (Eq operator)
func (qb *QueryBuilder) Eq(field string, value interface{}) *QueryBuilder {
	qb.filters = append(qb.filters, map[string]interface{}{
		"type": "Condition",
		"content": map[string]interface{}{
			"field":    field,
			"operator": "Eq",
			"value":    value,
		},
	})
	return qb
}

// Ne adds a not-equal filter (Ne operator)
func (qb *QueryBuilder) Ne(field string, value interface{}) *QueryBuilder {
	qb.filters = append(qb.filters, map[string]interface{}{
		"type": "Condition",
		"content": map[string]interface{}{
			"field":    field,
			"operator": "Ne",
			"value":    value,
		},
	})
	return qb
}

// Gt adds a greater-than filter (Gt operator)
func (qb *QueryBuilder) Gt(field string, value interface{}) *QueryBuilder {
	qb.filters = append(qb.filters, map[string]interface{}{
		"type": "Condition",
		"content": map[string]interface{}{
			"field":    field,
			"operator": "Gt",
			"value":    value,
		},
	})
	return qb
}

// Gte adds a greater-than-or-equal filter (Gte operator)
func (qb *QueryBuilder) Gte(field string, value interface{}) *QueryBuilder {
	qb.filters = append(qb.filters, map[string]interface{}{
		"type": "Condition",
		"content": map[string]interface{}{
			"field":    field,
			"operator": "Gte",
			"value":    value,
		},
	})
	return qb
}

// Lt adds a less-than filter (Lt operator)
func (qb *QueryBuilder) Lt(field string, value interface{}) *QueryBuilder {
	qb.filters = append(qb.filters, map[string]interface{}{
		"type": "Condition",
		"content": map[string]interface{}{
			"field":    field,
			"operator": "Lt",
			"value":    value,
		},
	})
	return qb
}

// Lte adds a less-than-or-equal filter (Lte operator)
func (qb *QueryBuilder) Lte(field string, value interface{}) *QueryBuilder {
	qb.filters = append(qb.filters, map[string]interface{}{
		"type": "Condition",
		"content": map[string]interface{}{
			"field":    field,
			"operator": "Lte",
			"value":    value,
		},
	})
	return qb
}

// In adds an in-array filter (In operator)
func (qb *QueryBuilder) In(field string, values []interface{}) *QueryBuilder {
	qb.filters = append(qb.filters, map[string]interface{}{
		"type": "Condition",
		"content": map[string]interface{}{
			"field":    field,
			"operator": "In",
			"value":    values,
		},
	})
	return qb
}

// Nin adds a not-in-array filter (NotIn operator)
func (qb *QueryBuilder) Nin(field string, values []interface{}) *QueryBuilder {
	qb.filters = append(qb.filters, map[string]interface{}{
		"type": "Condition",
		"content": map[string]interface{}{
			"field":    field,
			"operator": "NotIn",
			"value":    values,
		},
	})
	return qb
}

// Contains adds a contains filter (substring match)
func (qb *QueryBuilder) Contains(field string, substring string) *QueryBuilder {
	qb.filters = append(qb.filters, map[string]interface{}{
		"type": "Condition",
		"content": map[string]interface{}{
			"field":    field,
			"operator": "Contains",
			"value":    substring,
		},
	})
	return qb
}

// StartsWith adds a starts-with filter
func (qb *QueryBuilder) StartsWith(field string, prefix string) *QueryBuilder {
	qb.filters = append(qb.filters, map[string]interface{}{
		"type": "Condition",
		"content": map[string]interface{}{
			"field":    field,
			"operator": "StartsWith",
			"value":    prefix,
		},
	})
	return qb
}

// EndsWith adds an ends-with filter
func (qb *QueryBuilder) EndsWith(field string, suffix string) *QueryBuilder {
	qb.filters = append(qb.filters, map[string]interface{}{
		"type": "Condition",
		"content": map[string]interface{}{
			"field":    field,
			"operator": "EndsWith",
			"value":    suffix,
		},
	})
	return qb
}

// Regex adds a regex pattern match filter
func (qb *QueryBuilder) Regex(field string, pattern string) *QueryBuilder {
	qb.filters = append(qb.filters, map[string]interface{}{
		"type": "Condition",
		"content": map[string]interface{}{
			"field":    field,
			"operator": "Regex",
			"value":    pattern,
		},
	})
	return qb
}

// And combines filters with AND logic
func (qb *QueryBuilder) And(conditions []map[string]interface{}) *QueryBuilder {
	qb.filters = append(qb.filters, map[string]interface{}{
		"type": "Logical",
		"content": map[string]interface{}{
			"operator":    "And",
			"expressions": conditions,
		},
	})
	return qb
}

// Or combines filters with OR logic
func (qb *QueryBuilder) Or(conditions []map[string]interface{}) *QueryBuilder {
	qb.filters = append(qb.filters, map[string]interface{}{
		"type": "Logical",
		"content": map[string]interface{}{
			"operator":    "Or",
			"expressions": conditions,
		},
	})
	return qb
}

// Not negates a filter
func (qb *QueryBuilder) Not(condition map[string]interface{}) *QueryBuilder {
	qb.filters = append(qb.filters, map[string]interface{}{
		"type": "Logical",
		"content": map[string]interface{}{
			"operator":    "Not",
			"expressions": []map[string]interface{}{condition},
		},
	})
	return qb
}

// SortAscending adds a sort field in ascending order
func (qb *QueryBuilder) SortAscending(field string) *QueryBuilder {
	qb.sortFields = append(qb.sortFields, map[string]interface{}{
		"field":     field,
		"ascending": true,
	})
	return qb
}

// SortDescending adds a sort field in descending order
func (qb *QueryBuilder) SortDescending(field string) *QueryBuilder {
	qb.sortFields = append(qb.sortFields, map[string]interface{}{
		"field":     field,
		"ascending": false,
	})
	return qb
}

// Limit sets the maximum number of results
func (qb *QueryBuilder) Limit(limit int) *QueryBuilder {
	qb.limit = &limit
	return qb
}

// Skip sets the number of results to skip (for pagination)
func (qb *QueryBuilder) Skip(skip int) *QueryBuilder {
	qb.skip = &skip
	return qb
}

// Page sets page number and page size (convenience method)
func (qb *QueryBuilder) Page(page, pageSize int) *QueryBuilder {
	skip := page * pageSize
	qb.skip = &skip
	qb.limit = &pageSize
	return qb
}

// Join adds a join configuration
func (qb *QueryBuilder) Join(joinConfig map[string]interface{}) *QueryBuilder {
	qb.join = joinConfig
	return qb
}

// BypassCache bypasses cache for this query
func (qb *QueryBuilder) BypassCache(bypass bool) *QueryBuilder {
	qb.bypassCache = bypass
	return qb
}

// BypassRipple bypasses ripple for this query
func (qb *QueryBuilder) BypassRipple(bypass bool) *QueryBuilder {
	qb.bypassRipple = bypass
	return qb
}

// Build builds the final query map
func (qb *QueryBuilder) Build() map[string]interface{} {
	query := make(map[string]interface{})

	// Combine all filters with AND logic if multiple filters exist
	if len(qb.filters) > 0 {
		if len(qb.filters) == 1 {
			query["filter"] = qb.filters[0]
		} else {
			query["filter"] = map[string]interface{}{
				"type": "Logical",
				"content": map[string]interface{}{
					"operator":    "And",
					"expressions": qb.filters,
				},
			}
		}
	}

	// Add sort fields
	if len(qb.sortFields) > 0 {
		query["sort"] = qb.sortFields
	}

	// Add pagination
	if qb.limit != nil {
		query["limit"] = *qb.limit
	}
	if qb.skip != nil {
		query["skip"] = *qb.skip
	}

	// Add join
	if qb.join != nil {
		query["join"] = qb.join
	}

	// Add bypass flags
	if qb.bypassCache {
		query["bypass_cache"] = true
	}
	if qb.bypassRipple {
		query["bypass_ripple"] = true
	}

	return query
}

// BuildJSON builds the final query as JSON bytes
func (qb *QueryBuilder) BuildJSON() ([]byte, error) {
	query := qb.Build()
	return json.Marshal(query)
}
