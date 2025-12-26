package main

import (
	"context"
	"fmt"
	"time"

	"github.com/cybergodev/dd"
)

// Integration Patterns - Production-Ready Examples
//
// This example demonstrates practical integration patterns:
// 1. HTTP Middleware - Request/response logging
// 2. Database Operations - Query logging with timing
// 3. Context Propagation - Request ID tracking
// 4. Background Jobs - Async task logging
//
// All examples are production-ready and commonly used.
func main() {
	fmt.Println("DD Logger - Integration Patterns")
	fmt.Println("=================================")
	fmt.Println()

	example1HTTPMiddleware()
	example2DatabaseLogging()
	example3ContextPropagation()
	example4BackgroundJobs()

	fmt.Println("✅ All examples completed!")
}

// Example 1: HTTP Middleware
//
// Use Case: Log all HTTP requests with timing and status
// Perfect for: Web APIs, REST services, HTTP servers
func example1HTTPMiddleware() {
	fmt.Println("Example 1: HTTP Middleware")
	fmt.Println("--------------------------")

	logger, err := dd.NewWithOptions(dd.Options{
		Format:  dd.FormatJSON,
		Console: true,
		File:    "logs/http-access.log",
	})
	if err != nil {
		fmt.Printf("Failed to create logger: %v\n", err)
		return
	}
	defer logger.Close()

	// Simulate HTTP requests
	requests := []struct {
		method string
		path   string
		userID string
		status int
	}{
		{"GET", "/api/users", "user-123", 200},
		{"POST", "/api/orders", "user-456", 201},
		{"GET", "/api/products/999", "user-789", 404},
		{"PUT", "/api/users/123", "user-123", 200},
	}

	for _, req := range requests {
		start := time.Now()

		// Simulate request processing
		time.Sleep(10 * time.Millisecond)

		duration := time.Since(start)

		// Log request
		logger.InfoWith("HTTP request",
			dd.String("method", req.method),
			dd.String("path", req.path),
			dd.Int("status", req.status),
			dd.Float64("duration_ms", float64(duration.Microseconds())/1000),
			dd.String("user_id", req.userID),
		)

		// Log errors separately
		if req.status >= 400 {
			logger.WarnWith("HTTP error",
				dd.String("method", req.method),
				dd.String("path", req.path),
				dd.Int("status", req.status),
				dd.String("user_id", req.userID),
			)
		}
	}

	fmt.Println("✅ HTTP requests logged with timing and status")
	fmt.Println()
}

// Example 2: Database Operations
//
// Use Case: Log database queries with execution time
// Perfect for: ORM integration, query debugging, performance monitoring
func example2DatabaseLogging() {
	fmt.Println("Example 2: Database Operations")
	fmt.Println("-------------------------------")

	logger, err := dd.NewWithOptions(dd.Options{
		Format:  dd.FormatJSON,
		Console: true,
		File:    "logs/database.log",
	})
	if err != nil {
		fmt.Printf("Failed to create logger: %v\n", err)
		return
	}
	defer logger.Close()

	// Database wrapper with logging
	db := &DatabaseLogger{logger: logger}

	// Simulate database operations
	db.Query("SELECT * FROM users WHERE active = ?", true)
	db.Insert("users", map[string]any{
		"name":  "Alice",
		"email": "alice@example.com",
	})
	db.Update("users", "id = ?", map[string]any{
		"last_login": time.Now(),
	}, 123)
	db.Delete("sessions", "expired_at < ?", time.Now())

	fmt.Println("✅ Database operations logged with query details")
	fmt.Println()
}

// Example 3: Context Propagation
//
// Use Case: Track requests across function calls
// Perfect for: Distributed tracing, request correlation, debugging
func example3ContextPropagation() {
	fmt.Println("Example 3: Context Propagation")
	fmt.Println("-------------------------------")

	logger, err := dd.NewWithOptions(dd.Options{
		Format:  dd.FormatJSON,
		Console: true,
		File:    "logs/context.log",
	})
	if err != nil {
		fmt.Printf("Failed to create logger: %v\n", err)
		return
	}
	defer logger.Close()

	// Create context with request metadata
	ctx := context.Background()
	ctx = context.WithValue(ctx, "request_id", "req-abc-123")
	ctx = context.WithValue(ctx, "user_id", "user-456")

	// Helper function to log with context
	logWithContext := func(ctx context.Context, msg string, fields ...dd.Field) {
		allFields := []dd.Field{
			dd.String("request_id", ctx.Value("request_id").(string)),
			dd.String("user_id", ctx.Value("user_id").(string)),
		}
		allFields = append(allFields, fields...)
		logger.InfoWith(msg, allFields...)
	}

	// Simulate request flow
	logWithContext(ctx, "Request received",
		dd.String("endpoint", "/api/checkout"),
	)

	logWithContext(ctx, "Validating user",
		dd.String("step", "validation"),
	)

	logWithContext(ctx, "Processing payment",
		dd.String("step", "payment"),
		dd.Float64("amount", 99.99),
	)

	logWithContext(ctx, "Creating order",
		dd.String("step", "order_creation"),
		dd.String("order_id", "order-789"),
	)

	logWithContext(ctx, "Request completed",
		dd.String("status", "success"),
	)

	fmt.Println("✅ Request tracked across multiple steps")
	fmt.Println()
}

// Example 4: Background Jobs
//
// Use Case: Log async task execution
// Perfect for: Job queues, scheduled tasks, workers
func example4BackgroundJobs() {
	fmt.Println("Example 4: Background Jobs")
	fmt.Println("--------------------------")

	logger, err := dd.NewWithOptions(dd.Options{
		Format:  dd.FormatJSON,
		Console: true,
		File:    "logs/jobs.log",
	})
	if err != nil {
		fmt.Printf("Failed to create logger: %v\n", err)
		return
	}
	defer logger.Close()

	// Job processor
	processor := &JobProcessor{logger: logger}

	// Simulate background jobs
	jobs := []Job{
		{ID: "job-001", Type: "send_email", UserID: "user-123"},
		{ID: "job-002", Type: "generate_report", UserID: "user-456"},
		{ID: "job-003", Type: "process_image", UserID: "user-789"},
	}

	for _, job := range jobs {
		processor.Process(job)
	}

	fmt.Println("✅ Background jobs logged with execution details")
	fmt.Println()
}

// ============================================================================
// Production Helper Implementations
// ============================================================================

// DatabaseLogger - Wrapper for database operations with logging
type DatabaseLogger struct {
	logger *dd.Logger
}

func (db *DatabaseLogger) Query(query string, args ...any) {
	start := time.Now()

	// Simulate query execution
	time.Sleep(15 * time.Millisecond)

	duration := time.Since(start)

	db.logger.InfoWith("Database query",
		dd.String("operation", "SELECT"),
		dd.String("query", query),
		dd.Any("args", args),
		dd.Float64("duration_ms", float64(duration.Microseconds())/1000),
	)
}

func (db *DatabaseLogger) Insert(table string, data map[string]any) {
	start := time.Now()
	time.Sleep(8 * time.Millisecond)
	duration := time.Since(start)

	db.logger.InfoWith("Database insert",
		dd.String("operation", "INSERT"),
		dd.String("table", table),
		dd.Any("data", data),
		dd.Float64("duration_ms", float64(duration.Microseconds())/1000),
	)
}

func (db *DatabaseLogger) Update(table, where string, data map[string]any, args ...any) {
	start := time.Now()
	time.Sleep(12 * time.Millisecond)
	duration := time.Since(start)

	db.logger.InfoWith("Database update",
		dd.String("operation", "UPDATE"),
		dd.String("table", table),
		dd.String("where", where),
		dd.Any("data", data),
		dd.Float64("duration_ms", float64(duration.Microseconds())/1000),
	)
}

func (db *DatabaseLogger) Delete(table, where string, args ...any) {
	start := time.Now()
	time.Sleep(6 * time.Millisecond)
	duration := time.Since(start)

	db.logger.InfoWith("Database delete",
		dd.String("operation", "DELETE"),
		dd.String("table", table),
		dd.String("where", where),
		dd.Float64("duration_ms", float64(duration.Microseconds())/1000),
	)
}

// JobProcessor - Background job processor with logging
type JobProcessor struct {
	logger *dd.Logger
}

type Job struct {
	ID     string
	Type   string
	UserID string
}

func (jp *JobProcessor) Process(job Job) {
	start := time.Now()

	jp.logger.InfoWith("Job started",
		dd.String("job_id", job.ID),
		dd.String("job_type", job.Type),
		dd.String("user_id", job.UserID),
	)

	// Simulate job processing
	time.Sleep(50 * time.Millisecond)

	duration := time.Since(start)

	jp.logger.InfoWith("Job completed",
		dd.String("job_id", job.ID),
		dd.String("job_type", job.Type),
		dd.String("status", "success"),
		dd.Float64("duration_ms", float64(duration.Microseconds())/1000),
	)
}

// ============================================================================
// Real-World Usage Examples
// ============================================================================

// Example: HTTP handler with logging
func ExampleHTTPHandler(logger *dd.Logger) func(method, path, userID string) {
	return func(method, path, userID string) {
		start := time.Now()

		// Your handler logic here
		// ...

		duration := time.Since(start)

		logger.InfoWith("HTTP request",
			dd.String("method", method),
			dd.String("path", path),
			dd.String("user_id", userID),
			dd.Float64("duration_ms", float64(duration.Microseconds())/1000),
		)
	}
}

// Example: Database query wrapper
func ExampleDatabaseQuery(logger *dd.Logger, query string, args ...any) error {
	start := time.Now()

	// Execute query
	// err := db.Exec(query, args...)

	duration := time.Since(start)

	logger.InfoWith("Database query",
		dd.String("query", query),
		dd.Float64("duration_ms", float64(duration.Microseconds())/1000),
	)

	return nil
}

// Example: Context-aware logging
func ExampleContextLogger(ctx context.Context, logger *dd.Logger, msg string) {
	logger.InfoWith(msg,
		dd.String("request_id", ctx.Value("request_id").(string)),
		dd.String("user_id", ctx.Value("user_id").(string)),
	)
}
