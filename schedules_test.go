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
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "sched_1", "name": "Daily Backup", "cron": "0 0 * * *", "status": "active",
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.CreateSchedule(map[string]interface{}{
		"name": "Daily Backup",
		"cron": "0 0 * * *",
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
			json.NewEncoder(w).Encode(map[string]interface{}{
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
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "sched_1", "name": "Daily Backup", "cron": "0 0 * * *",
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
	if result["cron"] != "0 0 * * *" {
		t.Errorf("Expected cron '0 0 * * *', got %v", result["cron"])
	}
}

func TestUpdateSchedule(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"PUT /api/schedules/sched_1": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "sched_1", "name": "Weekly Backup", "cron": "0 0 * * 0",
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.UpdateSchedule("sched_1", map[string]interface{}{
		"name": "Weekly Backup",
		"cron": "0 0 * * 0",
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
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
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
		"POST /api/schedules/sched_1/pause": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "sched_1", "status": "paused",
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.PauseSchedule("sched_1")
	if err != nil {
		t.Fatalf("PauseSchedule failed: %v", err)
	}
	if result["status"] != "paused" {
		t.Errorf("Expected status paused, got %v", result["status"])
	}
}

func TestResumeSchedule(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/schedules/sched_1/resume": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "sched_1", "status": "active",
			})
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	result, err := client.ResumeSchedule("sched_1")
	if err != nil {
		t.Fatalf("ResumeSchedule failed: %v", err)
	}
	if result["status"] != "active" {
		t.Errorf("Expected status active, got %v", result["status"])
	}
}

// ============================================================================
// Error Tests
// ============================================================================

func TestGetScheduleNotFound(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"GET /api/schedules/nonexistent": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("Not Found"))
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
			w.Write([]byte("Not Found"))
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
		"POST /api/schedules/sched_1/pause": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte("Schedule already paused"))
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.PauseSchedule("sched_1")
	if err == nil {
		t.Fatal("Expected error for already paused schedule")
	}
}

func TestCreateScheduleServerError(t *testing.T) {
	server := createTestServer(t, map[string]http.HandlerFunc{
		"POST /api/schedules": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal Server Error"))
		},
	})
	defer server.Close()

	client := createTestClient(t, server)
	_, err := client.CreateSchedule(map[string]interface{}{"name": "Bad"})
	if err == nil {
		t.Fatal("Expected error for server error")
	}
}
