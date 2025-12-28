//go:build examples

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/cybergodev/dd"
)

func main() {
	fmt.Println("=== DD Advanced Configuration ===\n ")

	presetConfigs()
	jsonCustomization()
	securityFiltering()
	fileRotation()
	productionSetup()

	fmt.Println("\n✅ Examples completed")
}

// 1. Preset Configurations - Three ready-to-use configs
func presetConfigs() {
	fmt.Println("1. Preset Configurations")

	// DefaultConfig: INFO level, text format
	logger1, _ := dd.New(dd.DefaultConfig())
	defer logger1.Close()
	logger1.Info("Default: production-ready text logging")

	// DevelopmentConfig: DEBUG level, caller info, colored output
	logger2, _ := dd.New(dd.DevelopmentConfig())
	defer logger2.Close()
	logger2.Debug("Development: verbose debugging with caller info")

	// JSONConfig: structured JSON output for log aggregation
	logger3, _ := dd.New(dd.JSONConfig())
	defer logger3.Close()
	logger3.InfoWith("JSON: structured logging", dd.String("format", "json"))
}

// 2. JSON Customization - Pretty print and custom field names
func jsonCustomization() {
	fmt.Println("\n2. JSON Customization")

	// Pretty-printed JSON for readability
	config := dd.JSONConfig()
	config.JSON = &dd.JSONOptions{
		PrettyPrint: true,
		Indent:      "  ",
		FieldNames: &dd.JSONFieldNames{
			Timestamp: "time",
			Level:     "severity",
			Message:   "msg",
		},
	}

	logger, _ := dd.New(config)
	defer logger.Close()

	logger.InfoWith("Custom JSON format", dd.Int("user_id", 123), dd.String("action", "login"))
}

// 3. Security Filtering - Protect sensitive data
func securityFiltering() {
	fmt.Println("\n3. Security Filtering")

	// Basic filtering: passwords, API keys
	config1 := dd.DefaultConfig().EnableBasicFiltering()
	logger1, _ := dd.New(config1)
	defer logger1.Close()
	logger1.Info("password=secret123 api_key=sk-abc123")

	// Full filtering: emails, credit cards, IPs, etc.
	config2 := dd.DefaultConfig().EnableFullFiltering()
	logger2, _ := dd.New(config2)
	defer logger2.Close()
	logger2.Info("email=user@example.com card=4532015112830366")

	// Custom patterns for domain-specific data
	filter := dd.NewEmptySensitiveDataFilter()
	filter.AddPattern(`(?i)session[_-]?id[:\s=]+[^\s]+`)
	config3 := dd.DefaultConfig().WithFilter(filter)
	logger3, _ := dd.New(config3)
	defer logger3.Close()
	logger3.Info("session_id=xyz789 public_data=ok")
}

// 4. File Rotation - Automatic log management
func fileRotation() {
	fmt.Println("\n4. File Rotation")

	// Basic rotation: size, age, and backup limits
	config, _ := dd.DefaultConfig().WithFile("logs/app.log", dd.FileWriterConfig{
		MaxSizeMB:  10,                 // Rotate at 10MB
		MaxBackups: 5,                  // Keep 5 old files
		MaxAge:     7 * 24 * time.Hour, // Delete after 7 days
		Compress:   true,               // Compress old logs
	})

	logger, _ := dd.New(config)
	defer logger.Close()
	logger.Info("Logs rotate automatically at 10MB")

	// Multiple files with different policies
	config2 := dd.DefaultConfig()
	config2.WithFile("logs/app-all.log", dd.FileWriterConfig{}) // Default rotation
	config2.WithFile("logs/errors.log", dd.FileWriterConfig{
		MaxSizeMB:  100,                 // Larger for errors
		MaxBackups: 50,                  // Keep more backups
		MaxAge:     90 * 24 * time.Hour, // Longer retention
		Compress:   true,
	})

	logger2, _ := dd.New(config2)
	defer logger2.Close()
	logger2.Error("Errors have longer retention policy")
}

// 5. Production Setup - Real-world configuration patterns
func productionSetup() {
	fmt.Println("\n5. Production Setup")

	// Application logger: JSON, INFO level, full filtering
	appConfig := dd.JSONConfig()
	appConfig.Level = dd.LevelInfo
	appConfig.EnableFullFiltering()
	appConfig, _ = appConfig.WithFile("logs/app.log", dd.FileWriterConfig{
		MaxSizeMB:  100,
		MaxBackups: 30,
		MaxAge:     30 * 24 * time.Hour,
		Compress:   true,
	})

	appLogger, _ := dd.New(appConfig)
	defer appLogger.Close()

	appLogger.InfoWith("Application started",
		dd.String("version", "1.2.3"),
		dd.Int("pid", os.Getpid()),
	)

	// Error logger: separate file, longer retention
	errorConfig := dd.JSONConfig()
	errorConfig.Level = dd.LevelError
	errorConfig.EnableFullFiltering()
	errorConfig, _ = errorConfig.WithFileOnly("logs/errors.log", dd.FileWriterConfig{
		MaxSizeMB:  200,
		MaxBackups: 100,
		MaxAge:     90 * 24 * time.Hour,
		Compress:   true,
	})

	errorLogger, _ := dd.New(errorConfig)
	defer errorLogger.Close()

	errorLogger.ErrorWith("Database error",
		dd.Err(fmt.Errorf("connection timeout")),
		dd.String("host", "db.example.com"),
	)

	// Access logger: no filtering, high volume
	accessConfig := dd.JSONConfig()
	accessConfig.DisableFiltering()
	accessConfig, _ = accessConfig.WithFileOnly("logs/access.log", dd.FileWriterConfig{
		MaxSizeMB:  500,
		MaxBackups: 20,
		MaxAge:     14 * 24 * time.Hour,
		Compress:   true,
	})

	accessLogger, _ := dd.New(accessConfig)
	defer accessLogger.Close()

	accessLogger.InfoWith("HTTP request",
		dd.String("method", "GET"),
		dd.String("path", "/api/users"),
		dd.Int("status", 200),
		dd.String("ip", "192.168.1.100"),
	)

	fmt.Println("Check logs/*.log for output")
}
