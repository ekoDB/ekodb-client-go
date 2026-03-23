package ekodb

import (
	"encoding/json"
	"fmt"
)

// CreateSchedule creates a new schedule.
func (c *Client) CreateSchedule(data map[string]interface{}) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("POST", "/api/schedules", data)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ListSchedules lists all schedules.
func (c *Client) ListSchedules() (map[string]interface{}, error) {
	respBody, err := c.makeRequest("GET", "/api/schedules", nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetSchedule retrieves a schedule by ID.
func (c *Client) GetSchedule(id string) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("GET", fmt.Sprintf("/api/schedules/%s", id), nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// UpdateSchedule updates a schedule by ID.
func (c *Client) UpdateSchedule(id string, data map[string]interface{}) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("PUT", fmt.Sprintf("/api/schedules/%s", id), data)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// DeleteSchedule deletes a schedule by ID.
func (c *Client) DeleteSchedule(id string) error {
	_, err := c.makeRequest("DELETE", fmt.Sprintf("/api/schedules/%s", id), nil)
	return err
}

// PauseSchedule pauses a schedule by ID.
func (c *Client) PauseSchedule(id string) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("POST", fmt.Sprintf("/api/schedules/%s/pause", id), nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ResumeSchedule resumes a schedule by ID.
func (c *Client) ResumeSchedule(id string) (map[string]interface{}, error) {
	respBody, err := c.makeRequest("POST", fmt.Sprintf("/api/schedules/%s/resume", id), nil)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}
