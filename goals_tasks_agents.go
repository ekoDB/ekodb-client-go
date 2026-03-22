package ekodb

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// ── Goal CRUD ──────────────────────────────────────────────────────────────

// GoalCreate creates a new goal.
func (c *Client) GoalCreate(data map[string]interface{}) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("POST", "/api/chat/goals", data)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GoalList lists all goals.
func (c *Client) GoalList() (map[string]interface{}, error) {
	respBody, err := c.makeRequest("GET", "/api/chat/goals", nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GoalGet retrieves a goal by ID.
func (c *Client) GoalGet(id string) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("GET", fmt.Sprintf("/api/chat/goals/%s", id), nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GoalUpdate updates a goal by ID.
func (c *Client) GoalUpdate(id string, data map[string]interface{}) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("PUT", fmt.Sprintf("/api/chat/goals/%s", id), data)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GoalDelete deletes a goal by ID.
func (c *Client) GoalDelete(id string) error {
	_, err := c.makeRequest("DELETE", fmt.Sprintf("/api/chat/goals/%s", id), nil)
	return err
}

// GoalSearch searches goals by query string.
func (c *Client) GoalSearch(query string) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("GET", fmt.Sprintf("/api/chat/goals/search?q=%s", url.QueryEscape(query)), nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ── Goal Lifecycle ─────────────────────────────────────────────────────────

// GoalComplete atomically marks a goal as complete (status → pending_review).
func (c *Client) GoalComplete(id string, data map[string]interface{}) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("POST", fmt.Sprintf("/api/chat/goals/%s/complete", id), data)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GoalApprove atomically approves a goal (status → in_progress).
func (c *Client) GoalApprove(id string) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("POST", fmt.Sprintf("/api/chat/goals/%s/approve", id), nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GoalReject atomically rejects a goal (status → failed).
func (c *Client) GoalReject(id string, data map[string]interface{}) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("POST", fmt.Sprintf("/api/chat/goals/%s/reject", id), data)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ── Goal Step Lifecycle ────────────────────────────────────────────────────

// GoalStepStart atomically marks a goal step as in_progress.
func (c *Client) GoalStepStart(id string, stepIndex int) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("POST", fmt.Sprintf("/api/chat/goals/%s/steps/%d/start", id, stepIndex), nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GoalStepComplete atomically marks a goal step as completed.
func (c *Client) GoalStepComplete(id string, stepIndex int, data map[string]interface{}) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("POST", fmt.Sprintf("/api/chat/goals/%s/steps/%d/complete", id, stepIndex), data)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GoalStepFail atomically marks a goal step as failed.
func (c *Client) GoalStepFail(id string, stepIndex int, data map[string]interface{}) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("POST", fmt.Sprintf("/api/chat/goals/%s/steps/%d/fail", id, stepIndex), data)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ── Goal Template CRUD ─────────────────────────────────────────────────────

// GoalTemplateCreate creates a new goal template.
func (c *Client) GoalTemplateCreate(data map[string]interface{}) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("POST", "/api/chat/goal-templates", data)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GoalTemplateList lists all goal templates.
func (c *Client) GoalTemplateList() (map[string]interface{}, error) {
	respBody, err := c.makeRequest("GET", "/api/chat/goal-templates", nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GoalTemplateGet retrieves a goal template by ID.
func (c *Client) GoalTemplateGet(id string) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("GET", fmt.Sprintf("/api/chat/goal-templates/%s", id), nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GoalTemplateUpdate updates a goal template by ID.
func (c *Client) GoalTemplateUpdate(id string, data map[string]interface{}) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("PUT", fmt.Sprintf("/api/chat/goal-templates/%s", id), data)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GoalTemplateDelete deletes a goal template by ID.
func (c *Client) GoalTemplateDelete(id string) error {
	_, err := c.makeRequest("DELETE", fmt.Sprintf("/api/chat/goal-templates/%s", id), nil)
	return err
}

// ── Task CRUD ──────────────────────────────────────────────────────────────

// TaskCreate creates a new scheduled task.
func (c *Client) TaskCreate(data map[string]interface{}) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("POST", "/api/chat/tasks", data)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// TaskList lists all scheduled tasks.
func (c *Client) TaskList() (map[string]interface{}, error) {
	respBody, err := c.makeRequest("GET", "/api/chat/tasks", nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// TaskGet retrieves a task by ID.
func (c *Client) TaskGet(id string) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("GET", fmt.Sprintf("/api/chat/tasks/%s", id), nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// TaskUpdate updates a task by ID.
func (c *Client) TaskUpdate(id string, data map[string]interface{}) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("PUT", fmt.Sprintf("/api/chat/tasks/%s", id), data)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// TaskDelete deletes a task by ID.
func (c *Client) TaskDelete(id string) error {
	_, err := c.makeRequest("DELETE", fmt.Sprintf("/api/chat/tasks/%s", id), nil)
	return err
}

// TaskDue retrieves tasks that are due at the given time.
func (c *Client) TaskDue(now string) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("GET", fmt.Sprintf("/api/chat/tasks/due?now=%s", url.QueryEscape(now)), nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ── Task Lifecycle ─────────────────────────────────────────────────────────

// TaskStart atomically marks a task as running.
func (c *Client) TaskStart(id string) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("POST", fmt.Sprintf("/api/chat/tasks/%s/start", id), nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// TaskSucceed atomically marks a task as succeeded.
func (c *Client) TaskSucceed(id string, data map[string]interface{}) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("POST", fmt.Sprintf("/api/chat/tasks/%s/succeed", id), data)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// TaskFail atomically marks a task as failed.
func (c *Client) TaskFail(id string, data map[string]interface{}) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("POST", fmt.Sprintf("/api/chat/tasks/%s/fail", id), data)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// TaskPause atomically pauses a task.
func (c *Client) TaskPause(id string) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("POST", fmt.Sprintf("/api/chat/tasks/%s/pause", id), nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// TaskResume atomically resumes a paused task.
func (c *Client) TaskResume(id string, data map[string]interface{}) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("POST", fmt.Sprintf("/api/chat/tasks/%s/resume", id), data)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ── Agent CRUD ─────────────────────────────────────────────────────────────

// AgentCreate creates a new agent.
func (c *Client) AgentCreate(data map[string]interface{}) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("POST", "/api/chat/agents", data)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// AgentList lists all agents.
func (c *Client) AgentList() (map[string]interface{}, error) {
	respBody, err := c.makeRequest("GET", "/api/chat/agents", nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// AgentGet retrieves an agent by ID.
func (c *Client) AgentGet(id string) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("GET", fmt.Sprintf("/api/chat/agents/%s", id), nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// AgentGetByName retrieves an agent by name.
func (c *Client) AgentGetByName(name string) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("GET", fmt.Sprintf("/api/chat/agents/by-name/%s", name), nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// AgentUpdate updates an agent by ID.
func (c *Client) AgentUpdate(id string, data map[string]interface{}) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("PUT", fmt.Sprintf("/api/chat/agents/%s", id), data)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// AgentDelete deletes an agent by ID.
func (c *Client) AgentDelete(id string) error {
	_, err := c.makeRequest("DELETE", fmt.Sprintf("/api/chat/agents/%s", id), nil)
	return err
}

// AgentsByDeployment retrieves agents associated with a deployment ID.
func (c *Client) AgentsByDeployment(deploymentId string) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("GET", fmt.Sprintf("/api/chat/agents/by-deployment/%s", deploymentId), nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}
