package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/cybergodev/dd"
)

// Error Handling - Production-Ready Examples
//
// This example demonstrates practical error handling patterns:
// 1. Basic Error Logging - Log errors with context
// 2. Structured Error Fields - Rich error information
// 3. Panic Recovery - Gracefully handle panics
// 4. Graceful Shutdown - Proper cleanup
//
// All examples follow production best practices.
func main() {
	fmt.Println("DD Logger - Error Handling")
	fmt.Println("==========================")
	fmt.Println()

	example1BasicErrorLogging()
	example2StructuredErrors()
	example3PanicRecovery()
	example4GracefulShutdown()

	fmt.Println("✅ All examples completed!")
}

// Example 1: Basic Error Logging
//
// Use Case: Log errors with context for debugging
// Perfect for: Any application that needs error tracking
func example1BasicErrorLogging() {
	fmt.Println("Example 1: Basic Error Logging")
	fmt.Println("-------------------------------")

	logger, err := dd.NewWithOptions(dd.Options{
		Format:  dd.FormatJSON,
		Console: true,
		File:    "logs/errors.log",
	})
	if err != nil {
		fmt.Printf("Failed to create logger: %v\n", err)
		return
	}
	defer logger.Close()

	// Simple error logging
	err = errors.New("database connection failed")
	logger.ErrorWith("Database error",
		dd.Err(err),
		dd.String("component", "database"),
		dd.Int("retry_count", 3),
	)

	// Error with operation context
	if err := performDatabaseOperation(); err != nil {
		logger.ErrorWith("Operation failed",
			dd.Err(err),
			dd.String("operation", "user_query"),
			dd.String("host", "db.example.com"),
			dd.Int("port", 5432),
		)
	}

	// Wrapped errors (preserves error chain)
	originalErr := errors.New("connection timeout")
	wrappedErr := fmt.Errorf("database query failed: %w", originalErr)
	logger.ErrorWith("Query error",
		dd.Err(wrappedErr),
		dd.String("query", "SELECT * FROM users"),
	)

	fmt.Println("✅ Errors logged with context")
	fmt.Println()
}

// Example 2: Structured Error Fields
//
// Use Case: Log domain-specific errors with rich context
// Perfect for: APIs, microservices, business applications
func example2StructuredErrors() {
	fmt.Println("Example 2: Structured Error Fields")
	fmt.Println("-----------------------------------")

	logger, err := dd.NewWithOptions(dd.Options{
		Format:  dd.FormatJSON,
		Console: true,
		File:    "logs/structured-errors.log",
	})
	if err != nil {
		fmt.Printf("Failed to create logger: %v\n", err)
		return
	}
	defer logger.Close()

	// HTTP/API errors
	logger.ErrorWith("HTTP request failed",
		dd.String("error_type", "http_error"),
		dd.Int("status_code", 500),
		dd.String("method", "POST"),
		dd.String("path", "/api/users"),
		dd.String("request_id", "req-12345"),
		dd.String("user_id", "user-789"),
	)

	// Database errors
	logger.ErrorWith("Database query failed",
		dd.String("error_type", "database_error"),
		dd.String("query", "INSERT INTO orders"),
		dd.String("error_code", "23505"),
		dd.String("constraint", "unique_order_id"),
	)

	// Business logic errors
	logger.ErrorWith("Business rule violation",
		dd.String("error_type", "business_error"),
		dd.String("rule", "insufficient_funds"),
		dd.String("user_id", "user-456"),
		dd.Float64("requested", 100.50),
		dd.Float64("available", 25.00),
	)

	// External service errors
	logger.ErrorWith("External service timeout",
		dd.String("error_type", "service_error"),
		dd.String("service", "payment-gateway"),
		dd.String("endpoint", "https://api.payment.com/charge"),
		dd.Int("timeout_ms", 5000),
		dd.Int("retry_attempt", 3),
	)

	fmt.Println("✅ Structured errors logged for easy filtering")
	fmt.Println()
}

// Example 3: Panic Recovery
//
// Use Case: Gracefully handle panics and log them
// Perfect for: HTTP handlers, goroutines, critical sections
func example3PanicRecovery() {
	fmt.Println("Example 3: Panic Recovery")
	fmt.Println("-------------------------")

	logger, err := dd.NewWithOptions(dd.Options{
		Format:  dd.FormatJSON,
		Console: true,
		File:    "logs/panics.log",
	})
	if err != nil {
		fmt.Printf("Failed to create logger: %v\n", err)
		return
	}
	defer logger.Close()

	// Pattern 1: Recover from panic in function
	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.ErrorWith("Panic recovered",
					dd.String("panic_value", fmt.Sprintf("%v", r)),
					dd.String("function", "processRequest"),
					dd.String("component", "api"),
				)
			}
		}()

		// Simulate panic
		panic("nil pointer dereference")
	}()

	// Pattern 2: Recover from panic in goroutine
	done := make(chan bool)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.ErrorWith("Goroutine panic recovered",
					dd.String("panic_value", fmt.Sprintf("%v", r)),
					dd.String("goroutine", "background_worker"),
				)
			}
			done <- true
		}()

		// Simulate panic in goroutine
		panic("worker error")
	}()
	<-done

	// Pattern 3: HTTP handler panic recovery (common pattern)
	handleRequest := func(requestID string) {
		defer func() {
			if r := recover(); r != nil {
				logger.ErrorWith("HTTP handler panic",
					dd.String("panic_value", fmt.Sprintf("%v", r)),
					dd.String("request_id", requestID),
					dd.String("handler", "/api/users"),
				)
			}
		}()

		// Simulate handler panic
		var data []string
		_ = data[10] // index out of range
	}

	handleRequest("req-999")

	fmt.Println("✅ Panics recovered and logged")
	fmt.Println()
}

// Example 4: Graceful Shutdown
//
// Use Case: Ensure all logs are written before exit
// Perfect for: Application shutdown, signal handling
func example4GracefulShutdown() {
	fmt.Println("Example 4: Graceful Shutdown")
	fmt.Println("----------------------------")

	logger, err := dd.NewWithOptions(dd.Options{
		Format:  dd.FormatJSON,
		Console: true,
		File:    "logs/shutdown.log",
	})
	if err != nil {
		fmt.Printf("Failed to create logger: %v\n", err)
		return
	}

	// Log application lifecycle
	logger.InfoWith("Application started",
		dd.Int("pid", os.Getpid()),
		dd.String("version", "1.0.0"),
	)

	// Simulate some work
	logger.Info("Processing requests...")
	time.Sleep(100 * time.Millisecond)

	// Graceful shutdown
	logger.InfoWith("Shutdown initiated",
		dd.String("reason", "SIGTERM received"),
	)

	// Close logger to flush all pending logs
	if err := logger.Close(); err != nil {
		fmt.Printf("Error during shutdown: %v\n", err)
	} else {
		fmt.Println("✅ Logger closed gracefully - all logs flushed")
		fmt.Println()
	}

	// Logs after Close() are safely ignored (no panic)
	logger.Info("This is safely ignored after close")
}

// ============================================================================
// Production Helper Functions
// ============================================================================

// performDatabaseOperation simulates a database operation that fails
func performDatabaseOperation() error {
	return errors.New("connection refused: database unavailable")
}

// Example: Real-world HTTP handler with error logging
func exampleHTTPHandler(logger *dd.Logger) func(requestID string) {
	return func(requestID string) {
		defer func() {
			if r := recover(); r != nil {
				logger.ErrorWith("Handler panic",
					dd.String("panic", fmt.Sprintf("%v", r)),
					dd.String("request_id", requestID),
				)
			}
		}()

		// Your handler logic here
		// If panic occurs, it will be logged
	}
}

// Example: Goroutine with error logging
func exampleBackgroundWorker(logger *dd.Logger, workerID int) {
	defer func() {
		if r := recover(); r != nil {
			logger.ErrorWith("Worker panic",
				dd.String("panic", fmt.Sprintf("%v", r)),
				dd.Int("worker_id", workerID),
			)
		}
	}()

	// Your worker logic here
}
