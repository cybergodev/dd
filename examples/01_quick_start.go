//go:build examples

package main

import (
	"fmt"

	"github.com/cybergodev/dd"
)

// Quick Start - Master dd logger in 5 minutes
//
// Topics covered:
// 1. Package-level functions (zero setup)
// 2. Creating loggers with New()
// 3. Log levels and dynamic control
// 4. Formatted and structured logging
// 5. Global logger management
func main() {
	fmt.Println("=== DD Logger Quick Start ===")

	section1PackageLevel()
	section2CreateLogger()
	section3LogLevels()
	section4GlobalLogger()

	fmt.Println("\n✅ Quick start completed!")
}

// Section 1: Package-level functions (zero setup required)
func section1PackageLevel() {
	fmt.Println("1. Package-Level Functions")
	fmt.Println("---------------------------")

	// Use directly without any setup - outputs to stdout
	dd.Debug("Debug: detailed diagnostic info (may not show - default is INFO)")
	dd.Info("Info: general operational messages")
	dd.Warn("Warn: warning conditions")
	dd.Error("Error: error conditions")
	// dd.Fatal("Fatal: severe errors, exits program") // Uncomment to test

	// Structured logging with package-level functions
	dd.InfoWith("Request processed",
		dd.String("method", "GET"),
		dd.Int("status", 200),
	)

	fmt.Println()
}

// Section 2: Creating loggers with New()
func section2CreateLogger() {
	fmt.Println("2. Creating Loggers")
	fmt.Println("--------------------")

	// Simple logger with defaults
	logger, _ := dd.New()
	defer logger.Close()
	logger.Info("Default logger - console output, INFO level")

	// Logger with custom config
	cfg := dd.DefaultConfig()
	cfg.Level = dd.LevelDebug
	cfg.Format = dd.FormatJSON
	cfg.DynamicCaller = true

	logger2, _ := dd.New(cfg)
	defer logger2.Close()
	logger2.Debug("JSON format with caller info")

	// Preset configurations
	devLogger, _ := dd.New(dd.DevelopmentConfig())
	defer devLogger.Close()
	devLogger.Debug("Development preset - debug level enabled")

	fmt.Println()
}

// Section 3: Log levels and dynamic control
func section3LogLevels() {
	fmt.Println("3. Log Levels")
	fmt.Println("--------------")

	logger, _ := dd.New()
	defer logger.Close()

	// Level hierarchy: Debug < Info < Warn < Error < Fatal
	// Default is INFO, so Debug won't show
	logger.Debug("This won't show (below INFO)")
	logger.Info("This will show")

	// Dynamic level change
	logger.SetLevel(dd.LevelDebug)
	logger.Debug("Now visible! Level changed to DEBUG")

	// Check current level
	fmt.Printf("Current level: %s\n", logger.GetLevel().String())

	fmt.Println()
}

// Section 4: Global logger management
func section4GlobalLogger() {
	fmt.Println("4. Global Logger")
	fmt.Println("----------------")

	// Create custom logger and set as global default
	cfg := dd.DefaultConfig()
	cfg.Level = dd.LevelDebug
	customLogger, _ := dd.New(cfg)
	defer customLogger.Close()

	// SetDefault affects ALL package-level functions
	dd.SetDefault(customLogger)

	// Now dd.Info() etc. use the custom logger
	dd.Debug("Global logger now at DEBUG level")

	// Package-level GetLevel/SetLevel work on global logger
	fmt.Printf("Global level: %s\n", dd.GetLevel().String())
	dd.SetLevel(dd.LevelInfo)
	fmt.Printf("After SetLevel(INFO): %s\n", dd.GetLevel().String())

	fmt.Println("\n💡 Tip: Call SetDefault() in init() or start of main()")
}
