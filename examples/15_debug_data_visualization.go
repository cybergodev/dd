//go:build examples

package main

import (
	"fmt"

	"github.com/cybergodev/dd"
)

// Debug data visualization examples
func main() {
	fmt.Println("DD Logger - Debug Data Visualization")
	fmt.Println("====================================")

	example1PackageLevelDebug()
	example2LoggerInstanceDebug()
	example3ComplexDataStructures()
	example4RealWorldScenarios()

	fmt.Println("\n=== All examples completed ===")
}

// Example 1: Package-level debug functions
func example1PackageLevelDebug() {
	fmt.Println("\n=== Example 1: Package-Level Debug Functions ===")

	// Simple data types
	dd.Json("Hello World")
	dd.Json(42)
	dd.Json(true)
	dd.Json([]int{1, 2, 3, 4, 5})

	// Map data
	userData := map[string]any{
		"name":  "John Doe",
		"age":   30,
		"email": "john@example.com",
	}
	fmt.Println("\nCompact JSON output:")
	dd.Json(userData)

	fmt.Println("\nPretty-printed JSON output:")
	dd.Text(userData)
}

// Example 2: Logger instance debug methods
func example2LoggerInstanceDebug() {
	fmt.Println("\n=== Example 2: Logger Instance Debug Methods ===")

	logger := dd.ToConsole()
	defer logger.Close()

	config := map[string]any{
		"database": map[string]any{
			"host":     "localhost",
			"port":     5432,
			"database": "myapp",
		},
		"cache": map[string]any{
			"enabled": true,
			"ttl":     300,
		},
	}

	fmt.Println("\nUsing logger.Json():")
	logger.Json(config)

	fmt.Println("\nUsing logger.Text():")
	logger.Text(config)
}

// Example 3: Complex data structures
func example3ComplexDataStructures() {
	fmt.Println("\n=== Example 3: Complex Data Structures ===")

	type Address struct {
		Street  string `json:"street"`
		City    string `json:"city"`
		ZipCode string `json:"zip_code"`
	}

	type User struct {
		ID       int            `json:"id"`
		Name     string         `json:"name"`
		Email    string         `json:"email"`
		Age      int            `json:"age"`
		Active   bool           `json:"active"`
		Address  Address        `json:"address"`
		Tags     []string       `json:"tags"`
		Metadata map[string]any `json:"metadata"`
	}

	user := User{
		ID:     1001,
		Name:   "Alice Johnson",
		Email:  "alice@example.com",
		Age:    28,
		Active: true,
		Address: Address{
			Street:  "456 Oak Avenue",
			City:    "San Francisco",
			ZipCode: "94102",
		},
		Tags: []string{"premium", "verified", "developer"},
		Metadata: map[string]any{
			"last_login":   "2024-12-04T10:30:00Z",
			"login_count":  142,
			"account_type": "professional",
		},
	}

	fmt.Println("\nCompact JSON (for logs):")
	dd.Json(user)

	fmt.Println("\nPretty-printed JSON (for debugging):")
	dd.Text(user)
}

// Example 4: Real-world debugging scenarios
func example4RealWorldScenarios() {
	fmt.Println("\n=== Example 4: Real-World Debugging Scenarios ===")

	// Scenario 1: API response debugging
	fmt.Println("\nScenario 1: API Response Debugging")
	apiResponse := map[string]any{
		"status": "success",
		"code":   200,
		"data": map[string]any{
			"users": []map[string]any{
				{"id": 1, "name": "User 1"},
				{"id": 2, "name": "User 2"},
			},
			"total": 2,
		},
		"timestamp": "2024-12-04T10:30:00Z",
	}
	dd.Text(apiResponse)

	// Scenario 2: Configuration debugging
	fmt.Println("\nScenario 2: Configuration Debugging")
	appConfig := map[string]any{
		"server": map[string]any{
			"host": "0.0.0.0",
			"port": 8080,
		},
		"database": map[string]any{
			"connections": 10,
			"timeout":     30,
		},
		"features": map[string]bool{
			"auth":    true,
			"cache":   true,
			"metrics": false,
		},
	}
	dd.Text(appConfig)

	// Scenario 3: Error context debugging
	fmt.Println("\nScenario 3: Error Context Debugging")
	errorContext := map[string]any{
		"error":     "database connection failed",
		"timestamp": "2024-12-04T10:35:00Z",
		"context": map[string]any{
			"host":         "db.example.com",
			"port":         5432,
			"retry_count":  3,
			"last_attempt": "2024-12-04T10:34:55Z",
		},
	}
	dd.Text(errorContext)
}
