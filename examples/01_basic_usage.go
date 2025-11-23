//go:build examples

package main

import (
	"fmt"

	"github.com/cybergodev/dd"
)

// Basic usage examples - covering all core APIs
func main() {
	fmt.Println("DD Logger - Basic Usage Examples")
	fmt.Println("=================================")

	example1PackageLevelLogging()
	example2LoggerInstances()
	example3LogLevels()
	example4FormattedLogging()
	example5DynamicConfiguration()

	fmt.Println("\n=== All examples completed ===")
}

// Example 1: Package-level functions (simplest)
func example1PackageLevelLogging() {
	fmt.Println("\n=== Example 1: Package-Level Functions ===")

	// Use package-level functions directly (uses default logger)
	dd.Debug("Debug message")
	dd.Info("Application started successfully")
	dd.Warn("This is a warning message")
	dd.Error("An error occurred")

	// Formatted logging
	userID := 12345
	dd.Infof("User %d logged in", userID)
	dd.Errorf("Failed to process user %d: %s", userID, "invalid token")

	// Multiple arguments
	dd.Info("Processing", "user", userID, "time", "2024-01-01")
}

// Example 2: Logger instances
func example2LoggerInstances() {
	fmt.Println("\n=== Example 2: Logger Instances ===")

	// Console only
	logger1 := dd.ToConsole()
	defer logger1.Close()
	logger1.Info("Output to console only")

	// Development mode (Debug level + caller info)
	logger2, _ := dd.NewWithOptions(dd.Options{
		Level:         dd.LevelDebug,
		IncludeCaller: true,
		Console:       true,
	})
	defer logger2.Close()
	logger2.Debug("Development mode: Debug messages visible")
	logger2.Info("Development mode: includes caller information")

	// JSON file
	logger3 := dd.ToJSONFile("logs/basic-usage.json.log")
	defer logger3.Close()
	logger3.Info("JSON format output, suitable for log aggregation")

	// Console + file
	logger4 := dd.ToAll("logs/basic-usage.log")
	defer logger4.Close()
	logger4.Info("Output to both console and file")
}

// Example 3: Log levels
func example3LogLevels() {
	fmt.Println("\n=== Example 3: Log Levels ===")

	logger, _ := dd.NewWithOptions(dd.Options{
		Level:   dd.LevelDebug,
		Console: true,
	})
	defer logger.Close()

	logger.Debug("DEBUG: Debug information")
	logger.Info("INFO: General information")
	logger.Warn("WARN: Warning information")
	logger.Error("ERROR: Error information")
	// logger.Fatal("FATAL: Fatal error, will terminate program")

	fmt.Printf("Current log level: %s\n", logger.GetLevel().String())
}

// Example 4: Formatted logging
func example4FormattedLogging() {
	fmt.Println("\n=== Example 4: Formatted Logging ===")

	logger := dd.ToConsole()
	defer logger.Close()

	// Printf-style formatting
	name := "John"
	age := 30
	logger.Infof("User info: name=%s, age=%d", name, age)

	// Multiple arguments (space-separated)
	logger.Info("User", name, "age", age, "years")

	// Structured logging (recommended for JSON output)
	logger.InfoWith("User information updated",
		dd.Any("name", name),
		dd.Any("age", age),
		dd.Any("timestamp", "2024-01-01T12:00:00Z"),
	)

	// Error logging
	err := fmt.Errorf("database connection failed")
	logger.ErrorWith("Database operation failed",
		dd.Err(err),
		dd.Any("operation", "user_lookup"),
		dd.Any("retry_count", 3),
	)
}

// Example 5: Dynamic configuration
func example5DynamicConfiguration() {
	fmt.Println("\n=== Example 5: Dynamic Configuration ===")

	logger := dd.ToConsole()
	defer logger.Close()

	// Initial level (default INFO)
	logger.Debug("Debug message won't show")
	logger.Info("Info message will show")

	// Dynamically adjust level
	logger.SetLevel(dd.LevelDebug)
	logger.Debug("Now Debug messages are visible!")

	// Change back to INFO
	logger.SetLevel(dd.LevelInfo)
	logger.Debug("Debug messages hidden again")
	logger.Info("Info messages still visible")

	// Get current configuration
	currentLevel := logger.GetLevel()
	logger.Infof("Current log level: %s", currentLevel.String())
}
