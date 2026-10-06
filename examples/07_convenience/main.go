package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/cybergodev/dd"
)

// Convenience APIs & Error Handling
//
// Topics covered:
// 1. Print family (fmt replacement: Print/Printf/Println)
// 2. Verifying the default logger (DefaultWithErr, DefaultInitError)
// 3. Constructor error-handling patterns (incl. sentinel errors)
//
// Output targets (ConsoleOutput/FileOutput/CustomOutput) are covered in
// 03_configuration; individual writer types in 05_writers.
func main() {
	fmt.Println("=== DD Convenience & Error Handling ===")

	section1PrintFamily()
	section2DefaultVerification()
	section3ConstructorErrors()

	fmt.Println("\n✅ Convenience examples completed!")
}

// Section 1: The Print family — fmt-style output through the logger's
// configured writers. Unlike the raw debug helpers (dd.Text/dd.JSON, see
// 09_advanced), these ARE security-filtered and level-gated (INFO).
func section1PrintFamily() {
	fmt.Println("1. Print Family (fmt Replacement)")
	fmt.Println("----------------------------------")

	// Package-level: uses the default logger's writers
	dd.Print("Print:", "arguments", "joined with spaces")
	dd.Printf("Printf: formatted status=%d path=%s", 200, "/api/users")
	dd.Println("Println: behaves like Print (entries always end with a newline)")

	// Instance and entry variants share the same semantics
	logger, _ := dd.New()
	defer logger.Close()
	logger.Printf("logger.Printf: %s", "same behavior on instances")
	logger.WithField("request_id", "req-1").
		Printf("entry.Printf keeps the preset fields")

	fmt.Println()
}

// Section 2: DefaultWithErr / DefaultInitError — detect whether the default
// logger was built successfully or is running in fallback (stderr) mode.
func section2DefaultVerification() {
	fmt.Println("2. Default Logger Verification")
	fmt.Println("-------------------------------")

	// DefaultWithErr returns the default logger AND its init error
	logger, err := dd.DefaultWithErr()
	if err != nil {
		fmt.Printf("  Running in fallback mode: %v\n", err)
	}
	fmt.Printf("  Default logger ready: %v\n", logger != nil && !logger.IsClosed())

	// DefaultInitError reports the same error later, without the logger
	// (nil when the default logger is healthy)
	fmt.Printf("  DefaultInitError: %v\n", dd.DefaultInitError())

	fmt.Println()
}

// Section 3: Constructor error handling patterns
func section3ConstructorErrors() {
	fmt.Println("3. Constructor Error Patterns")
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

	// Pattern 4: Match constructor errors with sentinel errors (errors.Is)
	badCfg := dd.DefaultConfig()
	badCfg.Targets = []dd.OutputTarget{dd.FileOutput("")} // empty path is invalid
	_, err = dd.New(badCfg)
	fmt.Printf("  New() error: %v\n", err)
	if errors.Is(err, dd.ErrEmptyFilePath) {
		fmt.Println("  ✓ errors.Is matched sentinel dd.ErrEmptyFilePath")
	}

	// Constructors document their sentinel errors (see the Err* variables)
	_, err = dd.New(dd.DefaultConfig(), dd.DefaultConfig())
	fmt.Printf("  New() error: %v\n", err)
	if errors.Is(err, dd.ErrMultipleConfigs) {
		fmt.Println("  ✓ errors.Is matched sentinel dd.ErrMultipleConfigs")
	}

	fmt.Println("\n✓ Always handle errors explicitly for robust code")
}
