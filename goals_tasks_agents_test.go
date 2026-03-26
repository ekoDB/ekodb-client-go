package ekodb

import (
	"encoding/json"
	"net/http"
	"testing"
)

// ============================================================================
// Goal CRUD Tests
// ============================================================================

func TestGoalCreate(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/goals": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "goal_1", "title": "Test Goal", "status": "active",
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.GoalCreate(map[string]interface{}{"title": "Test Goal"})
	if err != nil {
		t.Fatalf("GoalCreate failed: %v", err)
	}
	if result["id"] != "goal_1" {
		t.Errorf("Expected id goal_1, got %v", result["id"])
	}
}

func TestGoalList(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/chat/goals": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"goals": []map[string]interface{}{{"id": "goal_1"}, {"id": "goal_2"}},
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.GoalList()
	if err != nil {
		t.Fatalf("GoalList failed: %v", err)
	}
	if result["goals"] == nil {
		t.Error("Expected goals field")
	}
}

func TestGoalGet(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/chat/goals/goal_1": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "goal_1", "title": "Test Goal"})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.GoalGet("goal_1")
	if err != nil {
		t.Fatalf("GoalGet failed: %v", err)
	}
	if result["id"] != "goal_1" {
		t.Errorf("Expected id goal_1, got %v", result["id"])
	}
}

func TestGoalUpdate(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"PUT /api/chat/goals/goal_1": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "goal_1", "title": "Updated"})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.GoalUpdate("goal_1", map[string]interface{}{"title": "Updated"})
	if err != nil {
		t.Fatalf("GoalUpdate failed: %v", err)
	}
	if result["title"] != "Updated" {
		t.Errorf("Expected title Updated, got %v", result["title"])
	}
}

func TestGoalDelete(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"DELETE /api/chat/goals/goal_1": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	err := client.GoalDelete("goal_1")
	if err != nil {
		t.Fatalf("GoalDelete failed: %v", err)
	}
}

func TestGoalSearch(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/chat/goals/search*": func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query().Get("q")
			if q == "" {
				t.Error("Expected q parameter")
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"goals": []interface{}{}})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.GoalSearch("test query")
	if err != nil {
		t.Fatalf("GoalSearch failed: %v", err)
	}
	if result["goals"] == nil {
		t.Error("Expected goals field")
	}
}

func TestGoalComplete(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/goals/goal_1/complete": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "goal_1", "status": "pending_review"})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.GoalComplete("goal_1", map[string]interface{}{"summary": "Done"})
	if err != nil {
		t.Fatalf("GoalComplete failed: %v", err)
	}
	if result["status"] != "pending_review" {
		t.Errorf("Expected status pending_review, got %v", result["status"])
	}
}

func TestGoalApprove(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/goals/goal_1/approve": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "goal_1", "status": "in_progress"})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.GoalApprove("goal_1")
	if err != nil {
		t.Fatalf("GoalApprove failed: %v", err)
	}
	if result["status"] != "in_progress" {
		t.Errorf("Expected status in_progress, got %v", result["status"])
	}
}

func TestGoalReject(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/goals/goal_1/reject": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "goal_1", "status": "failed"})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.GoalReject("goal_1", map[string]interface{}{"reason": "Bad plan"})
	if err != nil {
		t.Fatalf("GoalReject failed: %v", err)
	}
	if result["status"] != "failed" {
		t.Errorf("Expected status failed, got %v", result["status"])
	}
}

func TestGoalStepStart(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/goals/goal_1/steps/0/start": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "goal_1"})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.GoalStepStart("goal_1", 0)
	if err != nil {
		t.Fatalf("GoalStepStart failed: %v", err)
	}
}

func TestGoalStepComplete(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/goals/goal_1/steps/0/complete": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "goal_1"})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.GoalStepComplete("goal_1", 0, map[string]interface{}{"result": "Done"})
	if err != nil {
		t.Fatalf("GoalStepComplete failed: %v", err)
	}
}

func TestGoalStepFail(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/goals/goal_1/steps/0/fail": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "goal_1"})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.GoalStepFail("goal_1", 0, map[string]interface{}{"error": "Failed"})
	if err != nil {
		t.Fatalf("GoalStepFail failed: %v", err)
	}
}

// ============================================================================
// Task CRUD Tests
// ============================================================================

func TestTaskCreate(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/tasks": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "task_1", "name": "Test Task", "status": "active"})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.TaskCreate(map[string]interface{}{"name": "Test Task"})
	if err != nil {
		t.Fatalf("TaskCreate failed: %v", err)
	}
	if result["id"] != "task_1" {
		t.Errorf("Expected id task_1, got %v", result["id"])
	}
}

func TestTaskList(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/chat/tasks": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"tasks": []interface{}{}})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.TaskList()
	if err != nil {
		t.Fatalf("TaskList failed: %v", err)
	}
	if result["tasks"] == nil {
		t.Error("Expected tasks field")
	}
}

func TestTaskGet(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/chat/tasks/task_1": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "task_1"})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.TaskGet("task_1")
	if err != nil {
		t.Fatalf("TaskGet failed: %v", err)
	}
	if result["id"] != "task_1" {
		t.Errorf("Expected id task_1, got %v", result["id"])
	}
}

func TestTaskUpdate(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"PUT /api/chat/tasks/task_1": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "task_1", "name": "Updated"})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.TaskUpdate("task_1", map[string]interface{}{"name": "Updated"})
	if err != nil {
		t.Fatalf("TaskUpdate failed: %v", err)
	}
	if result["name"] != "Updated" {
		t.Errorf("Expected name Updated, got %v", result["name"])
	}
}

func TestTaskDelete(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"DELETE /api/chat/tasks/task_1": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	err := client.TaskDelete("task_1")
	if err != nil {
		t.Fatalf("TaskDelete failed: %v", err)
	}
}

func TestTaskDue(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/chat/tasks/due*": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"tasks": []interface{}{}})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.TaskDue("2026-03-20T00:00:00Z")
	if err != nil {
		t.Fatalf("TaskDue failed: %v", err)
	}
	if result["tasks"] == nil {
		t.Error("Expected tasks field")
	}
}

func TestTaskStart(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/tasks/task_1/start": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "task_1", "status": "running"})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.TaskStart("task_1")
	if err != nil {
		t.Fatalf("TaskStart failed: %v", err)
	}
	if result["status"] != "running" {
		t.Errorf("Expected status running, got %v", result["status"])
	}
}

func TestTaskSucceed(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/tasks/task_1/succeed": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "task_1", "status": "active"})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.TaskSucceed("task_1", map[string]interface{}{"output": "OK"})
	if err != nil {
		t.Fatalf("TaskSucceed failed: %v", err)
	}
	if result["status"] != "active" {
		t.Errorf("Expected status active, got %v", result["status"])
	}
}

func TestTaskFail(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/tasks/task_1/fail": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "task_1"})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.TaskFail("task_1", map[string]interface{}{"error": "Timeout"})
	if err != nil {
		t.Fatalf("TaskFail failed: %v", err)
	}
}

func TestTaskPause(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/tasks/task_1/pause": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "task_1", "status": "paused"})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.TaskPause("task_1")
	if err != nil {
		t.Fatalf("TaskPause failed: %v", err)
	}
	if result["status"] != "paused" {
		t.Errorf("Expected status paused, got %v", result["status"])
	}
}

func TestTaskResume(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/tasks/task_1/resume": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "task_1", "status": "active"})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.TaskResume("task_1", map[string]interface{}{})
	if err != nil {
		t.Fatalf("TaskResume failed: %v", err)
	}
	if result["status"] != "active" {
		t.Errorf("Expected status active, got %v", result["status"])
	}
}

// ============================================================================
// Agent CRUD Tests
// ============================================================================

func TestAgentCreate(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/agents": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "agent_1", "name": "TestAgent"})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.AgentCreate(map[string]interface{}{"name": "TestAgent"})
	if err != nil {
		t.Fatalf("AgentCreate failed: %v", err)
	}
	if result["name"] != "TestAgent" {
		t.Errorf("Expected name TestAgent, got %v", result["name"])
	}
}

func TestAgentList(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/chat/agents": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"agents": []interface{}{}})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.AgentList()
	if err != nil {
		t.Fatalf("AgentList failed: %v", err)
	}
	if result["agents"] == nil {
		t.Error("Expected agents field")
	}
}

func TestAgentGet(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/chat/agents/agent_1": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "agent_1", "name": "TestAgent"})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.AgentGet("agent_1")
	if err != nil {
		t.Fatalf("AgentGet failed: %v", err)
	}
	if result["id"] != "agent_1" {
		t.Errorf("Expected id agent_1, got %v", result["id"])
	}
}

func TestAgentGetByName(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/chat/agents/by-name/TestAgent": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "agent_1", "name": "TestAgent"})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.AgentGetByName("TestAgent")
	if err != nil {
		t.Fatalf("AgentGetByName failed: %v", err)
	}
	if result["name"] != "TestAgent" {
		t.Errorf("Expected name TestAgent, got %v", result["name"])
	}
}

func TestAgentUpdate(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"PUT /api/chat/agents/agent_1": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": "agent_1", "name": "Updated"})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.AgentUpdate("agent_1", map[string]interface{}{"name": "Updated"})
	if err != nil {
		t.Fatalf("AgentUpdate failed: %v", err)
	}
	if result["name"] != "Updated" {
		t.Errorf("Expected name Updated, got %v", result["name"])
	}
}

func TestAgentDelete(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"DELETE /api/chat/agents/agent_1": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	err := client.AgentDelete("agent_1")
	if err != nil {
		t.Fatalf("AgentDelete failed: %v", err)
	}
}

func TestAgentsByDeployment(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/chat/agents/by-deployment/deploy_1": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"agents": []interface{}{map[string]interface{}{"id": "agent_1"}}})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.AgentsByDeployment("deploy_1")
	if err != nil {
		t.Fatalf("AgentsByDeployment failed: %v", err)
	}
	if result["agents"] == nil {
		t.Error("Expected agents field")
	}
}

// ============================================================================
// Error Tests
// ============================================================================

func TestGoalGetNotFound(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/chat/goals/nonexistent": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("Not Found"))
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.GoalGet("nonexistent")
	if err == nil {
		t.Fatal("Expected error for non-existent goal")
	}
}

func TestTaskGetNotFound(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/chat/tasks/nonexistent": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("Not Found"))
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.TaskGet("nonexistent")
	if err == nil {
		t.Fatal("Expected error for non-existent task")
	}
}

func TestAgentGetNotFound(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/chat/agents/nonexistent": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("Not Found"))
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.AgentGet("nonexistent")
	if err == nil {
		t.Fatal("Expected error for non-existent agent")
	}
}

// ============================================================================
// Goal Template CRUD Tests
// ============================================================================

func TestGoalTemplateCreate(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat/goal-templates": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "tpl_1", "title": "Migration Template",
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.GoalTemplateCreate(map[string]interface{}{"title": "Migration Template"})
	if err != nil {
		t.Fatalf("GoalTemplateCreate failed: %v", err)
	}
	if result["id"] != "tpl_1" {
		t.Errorf("Expected id tpl_1, got %v", result["id"])
	}
}

func TestGoalTemplateList(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/chat/goal-templates": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"templates": []map[string]interface{}{{"id": "tpl_1"}, {"id": "tpl_2"}},
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.GoalTemplateList()
	if err != nil {
		t.Fatalf("GoalTemplateList failed: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
}

func TestGoalTemplateGet(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/chat/goal-templates/tpl_1": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "tpl_1", "title": "Migration Template",
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.GoalTemplateGet("tpl_1")
	if err != nil {
		t.Fatalf("GoalTemplateGet failed: %v", err)
	}
	if result["id"] != "tpl_1" {
		t.Errorf("Expected id tpl_1, got %v", result["id"])
	}
}

func TestGoalTemplateUpdate(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"PUT /api/chat/goal-templates/tpl_1": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "tpl_1", "title": "Updated Template",
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.GoalTemplateUpdate("tpl_1", map[string]interface{}{"title": "Updated Template"})
	if err != nil {
		t.Fatalf("GoalTemplateUpdate failed: %v", err)
	}
	if result["title"] != "Updated Template" {
		t.Errorf("Expected title 'Updated Template', got %v", result["title"])
	}
}

func TestGoalTemplateDelete(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"DELETE /api/chat/goal-templates/tpl_1": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	err := client.GoalTemplateDelete("tpl_1")
	if err != nil {
		t.Fatalf("GoalTemplateDelete failed: %v", err)
	}
}
