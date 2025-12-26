package main

import (
	"fmt"

	"github.com/cybergodev/dd"
)

func main() {
	// Example 1: Simple types with Text (no quotes)
	fmt.Println("=== Example 1: Simple Types with Text ===")
	dd.Text("hello world") // Output: hello world (no quotes)
	dd.Text(42)            // Output: 42
	dd.Text(3.14)          // Output: 3.14
	dd.Text(true)          // Output: true

	// Example 2: Simple types with Json (JSON format)
	fmt.Println("\n\n=== Example 2: Simple Types with Json ===")
	dd.Json("hello world") // Output: "hello world" (with quotes)
	dd.Json(42)            // Output: 42
	dd.Json(true)          // Output: true

	// Example 3: Complex types with Text (pretty JSON)
	fmt.Println("\n\n=== Example 3: Complex Types with Text ===")
	dd.Text(map[string]any{"name": "Alice", "age": 30})

	// Example 4: Multiple simple arguments with Text
	fmt.Println("\n\n=== Example 4: Multiple Simple Arguments - Text ===")
	dd.Text("User:", "Alice", "Age:", 30, "Active:", true)
	dd.Text("User:", "Alice", "Age:", 30, "Active:", true)

	// Example 5: Multiple arguments with Json (compact)
	fmt.Println("\n\n=== Example 5: Multiple Arguments - Json ===")
	dd.Json("user", 123, map[string]string{"status": "active"})
	dd.Json("user", 123, map[string]string{"status": "active"})

	// Example 6: Mixed simple and complex types
	fmt.Println("\n\n=== Example 6: Mixed Types - Text ===")
	dd.Text("Simple:", 123, "Complex:", map[string]int{"count": 10})
	dd.Json("data", []string{"a", "b", "c"}, "total", 3)

	// Example 7: Pointers with Text (dereferenced automatically)
	fmt.Println("\n\n=== Example 7: Pointers - Text ===")
	str := "pointer value"
	num := 999
	flag := true
	dd.Text(&str, &num, &flag)

	// Example 8: Pointers with Json
	fmt.Println("\n\n=== Example 8: Pointers - Json ===")
	dd.Json(&str, &num, []int{1, 2, 3})

	// Example 9: Complex nested structures
	fmt.Println("\n\n=== Example 9: Complex Nested Structures ===")
	type Address struct {
		Street  string `json:"street"`
		City    string `json:"city"`
		ZipCode string `json:"zip_code"`
	}

	type User struct {
		Name    string   `json:"name"`
		Age     int      `json:"age"`
		Address Address  `json:"address"`
		Tags    []string `json:"tags"`
	}

	user1 := User{
		Name: "Charlie",
		Age:  35,
		Address: Address{
			Street:  "123 Main St",
			City:    "New York",
			ZipCode: "10001",
		},
		Tags: []string{"developer", "golang"},
	}

	user2 := User{
		Name: "Diana",
		Age:  28,
		Address: Address{
			Street:  "456 Oak Ave",
			City:    "San Francisco",
			ZipCode: "94102",
		},
		Tags: []string{"designer", "ui/ux"},
	}

	dd.Text(user1, user2)

	// Example 10: Using with Logger instance
	fmt.Println("\n\n=== Example 10: Logger Instance Methods ===")
	logger := dd.ToConsole()
	defer logger.Close()

	logger.Text("Simple text with logger")
	logger.Json("request", map[string]any{"method": "GET", "path": "/api/users"})
	logger.Text(
		map[string]any{"response": "success", "code": 200},
		map[string]any{"data": []string{"user1", "user2"}},
	)

	// Example 11: Quick debug multiple variables
	fmt.Println("\n\n=== Example 11: Quick Debug Multiple Variables ===")
	requestID := "req-12345"
	userID := 789
	sessionToken := "token-abc-xyz"
	isAuthenticated := true

	// Text shows simple types without quotes
	fmt.Println("Using Text:")
	dd.Text("Request ID:", requestID, "User ID:", userID, "Authenticated:", isAuthenticated)

	// Json shows everything in JSON format
	fmt.Println("\nUsing Json:")
	dd.Json(requestID, userID, sessionToken, isAuthenticated)

	// Example 12: Comparing data structures
	fmt.Println("\n\n=== Example 12: Comparing Data Structures ===")
	before := map[string]any{"count": 10, "status": "pending"}
	after := map[string]any{"count": 15, "status": "completed"}

	fmt.Println("Before vs After:")
	dd.Text(before, after)

	// Example 13: Nil values
	fmt.Println("\n\n=== Example 13: Nil Values ===")
	var nilPtr *string
	dd.Text(nil, nilPtr, "not nil")

	// Example 14: Formatted output
	fmt.Println("\n\n=== Textf() - Formatted output ===")
	dd.Textf("User: %s, Age: %d, Score: %.2f", "Bob", 25, 95.5)

	fmt.Println("\n\n=== Jsonf() - Formatted JSON output ===")
	dd.Jsonf("Request from %s at %d", "192.168.1.1", 1234567890)

	// Example 15: Logger methods
	fmt.Println("\n\n=== Logger methods ===")
	logger2 := dd.ToConsole()
	defer logger2.Close()

	logger2.Text("Processing", "item", 42)
	logger2.Textf("Completed: %d/%d items", 42, 100)

	logger2.Json("result", true, "count", 100)
	logger2.Jsonf("Status: %s", "OK")

}
