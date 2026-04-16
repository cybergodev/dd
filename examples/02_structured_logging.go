//go:build examples

package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/cybergodev/dd"
)

// Structured Logging - Type-Safe Fields and Chaining
//
// Topics covered:
// 1. All field types (String, Int, Bool, Float64, Time, Duration, Err, ErrWithKey)
// 2. WithFields() chaining for reusable context
// 3. LoggerEntry for contextual logging
// 4. Best practices for production
func main() {
	fmt.Println("=== DD Structured Logging ===")

	section1FieldTypes()
	section2WithFields()
	section3BestPractices()

	fmt.Println("\n✅ Structured logging examples completed!")
}

// Section 1: All field types
func section1FieldTypes() {
	fmt.Println("1. Field Types")
	fmt.Println("---------------")

	logger, _ := dd.New()
	defer logger.Close()

	// Type-safe fields (recommended for performance)
	logger.InfoWith("All field types:",
		// Strings and numbers
		dd.String("user", "alice"),
		dd.Int("count", 42),
		dd.Int64("id", 9876543210),
		dd.Float64("score", 98.5),

		// Boolean and time
		dd.Bool("active", true),
		dd.Time("created_at", time.Now()),
		dd.Duration("elapsed", 150*time.Millisecond),

		// Error handling
		dd.Err(errors.New("connection failed")),

		// Complex types (use Any)
		dd.Any("tags", []string{"vip", "premium"}),
		dd.Any("meta", map[string]int{"requests": 100}),
	)

	fmt.Println()
}

// Section 2: WithFields() chaining
func section2WithFields() {
	fmt.Println("2. WithFields() Chaining")
	fmt.Println("-------------------------")

	logger, _ := dd.New()
	defer logger.Close()

	// Create logger entry with persistent fields
	userLogger := logger.WithFields(
		dd.String("service", "user-api"),
		dd.String("version", "1.0.0"),
	)

	// All logs from userLogger include service and version
	userLogger.Info("User authenticated:")
	userLogger.InfoWith("Profile loaded:",
		dd.String("user_id", "123"),
		dd.Int("roles", 3),
	)

	// Chain more fields - creates new entry, doesn't modify original
	requestLogger := userLogger.WithFields(
		dd.String("request_id", "req-abc-123"),
	)
	requestLogger.Info("Processing request:")
	requestLogger.InfoWith("Request completed:",
		dd.Int("status", 200),
		dd.Duration("latency", 45*time.Millisecond),
	)

	// Single field shorthand
	txLogger := logger.WithField("transaction_id", "tx-789")
	txLogger.Info("Transaction started:")

	// Original logger is unchanged
	logger.Info("This has no preset fields")

	fmt.Println()
}

// Section 3: Best practices
func section3BestPractices() {
	fmt.Println("3. Best Practices")
	fmt.Println("------------------")

	cfg := dd.DefaultConfig()
	cfg.Format = dd.FormatJSON
	cfg.Targets = []dd.OutputTarget{dd.FileOutput("logs/structured.log")}

	logger, _ := dd.New(cfg)
	defer logger.Close()

	// ✅ DO: Use consistent field names
	logger.InfoWith("HTTP request",
		dd.String("http_method", "POST"),
		dd.String("http_path", "/api/users"),
		dd.Int("http_status", 201),
		dd.Float64("http_duration_ms", 125.7),
	)

	// ✅ DO: Include error context
	logger.ErrorWith("Operation failed",
		dd.Err(errors.New("database timeout")),
		dd.String("operation", "db_query"),
		dd.String("host", "db.example.com"),
		dd.Int("retry_count", 3),
	)

	// ✅ DO: Keep 5-10 fields per entry
	logger.InfoWith("User action",
		dd.String("user_id", "user-123"),
		dd.String("action", "login"),
		dd.String("ip_address", "192.168.1.100"),
		dd.Time("timestamp", time.Now()),
	)

	fmt.Println("\nBest Practices:")
	fmt.Println("  • Use type-safe fields (String, Int) over Any")
	fmt.Println("  • Use consistent field naming (snake_case recommended)")
	fmt.Println("  • Keep 5-10 fields per entry for readability")
	fmt.Println("  • Use WithFields for request/transaction context")
}
