package main

import (
	"fmt"
	"os"
	"time"

	"github.com/cybergodev/dd"
)

// Configuration - Complete Config API Guide
//
// Topics covered:
// 1. DefaultConfig, validation, and direct modification
// 2. Preset configurations (Development, JSON, Production)
// 3. Output targets (console, file, dual output)
// 4. File output with rotation
// 5. JSON customization
// 6. Clone for multiple loggers
// 7. Configure package-level functions with InitDefault
//
// NOTE: constructor errors are ignored (logger, _) for brevity in these
// examples; see 07_convenience for error-handling patterns.
func main() {
	fmt.Println("=== DD Configuration ===")

	section1BasicConfig()
	section2Presets()
	section3OutputTargets()
	section4FileRotation()
	section5JSONCustomization()
	section6Clone()
	section7InitDefault()

	fmt.Println("\n✅ Configuration examples completed!")
	fmt.Println("\nCheck logs/ directory for output files")
}

// Section 1: Basic configuration
func section1BasicConfig() {
	fmt.Println("1. Basic Configuration")
	fmt.Println("----------------------")

	// DefaultConfig with IDE autocomplete support
	cfg := dd.DefaultConfig()
	cfg.Level = dd.LevelDebug
	cfg.Format = dd.FormatJSON
	cfg.DynamicCaller = true        // Show caller file:line
	cfg.TimeFormat = "15:04:05.000" // Custom timestamp layout (time.Time format)
	// cfg.IncludeTime = false  // omit timestamps entirely
	// cfg.IncludeLevel = false // omit the level tag

	// Validate before New() to catch config errors early
	// (New() validates too, but this separates config bugs from I/O failures)
	if err := cfg.Validate(); err != nil {
		fmt.Printf("Invalid config: %v\n", err)
		return
	}

	logger, _ := dd.New(cfg)
	defer logger.Close()

	logger.Debug("Direct field modification with autocomplete")

	// Simple: dd.New() uses defaults
	simpleLogger, _ := dd.New()
	defer simpleLogger.Close()
	simpleLogger.Info("No config needed - uses defaults")

	fmt.Println()
}

// Section 2: Preset configurations
func section2Presets() {
	fmt.Println("2. Preset Configurations")
	fmt.Println("-------------------------")

	// Development: Debug level, text format, caller info
	devLogger, _ := dd.New(dd.DevelopmentConfig())
	defer devLogger.Close()
	devLogger.Debug("Development mode - verbose output")

	// JSON: Debug level, JSON format, structured for production
	jsonLogger, _ := dd.New(dd.JSONConfig())
	defer jsonLogger.Close()
	jsonLogger.Info("JSON format ready for log aggregation")

	// Production: Info level, JSON format, RFC3339 timestamps
	prodLogger, _ := dd.New(dd.ProductionConfig())
	defer prodLogger.Close()
	prodLogger.Info("Production preset - Info level JSON output")

	fmt.Println()
}

// Section 3: Output targets (console, file, dual output)
func section3OutputTargets() {
	fmt.Println("3. Output Targets")
	fmt.Println("------------------")

	// Console only (stdout)
	consoleCfg := dd.DefaultConfig()
	consoleCfg.Targets = []dd.OutputTarget{dd.ConsoleOutput()}
	consoleLogger, _ := dd.New(consoleCfg)
	defer consoleLogger.Close()
	consoleLogger.Info("Console only - no file")

	// Dual output: console AND file in one logger
	// DefaultLogPath is the conventional "logs/app.log" convenience constant
	dualCfg := dd.DefaultConfig()
	dualCfg.Targets = []dd.OutputTarget{
		dd.ConsoleOutput(),
		dd.FileOutput(dd.DefaultLogPath),
	}
	dualLogger, _ := dd.New(dualCfg)
	defer dualLogger.Close()
	dualLogger.Info("Appears in BOTH console and file")

	fmt.Println()
}

// Section 4: File output with rotation
func section4FileRotation() {
	fmt.Println("4. File Rotation")
	fmt.Println("-----------------")

	cfg := dd.DefaultConfig()
	cfg.Format = dd.FormatJSON
	fileTarget := dd.FileOutput("logs/app.log")
	fileTarget.MaxSizeMB = 100              // Rotate at 100MB
	fileTarget.MaxBackups = 10              // Keep 10 old files
	fileTarget.MaxAge = 30 * 24 * time.Hour // Delete after 30 days
	fileTarget.Compress = true              // Gzip old files
	cfg.Targets = []dd.OutputTarget{fileTarget}

	logger, _ := dd.New(cfg)
	defer logger.Close()

	logger.InfoWith("File rotation configured",
		dd.Int("max_size_mb", 100),
		dd.Int("max_backups", 10),
		dd.Bool("compress", true),
	)

	fmt.Println("✓ Logs written to logs/app.log")
	fmt.Println("  (see 05_writers for rotation demonstrated live)")
}

// Section 5: JSON customization
func section5JSONCustomization() {
	fmt.Println("5. JSON Customization")
	fmt.Println("----------------------")

	cfg := dd.JSONConfig()

	// Customize JSON field names (for ELK, CloudWatch, etc.)
	cfg.JSON.FieldNames = &dd.JSONFieldNames{
		Timestamp: "@timestamp", // ELK standard
		Level:     "severity",
		Message:   "msg",
		Caller:    "source",
	}

	// Pretty print for development
	cfg.JSON.PrettyPrint = true
	cfg.JSON.Indent = "  "

	logger, _ := dd.New(cfg)
	defer logger.Close()

	logger.InfoWith("Custom JSON format",
		dd.String("service", "user-api"),
		dd.Int("version", 1),
	)

	fmt.Println()
}

// Section 6: Clone for multiple loggers
func section6Clone() {
	fmt.Println("6. Clone for Multiple Loggers")
	fmt.Println("-------------------------------")

	// Base configuration
	baseCfg := dd.DefaultConfig()
	baseCfg.Format = dd.FormatJSON
	baseCfg.DynamicCaller = true

	// Clone for application logs
	appCfg := baseCfg.Clone()
	appCfg.Level = dd.LevelInfo
	appCfg.Targets = []dd.OutputTarget{dd.FileOutput("logs/app.log")}
	appLogger, _ := dd.New(appCfg)
	defer appLogger.Close()

	// Clone for audit logs (larger size)
	auditCfg := baseCfg.Clone()
	auditCfg.Level = dd.LevelInfo
	auditTarget := dd.FileOutput("logs/audit.log")
	auditTarget.MaxSizeMB = 500
	auditTarget.MaxBackups = 50
	auditCfg.Targets = []dd.OutputTarget{auditTarget}
	auditLogger, _ := dd.New(auditCfg)
	defer auditLogger.Close()

	// Clone for error logs (errors only)
	errCfg := baseCfg.Clone()
	errCfg.Level = dd.LevelError
	errCfg.Targets = []dd.OutputTarget{dd.FileOutput("logs/errors.log")}
	errLogger, _ := dd.New(errCfg)
	defer errLogger.Close()

	appLogger.InfoWith("Application started",
		dd.Int("pid", os.Getpid()),
	)
	auditLogger.InfoWith("Audit entry",
		dd.String("action", "user_login"),
		dd.String("user_id", "123"),
	)
	errLogger.ErrorWith("Error logged",
		dd.Err(fmt.Errorf("example error")),
	)

	fmt.Println("✓ Multiple loggers from cloned config")
}

// Section 7: Configure package-level functions with InitDefault
func section7InitDefault() {
	fmt.Println("7. InitDefault - Configure Package-Level Functions")
	fmt.Println("----------------------------------------------------")

	// Package-level functions (dd.Debug, dd.Info, etc.) use a default logger.
	// InitDefault() configures this default logger.

	// Example: Disable caller info for cleaner output
	cfg := dd.DefaultConfig()
	cfg.Level = dd.LevelDebug
	cfg.DynamicCaller = false // Disable file:line output

	if err := dd.InitDefault(cfg); err != nil {
		fmt.Printf("Failed to init default logger: %v\n", err)
		return
	}

	dd.Debug("No caller info - DynamicCaller=false")
	dd.Info("Package-level function with custom config")

	// Re-enable caller info (discard is safe: this config already passed
	// validation on the first InitDefault call above)
	cfg.DynamicCaller = true
	_ = dd.InitDefault(cfg)

	dd.Info("With caller info - DynamicCaller=true")

	fmt.Println("✓ Package-level functions configured via InitDefault")
	fmt.Println()
}
