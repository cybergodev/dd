//go:build examples

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
// 1. DefaultConfig and direct modification
// 2. Preset configurations (Development, JSON)
// 3. File output with rotation
// 4. JSON customization
// 5. Clone for multiple loggers
// 6. Configure package-level functions with InitDefault
func main() {
	fmt.Println("=== DD Configuration ===")

	section1BasicConfig()
	section2Presets()
	section3FileRotation()
	section4JSONCustomization()
	section5Clone()
	section6InitDefault()

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
	cfg.DynamicCaller = true // Show caller file:line

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

	fmt.Println()
}

// Section 3: File output with rotation
func section3FileRotation() {
	fmt.Println("3. File Rotation")
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
}

// Section 4: JSON customization
func section4JSONCustomization() {
	fmt.Println("4. JSON Customization")
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

// Section 5: Clone for multiple loggers
func section5Clone() {
	fmt.Println("5. Clone for Multiple Loggers")
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

// Section 6: Configure package-level functions with InitDefault
func section6InitDefault() {
	fmt.Println("6. InitDefault - Configure Package-Level Functions")
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

	// Re-enable caller info
	cfg.DynamicCaller = true
	dd.InitDefault(cfg)

	dd.Info("With caller info - DynamicCaller=true")

	fmt.Println("✓ Package-level functions configured via InitDefault")
	fmt.Println()
}
