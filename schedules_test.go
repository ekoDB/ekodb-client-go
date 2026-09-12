package ekodb

import (
	"encoding/json"
	"net/http"
	"testing"
)

// ============================================================================
// Schedule CRUD Tests
// ============================================================================

func TestCreateSchedule(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/schedules": func(w http.ResponseWriter, r *http.Request) {
			var request map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if request["cron_expression"] != "0 0 2 * * *" {
				t.Errorf("Expected six-field cron_expression, got %v", request["cron_expression"])
			}
			if request["function_label"] != "nightly_backup" {
				t.Errorf("Expected function_label nightly_backup, got %v", request["function_label"])
			}
			if _, ok := request["task_type"]; ok {
				t.Error("CreateSchedule request must not contain task_type")
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "sched_1", "name": "Daily Backup", "cron_expression": "0 0 2 * * *", "enabled": true,
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.CreateSchedule(map[string]interface{}{
		"name":            "Daily Backup",
		"cron_expression": "0 0 2 * * *",
		"function_label":  "nightly_backup",
	})
	if err != nil {
		t.Fatalf("CreateSchedule failed: %v", err)
	}
	if result["id"] != "sched_1" {
		t.Errorf("Expected id sched_1, got %v", result["id"])
	}
	if result["name"] != "Daily Backup" {
		t.Errorf("Expected name Daily Backup, got %v", result["name"])
	}
}

func TestListSchedules(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/schedules": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"schedules": []map[string]interface{}{
					{"id": "sched_1", "name": "Daily Backup"},
					{"id": "sched_2", "name": "Hourly Sync"},
				},
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.ListSchedules()
	if err != nil {
		t.Fatalf("ListSchedules failed: %v", err)
	}
	if result["schedules"] == nil {
		t.Error("Expected schedules field")
	}
}

func TestGetSchedule(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/schedules/sched_1": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "sched_1", "name": "Daily Backup", "cron_expression": "0 0 2 * * *",
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.GetSchedule("sched_1")
	if err != nil {
		t.Fatalf("GetSchedule failed: %v", err)
	}
	if result["id"] != "sched_1" {
		t.Errorf("Expected id sched_1, got %v", result["id"])
	}
	if result["cron_expression"] != "0 0 2 * * *" {
		t.Errorf("Expected cron_expression '0 0 2 * * *', got %v", result["cron_expression"])
	}
}

func TestUpdateSchedule(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"PUT /api/schedules/sched_1": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "sched_1", "name": "Weekly Backup", "cron_expression": "0 0 3 * * *",
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.UpdateSchedule("sched_1", map[string]interface{}{
		"name":            "Weekly Backup",
		"cron_expression": "0 0 3 * * *",
	})
	if err != nil {
		t.Fatalf("UpdateSchedule failed: %v", err)
	}
	if result["name"] != "Weekly Backup" {
		t.Errorf("Expected name Weekly Backup, got %v", result["name"])
	}
}

func TestDeleteSchedule(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"DELETE /api/schedules/sched_1": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	err := client.DeleteSchedule("sched_1")
	if err != nil {
		t.Fatalf("DeleteSchedule failed: %v", err)
	}
}

func TestPauseSchedule(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"PUT /api/schedules/sched_1": func(w http.ResponseWriter, r *http.Request) {
			var request map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if request["enabled"] != false {
				t.Errorf("Expected enabled false, got %v", request["enabled"])
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "sched_1", "enabled": false,
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.PauseSchedule("sched_1")
	if err != nil {
		t.Fatalf("PauseSchedule failed: %v", err)
	}
	if result["enabled"] != false {
		t.Errorf("Expected enabled false, got %v", result["enabled"])
	}
}

func TestResumeSchedule(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"PUT /api/schedules/sched_1": func(w http.ResponseWriter, r *http.Request) {
			var request map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if request["enabled"] != true {
				t.Errorf("Expected enabled true, got %v", request["enabled"])
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "sched_1", "enabled": true,
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.ResumeSchedule("sched_1")
	if err != nil {
		t.Fatalf("ResumeSchedule failed: %v", err)
	}
	if result["enabled"] != true {
		t.Errorf("Expected enabled true, got %v", result["enabled"])
	}
}

func TestTriggerSchedule(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/schedules/sched_1/trigger": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "triggered", "schedule_id": "sched_1",
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.TriggerSchedule("sched_1")
	if err != nil {
		t.Fatalf("TriggerSchedule failed: %v", err)
	}
	if result["status"] != "triggered" || result["schedule_id"] != "sched_1" {
		t.Errorf("Unexpected trigger response: %v", result)
	}
}

// ============================================================================
// Error Tests
// ============================================================================

func TestGetScheduleNotFound(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/schedules/nonexistent": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("Not Found"))
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.GetSchedule("nonexistent")
	if err == nil {
		t.Fatal("Expected error for non-existent schedule")
	}
}

func TestDeleteScheduleNotFound(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"DELETE /api/schedules/nonexistent": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("Not Found"))
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	err := client.DeleteSchedule("nonexistent")
	if err == nil {
		t.Fatal("Expected error for non-existent schedule")
	}
}

func TestPauseScheduleAlreadyPaused(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"PUT /api/schedules/sched_1": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte("Schedule already paused"))
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.PauseSchedule("sched_1")
	if err == nil {
		t.Fatal("Expected error for already paused schedule")
	}
}

func TestTriggerScheduleConflict(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/schedules/sched_1/trigger": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte("Schedule already executing"))
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.TriggerSchedule("sched_1")
	if err == nil {
		t.Fatal("Expected error for a schedule already executing")
	}
}

func TestCreateScheduleServerError(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/schedules": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("Internal Server Error"))
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.CreateSchedule(map[string]interface{}{"name": "Bad"})
	if err == nil {
		t.Fatal("Expected error for server error")
	}
}
