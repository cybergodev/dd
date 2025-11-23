//go:build examples

package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/cybergodev/dd"
)

// Structured Logging Examples - Master in 5 minutes, production-ready
func main() {
	fmt.Println("DD Structured Logging - Quick Start")
	fmt.Println("===================================\n ")

	quickStart()
	productionReady()
	completeReference()

	fmt.Println("\n✅ All examples completed! Check logs/ directory for output")
}

// Quick Start - Master core usage in 3 minutes
func quickStart() {

	logger := dd.ToJSONFile("logs/quick-start.json.log")
	defer logger.Close()

	// 1. Log business operations (most common - 80% of use cases)
	fmt.Println("1️⃣ Log user action:")
	logger.InfoWith("User login successful",
		dd.Int("user_id", 12345),
		dd.String("username", "john_doe"),
		dd.String("ip", "192.168.1.100"),
	)

	// 2. Log errors (must know)
	fmt.Println("\n2️⃣ Log error:")
	err := errors.New("database connection failed")
	logger.ErrorWith("Operation failed",
		dd.Err(err),
		dd.String("operation", "user_query"),
		dd.Int("retry_count", 3),
	)

	// 3. Log performance metrics
	fmt.Println("\n3️⃣ Log performance:")
	logger.InfoWith("API request completed",
		dd.String("method", "POST"),
		dd.String("path", "/api/users"),
		dd.Int("status", 200),
		dd.Float64("duration_ms", 45.2),
	)

	// 4. Common field types quick reference
	fmt.Println("\n4️⃣ Field types quick reference:")
	logger.InfoWith("Field type examples",
		dd.String("name", "John Doe"),   // String
		dd.Int("age", 30),               // Integer
		dd.Float64("score", 98.5),       // Float
		dd.Bool("active", true),         // Boolean
		dd.Err(nil),                     // Error (can be nil)
		dd.Any("tags", []string{"vip"}), // Complex types (arrays, maps)
	)

	fmt.Println("\n✅ Core usage mastered! These 4 patterns cover 90% of daily needs\n ")
}

// Production Ready - Copy-paste templates to your project
func productionReady() {

	logger := dd.ToJSONFile("logs/production.json.log")
	defer logger.Close()

	fmt.Println("📌 Scenario 1: HTTP API request logging")
	logHTTPRequest(logger)

	fmt.Println("\n📌 Scenario 2: Database operation logging")
	logDatabaseOperation(logger)

	fmt.Println("\n📌 Scenario 3: Business event logging")
	logBusinessEvent(logger)

	fmt.Println("\n📌 Scenario 4: Error and alert logging")
	logErrorsAndAlerts(logger)

	fmt.Println("\n📌 Scenario 5: Microservice tracing")
	logMicroserviceTrace(logger)

	fmt.Println("\n✅ Production templates ready! Copy functions to your project\n ")
}

// HTTP API request logging template
func logHTTPRequest(logger *dd.Logger) {
	logger.InfoWith("HTTP request",
		dd.String("request_id", "req-abc-123"),
		dd.String("method", "POST"),
		dd.String("path", "/api/v1/users"),
		dd.String("client_ip", "192.168.1.100"),
		dd.Int("user_id", 12345),
	)

	logger.InfoWith("HTTP response",
		dd.String("request_id", "req-abc-123"),
		dd.Int("status", 201),
		dd.Float64("duration_ms", 125.7),
		dd.Int("response_size", 512),
	)
}

// Database operation logging template
func logDatabaseOperation(logger *dd.Logger) {
	logger.InfoWith("Database query",
		dd.String("operation", "SELECT"),
		dd.String("table", "users"),
		dd.Float64("duration_ms", 12.5),
		dd.Int("rows", 150),
		dd.Bool("cache_hit", true),
	)
}

// Business event logging template
func logBusinessEvent(logger *dd.Logger) {
	logger.InfoWith("Order created",
		dd.String("event", "order_created"),
		dd.String("order_id", "ORD-2024-001"),
		dd.String("user_id", "user-12345"),
		dd.Float64("amount", 1459.97),
		dd.String("currency", "USD"),
		dd.Int("item_count", 3),
	)
}

// Error and alert logging template
func logErrorsAndAlerts(logger *dd.Logger) {
	err := errors.New("connection timeout")
	logger.ErrorWith("Operation failed",
		dd.Err(err),
		dd.String("operation", "user_query"),
		dd.String("host", "db.example.com"),
		dd.Int("retry_count", 3),
	)

	logger.WarnWith("Resource alert",
		dd.String("alert_type", "high_memory"),
		dd.Float64("memory_percent", 85.5),
		dd.Float64("threshold", 80.0),
		dd.String("host", "app-server-01"),
	)
}

// Microservice tracing template
func logMicroserviceTrace(logger *dd.Logger) {
	logger.InfoWith("Service call",
		dd.String("trace_id", "trace-abc-123"),
		dd.String("span_id", "span-def-456"),
		dd.String("caller", "order-service"),
		dd.String("callee", "inventory-service"),
		dd.String("method", "CheckStock"),
		dd.Float64("duration_ms", 45.2),
		dd.Int("status", 200),
	)
}

// Complete Reference - All field types and advanced usage
func completeReference() {

	logger := dd.ToJSONFile("logs/reference.json.log")
	defer logger.Close()

	fmt.Println("📋 All field types:")
	allFieldTypes(logger)

	fmt.Println("\n📊 Log levels:")
	logLevels(logger)

	fmt.Println("\n🔧 Complex data:")
	complexData(logger)

	fmt.Println("\n⚡ Performance tips:")
	performanceTips()

	fmt.Println("\n✅ Complete reference shown! Refer when needed\n ")
}

// All field types example
func allFieldTypes(logger *dd.Logger) {
	logger.InfoWith("All field types",
		// Basic types (recommended, the best performance)
		dd.String("name", "John Doe"),
		dd.Int("age", 30),
		dd.Int64("id", 9876543210),
		dd.Float64("score", 98.5),
		dd.Bool("active", true),
		dd.Err(errors.New("example error")),

		// Complex types (use Any)
		dd.Any("tags", []string{"vip", "premium"}),
		dd.Any("metadata", map[string]int{"count": 100}),
		dd.Any("timestamp", time.Now()),
	)
}

// Log levels example
func logLevels(logger *dd.Logger) {
	logger.DebugWith("Debug info", dd.String("detail", "verbose debug data"))
	logger.InfoWith("Normal operation", dd.String("action", "user_login"))
	logger.WarnWith("Warning", dd.Float64("cpu_usage", 85.5))
	logger.ErrorWith("Error", dd.Err(errors.New("operation failed")))
}

// Complex data structures example
func complexData(logger *dd.Logger) {
	userProfile := map[string]any{
		"id":   12345,
		"name": "John Doe",
		"profile": map[string]any{
			"age":  30,
			"city": "New York",
			"tags": []string{"vip", "active"},
		},
	}

	logger.InfoWith("Complex data",
		dd.Any("user", userProfile),
		dd.Any("ids", []int{1, 2, 3, 4, 5}),
	)
}

// Performance optimization tips
func performanceTips() {
	fmt.Println("  1. Use String/Int/Bool over Any (20% faster)")
	fmt.Println("  2. Text format for dev, JSON for production")
	fmt.Println("  3. Keep 5-10 fields per log entry")
	fmt.Println("  4. Store repeated fields in variables")
}
