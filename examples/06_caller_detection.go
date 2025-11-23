//go:build examples

package main

import (
	"fmt"

	"github.com/cybergodev/dd"
)

// Caller information detection examples
func main() {
	fmt.Println("DD Logger - Caller Detection Examples")
	fmt.Println("=====================================")

	example1BasicCaller()
	example2DynamicCaller()
	example3CallerWithWrappers()
	example4BestPractices()

	fmt.Println("\n=== All examples completed ===")
}

// Example 1: Basic caller information
func example1BasicCaller() {
	fmt.Println("\n=== Example 1: Basic Caller Information ===")

	// Don't show caller info (default)
	logger1 := dd.ToConsole()
	defer logger1.Close()
	logger1.Info("Default config: no caller information")

	// Show caller info (filename:line)
	logger2, _ := dd.NewWithOptions(dd.Options{
		IncludeCaller: true,
		Console:       true,
	})
	defer logger2.Close()
	logger2.Info("Show caller info: filename and line number")

	// Show full path
	logger3, _ := dd.NewWithOptions(dd.Options{
		IncludeCaller: true,
		FullPath:      true, // Show full file path
		Console:       true,
	})
	defer logger3.Close()
	logger3.Info("Show full file path")

	fmt.Println("\nTips:")
	fmt.Println("- FullPath: false (default) shows filename, e.g. main.go:42")
	fmt.Println("- FullPath: true shows full path, e.g. /path/to/project/main.go:42")
}

// Example 2: Dynamic caller detection
func example2DynamicCaller() {
	fmt.Println("\n=== Example 2: Dynamic Caller Detection ===")

	// Enable dynamic caller detection
	logger, _ := dd.NewWithOptions(dd.Options{
		IncludeCaller: true,
		DynamicCaller: true, // Auto-detect call depth
		Console:       true,
	})
	defer logger.Close()

	fmt.Println("Dynamic detection can accurately find the real call location, even through wrapper functions")

	// Direct call
	logger.Info("Direct call - shows this line location")

	// Through wrapper function
	logThroughWrapper(logger, "Through wrapper - shows call location in main function")

	// Through nested wrappers
	logThroughNestedWrapper(logger, "Nested wrappers - still shows call location in main function")
}

// Example 3: Caller info in wrapper functions
func example3CallerWithWrappers() {
	fmt.Println("\n=== Example 3: Caller Info in Wrapper Functions ===")

	// Without dynamic detection
	logger1, _ := dd.NewWithOptions(dd.Options{
		IncludeCaller: true,
		DynamicCaller: false, // Don't enable dynamic detection
		Console:       true,
	})
	defer logger1.Close()

	fmt.Println("Without dynamic detection, shows call location inside wrapper function:")
	logThroughWrapper(logger1, "Shows location inside logThroughWrapper function")

	// With dynamic detection
	logger2, _ := dd.NewWithOptions(dd.Options{
		IncludeCaller: true,
		DynamicCaller: true, // Enable dynamic detection
		Console:       true,
	})
	defer logger2.Close()

	fmt.Println("\nWith dynamic detection, shows real call location:")
	logThroughWrapper(logger2, "Shows call location in main function")
}

// Example 4: Best practices
func example4BestPractices() {
	fmt.Println("\n=== Example 4: Best Practices ===")

	logger, _ := dd.NewWithOptions(dd.Options{
		IncludeCaller: true,
		DynamicCaller: true,
		Console:       true,
	})
	defer logger.Close()

	fmt.Println("Recommended practices:")
	fmt.Println("1. Enable caller info and dynamic detection in development")
	fmt.Println("2. Disable caller info in production for better performance")
	fmt.Println("3. Use structured logging to record context information")

	// Direct call (recommended)
	logger.InfoWith("User action",
		dd.Any("user_id", "12345"),
		dd.Any("action", "login"),
		dd.Any("ip", "192.168.1.1"),
	)

	// Through business function
	processUserLogin(logger, "user-456")
}

// Helper functions

func logThroughWrapper(logger *dd.Logger, msg string) {
	// Wrapper function: calls logger.Info
	logger.Info(msg)
}

func logThroughNestedWrapper(logger *dd.Logger, msg string) {
	// Nested wrapper
	logThroughWrapper(logger, msg)
}

func processUserLogin(logger *dd.Logger, userID string) {
	// Business logic function
	logger.InfoWith("Processing user login",
		dd.Any("user_id", userID),
		dd.Any("timestamp", "2024-01-15T10:30:45Z"),
	)
}
