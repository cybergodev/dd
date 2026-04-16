//go:build examples

package main

import (
	"bytes"
	"fmt"
	"log"
	"os"

	"github.com/cybergodev/dd"
)

// Quick Logger Setup - Config and Targets Patterns
//
// Topics covered:
// 1. FileOutput - File output with rotation
// 2. ConsoleOutput - Console only
// 3. Multiple Targets - Dual output (console + file)
// 4. CustomOutput - Custom writers
// 5. Proper error handling patterns
func main() {
	fmt.Println("=== DD Quick Logger Setup ===")

	section1FileOutput()
	section2ConsoleOutput()
	section3DualOutput()
	section4CustomWriters()
	section5ConstructorErrors()

	fmt.Println("\n✅ Quick setup examples completed!")
	fmt.Println("\nCheck logs/ directory for output files")
}

// Section 1: File output
func section1FileOutput() {
	fmt.Println("1. File Output")
	fmt.Println("---------------")

	// Default file: logs/app.log (text format)
	cfg := dd.DefaultConfig()
	cfg.Targets = []dd.OutputTarget{dd.FileOutput("logs/app.log")}
	logger, _ := dd.New(cfg)
	defer logger.Close()
	logger.Info("Text format to logs/app.log")

	// Custom path
	cfg2 := dd.DefaultConfig()
	cfg2.Targets = []dd.OutputTarget{dd.FileOutput("logs/custom.log")}
	logger2, _ := dd.New(cfg2)
	defer logger2.Close()
	logger2.Info("Text format to logs/custom.log")

	// JSON format to file
	cfg3 := dd.JSONConfig()
	cfg3.Targets = []dd.OutputTarget{dd.FileOutput("logs/json.log")}
	logger3, _ := dd.New(cfg3)
	defer logger3.Close()
	logger3.InfoWith("JSON format",
		dd.String("format", "json"),
		dd.Bool("structured", true),
	)

	fmt.Println("✓ Files: logs/app.log, logs/custom.log, logs/json.log")
	fmt.Println()
}

// Section 2: Console output
func section2ConsoleOutput() {
	fmt.Println("2. Console Output")
	fmt.Println("------------------")

	// Console only (stdout)
	cfg := dd.DefaultConfig()
	cfg.Targets = []dd.OutputTarget{dd.ConsoleOutput()}
	logger, _ := dd.New(cfg)
	defer logger.Close()

	logger.Info("Console only - no file")
	logger.InfoWith("Structured console output",
		dd.String("source", "console"),
	)

	fmt.Println()
}

// Section 3: Dual output (console + file)
func section3DualOutput() {
	fmt.Println("3. Dual Output (Console + File)")
	fmt.Println("--------------------------------")

	// Text format to both console and file
	cfg := dd.DefaultConfig()
	cfg.Targets = []dd.OutputTarget{
		dd.ConsoleOutput(),
		dd.FileOutput("logs/dual.log"),
	}
	logger, _ := dd.New(cfg)
	defer logger.Close()
	logger.Info("Appears in BOTH console and file")

	// JSON format to both console and file
	cfg2 := dd.JSONConfig()
	cfg2.Targets = []dd.OutputTarget{
		dd.ConsoleOutput(),
		dd.FileOutput("logs/dual-json.log"),
	}
	logger2, _ := dd.New(cfg2)
	defer logger2.Close()
	logger2.InfoWith("JSON to both outputs",
		dd.String("format", "json"),
	)

	fmt.Println()
}

// Section 4: Custom writers
func section4CustomWriters() {
	fmt.Println("4. Custom Writers")
	fmt.Println("------------------")

	// Single custom writer
	var buf bytes.Buffer
	cfg := dd.DefaultConfig()
	cfg.Targets = []dd.OutputTarget{dd.CustomOutput(&buf)}
	logger, _ := dd.New(cfg)
	defer logger.Close()

	logger.Info("Written to buffer")
	fmt.Printf("  Buffer content: %s", buf.String()[:50])

	// Multiple writers via custom MultiWriter
	file, err := os.Create("logs/multi-writer.log")
	if err != nil {
		fmt.Printf("  Failed to create file: %v\n", err)
		return
	}
	defer file.Close()

	multiWriter := dd.NewMultiWriter(os.Stdout, file)
	cfg2 := dd.DefaultConfig()
	cfg2.Targets = []dd.OutputTarget{dd.CustomOutput(multiWriter)}
	logger2, _ := dd.New(cfg2)
	defer logger2.Close()

	logger2.Info("Goes to stdout AND file")

	fmt.Println()
}

// Section 5: Constructor error handling patterns
func section5ConstructorErrors() {
	fmt.Println("5. Constructor Error Patterns")
	fmt.Println("------------------------------")

	// Pattern 1: Explicit error handling with log.Fatal
	logger, err := dd.New(dd.DevelopmentConfig())
	if err != nil {
		log.Fatalf("failed to create logger: %v", err)
	}
	defer logger.Close()
	logger.Debug("Created with explicit error handling")

	// Pattern 2: File output with fallback to console
	cfg2 := dd.DefaultConfig()
	cfg2.Targets = []dd.OutputTarget{dd.FileOutput("logs/safe.log")}
	logger2, err := dd.New(cfg2)
	if err != nil {
		log.Printf("warning: could not create file logger: %v", err)
		// Fall back to console
		fallbackCfg := dd.DefaultConfig()
		fallbackCfg.Targets = []dd.OutputTarget{dd.ConsoleOutput()}
		logger2, _ = dd.New(fallbackCfg)
	}
	defer logger2.Close()
	logger2.Info("Created with fallback handling")

	// Pattern 3: Console output (rarely fails)
	cfg3 := dd.DefaultConfig()
	cfg3.Targets = []dd.OutputTarget{dd.ConsoleOutput()}
	logger3, err := dd.New(cfg3)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create console logger: %v\n", err)
		return
	}
	defer logger3.Close()
	logger3.Info("Created with console fallback")

	// Pattern 4: Dual output with error handling
	cfg4 := dd.DefaultConfig()
	cfg4.Targets = []dd.OutputTarget{
		dd.ConsoleOutput(),
		dd.FileOutput("logs/safe-dual.log"),
	}
	logger4, err := dd.New(cfg4)
	if err != nil {
		log.Printf("warning: could not create dual logger: %v", err)
		return
	}
	defer logger4.Close()
	logger4.Info("Created with dual output")

	fmt.Println("\n✓ Always handle errors explicitly for robust code")
}
