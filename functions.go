package ekodb

import (
	"encoding/json"
	"fmt"
	"time"
)

// SavedFunction represents a server-side data processing pipeline
type SavedFunction struct {
	Label       string                          `json:"label"`
	Name        string                          `json:"name"`
	Description *string                         `json:"description,omitempty"`
	Version     string                          `json:"version"`
	Parameters  map[string]ParameterDefinition  `json:"parameters"`
	Pipeline    []FunctionStageConfig           `json:"pipeline"`
	Tags        []string                        `json:"tags"`
	CreatedAt   *time.Time                      `json:"created_at,omitempty"`
	UpdatedAt   *time.Time                      `json:"updated_at,omitempty"`
}

// ParameterDefinition for function parameters
type ParameterDefinition struct {
	Required    bool        `json:"required"`
	Default     interface{} `json:"default,omitempty"`
	Description string      `json:"description,omitempty"`
}

// ParameterValue represents a literal or parameter reference
type ParameterValue struct {
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

// NewLiteralValue creates a literal parameter value
func NewLiteralValue(value interface{}) ParameterValue {
	return ParameterValue{Type: "Literal", Value: value}
}

// NewParameterReference creates a parameter reference
func NewParameterReference(name string) ParameterValue {
	return ParameterValue{Type: "Parameter", Value: name}
}

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

// Stage builders
func StageFindAll(collection string) FunctionStageConfig {
	return FunctionStageConfig{
		Stage: "FindAll",
		Data:  map[string]interface{}{"collection": collection},
	}
}

func StageQuery(collection string, expression map[string]interface{}) FunctionStageConfig {
	return FunctionStageConfig{
		Stage: "Query",
		Data: map[string]interface{}{
			"collection": collection,
			"expression": expression,
		},
	}
}

func StageProject(fields []string) FunctionStageConfig {
	return FunctionStageConfig{
		Stage: "Project",
		Data:  map[string]interface{}{"fields": fields},
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

func StageCount() FunctionStageConfig {
	return FunctionStageConfig{Stage: "Count", Data: map[string]interface{}{}}
}

func StageInsert(collection string, data map[string]interface{}, bypassRipple bool) FunctionStageConfig {
	return FunctionStageConfig{
		Stage: "Insert",
		Data: map[string]interface{}{
			"collection":    collection,
			"data":          data,
			"bypass_ripple": bypassRipple,
		},
	}
}

func StageDelete(collection string, id ParameterValue, bypassRipple bool) FunctionStageConfig {
	return FunctionStageConfig{
		Stage: "Delete",
		Data: map[string]interface{}{
			"collection":    collection,
			"id":            id,
			"bypass_ripple": bypassRipple,
		},
	}
}

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

func StageBatchDelete(collection string, ids []ParameterValue, bypassRipple bool) FunctionStageConfig {
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

func StageVectorSearch(collection string, queryVector []float64, options map[string]interface{}) FunctionStageConfig {
	data := map[string]interface{}{
		"collection":   collection,
		"query_vector": queryVector,
	}
	if options != nil {
		data["options"] = options
	}
	return FunctionStageConfig{Stage: "VectorSearch", Data: data}
}

func StageTextSearch(collection string, query string, options map[string]interface{}) FunctionStageConfig {
	data := map[string]interface{}{
		"collection": collection,
		"query":      query,
	}
	if options != nil {
		data["options"] = options
	}
	return FunctionStageConfig{Stage: "TextSearch", Data: data}
}

func StageHybridSearch(collection string, textQuery string, vectorQuery []float64, options map[string]interface{}) FunctionStageConfig {
	data := map[string]interface{}{
		"collection":   collection,
		"text_query":   textQuery,
		"vector_query": vectorQuery,
	}
	if options != nil {
		data["options"] = options
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

// ChatMessage for AI operations
type ChatMessage struct {
	Role    ParameterValue `json:"role"`
	Content ParameterValue `json:"content"`
}

// NewChatMessage creates a chat message with literal values
func NewChatMessage(role, content string) ChatMessage {
	return ChatMessage{
		Role:    NewLiteralValue(role),
		Content: NewLiteralValue(content),
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

// Client methods for saved functions

// SaveFunction creates a new saved function
func (c *Client) SaveFunction(function SavedFunction) (string, error) {
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

// GetFunction retrieves a function by label
func (c *Client) GetFunction(label string) (*SavedFunction, error) {
	respBody, err := c.makeRequest("GET", fmt.Sprintf("/api/functions/%s", label), nil)
	if err != nil {
		return nil, err
	}

	var function SavedFunction
	if err := json.Unmarshal(respBody, &function); err != nil {
		return nil, err
	}

	return &function, nil
}

// ListFunctions lists all functions, optionally filtered by tags
func (c *Client) ListFunctions(tags []string) ([]SavedFunction, error) {
	url := "/api/functions"
	if len(tags) > 0 {
		url += "?tags=" + joinStrings(tags, ",")
	}

	respBody, err := c.makeRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	var functions []SavedFunction
	if err := json.Unmarshal(respBody, &functions); err != nil {
		return nil, err
	}

	return functions, nil
}

// UpdateFunction updates an existing function
func (c *Client) UpdateFunction(label string, function SavedFunction) error {
	_, err := c.makeRequest("PUT", fmt.Sprintf("/api/functions/%s", label), function)
	return err
}

// DeleteFunction deletes a function by label
func (c *Client) DeleteFunction(label string) error {
	_, err := c.makeRequest("DELETE", fmt.Sprintf("/api/functions/%s", label), nil)
	return err
}

// CallFunction executes a saved function
func (c *Client) CallFunction(label string, params map[string]interface{}) (*FunctionResult, error) {
	// Convert nil params to empty map to avoid sending JSON null
	if params == nil {
		params = make(map[string]interface{})
	}
	
	respBody, err := c.makeRequest("POST", fmt.Sprintf("/api/functions/%s", label), params)
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
