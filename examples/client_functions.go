/**
 * Scripts Example for ekoDB Go Client
 *
 * Demonstrates creating, managing, and executing scripts with the Go client.
 * Covers: FindAll, Group, Project, Count, and Script management operations.
 */

package main

import (
	"fmt"
	"log"
	"os"

	ekodb "github.com/ekodb/ekodb-client-go"
)

func setupTestData(client *ekodb.Client) error {
	fmt.Println("📋 Setting up test data...")

	for i := 1; i <= 10; i++ {
		record := map[string]interface{}{
			"name":   fmt.Sprintf("User %d", i),
			"age":    20 + i,
			"status": map[string]interface{}{"active": i%2 == 0, "inactive": i%2 != 0}[fmt.Sprintf("%v", i%2 == 0)],
			"score":  i * 10,
		}
		if i%2 == 0 {
			record["status"] = "active"
		} else {
			record["status"] = "inactive"
		}

		if _, err := client.Insert("users", record); err != nil {
			return err
		}
	}

	fmt.Println("✅ Test data ready\n")
	return nil
}

func simpleQueryScript(client *ekodb.Client) (string, error) {
	fmt.Println("📝 Example 1: Simple Query Script\n")

	script := ekodb.Script{
		Label:       "get_active_users",
		Name:        "Get Active Users",
		Description: strPtr("Retrieve all active users"),
		Version:     "1.0",
		Parameters:  map[string]ekodb.ParameterDefinition{},
		Functions: []ekodb.FunctionStageConfig{
			ekodb.StageFindAll("users"),
		},
		Tags: []string{"users", "query"},
	}

	scriptID, err := client.SaveScript(script)
	if err != nil {
		return "", err
	}
	fmt.Printf("✅ Script saved: %s\n", scriptID)

	result, err := client.CallScript("get_active_users", nil)
	if err != nil {
		return "", err
	}
	fmt.Printf("📊 Found %d records\n", len(result.Records))
	fmt.Printf("⏱️  Execution time: %dms\n\n", result.Stats.ExecutionTimeMs)

	return scriptID, nil
}

func parameterizedScript(client *ekodb.Client) error {
	fmt.Println("📝 Example 2: Parameterized Script\n")

	script := ekodb.Script{
		Label:   "get_users_by_status",
		Name:    "Get Users By Status",
		Version: "1.0",
		Parameters: map[string]ekodb.ParameterDefinition{
			"status": {
				Required:    false,
				Default:     "active",
				Description: "Filter by user status",
			},
			"limit": {
				Required:    false,
				Default:     10,
				Description: "Maximum number of results",
			},
		},
		Functions: []ekodb.FunctionStageConfig{
			ekodb.StageFindAll("users"),
		},
		Tags: []string{"users", "parameterized"},
	}

	_, err := client.SaveScript(script)
	if err != nil {
		return err
	}
	fmt.Println("✅ Script saved")

	params := map[string]interface{}{
		"status": "active",
		"limit":  3,
	}
	result, err := client.CallScript("get_users_by_status", params)
	if err != nil {
		return err
	}
	fmt.Printf("📊 Found %d users (limited)\n", len(result.Records))
	fmt.Printf("⏱️  Execution time: %dms\n\n", result.Stats.ExecutionTimeMs)

	return nil
}

func aggregationScript(client *ekodb.Client) (string, error) {
	fmt.Println("📝 Example 3: Aggregation Script\n")

	script := ekodb.Script{
		Label:      "user_stats",
		Name:       "User Statistics",
		Version:    "1.0",
		Parameters: map[string]ekodb.ParameterDefinition{},
		Functions: []ekodb.FunctionStageConfig{
			ekodb.StageFindAll("users"),
			ekodb.StageGroup(
				[]string{"status"},
				[]ekodb.GroupFunctionConfig{
					{
						OutputField: "count",
						Operation:   ekodb.GroupFunctionCount,
					},
					{
						OutputField: "avg_score",
						Operation:   ekodb.GroupFunctionAverage,
						InputField:  strPtr("score"),
					},
				},
			),
		},
		Tags: []string{"analytics"},
	}

	scriptID, err := client.SaveScript(script)
	if err != nil {
		return "", err
	}
	fmt.Println("✅ Script saved")

	result, err := client.CallScript("user_stats", nil)
	if err != nil {
		return "", err
	}
	fmt.Printf("📊 Statistics: %d groups\n", len(result.Records))
	for _, record := range result.Records {
		fmt.Printf("   %v\n", record)
	}
	fmt.Printf("⏱️  Execution time: %dms\n\n", result.Stats.ExecutionTimeMs)

	return scriptID, nil
}

func scriptManagement(client *ekodb.Client, getActiveUsersID, userStatsID string) error {
	fmt.Println("📝 Example 4: Script Management\n")

	// List all scripts
	scripts, err := client.ListScripts(nil)
	if err != nil {
		return err
	}
	fmt.Printf("📋 Total scripts: %d\n", len(scripts))

	// Get specific script (use encrypted ID)
	script, err := client.GetScript(getActiveUsersID)
	if err != nil {
		return err
	}
	fmt.Printf("🔍 Retrieved script: %s\n", script.Name)

	// Update script (use encrypted ID)
	updated := ekodb.Script{
		Label:       "get_active_users",
		Name:        "Get Active Users (Updated)",
		Description: strPtr("Updated description"),
		Version:     "1.1",
		Parameters:  map[string]ekodb.ParameterDefinition{},
		Functions: []ekodb.FunctionStageConfig{
			ekodb.StageFindAll("users"),
		},
		Tags: []string{"users"},
	}
	if err := client.UpdateScript(getActiveUsersID, updated); err != nil {
		return err
	}
	fmt.Println("✏️  Script updated")

	// Delete script (use ID) - handle error gracefully
	if err := client.DeleteScript(userStatsID); err != nil {
		fmt.Println("ℹ️  Script delete skipped (may not exist)")
	} else {
		fmt.Println("🗑️  Script deleted")
	}
	fmt.Println()

	fmt.Println("ℹ️  Note: GET/UPDATE/DELETE operations require the encrypted ID")
	fmt.Println("ℹ️  Only CALL can use either ID or label\n")

	return nil
}

func multiStageScript(client *ekodb.Client) error {
	fmt.Println("📝 Example 5: Multi-Stage Pipeline\n")

	script := ekodb.Script{
		Label:   "top_users",
		Name:    "Top Performing Users",
		Version: "1.0",
		Parameters: map[string]ekodb.ParameterDefinition{
			"min_score": {
				Required: false,
				Default:  50,
			},
		},
		Functions: []ekodb.FunctionStageConfig{
			ekodb.StageFindAll("users"),
			ekodb.StageProject([]string{"name", "score", "status"}, false),
		},
		Tags: []string{"analytics", "reporting"},
	}

	_, err := client.SaveScript(script)
	if err != nil {
		return err
	}
	fmt.Println("✅ Multi-stage script saved")

	params := map[string]interface{}{"min_score": 50}
	result, err := client.CallScript("top_users", params)
	if err != nil {
		return err
	}
	fmt.Printf("📊 Pipeline executed %d stages\n", result.Stats.StagesExecuted)
	fmt.Printf("⏱️  Total execution time: %dms\n", result.Stats.ExecutionTimeMs)
	fmt.Println("📈 Stage breakdown:")
	for i, stage := range result.Stats.StageStats {
		fmt.Printf("   %d. %s: %dms (%d → %d records)\n",
			i+1, stage.Stage, stage.ExecutionTimeMs, stage.InputCount, stage.OutputCount)
	}
	fmt.Println()

	return nil
}

func countScript(client *ekodb.Client) error {
	fmt.Println("📝 Example 6: Count Users\n")

	script := ekodb.Script{
		Label:      "count_users",
		Name:       "Count All Users",
		Version:    "1.0",
		Parameters: map[string]ekodb.ParameterDefinition{},
		Functions: []ekodb.FunctionStageConfig{
			ekodb.StageFindAll("users"),
			ekodb.StageCount("count"),
		},
		Tags: []string{"users", "count"},
	}

	_, err := client.SaveScript(script)
	if err != nil {
		return err
	}
	fmt.Println("✅ Count script saved")

	result, err := client.CallScript("count_users", nil)
	if err != nil {
		return err
	}
	count := 0
	if len(result.Records) > 0 {
		if c, ok := result.Records[0]["count"].(float64); ok {
			count = int(c)
		} else if c, ok := result.Records[0]["count"].(int); ok {
			count = c
		}
	}
	fmt.Printf("📊 Total user count: %d\n", count)
	fmt.Printf("⏱️  Execution time: %dms\n\n", result.Stats.ExecutionTimeMs)

	return nil
}

func cleanup(client *ekodb.Client) error {
	fmt.Println("🧹 Cleaning up...")

	// Delete test collection
	if err := client.DeleteCollection("users"); err != nil {
		return err
	}
	fmt.Println("✅ Deleted collection")

	// List and delete all test scripts
	scripts, err := client.ListScripts(nil)
	if err != nil {
		return err
	}
	for _, script := range scripts {
		if len(script.Label) > 4 && (script.Label[:4] == "get_" || script.Label[:5] == "user_" ||
			script.Label[:4] == "top_" || script.Label[:6] == "count_") {
			if script.ID != nil {
				_ = client.DeleteScript(*script.ID)
			}
		}
	}
	fmt.Println("✅ Deleted test scripts\n")

	return nil
}

func main() {
	fmt.Println("🚀 ekoDB Scripts Example (Go Client)\n")

	baseURL := os.Getenv("API_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	apiKey := os.Getenv("API_BASE_KEY")
	if apiKey == "" {
		apiKey = "a-test-api-key-from-ekodb"
	}

	client, err := ekodb.NewClient(baseURL, apiKey)
	if err != nil {
		log.Fatalf("❌ Failed to create client: %v", err)
	}
	fmt.Println("✅ Client initialized\n")

	if err := setupTestData(client); err != nil {
		log.Fatalf("❌ Error: %v", err)
	}

	getActiveUsersID, err := simpleQueryScript(client)
	if err != nil {
		log.Fatalf("❌ Error: %v", err)
	}

	if err := parameterizedScript(client); err != nil {
		log.Fatalf("❌ Error: %v", err)
	}

	userStatsID, err := aggregationScript(client)
	if err != nil {
		log.Fatalf("❌ Error: %v", err)
	}

	if err := scriptManagement(client, getActiveUsersID, userStatsID); err != nil {
		log.Fatalf("❌ Error: %v", err)
	}

	if err := multiStageScript(client); err != nil {
		log.Fatalf("❌ Error: %v", err)
	}

	if err := countScript(client); err != nil {
		log.Fatalf("❌ Error: %v", err)
	}

	if err := cleanup(client); err != nil {
		log.Fatalf("❌ Error: %v", err)
	}

	fmt.Println("✅ All examples completed successfully!")
	fmt.Println("\n💡 Key Advantages of Using the Client:")
	fmt.Println("   • Automatic token management")
	fmt.Println("   • Type-safe Stage builders")
	fmt.Println("   • Built-in error handling")
}

// Helper function to create string pointers
func strPtr(s string) *string {
	return &s
}
