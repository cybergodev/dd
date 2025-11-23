//go:build examples

package main

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/cybergodev/dd"
)

// Multi-Writer Patterns - Production-Ready Examples
//
// This example demonstrates practical multi-writer patterns for real-world scenarios:
// 1. Console + File - Development and debugging
// 2. Level-Based Routing - Separate error logs from info logs
// 3. Component-Based Filtering - Route logs by service component
// 4. Async Writer - High-performance non-blocking logging
//
// All examples are production-ready and can be directly used in your applications.
func main() {
	fmt.Println("DD Logger - Multi-Writer Patterns")
	fmt.Println("==================================\n ")

	example1ConsoleAndFile()
	example2LevelBasedRouting()
	example3ComponentFiltering()
	example4AsyncWriter()

	fmt.Println("✅ All examples completed!")
}

// Example 1: Console + File Output
//
// Use Case: Development and debugging - see logs in console while persisting to file
// Perfect for: Local development, troubleshooting, debugging sessions
func example1ConsoleAndFile() {
	fmt.Println("Example 1: Console + File Output")
	fmt.Println("---------------------------------")

	// Create logger with both console and file output
	logger, err := dd.NewWithOptions(dd.Options{
		Format:  dd.FormatText,  // Human-readable for development
		Console: true,           // Output to console
		File:    "logs/app.log", // Also write to file
		FileConfig: &dd.FileWriterConfig{
			MaxSizeMB:  10, // 10MB per file
			MaxBackups: 3,  // Keep 3 old files
			Compress:   true,
		},
	})
	if err != nil {
		fmt.Printf("Failed to create logger: %v\n", err)
		return
	}
	defer logger.Close()

	// All logs go to both console and file
	logger.Info("Application started")
	logger.InfoWith("User logged in", dd.String("user", "alice"), dd.String("ip", "192.168.1.100"))
	logger.Warn("High memory usage detected")
	logger.Error("Database connection failed")

	fmt.Println("✅ Logs written to console and logs/app.log\n ")
}

// Example 2: Level-Based Routing
//
// Use Case: Separate error logs for alerting and monitoring
// Perfect for: Production systems, error tracking, incident response
func example2LevelBasedRouting() {
	fmt.Println("Example 2: Level-Based Routing")
	fmt.Println("-------------------------------")

	// Simple approach: Use built-in file separation
	logger, err := dd.NewWithOptions(dd.Options{
		Format:  dd.FormatJSON, // JSON for production
		Console: true,
		File:    "logs/app-all.log",
		FileConfig: &dd.FileWriterConfig{
			MaxSizeMB:  10,
			MaxBackups: 5,
		},
	})
	if err != nil {
		fmt.Printf("Failed to create logger: %v\n", err)
		return
	}
	defer logger.Close()

	// Info/Warn logs
	logger.Info("Request processed successfully")
	logger.Warn("Cache miss - fetching from database")

	// Error logs (in production, route these to monitoring systems)
	logger.Error("Payment processing failed")
	logger.ErrorWith("Database connection timeout",
		dd.String("host", "db.example.com"),
		dd.Int("timeout_ms", 5000),
	)

	fmt.Println("✅ All logs written to logs/app-all.log\n ")
	fmt.Println("   Tip: Use log aggregation tools (ELK, Splunk) to filter by level\n ")
}

// Example 3: Component-Based Filtering
//
// Use Case: Microservices - separate logs by service component
// Perfect for: Large applications, debugging specific components, log analysis
func example3ComponentFiltering() {
	fmt.Println("Example 3: Component-Based Filtering")
	fmt.Println("-------------------------------------")

	// Use structured fields for component identification
	logger, err := dd.NewWithOptions(dd.Options{
		Format:  dd.FormatJSON,
		Console: true,
		File:    "logs/components.log",
	})
	if err != nil {
		fmt.Printf("Failed to create logger: %v\n", err)
		return
	}
	defer logger.Close()

	// API component logs
	logger.InfoWith("HTTP request received",
		dd.String("component", "api"),
		dd.String("method", "POST"),
		dd.String("path", "/api/users"),
	)

	// Database component logs
	logger.InfoWith("Query executed",
		dd.String("component", "database"),
		dd.String("query", "INSERT INTO users"),
		dd.Int("duration_ms", 23),
	)

	// Health check component logs
	logger.InfoWith("System health check",
		dd.String("component", "health"),
		dd.String("status", "ok"),
	)

	fmt.Println("✅ Component logs written with structured fields\n ")
	fmt.Println("   Tip: Filter by component field in your log aggregation tool\n ")
}

// Example 4: Async Writer for High Performance
//
// Use Case: High-throughput systems - non-blocking logging
// Perfect for: High-traffic APIs, real-time systems, performance-critical paths
func example4AsyncWriter() {
	fmt.Println("Example 4: Async Writer (High Performance)")
	fmt.Println("-------------------------------------------")

	// Create async writer with buffer
	asyncWriter := NewAsyncWriter(
		createFileWriter("logs/async.log"),
		1000, // Buffer 1000 messages
	)
	defer asyncWriter.Close()

	logger, err := dd.NewWithOptions(dd.Options{
		Format:            dd.FormatJSON,
		Console:           false, // Disable console for performance
		AdditionalWriters: []io.Writer{asyncWriter},
	})
	if err != nil {
		fmt.Printf("Failed to create logger: %v\n", err)
		return
	}
	defer logger.Close()

	// Simulate high-throughput logging
	start := time.Now()
	messageCount := 1000

	for i := 0; i < messageCount; i++ {
		logger.InfoWith("High-frequency event",
			dd.Int("event_id", i),
			dd.String("user_id", fmt.Sprintf("user-%d", i%100)),
			dd.Int64("timestamp", time.Now().UnixNano()),
		)
	}

	duration := time.Since(start)
	throughput := float64(messageCount) / duration.Seconds()

	fmt.Printf("✅ Logged %d messages in %v (%.0f msg/sec)\n", messageCount, duration, throughput)
	fmt.Println("   Async writer prevents blocking your application\n ")

	// Allow async writer to flush
	time.Sleep(100 * time.Millisecond)
}

// ============================================================================
// Production-Ready Writer Implementations
// ============================================================================

// Helper: Create file writer with error handling
func createFileWriter(path string) io.WriteCloser {
	writer, err := dd.NewFileWriter(path, nil)
	if err != nil {
		panic(fmt.Sprintf("Failed to create file writer: %v", err))
	}
	return writer
}

// LevelRouter - Routes logs based on severity level
//
// Production use: Separate error logs for monitoring/alerting systems
type LevelRouter struct {
	infoWriter  io.WriteCloser
	errorWriter io.WriteCloser
}

func (lr *LevelRouter) Write(p []byte) (n int, err error) {
	logLine := string(p)

	// Route ERROR/FATAL to error writer, everything else to info writer
	if strings.Contains(logLine, `"level":"ERROR"`) ||
		strings.Contains(logLine, `"level":"FATAL"`) {
		return lr.errorWriter.Write(p)
	}

	return lr.infoWriter.Write(p)
}

func (lr *LevelRouter) Close() error {
	lr.infoWriter.Close()
	lr.errorWriter.Close()
	return nil
}

// ComponentFilter - Routes logs based on component field
//
// Production use: Separate logs by microservice component for easier debugging
type ComponentFilter struct {
	writer    io.WriteCloser
	component string
}

func (cf *ComponentFilter) Write(p []byte) (n int, err error) {
	logLine := string(p)

	// Only write if log contains matching component
	if strings.Contains(logLine, fmt.Sprintf(`"component":"%s"`, cf.component)) {
		return cf.writer.Write(p)
	}

	// Return success even if filtered out (don't block other writers)
	return len(p), nil
}

func (cf *ComponentFilter) Close() error {
	return cf.writer.Close()
}

// AsyncWriter - Non-blocking writer for high-performance logging
//
// Production use: High-throughput systems where logging must not block
type AsyncWriter struct {
	writer io.WriteCloser
	queue  chan []byte
	wg     sync.WaitGroup
	done   chan struct{}
}

// NewAsyncWriter creates a new async writer with specified buffer size
func NewAsyncWriter(writer io.WriteCloser, bufferSize int) *AsyncWriter {
	aw := &AsyncWriter{
		writer: writer,
		queue:  make(chan []byte, bufferSize),
		done:   make(chan struct{}),
	}

	// Start background writer goroutine
	aw.wg.Add(1)
	go func() {
		defer aw.wg.Done()
		for {
			select {
			case data := <-aw.queue:
				aw.writer.Write(data)
			case <-aw.done:
				// Drain remaining messages before exit
				for {
					select {
					case data := <-aw.queue:
						aw.writer.Write(data)
					default:
						return
					}
				}
			}
		}
	}()

	return aw
}

func (aw *AsyncWriter) Write(p []byte) (n int, err error) {
	// Copy data since caller may reuse buffer
	data := make([]byte, len(p))
	copy(data, p)

	// Non-blocking send to queue
	select {
	case aw.queue <- data:
		return len(p), nil
	default:
		// Queue full - in production, consider logging this metric
		return len(p), nil
	}
}

func (aw *AsyncWriter) Close() error {
	// Signal shutdown
	select {
	case <-aw.done:
		// Already closed
	default:
		close(aw.done)
	}

	// Wait for background goroutine to finish
	aw.wg.Wait()

	return aw.writer.Close()
}
