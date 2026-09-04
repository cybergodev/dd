//go:build examples

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/cybergodev/dd"
)

// Production Patterns - Real-World Usage
//
// Topics covered:
// 1. Error handling and panic recovery
// 2. Request tracing patterns
// 3. Graceful shutdown (Shutdown with context)
// 4. Concurrent logging
// 5. Performance optimization
// 6. Caller detection
func main() {
	fmt.Println("=== DD Production Patterns ===")

	section1ErrorHandling()
	section2RequestTracing()
	section3GracefulShutdown()
	section4ConcurrentLogging()
	section5Performance()
	section6CallerDetection()

	fmt.Println("\n✅ Production patterns completed!")
}

// Section 1: Error handling and panic recovery
func section1ErrorHandling() {
	fmt.Println("1. Error Handling & Panic Recovery")
	fmt.Println("-----------------------------------")

	cfg := dd.DefaultConfig()
	cfg.Format = dd.FormatJSON
	cfg.Targets = []dd.OutputTarget{dd.FileOutput("logs/errors.log")}

	logger, _ := dd.New(cfg)
	defer logger.Close()

	// Structured error logging
	logger.ErrorWith("Operation failed",
		dd.Err(errors.New("database connection failed")),
		dd.String("operation", "db_query"),
		dd.String("host", "db.example.com"),
		dd.Int("retry_count", 3),
	)

	// Error with stack trace (use sparingly - has overhead)
	logger.ErrorWith("With stack trace",
		dd.ErrWithStack(errors.New("critical error")),
	)

	// Panic recovery pattern
	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.ErrorWith("Panic recovered",
					dd.String("panic", fmt.Sprintf("%v", r)),
					dd.String("function", "processRequest"),
				)
			}
		}()
		panic("nil pointer dereference")
	}()

	fmt.Println("✓ Errors logged, panic recovered")
	fmt.Println()
}

// Section 2: Request tracing pattern
func section2RequestTracing() {
	fmt.Println("2. Request Tracing")
	fmt.Println("-------------------")

	cfg := dd.DefaultConfig()
	cfg.Format = dd.FormatJSON
	cfg.Targets = []dd.OutputTarget{dd.FileOutput("logs/requests.log")}

	logger, _ := dd.New(cfg)
	defer logger.Close()

	// Simulate request flow with tracing
	processRequest := func(ctx context.Context, path string) {
		start := time.Now()
		requestID := fmt.Sprintf("req-%d", time.Now().UnixNano())

		ctx = dd.WithRequestID(ctx, requestID)
		ctx = dd.WithTraceID(ctx, "trace-from-header")

		// Create request-scoped logger with context fields
		reqLogger := logger.WithFields(
			dd.String("trace_id", dd.GetTraceID(ctx)),
			dd.String("request_id", dd.GetRequestID(ctx)),
			dd.String("path", path),
		)

		// Log request start
		reqLogger.InfoWith("Request started",
			dd.String("method", "POST"),
		)

		// Simulate processing steps
		reqLogger.DebugWith("Validating input",
			dd.String("step", "validation"),
		)

		reqLogger.DebugWith("Processing business logic",
			dd.String("step", "processing"),
		)

		// Log request completion
		reqLogger.InfoWith("Request completed",
			dd.Int("status", 200),
			dd.Duration("duration", time.Since(start)),
		)
	}

	ctx := context.Background()
	processRequest(ctx, "/api/users")
	processRequest(ctx, "/api/orders")

	fmt.Println("✓ Request flow logged with trace IDs")
	fmt.Println()
}

// Section 3: Graceful shutdown
func section3GracefulShutdown() {
	fmt.Println("3. Graceful Shutdown")
	fmt.Println("---------------------")

	cfg := dd.DefaultConfig()
	cfg.Format = dd.FormatJSON
	cfg.Targets = []dd.OutputTarget{dd.FileOutput("logs/shutdown.log")}

	logger, _ := dd.New(cfg)

	logger.InfoWith("Application started",
		dd.Int("pid", os.Getpid()),
		dd.String("version", "1.0.0"),
	)

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	done := make(chan bool)

	// Background worker
	go func() {
		for i := 0; ; i++ {
			select {
			case <-done:
				logger.Info("Worker stopped")
				return
			default:
				logger.DebugWith("Working",
					dd.Int("iteration", i),
				)
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()

	// Auto-trigger shutdown for demo
	go func() {
		time.Sleep(500 * time.Millisecond)
		sigChan <- syscall.SIGTERM
	}()

	// Wait for signal
	sig := <-sigChan
	logger.InfoWith("Shutdown signal received",
		dd.String("signal", sig.String()),
	)

	// Cleanup
	close(done)
	time.Sleep(100 * time.Millisecond)

	logger.Info("Shutting down gracefully")

	// Recommended sequence: wait for in-flight security filter goroutines to
	// drain, then Shutdown flushes writers and closes with a timeout
	if !logger.WaitForFilterGoroutines(2 * time.Second) {
		fmt.Println("  Warning: filter goroutines did not drain in time")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := logger.Shutdown(shutdownCtx); err != nil {
		fmt.Printf("  Shutdown error: %v\n", err)
	}

	fmt.Println("✓ Graceful shutdown completed")
	fmt.Println()
}

// Section 4: Concurrent logging
func section4ConcurrentLogging() {
	fmt.Println("4. Concurrent Logging")
	fmt.Println("----------------------")

	cfg := dd.DefaultConfig()
	cfg.Targets = []dd.OutputTarget{dd.CustomOutput(io.Discard)} // Avoid I/O overhead for demo

	logger, _ := dd.New(cfg)
	defer logger.Close()

	numWorkers := runtime.NumCPU()
	msgsPerWorker := 1000
	total := numWorkers * msgsPerWorker

	var wg sync.WaitGroup
	start := time.Now()

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < msgsPerWorker; j++ {
				logger.InfoWith("Concurrent message",
					dd.Int("worker", id),
					dd.Int("msg", j),
				)
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)
	opsPerSec := float64(total) / duration.Seconds()

	fmt.Printf("  %d workers × %d messages = %d total\n", numWorkers, msgsPerWorker, total)
	fmt.Printf("  Duration: %v\n", duration)
	fmt.Printf("  Throughput: %.0f ops/sec\n\n", opsPerSec)
	fmt.Println()
}

// Section 5: Performance optimization
func section5Performance() {
	fmt.Println("5. Performance Optimization")
	fmt.Println("-----------------------------")

	// Tip 1: Disable security filtering for max performance
	fastCfg := dd.DefaultConfig()
	fastCfg.Targets = []dd.OutputTarget{dd.CustomOutput(io.Discard)}
	fastCfg.Security = dd.SecurityConfigForLevel(dd.SecurityLevelDevelopment)

	fastLogger, _ := dd.New(fastCfg)
	defer fastLogger.Close()

	// Tip 2: Use type-safe fields instead of Any
	start := time.Now()
	for i := 0; i < 10000; i++ {
		fastLogger.InfoWith("Fast logging",
			dd.String("key", "value"),
			dd.Int("count", i),
		)
	}
	duration := time.Since(start)
	fmt.Printf("  Type-safe fields: %v for 10k messages\n", duration)

	// Tip 3: Use IsLevelEnabled to skip expensive operations
	if fastLogger.IsDebugEnabled() {
		expensiveData := computeExpensiveDebugInfo()
		fastLogger.Debug(expensiveData)
	}

	// Tip 4: Use WithFields for repeated context
	reqLogger := fastLogger.WithFields(
		dd.String("request_id", "req-123"),
		dd.String("user_id", "user-456"),
	)
	for i := 0; i < 1000; i++ {
		reqLogger.Info("Processing") // Context included automatically
	}

	fmt.Println()
}

// Section 6: Caller detection configuration
//
// DynamicCaller reports the user's call site by skipping dd-internal frames;
// FullPath reports the absolute file path instead of the base name. The exact
// frame reported depends on the call stack and build-time inlining.
func section6CallerDetection() {
	fmt.Println("6. Caller Detection")
	fmt.Println("--------------------")

	// Default caller behavior
	logger1, _ := dd.New()
	defer logger1.Close()
	logger1.Info("Default caller configuration")

	// DynamicCaller: skip internal frames toward the user call site
	cfg := dd.DefaultConfig()
	cfg.DynamicCaller = true

	logger2, _ := dd.New(cfg)
	defer logger2.Close()
	logger2.Info("DynamicCaller enabled")

	// Through a wrapper function
	wrapperLog := func(msg string) {
		logger2.Info(msg)
	}
	wrapperLog("DynamicCaller through a wrapper")

	// FullPath: absolute path instead of base name
	cfg.FullPath = true
	logger3, _ := dd.New(cfg)
	defer logger3.Close()
	logger3.Info("FullPath enabled")

	fmt.Println()
}

func computeExpensiveDebugInfo() string {
	return "Expensive debug data computed only when DEBUG level is enabled"
}
