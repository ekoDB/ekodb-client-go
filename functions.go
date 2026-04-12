package ekodb

import (
	"encoding/json"
	"fmt"
	"time"
)

// UserFunction is a reusable sequence of Functions stored in ekoDB.
// Called by label via the call_function chat tool or REST API.
type UserFunction struct {
	Label       string                         `json:"label"`
	Name        string                         `json:"name"`
	Description *string                        `json:"description,omitempty"`
	Version     *string                        `json:"version,omitempty"`
	Parameters  map[string]ParameterDefinition `json:"parameters"`
	Functions   []FunctionStageConfig          `json:"functions"`
	Tags        []string                       `json:"tags,omitempty"`
	ID          *string                        `json:"id,omitempty"`
	CreatedAt   *time.Time                     `json:"created_at,omitempty"`
	UpdatedAt   *time.Time                     `json:"updated_at,omitempty"`
}

// ParameterDefinition for function parameters
type ParameterDefinition struct {
	Required    bool        `json:"required"`
	Default     interface{} `json:"default,omitempty"`
	Description string      `json:"description,omitempty"`
}

// ParameterValue removed - use direct values or string interpolation "{{param}}" instead

// FunctionStageConfig represents a pipeline stage
type FunctionStageConfig struct {
	Stage string                 `json:"stage"`
	Data  map[string]interface{} `json:",inline"`
}

// MarshalJSON custom marshaling for FunctionStageConfig
func (f FunctionStageConfig) MarshalJSON() ([]byte, error) {
	m := make(map[string]interface{})
	m["type"] = f.Stage
	for k, v := range f.Data {
		m[k] = v
	}
	return json.Marshal(m)
}

// Parameter returns the structural placeholder
// `{"type": "Parameter", "name": name}` that ekoDB's `resolve_json_parameters`
// recognizes inside Insert.record, Update.updates, UpdateById.updates,
// FindOneAndUpdate.updates, BatchInsert.records, and any query filter
// expression value.
//
// At function-call time, ekoDB replaces the placeholder with the actual
// parameter value, preserving the native FieldType (Binary, DateTime, UUID,
// Decimal, Duration, Number, Set, Vector) via the `{type,value}` wrapped
// form. Safe to use for any type — scalars and containers alike.
//
// This is the structural alternative to the text-level `"{{name}}"`
// placeholder; both are accepted by the server. Prefer structural when the
// parameter is a whole-object record or a value whose type would be lost
// on a raw-JSON round-trip.
//
// Example — items_create function:
//
//	StageInsert("items", Parameter("record"), false, nil)
//
// Example — items_update function:
//
//	StageUpdateById("items", "{{id}}", Parameter("updates"), false, nil)
func Parameter(name string) map[string]interface{} {
	return map[string]interface{}{
		"type": "Parameter",
		"name": name,
	}
}

// Stage builders
func StageFindAll(collection string) FunctionStageConfig {
	return FunctionStageConfig{
		Stage: "FindAll",
		Data:  map[string]interface{}{"collection": collection},
	}
}

// StageQuery queries a collection with optional filter, sort, limit, skip
func StageQuery(collection string, filter interface{}, sort []SortFieldConfig, limit, skip *int) FunctionStageConfig {
	data := map[string]interface{}{
		"collection": collection,
	}
	if filter != nil {
		data["filter"] = filter
	}
	if sort != nil {
		data["sort"] = sort
	}
	if limit != nil {
		data["limit"] = limit
	}
	if skip != nil {
		data["skip"] = skip
	}
	return FunctionStageConfig{
		Stage: "Query",
		Data:  data,
	}
}

// StageProject selects or excludes specific fields from results
// exclude=false means include only these fields, exclude=true means exclude these fields
func StageProject(fields []string, exclude bool) FunctionStageConfig {
	return FunctionStageConfig{
		Stage: "Project",
		Data: map[string]interface{}{
			"fields":  fields,
			"exclude": exclude,
		},
	}
}

func StageGroup(by_fields []string, functions []GroupFunctionConfig) FunctionStageConfig {
	return FunctionStageConfig{
		Stage: "Group",
		Data: map[string]interface{}{
			"by_fields": by_fields,
			"functions": functions,
		},
	}
}

func StageCount(outputField string) FunctionStageConfig {
	if outputField == "" {
		outputField = "count"
	}
	return FunctionStageConfig{Stage: "Count", Data: map[string]interface{}{
		"output_field": outputField,
	}}
}

// StageInsert inserts a single record into a collection
func StageInsert(collection string, record map[string]interface{}, bypassRipple bool, ttl *int64) FunctionStageConfig {
	data := map[string]interface{}{
		"collection":    collection,
		"record":        record,
		"bypass_ripple": bypassRipple,
	}
	if ttl != nil {
		data["ttl"] = ttl
	}
	return FunctionStageConfig{
		Stage: "Insert",
		Data:  data,
	}
}

// StageDelete deletes records matching a filter (use StageDeleteById for ID-based deletion)
func StageDelete(collection string, filter interface{}, bypassRipple bool) FunctionStageConfig {
	return FunctionStageConfig{
		Stage: "Delete",
		Data: map[string]interface{}{
			"collection":    collection,
			"filter":        filter,
			"bypass_ripple": bypassRipple,
		},
	}
}

// StageDeleteById deletes a specific record by ID
func StageDeleteById(collection string, recordId string, bypassRipple bool) FunctionStageConfig {
	return FunctionStageConfig{
		Stage: "DeleteById",
		Data: map[string]interface{}{
			"collection":    collection,
			"record_id":     recordId,
			"bypass_ripple": bypassRipple,
		},
	}
}

// StageBatchInsert inserts multiple records into a collection
func StageBatchInsert(collection string, records []map[string]interface{}, bypassRipple bool) FunctionStageConfig {
	return FunctionStageConfig{
		Stage: "BatchInsert",
		Data: map[string]interface{}{
			"collection":    collection,
			"records":       records,
			"bypass_ripple": bypassRipple,
		},
	}
}

func StageBatchDelete(collection string, ids []string, bypassRipple bool) FunctionStageConfig {
	return FunctionStageConfig{
		Stage: "BatchDelete",
		Data: map[string]interface{}{
			"collection":    collection,
			"ids":           ids,
			"bypass_ripple": bypassRipple,
		},
	}
}

func StageHttpRequest(url, method string, headers map[string]string, body interface{}) FunctionStageConfig {
	data := map[string]interface{}{
		"url":    url,
		"method": method,
	}
	if headers != nil {
		data["headers"] = headers
	}
	if body != nil {
		data["body"] = body
	}
	return FunctionStageConfig{Stage: "HttpRequest", Data: data}
}

// StageVectorSearch performs vector similarity search
func StageVectorSearch(collection string, queryVector []float64, limit *int, threshold *float64) FunctionStageConfig {
	data := map[string]interface{}{
		"collection":   collection,
		"query_vector": queryVector,
	}
	if limit != nil {
		data["limit"] = limit
	}
	if threshold != nil {
		data["threshold"] = threshold
	}
	return FunctionStageConfig{Stage: "VectorSearch", Data: data}
}

func StageTextSearch(collection string, queryText string, options map[string]interface{}) FunctionStageConfig {
	data := map[string]interface{}{
		"collection": collection,
		"query_text": queryText,
	}
	if options != nil {
		if fields, ok := options["fields"].([]string); ok {
			data["fields"] = fields
		}
		if limit, ok := options["limit"]; ok {
			data["limit"] = limit
		}
		if fuzzy, ok := options["fuzzy"].(bool); ok {
			data["fuzzy"] = fuzzy
		}
	}
	return FunctionStageConfig{Stage: "TextSearch", Data: data}
}

func StageHybridSearch(collection string, queryText string, queryVector []float64, options map[string]interface{}) FunctionStageConfig {
	data := map[string]interface{}{
		"collection":   collection,
		"query_text":   queryText,
		"query_vector": queryVector,
	}
	if options != nil {
		if limit, ok := options["limit"]; ok {
			data["limit"] = limit
		}
	}
	return FunctionStageConfig{Stage: "HybridSearch", Data: data}
}

func StageChat(messages []ChatMessage, model *string, temperature *float64) FunctionStageConfig {
	data := map[string]interface{}{"messages": messages}
	if model != nil {
		data["model"] = model
	}
	if temperature != nil {
		data["temperature"] = temperature
	}
	return FunctionStageConfig{Stage: "Chat", Data: data}
}

func StageEmbed(texts interface{}, model *string) FunctionStageConfig {
	data := map[string]interface{}{"texts": texts}
	if model != nil {
		data["model"] = model
	}
	return FunctionStageConfig{Stage: "Embed", Data: data}
}

// StageFindById finds a specific record by ID
func StageFindById(collection string, recordId string) FunctionStageConfig {
	return FunctionStageConfig{
		Stage: "FindById",
		Data: map[string]interface{}{
			"collection": collection,
			"record_id":  recordId,
		},
	}
}

// StageFindOne finds one record by key/value pair
func StageFindOne(collection string, key string, value interface{}) FunctionStageConfig {
	return FunctionStageConfig{
		Stage: "FindOne",
		Data: map[string]interface{}{
			"collection": collection,
			"key":        key,
			"value":      value,
		},
	}
}

// StageUpdate updates records matching a filter
func StageUpdate(collection string, filter interface{}, updates map[string]interface{}, bypassRipple bool, ttl *int64) FunctionStageConfig {
	data := map[string]interface{}{
		"collection":    collection,
		"filter":        filter,
		"updates":       updates,
		"bypass_ripple": bypassRipple,
	}
	if ttl != nil {
		data["ttl"] = ttl
	}
	return FunctionStageConfig{
		Stage: "Update",
		Data:  data,
	}
}

// StageUpdateById updates a specific record by ID
func StageUpdateById(collection string, recordId string, updates map[string]interface{}, bypassRipple bool, ttl *int64) FunctionStageConfig {
	data := map[string]interface{}{
		"collection":    collection,
		"record_id":     recordId,
		"updates":       updates,
		"bypass_ripple": bypassRipple,
	}
	if ttl != nil {
		data["ttl"] = ttl
	}
	return FunctionStageConfig{
		Stage: "UpdateById",
		Data:  data,
	}
}

// StageFindOneAndUpdate finds and updates a record atomically
func StageFindOneAndUpdate(collection string, recordId string, updates map[string]interface{}, bypassRipple bool, ttl *int64) FunctionStageConfig {
	data := map[string]interface{}{
		"collection":    collection,
		"record_id":     recordId,
		"updates":       updates,
		"bypass_ripple": bypassRipple,
	}
	if ttl != nil {
		data["ttl"] = ttl
	}
	return FunctionStageConfig{
		Stage: "FindOneAndUpdate",
		Data:  data,
	}
}

// UpdateAction represents valid actions for StageUpdateWithAction
type UpdateAction string

const (
	// UpdateActionPush appends a value to an array field
	UpdateActionPush UpdateAction = "push"
	// UpdateActionPop removes the last element from an array field
	UpdateActionPop UpdateAction = "pop"
	// UpdateActionIncrement adds a numeric value to a field
	UpdateActionIncrement UpdateAction = "increment"
	// UpdateActionDecrement subtracts a numeric value from a field
	UpdateActionDecrement UpdateAction = "decrement"
	// UpdateActionRemove removes a specific value from an array field
	UpdateActionRemove UpdateAction = "remove"
)

// StageUpdateWithAction updates a record with a specific action (push, pop, increment, decrement, remove).
// Use the UpdateAction constants for type safety: UpdateActionPush, UpdateActionPop, UpdateActionIncrement,
// UpdateActionDecrement, UpdateActionRemove.
func StageUpdateWithAction(collection string, recordId string, action string, field string, value interface{}, bypassRipple bool) FunctionStageConfig {
	return FunctionStageConfig{
		Stage: "UpdateWithAction",
		Data: map[string]interface{}{
			"collection":    collection,
			"record_id":     recordId,
			"action":        action,
			"field":         field,
			"value":         value,
			"bypass_ripple": bypassRipple,
		},
	}
}

// FunctionCondition represents a conditional expression for If stages in functions.
// Conditions are evaluated against the current pipeline state (records, counts, field values)
// and can be composed using logical operators (And, Or, Not).
//
// Example usage:
//
//	cond := ConditionAnd([]FunctionCondition{
//		ConditionHasRecords(),
//		ConditionCountGreaterThan(5),
//	})
//	StageIf(cond, thenFunctions, elseFunctions)
type FunctionCondition struct {
	Type       string              // Condition type (HasRecords, FieldEquals, CountEquals, And, Or, Not, etc.)
	Field      string              // Field name for field-based conditions
	FieldValue interface{}         // Expected value for comparison conditions (FieldEquals)
	Count      int                 // Count threshold for count-based conditions
	Conditions []FunctionCondition // Child conditions for And/Or operators
	Condition  *FunctionCondition  // Single child condition for Not operator
}

// MarshalJSON implements adjacently-tagged serialization for FunctionCondition.
// Format: { "type": "...", "value": { ...data } } for variants with data
// Unit variants like HasRecords have no value field.
func (c FunctionCondition) MarshalJSON() ([]byte, error) {
	switch c.Type {
	case "HasRecords":
		return json.Marshal(map[string]string{"type": c.Type})
	case "FieldEquals":
		return json.Marshal(map[string]interface{}{
			"type": c.Type,
			"value": map[string]interface{}{
				"field": c.Field,
				"value": c.FieldValue,
			},
		})
	case "FieldExists":
		return json.Marshal(map[string]interface{}{
			"type": c.Type,
			"value": map[string]interface{}{
				"field": c.Field,
			},
		})
	case "CountEquals", "CountGreaterThan", "CountLessThan":
		return json.Marshal(map[string]interface{}{
			"type": c.Type,
			"value": map[string]interface{}{
				"count": c.Count,
			},
		})
	case "And", "Or":
		return json.Marshal(map[string]interface{}{
			"type": c.Type,
			"value": map[string]interface{}{
				"conditions": c.Conditions,
			},
		})
	case "Not":
		return json.Marshal(map[string]interface{}{
			"type": c.Type,
			"value": map[string]interface{}{
				"condition": c.Condition,
			},
		})
	default:
		return json.Marshal(map[string]string{"type": c.Type})
	}
}

// Condition builders

// ConditionHasRecords creates a condition that is satisfied when the current pipeline
// stage has one or more records. Useful for checking if a query returned any results.
func ConditionHasRecords() FunctionCondition {
	return FunctionCondition{Type: "HasRecords"}
}

// ConditionFieldEquals creates a condition that is satisfied when the specified field
// in the current record(s) equals the provided value. Field comparison is type-aware.
func ConditionFieldEquals(field string, value interface{}) FunctionCondition {
	return FunctionCondition{Type: "FieldEquals", Field: field, FieldValue: value}
}

// ConditionFieldExists creates a condition that is satisfied when the specified field
// exists in the current record(s), regardless of its value (including null).
func ConditionFieldExists(field string) FunctionCondition {
	return FunctionCondition{Type: "FieldExists", Field: field}
}

// ConditionCountEquals creates a condition that is satisfied when the number of
// records in the current pipeline stage exactly equals the provided count.
func ConditionCountEquals(count int) FunctionCondition {
	return FunctionCondition{Type: "CountEquals", Count: count}
}

// ConditionCountGreaterThan creates a condition that is satisfied when the number
// of records in the current pipeline stage is strictly greater than the provided count.
func ConditionCountGreaterThan(count int) FunctionCondition {
	return FunctionCondition{Type: "CountGreaterThan", Count: count}
}

// ConditionCountLessThan creates a condition that is satisfied when the number
// of records in the current pipeline stage is strictly less than the provided count.
func ConditionCountLessThan(count int) FunctionCondition {
	return FunctionCondition{Type: "CountLessThan", Count: count}
}

// ConditionAnd creates a condition that requires all of the provided child conditions
// to be satisfied (logical AND). All conditions are evaluated and must pass.
func ConditionAnd(conditions []FunctionCondition) FunctionCondition {
	return FunctionCondition{Type: "And", Conditions: conditions}
}

// ConditionOr creates a condition that is satisfied when at least one of the provided
// child conditions is satisfied (logical OR). Evaluation may short-circuit.
func ConditionOr(conditions []FunctionCondition) FunctionCondition {
	return FunctionCondition{Type: "Or", Conditions: conditions}
}

// ConditionNot creates a condition that inverts the result of the provided child
// condition (logical NOT). Returns true when the child condition is false.
func ConditionNot(condition FunctionCondition) FunctionCondition {
	return FunctionCondition{Type: "Not", Condition: &condition}
}

// StageIf executes functions conditionally
func StageIf(condition FunctionCondition, thenFunctions []FunctionStageConfig, elseFunctions []FunctionStageConfig) FunctionStageConfig {
	data := map[string]interface{}{
		"condition":      condition,
		"then_functions": thenFunctions,
	}
	if len(elseFunctions) > 0 {
		data["else_functions"] = elseFunctions
	}
	return FunctionStageConfig{
		Stage: "If",
		Data:  data,
	}
}

// StageForEach executes functions for each record
func StageForEach(functions []FunctionStageConfig) FunctionStageConfig {
	return FunctionStageConfig{
		Stage: "ForEach",
		Data: map[string]interface{}{
			"functions": functions,
		},
	}
}

// StageCallFunction calls a saved UserFunction by label
func StageCallFunction(functionLabel string, params map[string]interface{}) FunctionStageConfig {
	data := map[string]interface{}{
		"function_label": functionLabel,
	}
	if params != nil {
		data["params"] = params
	}
	return FunctionStageConfig{
		Stage: "CallFunction",
		Data:  data,
	}
}

// StageCreateSavepoint creates a savepoint for partial rollback
func StageCreateSavepoint(name string) FunctionStageConfig {
	return FunctionStageConfig{
		Stage: "CreateSavepoint",
		Data: map[string]interface{}{
			"name": name,
		},
	}
}

// StageRollbackToSavepoint rolls back to a specific savepoint
func StageRollbackToSavepoint(name string) FunctionStageConfig {
	return FunctionStageConfig{
		Stage: "RollbackToSavepoint",
		Data: map[string]interface{}{
			"name": name,
		},
	}
}

// StageReleaseSavepoint releases a savepoint
func StageReleaseSavepoint(name string) FunctionStageConfig {
	return FunctionStageConfig{
		Stage: "ReleaseSavepoint",
		Data: map[string]interface{}{
			"name": name,
		},
	}
}

// ============================================================================
// KV Store Operations
// ============================================================================

// StageKvGet retrieves a value from the KV store.
// Returns {value: <data>} on hit, {value: null} on miss.
func StageKvGet(key string) FunctionStageConfig {
	return FunctionStageConfig{
		Stage: "KvGet",
		Data: map[string]interface{}{
			"key": key,
		},
	}
}

// StageKvSet stores a value in the KV store
func StageKvSet(key string, value interface{}, ttl *int64) FunctionStageConfig {
	data := map[string]interface{}{
		"key":   key,
		"value": value,
	}
	if ttl != nil {
		data["ttl"] = *ttl
	}
	return FunctionStageConfig{
		Stage: "KvSet",
		Data:  data,
	}
}

// StageKvDelete deletes a key from the KV store
func StageKvDelete(key string) FunctionStageConfig {
	return FunctionStageConfig{
		Stage: "KvDelete",
		Data: map[string]interface{}{
			"key": key,
		},
	}
}

// StageKvExists checks if a key exists in the KV store
func StageKvExists(key string, outputField *string) FunctionStageConfig {
	data := map[string]interface{}{
		"key": key,
	}
	if outputField != nil {
		data["output_field"] = *outputField
	}
	return FunctionStageConfig{
		Stage: "KvExists",
		Data:  data,
	}
}

// StageKvQuery queries the KV store with a pattern
func StageKvQuery(pattern *string, includeExpired bool) FunctionStageConfig {
	data := map[string]interface{}{
		"include_expired": includeExpired,
	}
	if pattern != nil {
		data["pattern"] = *pattern
	}
	return FunctionStageConfig{
		Stage: "KvQuery",
		Data:  data,
	}
}

// StageSWR creates a Stale-While-Revalidate pattern for external API caching.
// Automatically handles: KV cache check → HTTP request → KV cache set → optional audit storage.
//
// Parameters:
//   - cacheKey: KV key for caching (supports parameter substitution like "user:{{user_id}}")
//   - ttl: Cache TTL - server accepts duration strings ("15m", "1h"), integers (seconds), or ISO timestamps
//   - url: HTTP URL to fetch from (supports parameter substitution)
//   - method: HTTP method (e.g., "GET", "POST")
//   - headers: Optional HTTP headers
//   - body: Optional HTTP request body
//   - timeoutSeconds: Optional HTTP timeout
//   - outputField: Field name for response in enriched params (nil uses server default "response")
//   - collection: Optional collection for audit trail storage
func StageSWR(
	cacheKey string,
	ttl interface{},
	url string,
	method string,
	headers map[string]string,
	body interface{},
	timeoutSeconds *int,
	outputField *string,
	collection *string,
) FunctionStageConfig {
	data := map[string]interface{}{
		"cache_key": cacheKey,
		"ttl":       ttl,
		"url":       url,
		"method":    method,
	}
	if headers != nil {
		data["headers"] = headers
	}
	if body != nil {
		data["body"] = body
	}
	if timeoutSeconds != nil {
		data["timeout_seconds"] = *timeoutSeconds
	}
	if outputField != nil {
		data["output_field"] = *outputField
	}
	if collection != nil {
		data["collection"] = *collection
	}
	return FunctionStageConfig{
		Stage: "SWR",
		Data:  data,
	}
}

// StageBcryptHash bcrypt-hashes a plaintext value and writes the result into
// every record in the working data as outputField. Use in a compound
// users_register function between input validation and Insert.
//
// The plain argument is typically a text-level placeholder like
// "{{password}}" — the substituter replaces it with the call-time param
// before this stage runs. The cost argument is the bcrypt work factor.
// ekoDB accepts values in the range 4..=31; pass nil for the ekoDB default
// (12). This builder does not validate the range client-side, so invalid
// values will error when the stage is executed by ekoDB.
//
// Requires ekoDB >= 0.41.0.
func StageBcryptHash(plain, outputField string, cost *int) FunctionStageConfig {
	data := map[string]interface{}{
		"plain":        plain,
		"output_field": outputField,
	}
	if cost != nil {
		data["cost"] = *cost
	}
	return FunctionStageConfig{
		Stage: "BcryptHash",
		Data:  data,
	}
}

// StageBcryptVerify verifies a plaintext against a bcrypt hash stored on
// the first record in the working data and writes a boolean result into
// outputField. Pair with StageIf to branch on success / failure.
//
// plain is typically "{{password}}"; hashField is the name of the field
// on the current record holding the stored hash (e.g. "password_hash").
//
// Requires ekoDB >= 0.41.0.
func StageBcryptVerify(plain, hashField, outputField string) FunctionStageConfig {
	return FunctionStageConfig{
		Stage: "BcryptVerify",
		Data: map[string]interface{}{
			"plain":        plain,
			"hash_field":   hashField,
			"output_field": outputField,
		},
	}
}

// StageRandomToken generates a cryptographically-random token and adds it
// to every record in the working data. encoding is one of "hex" (default),
// "base64", or "base64url"; pass an empty string to use the server default.
// The bytes argument controls the token length; ekoDB enforces a server-side
// limit of 1..=1024 — this builder does not validate the range client-side,
// so out-of-range values will error when the stage is executed by ekoDB.
//
// Requires ekoDB >= 0.41.0.
func StageRandomToken(bytes int, encoding, outputField string) FunctionStageConfig {
	data := map[string]interface{}{
		"bytes":        bytes,
		"output_field": outputField,
	}
	if encoding != "" {
		data["encoding"] = encoding
	}
	return FunctionStageConfig{
		Stage: "RandomToken",
		Data:  data,
	}
}

// StageTryCatch creates a try/catch error handling stage for graceful failure recovery.
// Executes tryFunctions, and if any fail, executes catchFunctions.
// outputErrorField (optional) is the field name to store error details (default: "error").
func StageTryCatch(tryFunctions, catchFunctions []FunctionStageConfig, outputErrorField string) FunctionStageConfig {
	data := map[string]interface{}{
		"try_functions":   tryFunctions,
		"catch_functions": catchFunctions,
	}
	if outputErrorField != "" {
		data["output_error_field"] = outputErrorField
	}
	return FunctionStageConfig{
		Stage: "TryCatch",
		Data:  data,
	}
}

// StageParallel executes multiple functions in parallel (concurrently).
// All functions run simultaneously, results are merged.
// If waitForAll is true, waits for all to complete; if false, returns on first completion.
func StageParallel(functions []FunctionStageConfig, waitForAll bool) FunctionStageConfig {
	return FunctionStageConfig{
		Stage: "Parallel",
		Data: map[string]interface{}{
			"functions":    functions,
			"wait_for_all": waitForAll,
		},
	}
}

// StageSleep creates a sleep/delay stage for rate limiting or timing control.
// durationMs is the duration in milliseconds — pass an int or a string like "{{delay_param}}".
func StageSleep(durationMs interface{}) FunctionStageConfig {
	return FunctionStageConfig{
		Stage: "Sleep",
		Data: map[string]interface{}{
			"duration_ms": durationMs,
		},
	}
}

// StageReturn creates a Return stage that shapes the final response.
// fields are the fields to include in the response (supports {{param}} substitution).
// statusCode is the HTTP status code (pass 0 to omit; default on server: 200).
func StageReturn(fields map[string]interface{}, statusCode int) FunctionStageConfig {
	data := map[string]interface{}{
		"fields": fields,
	}
	if statusCode > 0 {
		data["status_code"] = statusCode
	}
	return FunctionStageConfig{
		Stage: "Return",
		Data:  data,
	}
}

// StageValidate validates data against a JSON schema before processing.
// schema is the JSON Schema to validate against.
// dataField is the field containing data to validate.
// onError (optional) are functions to execute on validation failure; pass nil to omit.
func StageValidate(schema map[string]interface{}, dataField string, onError []FunctionStageConfig) FunctionStageConfig {
	data := map[string]interface{}{
		"schema":     schema,
		"data_field": dataField,
	}
	if onError != nil {
		data["on_error"] = onError
	}
	return FunctionStageConfig{
		Stage: "Validate",
		Data:  data,
	}
}

// ChatMessage for AI operations
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// NewChatMessage creates a chat message
func NewChatMessage(role, content string) ChatMessage {
	return ChatMessage{
		Role:    role,
		Content: content,
	}
}

// GroupFunctionConfig for Group stage
type GroupFunctionConfig struct {
	OutputField string          `json:"output_field"`
	Operation   GroupFunctionOp `json:"operation"`
	InputField  *string         `json:"input_field,omitempty"`
}

type GroupFunctionOp string

const (
	GroupFunctionSum     GroupFunctionOp = "Sum"
	GroupFunctionAverage GroupFunctionOp = "Average"
	GroupFunctionCount   GroupFunctionOp = "Count"
	GroupFunctionMin     GroupFunctionOp = "Min"
	GroupFunctionMax     GroupFunctionOp = "Max"
	GroupFunctionFirst   GroupFunctionOp = "First"
	GroupFunctionLast    GroupFunctionOp = "Last"
	GroupFunctionPush    GroupFunctionOp = "Push"
)

// SortFieldConfig for Sort stage
type SortFieldConfig struct {
	Field     string `json:"field"`
	Ascending bool   `json:"ascending"`
}

// FunctionResult from execution
type FunctionResult struct {
	Records []map[string]interface{} `json:"records"`
	Stats   FunctionStats            `json:"stats"`
}

// FunctionStats contains execution statistics
type FunctionStats struct {
	InputCount      int          `json:"input_count"`
	OutputCount     int          `json:"output_count"`
	ExecutionTimeMs int64        `json:"execution_time_ms"`
	StagesExecuted  int          `json:"stages_executed"`
	StageStats      []StageStats `json:"stage_stats"`
}

// StageStats contains statistics for a single stage
type StageStats struct {
	Stage           string `json:"stage"`
	InputCount      int    `json:"input_count"`
	OutputCount     int    `json:"output_count"`
	ExecutionTimeMs int64  `json:"execution_time_ms"`
}

// Client methods for functions

// SaveFunction creates a new function
func (c *Client) SaveFunction(function UserFunction) (string, error) {
	respBody, err := c.makeRequest("POST", "/api/functions", function)
	if err != nil {
		return "", err
	}

	var result struct {
		Status string `json:"status"`
		ID     string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}

	return result.ID, nil
}

// GetFunction retrieves a function by ID
func (c *Client) GetFunction(id string) (*UserFunction, error) {
	respBody, err := c.makeRequest("GET", fmt.Sprintf("/api/functions/%s", id), nil)
	if err != nil {
		return nil, err
	}

	var fn UserFunction
	if err := json.Unmarshal(respBody, &fn); err != nil {
		return nil, err
	}

	return &fn, nil
}

// ListFunctions lists all functions, optionally filtered by tags
func (c *Client) ListFunctions(tags []string) ([]UserFunction, error) {
	url := "/api/functions"
	if len(tags) > 0 {
		url += "?tags=" + joinStrings(tags, ",")
	}

	respBody, err := c.makeRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	var fns []UserFunction
	if err := json.Unmarshal(respBody, &fns); err != nil {
		return nil, err
	}

	return fns, nil
}

// UpdateFunction updates an existing function by ID
func (c *Client) UpdateFunction(id string, function UserFunction) error {
	_, err := c.makeRequest("PUT", fmt.Sprintf("/api/functions/%s", id), function)
	return err
}

// DeleteFunction deletes a function by ID
func (c *Client) DeleteFunction(id string) error {
	_, err := c.makeRequest("DELETE", fmt.Sprintf("/api/functions/%s", id), nil)
	return err
}

// CallFunction executes a function by label or ID
func (c *Client) CallFunction(labelOrID string, params map[string]interface{}) (*FunctionResult, error) {
	// Convert nil params to empty map to avoid sending JSON null
	if params == nil {
		params = make(map[string]interface{})
	}

	respBody, err := c.makeRequest("POST", fmt.Sprintf("/api/functions/%s", labelOrID), params)
	if err != nil {
		return nil, err
	}

	var result FunctionResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// Helper function to join strings
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}

// ============================================================================
// User Functions API
// ============================================================================

// SaveUserFunction creates a new reusable user function
func (c *Client) SaveUserFunction(userFunction UserFunction) (string, error) {
	respBody, err := c.makeRequest("POST", "/api/functions", userFunction)
	if err != nil {
		return "", err
	}

	var result struct {
		Status string `json:"status"`
		ID     string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}

	return result.ID, nil
}

// GetUserFunction retrieves a user function by label
func (c *Client) GetUserFunction(label string) (*UserFunction, error) {
	respBody, err := c.makeRequest("GET", fmt.Sprintf("/api/functions/%s", label), nil)
	if err != nil {
		return nil, err
	}

	var userFunction UserFunction
	if err := json.Unmarshal(respBody, &userFunction); err != nil {
		return nil, err
	}

	return &userFunction, nil
}

// ListUserFunctions lists all user functions, optionally filtered by tags
func (c *Client) ListUserFunctions(tags []string) ([]UserFunction, error) {
	url := "/api/functions"
	if len(tags) > 0 {
		url += "?tags=" + joinStrings(tags, ",")
	}

	respBody, err := c.makeRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	var userFunctions []UserFunction
	if err := json.Unmarshal(respBody, &userFunctions); err != nil {
		return nil, err
	}

	return userFunctions, nil
}

// UpdateUserFunction updates an existing user function by label
func (c *Client) UpdateUserFunction(label string, userFunction UserFunction) error {
	_, err := c.makeRequest("PUT", fmt.Sprintf("/api/functions/%s", label), userFunction)
	return err
}

// DeleteUserFunction deletes a user function by label
func (c *Client) DeleteUserFunction(label string) error {
	_, err := c.makeRequest("DELETE", fmt.Sprintf("/api/functions/%s", label), nil)
	return err
}
