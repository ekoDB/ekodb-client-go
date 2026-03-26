// Package ekodb provides a Go client for ekoDB
package ekodb

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ========== Helper Functions ==========

// IntPtr returns a pointer to an int value
func IntPtr(v int) *int {
	return &v
}

// StringPtr returns a pointer to a string value
func StringPtr(v string) *string {
	return &v
}

// BoolPtr returns a pointer to a bool value
func BoolPtr(v bool) *bool {
	return &v
}

// Float32Ptr returns a pointer to a float32 value
func Float32Ptr(v float32) *float32 {
	return &v
}

// Int32Ptr returns a pointer to an int32 value
func Int32Ptr(v int32) *int32 {
	return &v
}

// ========== Chat Types ==========

// ToolChoice controls how the LLM decides whether to use tools
type ToolChoice struct {
	Type string `json:"type"`           // "auto", "none", "required", "tool"
	Name string `json:"name,omitempty"` // Only used when Type is "tool"
}

// ToolConfig configures which tools are available in a chat session
type ToolConfig struct {
	Enabled              bool        `json:"enabled"`
	AllowedTools         []string    `json:"allowed_tools,omitempty"`
	AllowedCollections   []string    `json:"allowed_collections,omitempty"`
	MaxIterations        *int        `json:"max_iterations,omitempty"`
	AllowWriteOperations *bool       `json:"allow_write_operations,omitempty"`
	ToolChoice           *ToolChoice `json:"tool_choice,omitempty"`
}

// CollectionConfig represents configuration for searching a specific collection
type CollectionConfig struct {
	CollectionName string        `json:"collection_name"`
	Fields         []interface{} `json:"fields"`
	SearchOptions  interface{}   `json:"search_options,omitempty"`
}

// ChatRequest represents a request to send a chat message
type ChatRequest struct {
	Collections  []CollectionConfig `json:"collections"`
	LLMProvider  string             `json:"llm_provider"`
	LLMModel     *string            `json:"llm_model,omitempty"`
	Message      string             `json:"message"`
	SystemPrompt *string            `json:"system_prompt,omitempty"`
}

// CreateChatSessionRequest represents a request to create a new chat session
type CreateChatSessionRequest struct {
	Collections        []CollectionConfig `json:"collections"`
	LLMProvider        string             `json:"llm_provider"`
	LLMModel           *string            `json:"llm_model,omitempty"`
	SystemPrompt       *string            `json:"system_prompt,omitempty"`
	BypassRipple       *bool              `json:"bypass_ripple,omitempty"`
	ParentID           *string            `json:"parent_id,omitempty"`
	BranchPointIdx     *int               `json:"branch_point_idx,omitempty"`
	MaxContextMessages *int               `json:"max_context_messages,omitempty"`
	MaxTokens          *int32             `json:"max_tokens,omitempty"`
	Temperature        *float32           `json:"temperature,omitempty"`
	ToolConfig         *ToolConfig        `json:"tool_config,omitempty"`
}

// ChatMessageRequest represents a request to send a message in an existing session
type ChatMessageRequest struct {
	Message        string      `json:"message"`
	BypassRipple   *bool       `json:"bypass_ripple,omitempty"`
	ForceSummarize *bool       `json:"force_summarize,omitempty"`
	MaxIterations  *int        `json:"max_iterations,omitempty"`
	ToolConfig     *ToolConfig `json:"tool_config,omitempty"`
	LLMModel       *string     `json:"llm_model,omitempty"`
}

// TokenUsage represents token usage statistics
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatResponse represents a response from a chat operation
type ChatResponse struct {
	ChatID          string        `json:"chat_id"`
	MessageID       string        `json:"message_id"`
	Responses       []string      `json:"responses"`
	ContextSnippets []interface{} `json:"context_snippets"`
	ExecutionTimeMs int           `json:"execution_time_ms"`
	TokenUsage      *TokenUsage   `json:"token_usage,omitempty"`
}

// ChatSession represents a chat session
type ChatSession struct {
	ChatID       string             `json:"chat_id"`
	CreatedAt    string             `json:"created_at"`
	UpdatedAt    string             `json:"updated_at"`
	LLMProvider  string             `json:"llm_provider"`
	LLMModel     string             `json:"llm_model"`
	Collections  []CollectionConfig `json:"collections"`
	SystemPrompt *string            `json:"system_prompt,omitempty"`
	Title        *string            `json:"title,omitempty"`
	MessageCount int                `json:"message_count"`
}

// ChatSessionResponse represents a response containing session details
type ChatSessionResponse struct {
	Session      Record `json:"session"`
	MessageCount int    `json:"message_count"`
}

// ListSessionsQuery represents query parameters for listing sessions
type ListSessionsQuery struct {
	Limit *int    `json:"limit,omitempty"`
	Skip  *int    `json:"skip,omitempty"`
	Sort  *string `json:"sort,omitempty"`
}

// ListSessionsResponse represents a response containing list of sessions
type ListSessionsResponse struct {
	Sessions []ChatSession `json:"sessions"`
	Total    int           `json:"total"`
	Returned int           `json:"returned"`
}

// GetMessagesQuery represents query parameters for getting messages
type GetMessagesQuery struct {
	Limit *int    `json:"limit,omitempty"`
	Skip  *int    `json:"skip,omitempty"`
	Sort  *string `json:"sort,omitempty"`
}

// GetMessagesResponse represents a response containing messages
type GetMessagesResponse struct {
	Messages []Record `json:"messages"`
	Total    int      `json:"total"`
	Skip     int      `json:"skip"`
	Limit    *int     `json:"limit,omitempty"`
	Returned int      `json:"returned"`
}

// UpdateSessionRequest represents a request to update session metadata
type UpdateSessionRequest struct {
	SystemPrompt       *string            `json:"system_prompt,omitempty"`
	LLMModel           *string            `json:"llm_model,omitempty"`
	Collections        []CollectionConfig `json:"collections,omitempty"`
	MaxContextMessages *int               `json:"max_context_messages,omitempty"`
	BypassRipple       *bool              `json:"bypass_ripple,omitempty"`
	Title              *string            `json:"title,omitempty"`
	Memory             interface{}        `json:"memory,omitempty"`
}

// MergeStrategy represents how to merge chat sessions
type MergeStrategy string

const (
	MergeStrategyChronological MergeStrategy = "Chronological"
	MergeStrategySummarized    MergeStrategy = "Summarized"
	MergeStrategyLatestOnly    MergeStrategy = "LatestOnly"
	MergeStrategyInterleaved   MergeStrategy = "Interleaved"
)

// MergeSessionsRequest represents a request to merge multiple chat sessions
type MergeSessionsRequest struct {
	SourceChatIDs []string      `json:"source_chat_ids"`
	TargetChatID  string        `json:"target_chat_id"`
	MergeStrategy MergeStrategy `json:"merge_strategy"`
	BypassRipple  *bool         `json:"bypass_ripple,omitempty"`
}

// ChatModels contains available models for each provider
type ChatModels struct {
	OpenAI     []string `json:"openai"`
	Anthropic  []string `json:"anthropic"`
	Perplexity []string `json:"perplexity"`
}

// EmbedRequest is the request body for POST /api/embed
type EmbedRequest struct {
	Text  *string  `json:"text,omitempty"`
	Texts []string `json:"texts,omitempty"`
	Model *string  `json:"model,omitempty"`
}

// EmbedResponse is the response from POST /api/embed
type EmbedResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
	Model      string      `json:"model"`
	Dimensions int         `json:"dimensions"`
}

// RawCompletionRequest is the request body for POST /api/chat/complete.
// Stateless raw LLM completion — no session, no history, no RAG.
type RawCompletionRequest struct {
	SystemPrompt string  `json:"system_prompt"`
	Message      string  `json:"message"`
	Provider     *string `json:"provider,omitempty"`
	Model        *string `json:"model,omitempty"`
	MaxTokens    *int    `json:"max_tokens,omitempty"`
}

// RawCompletionResponse is returned by RawCompletion.
type RawCompletionResponse struct {
	Content string `json:"content"`
}

// ========== Chat Methods ==========

// RawCompletion sends a stateless raw LLM completion request — no session,
// no history, no RAG context injection. Use this for structured-output tasks
// such as planning where the response must be parsed programmatically.
//
// Example:
//
//	resp, err := client.RawCompletion(RawCompletionRequest{
//	    SystemPrompt: "You are a helpful assistant.",
//	    Message:      "Summarize this in JSON.",
//	})
func (c *Client) RawCompletion(request RawCompletionRequest) (*RawCompletionResponse, error) {
	respBody, err := c.makeRequest("POST", "/api/chat/complete", request)
	if err != nil {
		return nil, err
	}

	var result RawCompletionResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// RawCompletionStream performs a stateless raw LLM completion via SSE streaming.
//
// Same as RawCompletion but uses Server-Sent Events to keep the connection alive.
// Preferred for deployed instances where reverse proxies may kill idle HTTP
// connections before the LLM responds.
func (c *Client) RawCompletionStream(request RawCompletionRequest) (*RawCompletionResponse, error) {
	// Serialize request body as JSON
	bodyBytes, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", c.baseURL+"/api/chat/complete/stream", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	token := c.getToken()
	if token == "" {
		if err := c.refreshToken(); err != nil {
			return nil, fmt.Errorf("failed to get auth token: %w", err)
		}
		token = c.getToken()
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("SSE request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("SSE raw completion failed (%d): %s", resp.StatusCode, string(respBody))
	}

	// Parse SSE events
	scanner := bufio.NewScanner(resp.Body)
	var content string
	var lastError string

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		dataStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if dataStr == "" {
			continue
		}
		var eventData map[string]interface{}
		if err := json.Unmarshal([]byte(dataStr), &eventData); err != nil {
			continue
		}
		if token, ok := eventData["token"].(string); ok {
			content += token
		}
		if c, ok := eventData["content"].(string); ok {
			content = c
		}
		if e, ok := eventData["error"].(string); ok {
			lastError = e
		}
	}

	if lastError != "" {
		return nil, fmt.Errorf("LLM error: %s", lastError)
	}

	return &RawCompletionResponse{Content: content}, nil
}

// RawCompletionStreamWithProgress performs a stateless raw LLM completion via SSE
// streaming, calling onToken for each token as it arrives.
//
// Same as RawCompletionStream but provides incremental progress via the callback,
// allowing callers to show real-time output during long-running LLM calls.
func (c *Client) RawCompletionStreamWithProgress(request RawCompletionRequest, onToken func(string)) (*RawCompletionResponse, error) {
	bodyBytes, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", c.baseURL+"/api/chat/complete/stream", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	token := c.getToken()
	if token == "" {
		if err := c.refreshToken(); err != nil {
			return nil, fmt.Errorf("failed to get auth token: %w", err)
		}
		token = c.getToken()
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("SSE request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("SSE raw completion failed (%d): %s", resp.StatusCode, string(respBody))
	}

	scanner := bufio.NewScanner(resp.Body)
	var content string
	var lastError string

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		dataStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if dataStr == "" {
			continue
		}
		var eventData map[string]interface{}
		if err := json.Unmarshal([]byte(dataStr), &eventData); err != nil {
			continue
		}
		if tok, ok := eventData["token"].(string); ok {
			content += tok
			if onToken != nil {
				onToken(tok)
			}
		}
		if c, ok := eventData["content"].(string); ok {
			content = c
		}
		if e, ok := eventData["error"].(string); ok {
			lastError = e
		}
	}

	if lastError != "" {
		return nil, fmt.Errorf("LLM error: %s", lastError)
	}

	return &RawCompletionResponse{Content: content}, nil
}

// ExecuteToolRequest is the request body for POST /api/chat/tools/execute.
type ExecuteToolRequest struct {
	Tool   string                 `json:"tool"`
	Params map[string]interface{} `json:"params"`
	ChatID string                 `json:"chat_id,omitempty"`
}

// ExecuteToolResult is the response from POST /api/chat/tools/execute.
type ExecuteToolResult struct {
	Success bool                   `json:"success"`
	Result  map[string]interface{} `json:"result,omitempty"`
	Error   string                 `json:"error,omitempty"`
}

// ExecuteTool executes a tool via ekoDB's server-side tool pipeline.
//
// Calls POST /api/chat/tools/execute which goes through the same
// execute_tool function as the LLM tool-calling loop — with all
// collection filtering, permission enforcement, and internal collection
// blocking. No LLM round-trip.
//
// Returns the tool result if executed, nil if the server doesn't
// support the endpoint (older ekoDB versions), or an error.
func (c *Client) ExecuteTool(toolName string, params map[string]interface{}, chatID string) (map[string]interface{}, error) {
	request := ExecuteToolRequest{
		Tool:   toolName,
		Params: params,
		ChatID: chatID,
	}

	respBody, err := c.makeRequest("POST", "/api/chat/tools/execute", request)
	if err != nil {
		// If 404/405, server doesn't support the endpoint — return nil
		var httpErr *HTTPError
		if errors.As(err, &httpErr) && (httpErr.StatusCode == 404 || httpErr.StatusCode == 405) {
			return nil, nil
		}
		return nil, err
	}

	var result ExecuteToolResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	if result.Success {
		return result.Result, nil
	}

	errMsg := result.Error
	if errMsg == "" {
		errMsg = "tool execution failed"
	}
	return nil, fmt.Errorf("%s", errMsg)
}

// GetChatTools retrieves all built-in server-side chat tool definitions.
// Returns a slice of tool objects with name, description, and parameters fields.
// Used by planning agents to discover available tools dynamically.
func (c *Client) GetChatTools() ([]map[string]interface{}, error) {
	respBody, err := c.makeRequest("GET", "/api/chat/tools", nil)
	if err != nil {
		return nil, err
	}

	var result []map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetChatModels retrieves all available chat models from all providers
func (c *Client) GetChatModels() (*ChatModels, error) {
	respBody, err := c.makeRequest("GET", "/api/chat_models", nil)
	if err != nil {
		return nil, err
	}

	var result ChatModels
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetChatModel retrieves available models for a specific provider
func (c *Client) GetChatModel(providerName string) ([]string, error) {
	respBody, err := c.makeRequest("GET", fmt.Sprintf("/api/chat_models/%s", providerName), nil)
	if err != nil {
		return nil, err
	}

	var result []string
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// CreateChatSession creates a new chat session
func (c *Client) CreateChatSession(request CreateChatSessionRequest) (*ChatResponse, error) {
	respBody, err := c.makeRequest("POST", "/api/chat", request)
	if err != nil {
		return nil, err
	}

	var result ChatResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ChatMessage sends a message in an existing chat session
func (c *Client) ChatMessage(sessionID string, request ChatMessageRequest) (*ChatResponse, error) {
	respBody, err := c.makeRequest("POST", fmt.Sprintf("/api/chat/%s/messages", sessionID), request)
	if err != nil {
		return nil, err
	}

	var result ChatResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetChatSession gets a chat session by ID
func (c *Client) GetChatSession(sessionID string) (*ChatSessionResponse, error) {
	respBody, err := c.makeRequest("GET", fmt.Sprintf("/api/chat/%s", sessionID), nil)
	if err != nil {
		return nil, err
	}

	var result ChatSessionResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ListChatSessions lists all chat sessions
func (c *Client) ListChatSessions(query *ListSessionsQuery) (*ListSessionsResponse, error) {
	path := "/api/chat"

	if query != nil {
		params := url.Values{}
		if query.Limit != nil {
			params.Add("limit", fmt.Sprintf("%d", *query.Limit))
		}
		if query.Skip != nil {
			params.Add("skip", fmt.Sprintf("%d", *query.Skip))
		}
		if query.Sort != nil {
			params.Add("sort", *query.Sort)
		}
		if len(params) > 0 {
			path += "?" + params.Encode()
		}
	}

	respBody, err := c.makeRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var result ListSessionsResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetChatSessionMessages gets messages from a chat session
func (c *Client) GetChatSessionMessages(sessionID string, query *GetMessagesQuery) (*GetMessagesResponse, error) {
	path := fmt.Sprintf("/api/chat/%s/messages", sessionID)

	if query != nil {
		params := url.Values{}
		if query.Limit != nil {
			params.Add("limit", fmt.Sprintf("%d", *query.Limit))
		}
		if query.Skip != nil {
			params.Add("skip", fmt.Sprintf("%d", *query.Skip))
		}
		if query.Sort != nil {
			params.Add("sort", *query.Sort)
		}
		if len(params) > 0 {
			path += "?" + params.Encode()
		}
	}

	respBody, err := c.makeRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var result GetMessagesResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// UpdateChatSession updates a chat session
func (c *Client) UpdateChatSession(sessionID string, request UpdateSessionRequest) (*ChatSessionResponse, error) {
	respBody, err := c.makeRequest("PUT", fmt.Sprintf("/api/chat/%s", sessionID), request)
	if err != nil {
		return nil, err
	}

	var result ChatSessionResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// BranchChatSession branches a chat session
func (c *Client) BranchChatSession(request CreateChatSessionRequest) (*ChatResponse, error) {
	respBody, err := c.makeRequest("POST", "/api/chat/branch", request)
	if err != nil {
		return nil, err
	}

	var result ChatResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// DeleteChatSession deletes a chat session
func (c *Client) DeleteChatSession(sessionID string) error {
	_, err := c.makeRequest("DELETE", fmt.Sprintf("/api/chat/%s", sessionID), nil)
	return err
}

// RegenerateChatMessage regenerates an AI response message
func (c *Client) RegenerateChatMessage(sessionID, messageID string) (*ChatResponse, error) {
	respBody, err := c.makeRequest("POST", fmt.Sprintf("/api/chat/%s/messages/%s/regenerate", sessionID, messageID), nil)
	if err != nil {
		return nil, err
	}

	var result ChatResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// UpdateChatMessage updates a specific message
func (c *Client) UpdateChatMessage(sessionID, messageID, content string) error {
	request := map[string]string{"content": content}
	_, err := c.makeRequest("PUT", fmt.Sprintf("/api/chat/%s/messages/%s", sessionID, messageID), request)
	return err
}

// GetChatMessage gets a specific message by ID
func (c *Client) GetChatMessage(sessionID, messageID string) (Record, error) {
	respBody, err := c.makeRequest("GET", fmt.Sprintf("/api/chat/%s/messages/%s", sessionID, messageID), nil)
	if err != nil {
		return nil, err
	}

	var result Record
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// DeleteChatMessage deletes a specific message
func (c *Client) DeleteChatMessage(sessionID, messageID string) error {
	_, err := c.makeRequest("DELETE", fmt.Sprintf("/api/chat/%s/messages/%s", sessionID, messageID), nil)
	return err
}

// ToggleForgottenMessage toggles the "forgotten" status of a message
func (c *Client) ToggleForgottenMessage(sessionID, messageID string, forgotten bool) error {
	request := map[string]bool{"forgotten": forgotten}
	_, err := c.makeRequest("PATCH", fmt.Sprintf("/api/chat/%s/messages/%s/forgotten", sessionID, messageID), request)
	return err
}

// MergeChatSessions merges multiple chat sessions into one
func (c *Client) MergeChatSessions(request MergeSessionsRequest) (*ChatSessionResponse, error) {
	respBody, err := c.makeRequest("POST", "/api/chat/merge", request)
	if err != nil {
		return nil, err
	}

	var result ChatSessionResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ========== Chat Streaming (SSE) ==========

// ChatMessageStream sends a message in an existing chat session via SSE streaming.
// Uses the same ChatStreamEvent type as the WebSocket ChatSend method.
//
// Returns a channel that yields ChatStreamEvent values as they arrive:
//   - Chunk events carry incremental token text in Content.
//   - An End event signals completion and includes MessageID, TokenUsage, etc.
//   - An Error event carries a description string in Error.
//
// The channel is closed when the stream finishes or an error occurs.
//
// Example:
//
//	ch, err := client.ChatMessageStream("chat-id", ChatMessageRequest{Message: "Hello"})
//	if err != nil { log.Fatal(err) }
//	for event := range ch {
//	    switch event.Type {
//	    case "chunk":
//	        fmt.Print(event.Content)
//	    case "end":
//	        fmt.Printf("\nDone: %s (%d ms)\n", event.MessageID, event.ExecutionTimeMs)
//	    case "error":
//	        fmt.Println("Error:", event.Error)
//	    }
//	}
func (c *Client) ChatMessageStream(chatID string, request ChatMessageRequest) (chan ChatStreamEvent, error) {
	bodyBytes, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", c.baseURL+fmt.Sprintf("/api/chat/%s/messages/stream", chatID), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	token := c.getToken()
	if token == "" {
		if err := c.refreshToken(); err != nil {
			return nil, fmt.Errorf("failed to get auth token: %w", err)
		}
		token = c.getToken()
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("SSE request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("SSE chat message stream failed (%d): %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan ChatStreamEvent, 128)

	go func() {
		defer resp.Body.Close()
		defer close(ch)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			dataStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if dataStr == "" {
				continue
			}
			var eventData map[string]interface{}
			if err := json.Unmarshal([]byte(dataStr), &eventData); err != nil {
				continue
			}

			// Token chunk
			if tok, ok := eventData["token"].(string); ok {
				ch <- ChatStreamEvent{Type: "chunk", Content: tok}
			}

			// Done event (has "content" field)
			if _, hasContent := eventData["content"]; hasContent {
				event := ChatStreamEvent{Type: "end"}
				if mid, ok := eventData["message_id"].(string); ok {
					event.MessageID = mid
				}
				if ms, ok := eventData["execution_time_ms"].(float64); ok {
					event.ExecutionTimeMs = uint64(ms)
				}
				if tu, ok := eventData["token_usage"]; ok {
					if raw, err := json.Marshal(tu); err == nil {
						event.TokenUsage = json.RawMessage(raw)
					}
				}
				if tch, ok := eventData["tool_call_history"]; ok {
					if raw, err := json.Marshal(tch); err == nil {
						event.ToolCallHistory = json.RawMessage(raw)
					}
				}
				if cw, ok := eventData["context_window"].(float64); ok {
					event.ContextWindow = uint32(cw)
				}
				ch <- event
				return
			}

			// Error event
			if errMsg, ok := eventData["error"].(string); ok {
				ch <- ChatStreamEvent{Type: "error", Error: errMsg}
				return
			}
		}
	}()

	return ch, nil
}
