package main

import (
	"fmt"

	"github.com/cybergodev/dd"
)

func main() {
	fmt.Println("DD Logger - Quick Start Guide\n=============================")

	// 1. Simplest usage - package-level functions
	fmt.Println("\n1. Package-level logging (simplest)")
	dd.Info("Application started")
	dd.Warn("This is a warning")
	dd.Error("Something went wrong")

	// 2. Console-only logger
	fmt.Println("\n2. Console-only logger")
	logger := dd.ToConsole()
	defer logger.Close()
	logger.Info("Using ToConsole() for console output")

	// 3. Development logger (debug level + caller info)
	fmt.Println("\n3. Development logger with custom options")
	devLogger, _ := dd.NewWithOptions(dd.Options{
		Level:         dd.LevelDebug,
		IncludeCaller: true,
		DynamicCaller: true,
		Console:       true,
	})
	defer devLogger.Close()
	devLogger.Debug("Debug information visible in dev mode")
	devLogger.Info("Application ready")

	// 4. JSON logger for structured data
	fmt.Println("\n4. JSON structured logging to file")
	jsonLogger := dd.ToJSONFile()
	defer jsonLogger.Close()
	jsonLogger.InfoWith("User login",
		dd.Any("user_id", 12345),
		dd.Any("username", "john_doe"),
		dd.Any("success", true),
	)

	// 5. File output (console + file) with default filename
	fmt.Println("\n5. File logging with default filename")
	fileLogger := dd.ToAll()
	defer fileLogger.Close()
	fileLogger.Info("This message goes to both console and logs/app.log (default)")

	// 6. File output with custom filename
	fmt.Println("\n6. File logging with custom filename")
	customFileLogger := dd.ToAll("logs/quick-start.log")
	defer customFileLogger.Close()
	customFileLogger.Info("This message goes to both console and custom file")

	// 7. Options struct for custom configuration
	fmt.Println("\n7. Custom configuration with Options")
	customLogger, err := dd.NewWithOptions(dd.Options{
		Level:         dd.LevelDebug,
		Format:        dd.FormatJSON,
		IncludeCaller: true,
		Console:       true,
		File:          "logs/custom-quick.log",
	})
	if err != nil {
		dd.Error("Failed to create custom logger:", err)
		return
	}
	defer customLogger.Close()

	customLogger.InfoWith("Custom configured logger",
		dd.Any("level", "debug"),
		dd.Any("format", "json"),
		dd.Any("caller", true),
	)

	fmt.Println("\n✅ Quick start completed! Check the logs/ directory for output files.")

}
