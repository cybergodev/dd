//go:build examples

package main

import (
	"fmt"

	"github.com/cybergodev/dd"
)

// Testing with LoggerRecorder - Capture and Assert Log Output
//
// Topics covered:
// 1. Basic capture and assertion
// 2. Filtering by level
// 3. Field inspection
// 4. Dependency injection via CoreLogger (swap in a recorder-backed logger)
func main() {
	fmt.Println("=== DD Testing with LoggerRecorder ===")

	section1BasicCapture()
	section2LevelFiltering()
	section3FieldInspection()
	section4DependencyInjection()

	fmt.Println("\n✅ Testing examples completed!")
}

// Section 1: Basic capture and assertion
func section1BasicCapture() {
	fmt.Println("1. Basic Capture & Assertion")
	fmt.Println("------------------------------")

	// Create recorder and logger
	recorder := dd.NewLoggerRecorder()
	logger, _ := recorder.NewLogger()
	defer logger.Close()

	// Generate log entries
	logger.Info("user logged in")
	logger.Warn("slow query detected")
	logger.Error("connection failed")

	// Assertions
	fmt.Printf("  Total entries: %d\n", recorder.Count())
	fmt.Printf("  Has entries: %v\n", recorder.HasEntries())

	// Check last entry
	last := recorder.LastEntry()
	if last != nil {
		fmt.Printf("  Last entry: [%s] %s\n", last.Level.String(), last.Message)
	}

	// Search messages
	fmt.Printf("  Contains 'logged in': %v\n", recorder.ContainsMessage("logged in"))
	fmt.Printf("  Contains 'timeout': %v\n", recorder.ContainsMessage("timeout"))

	// Clear for next test
	recorder.Clear()
	fmt.Printf("  After clear: %d entries\n", recorder.Count())

	fmt.Println()
}

// Section 2: Filtering by level
func section2LevelFiltering() {
	fmt.Println("2. Level Filtering")
	fmt.Println("--------------------")

	recorder := dd.NewLoggerRecorder()
	logger, _ := recorder.NewLogger()
	defer logger.Close()

	// Log at different levels
	logger.Debug("debug msg")
	logger.Info("info msg")
	logger.Warn("warn msg")
	logger.Error("error msg")

	// Filter by level
	errors := recorder.EntriesAtLevel(dd.LevelError)
	warns := recorder.EntriesAtLevel(dd.LevelWarn)

	fmt.Printf("  Error entries: %d\n", len(errors))
	fmt.Printf("  Warn entries: %d\n", len(warns))

	if len(errors) > 0 {
		fmt.Printf("  First error: %s\n", errors[0].Message)
	}

	fmt.Println()
}

// Section 3: Field inspection with JSON format
func section3FieldInspection() {
	fmt.Println("3. Field Inspection")
	fmt.Println("--------------------")

	// Use JSON format for reliable field parsing
	cfg := dd.JSONConfig()
	recorder := dd.NewLoggerRecorder()
	recorder.SetFormat(dd.FormatJSON) // must match the logger's format for parsing
	logger, _ := recorder.NewLogger(cfg)
	defer logger.Close()

	// Log structured data
	logger.InfoWith("HTTP request",
		dd.String("method", "GET"),
		dd.String("path", "/api/users"),
		dd.Int("status", 200),
	)

	// Inspect fields
	if recorder.ContainsField("method") {
		fmt.Println("  Found 'method' field")
	}

	if val := recorder.GetFieldValue("status"); val != nil {
		fmt.Printf("  Status field value: %v\n", val)
	}

	// Show raw output
	last := recorder.LastEntry()
	if last != nil {
		fmt.Printf("  Raw output: %s", last.RawOutput)
	}

	fmt.Println()
}

// orderService depends on the dd.CoreLogger interface rather than *dd.Logger,
// so production code and tests can supply different implementations.
type orderService struct {
	logger dd.CoreLogger
}

func newOrderService(logger dd.CoreLogger) *orderService {
	return &orderService{logger: logger}
}

func (s *orderService) Checkout(orderID string) {
	s.logger.InfoWith("checkout completed",
		dd.String("order_id", orderID),
		dd.Int("items", 3),
	)
}

// Section 4: Dependency injection — the service logs through the CoreLogger
// interface; the test substitutes a recorder-backed logger for the real one.
func section4DependencyInjection() {
	fmt.Println("4. Dependency Injection (CoreLogger)")
	fmt.Println("--------------------------------------")

	recorder := dd.NewLoggerRecorder()
	recorder.SetFormat(dd.FormatJSON) // parsing must match the logger's format
	logger, err := recorder.NewLogger(dd.JSONConfig())
	if err != nil {
		fmt.Printf("  Failed to create test logger: %v\n", err)
		return
	}
	defer logger.Close()

	// Run the service under test with the recorder in place of a real logger
	service := newOrderService(logger)
	service.Checkout("order-42")

	// Assert on the captured output — in a real test, fail with t.Errorf
	fmt.Printf("  Captured entries: %d\n", recorder.Count())
	fmt.Printf("  Has 'checkout completed': %v\n", recorder.ContainsMessage("checkout completed"))
	if val := recorder.GetFieldValue("order_id"); val != nil {
		fmt.Printf("  order_id field: %v\n", val)
	}
	fmt.Println("  ✓ Production swaps in dd.New() — service code is unchanged")
}
