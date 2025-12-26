package main

import (
	"fmt"
	"io"
	"time"

	"github.com/cybergodev/dd"
)

func main() {
	fmt.Println("DD Logger - File Output Examples")
	fmt.Println("================================")

	example1QuickFileOutput()
	example2FileOnly()
	example3JSONFileOutput()
	example4BuilderFileOutput()
	example5CustomRotation()
	example6MultipleFiles()
	example7DateBasedLogs()
	example8LargeFileRotation()
	example9FileSecurity()
	example10RealWorldScenario()

	fmt.Println("\n=== All file output examples completed! ===")
	fmt.Println("Check the logs/ directory for generated log files")
}

// Example 1: File output only
func example1QuickFileOutput() {
	fmt.Println("\n=== Example 1: File Output Only ===")

	// Use convenience method to create file logger (file only)
	logger := dd.ToFile("logs/app.log")
	defer logger.Close()

	logger.Info("This log only goes to file")
	fmt.Println("Log written to logs/app.log (not shown in console)")
}

// Example 2: Console and file output
func example2FileOnly() {
	fmt.Println("\n=== Example 2: Console and File Output ===")

	// Use convenience method to create logger that outputs to both console and file
	logger := dd.ToAll("logs/all.log")
	defer logger.Close()

	logger.Info("This log goes to both console and file")
	logger.Warn("File path: logs/all.log")
}

// Example 3: JSON format file output
func example3JSONFileOutput() {
	fmt.Println("\n=== Example 3: JSON Format File Output ===")

	// Use convenience method to create JSON file logger (file only)
	logger := dd.ToJSONFile("logs/json.log")
	defer logger.Close()

	logger.InfoWith("JSON format log",
		dd.Any("user", "john_doe"),
		dd.Any("user_id", 12345),
		dd.Any("active", true),
	)
	fmt.Println("JSON log written to logs/json.log")
}

// Example 4: Custom file logger using Options
func example4BuilderFileOutput() {
	fmt.Println("\n=== Example 4: Custom File Logger Using Options ===")

	logger, err := dd.NewWithOptions(dd.Options{
		Level:         dd.LevelDebug,
		Format:        dd.FormatJSON,
		IncludeCaller: true,
		Console:       true,
		File:          "logs/custom.log",
	})
	if err != nil {
		fmt.Printf("Failed to create logger: %v\n", err)
		return
	}
	defer logger.Close()

	logger.DebugWith("Custom configured log",
		dd.Any("feature", "options_api"),
		dd.Any("output", "both_console_and_file"),
	)
}

// Example 5: Custom file rotation configuration
func example5CustomRotation() {
	fmt.Println("\n=== Example 5: Custom File Rotation Configuration ===")

	// Create file writer with rotation parameters
	fileWriter, err := dd.NewFileWriter("logs/rotation.log", dd.FileWriterConfig{
		MaxSizeMB:  50,                 // Rotate when file reaches 50MB
		MaxBackups: 5,                  // Keep 5 backup files
		MaxAge:     7 * 24 * time.Hour, // Keep for 7 days
		Compress:   true,               // Compress old files
	})
	if err != nil {
		fmt.Printf("Failed to create file writer: %v\n", err)
		return
	}
	defer fileWriter.Close()

	logger, err := dd.NewWithOptions(dd.Options{
		Console:           true,
		AdditionalWriters: []io.Writer{fileWriter},
	})
	if err != nil {
		fmt.Printf("Failed to create logger: %v\n", err)
		return
	}
	defer logger.Close()

	logger.Info("Using custom rotation configuration")
	logger.Infof("Max file size: 50MB")
	logger.Infof("Max backups: 5")
	logger.Infof("Max retention: 7 days")
}

// Example 6: Multiple file outputs
func example6MultipleFiles() {
	fmt.Println("\n=== Example 6: Multiple File Outputs ===")

	// Create multiple file writers
	infoWriter, _ := dd.NewFileWriter("logs/info.log", dd.FileWriterConfig{
		MaxSizeMB:  50,
		MaxBackups: 5,
		Compress:   true,
	})
	defer infoWriter.Close()

	errorWriter, _ := dd.NewFileWriter("logs/error.log", dd.FileWriterConfig{
		MaxSizeMB:  100,
		MaxBackups: 20,
		Compress:   true,
	})
	defer errorWriter.Close()

	logger, err := dd.NewWithOptions(dd.Options{
		Console:           true,
		AdditionalWriters: []io.Writer{infoWriter, errorWriter},
	})
	if err != nil {
		fmt.Printf("Failed to create logger: %v\n", err)
		return
	}
	defer logger.Close()

	logger.Info("Regular log (written to both files)")
	logger.Error("Error log (written to both files)")
}

// Example 7: Date-based log splitting
func example7DateBasedLogs() {
	fmt.Println("\n=== Example 7: Date-Based Log Splitting ===")

	// Use date as part of filename
	today := time.Now().Format("2006-01-02")
	filename := "logs/app-" + today + ".log"

	logger := dd.ToAll(filename)
	defer logger.Close()

	logger.Info("Using date-based log file splitting")
	logger.Infof("Filename: %s", filename)
}

// Example 8: Large file rotation test
func example8LargeFileRotation() {
	fmt.Println("\n=== Example 8: Large File Rotation Test ===")

	// Set small file size for testing rotation
	fileWriter, err := dd.NewFileWriter("logs/rotation_test.log", dd.FileWriterConfig{
		MaxSizeMB:  1, // Rotate at 1MB
		MaxBackups: 3,
		Compress:   true,
	})
	if err != nil {
		fmt.Printf("Failed to create rotation test writer: %v\n", err)
		return
	}
	defer fileWriter.Close()

	logger, err := dd.NewWithOptions(dd.Options{
		Console:           true,
		AdditionalWriters: []io.Writer{fileWriter},
	})
	if err != nil {
		fmt.Printf("Failed to create logger: %v\n", err)
		return
	}
	defer logger.Close()

	// Write many logs to trigger rotation
	for i := 0; i < 1000; i++ {
		logger.Infof("Log %d: This is a test log to test file rotation functionality. Contains extra text to increase file size", i)
	}

	logger.Info("Rotation test complete, check logs/ directory")
}

// Example 9: File permissions and security
func example9FileSecurity() {
	fmt.Println("\n=== Example 9: File Permissions and Security ===")

	// DD automatically prevents path traversal attacks
	// The following path will be rejected
	_, err := dd.NewFileWriter("../../../etc/passwd", dd.FileWriterConfig{})
	if err != nil {
		fmt.Printf("Path traversal attack blocked: %v\n", err)
	}

	// Safe path
	logger := dd.ToFile("logs/secure.log")
	defer logger.Close()

	logger.Info("Safe file path")
}

// Example 10: Real-world scenario
func example10RealWorldScenario() {
	fmt.Println("\n=== Example 10: Real-World Scenario ===")

	// Application logs
	appWriter, _ := dd.NewFileWriter("logs/app.log", dd.FileWriterConfig{
		MaxSizeMB:  100,
		MaxBackups: 30,
		MaxAge:     30 * 24 * time.Hour,
		Compress:   true,
	})
	defer appWriter.Close()

	// Error logs (kept longer)
	errorWriter, _ := dd.NewFileWriter("logs/error.log", dd.FileWriterConfig{
		MaxSizeMB:  200,
		MaxBackups: 90,
		MaxAge:     90 * 24 * time.Hour,
		Compress:   true,
	})
	defer errorWriter.Close()

	logger, err := dd.NewWithOptions(dd.Options{
		Format:            dd.FormatJSON,
		Console:           true,
		AdditionalWriters: []io.Writer{appWriter, errorWriter},
	})
	if err != nil {
		fmt.Printf("Failed to create logger: %v\n", err)
		return
	}
	defer logger.Close()

	logger.Info("Application started")
	logger.InfoWith("Configuration loaded",
		dd.Any("env", "prod"),
		dd.Any("version", "1.0.0"),
	)
}
