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
// 4. Custom config with recorder
func main() {
	fmt.Println("=== DD Testing with LoggerRecorder ===")

	section1BasicCapture()
	section2LevelFiltering()
	section3FieldInspection()

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
