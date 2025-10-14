// Package ekodb provides a Go client for ekoDB
package ekodb

import (
	"encoding/json"
	"fmt"
	"net/url"
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

// ========== Chat Types ==========

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
}

// ChatMessageRequest represents a request to send a message in an existing session
type ChatMessageRequest struct {
	Message        string `json:"message"`
	BypassRipple   *bool  `json:"bypass_ripple,omitempty"`
	ForceSummarize *bool  `json:"force_summarize,omitempty"`
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
	SystemPrompt *string            `json:"system_prompt,omitempty"`
	LLMModel     *string            `json:"llm_model,omitempty"`
	Collections  []CollectionConfig `json:"collections,omitempty"`
	Title        *string            `json:"title,omitempty"`
}

// MergeStrategy represents how to merge chat sessions
type MergeStrategy string

const (
	MergeStrategyChronological MergeStrategy = "Chronological"
	MergeStrategySummarized    MergeStrategy = "Summarized"
	MergeStrategyLatestOnly    MergeStrategy = "LatestOnly"
)

// MergeSessionsRequest represents a request to merge multiple chat sessions
type MergeSessionsRequest struct {
	SourceChatIDs []string      `json:"source_chat_ids"`
	TargetChatID  string        `json:"target_chat_id"`
	MergeStrategy MergeStrategy `json:"merge_strategy"`
}

// ========== Chat Methods ==========

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
