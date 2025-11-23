//go:build examples

package main

import (
	"fmt"
	"io"
	"runtime"
	"sync"
	"time"

	"github.com/cybergodev/dd"
)

func main() {
	fmt.Println("=== DD Performance Optimization ===\n ")

	basicPerformance()
	concurrentPerformance()
	formatComparison()
	filteringImpact()

	fmt.Println("\n✅ Examples completed")
	printOptimizationTips()
}

// 1. Basic Performance - Measure simple logging throughput
func basicPerformance() {
	fmt.Println("1. Basic Performance")

	config := dd.DefaultConfig()
	config.Writers = []io.Writer{io.Discard} // Avoid I/O overhead
	logger, _ := dd.New(config)
	defer logger.Close()

	iterations := 10000
	start := time.Now()

	for i := 0; i < iterations; i++ {
		logger.Info("Performance test message")
	}

	duration := time.Since(start)
	opsPerSec := float64(iterations) / duration.Seconds()

	fmt.Printf("  %d messages in %v\n", iterations, duration)
	fmt.Printf("  Throughput: %.0f ops/sec\n", opsPerSec)
}

// 2. Concurrent Performance - Test thread-safe logging
func concurrentPerformance() {
	fmt.Println("\n2. Concurrent Performance")

	config := dd.DefaultConfig()
	config.Writers = []io.Writer{io.Discard}
	logger, _ := dd.New(config)
	defer logger.Close()

	numGoroutines := runtime.NumCPU()
	messagesPerGoroutine := 5000
	totalMessages := numGoroutines * messagesPerGoroutine

	var wg sync.WaitGroup
	start := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < messagesPerGoroutine; j++ {
				logger.InfoWith("Concurrent message",
					dd.Int("goroutine", id),
					dd.Int("message", j),
				)
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)
	opsPerSec := float64(totalMessages) / duration.Seconds()

	fmt.Printf("  %d goroutines × %d messages = %d total\n", numGoroutines, messagesPerGoroutine, totalMessages)
	fmt.Printf("  Duration: %v\n", duration)
	fmt.Printf("  Throughput: %.0f ops/sec\n", opsPerSec)
}

// 3. Format Comparison - Text vs JSON performance
func formatComparison() {
	fmt.Println("\n3. Format Comparison")

	iterations := 5000

	// Text format
	textConfig := dd.DefaultConfig()
	textConfig.Format = dd.FormatText
	textConfig.Writers = []io.Writer{io.Discard}
	textLogger, _ := dd.New(textConfig)
	defer textLogger.Close()

	start := time.Now()
	for i := 0; i < iterations; i++ {
		textLogger.InfoWith("Test message",
			dd.Int("iteration", i),
			dd.String("data", "test"),
		)
	}
	textDuration := time.Since(start)

	// JSON format
	jsonConfig := dd.JSONConfig()
	jsonConfig.Writers = []io.Writer{io.Discard}
	jsonLogger, _ := dd.New(jsonConfig)
	defer jsonLogger.Close()

	start = time.Now()
	for i := 0; i < iterations; i++ {
		jsonLogger.InfoWith("Test message",
			dd.Int("iteration", i),
			dd.String("data", "test"),
		)
	}
	jsonDuration := time.Since(start)

	fmt.Printf("  Text: %v (%.0f ops/sec)\n",
		textDuration, float64(iterations)/textDuration.Seconds())
	fmt.Printf("  JSON: %v (%.0f ops/sec)\n",
		jsonDuration, float64(iterations)/jsonDuration.Seconds())

	if textDuration < jsonDuration {
		fmt.Printf("  Text is %.1fx faster\n", jsonDuration.Seconds()/textDuration.Seconds())
	} else {
		fmt.Printf("  JSON is %.1fx faster\n", textDuration.Seconds()/jsonDuration.Seconds())
	}
}

// 4. Filtering Impact - Cost of security filtering
func filteringImpact() {
	fmt.Println("\n4. Filtering Impact")

	iterations := 5000

	// No filtering
	noFilterConfig := dd.DefaultConfig()
	noFilterConfig.DisableFiltering()
	noFilterConfig.Writers = []io.Writer{io.Discard}
	noFilterLogger, _ := dd.New(noFilterConfig)
	defer noFilterLogger.Close()

	start := time.Now()
	for i := 0; i < iterations; i++ {
		noFilterLogger.Info("password=secret123 api_key=sk-abc")
	}
	noFilterDuration := time.Since(start)

	// Basic filtering
	basicFilterConfig := dd.DefaultConfig()
	basicFilterConfig.EnableBasicFiltering()
	basicFilterConfig.Writers = []io.Writer{io.Discard}
	basicFilterLogger, _ := dd.New(basicFilterConfig)
	defer basicFilterLogger.Close()

	start = time.Now()
	for i := 0; i < iterations; i++ {
		basicFilterLogger.Info("password=secret123 api_key=sk-abc")
	}
	basicFilterDuration := time.Since(start)

	fmt.Printf("  No filtering: %v (%.0f ops/sec)\n",
		noFilterDuration, float64(iterations)/noFilterDuration.Seconds())
	fmt.Printf("  Basic filtering: %v (%.0f ops/sec)\n",
		basicFilterDuration, float64(iterations)/basicFilterDuration.Seconds())
	fmt.Printf("  Performance impact: %.1f%%\n",
		(basicFilterDuration.Seconds()-noFilterDuration.Seconds())/noFilterDuration.Seconds()*100)
}

func printOptimizationTips() {
	fmt.Println("\nOptimization Tips:")
	fmt.Println("  • Use text format for better performance")
	fmt.Println("  • Disable caller info in production (IncludeCaller: false)")
	fmt.Println("  • Enable filtering only when needed")
	fmt.Println("  • Use type-safe fields (dd.String, dd.Int) instead of dd.Any")
	fmt.Println("  • Logger is thread-safe, share one instance across goroutines")
}
