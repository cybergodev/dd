package main

import (
	"context"
	"fmt"
	"time"

	"github.com/cybergodev/dd"
)

// Advanced Features - Sampling, Validation, Level Resolver, Fatal Handler
//
// Topics covered:
// 1. Log sampling for high-throughput scenarios
// 2. Field validation with naming conventions
// 3. Dynamic level resolver
// 4. Custom fatal handler
// 5. Debug utilities
// 6. Dynamic-level logging (Log/Logf/LogWith) and package-level WithFields
//
// NOTE: constructor errors are ignored (logger, _) for brevity in these
// examples; see 07_convenience for error-handling patterns.
func main() {
	fmt.Println("=== DD Advanced Features ===")

	section1LogSampling()
	section2FieldValidation()
	section3LevelResolver()
	section4FatalHandler()
	section5DebugUtilities()
	section6DynamicLevels()

	fmt.Println("\n✅ Advanced features completed!")
}

// Section 1: Log sampling
func section1LogSampling() {
	fmt.Println("1. Log Sampling")
	fmt.Println("----------------")

	// Start from DefaultSamplingConfig and adjust — Initial messages are
	// always logged, afterwards only 1 in Thereafter. Tick=1s resets the
	// budget every second (burst traffic is reduced, steady traffic is not).
	sampling := dd.DefaultSamplingConfig()
	sampling.Initial = 10    // per second: log first 10
	sampling.Thereafter = 10 // then 1 in 10

	cfg := dd.DefaultConfig()
	cfg.Sampling = &sampling
	cfg.Targets = []dd.OutputTarget{dd.ConsoleOutput()}

	logger, _ := dd.New(cfg)
	defer logger.Close()

	// Simulate high-throughput logging
	for i := 0; i < 50; i++ {
		logger.InfoWith("High throughput message",
			dd.Int("sequence", i),
		)
	}

	// Check sampling config
	current := logger.GetSampling()
	if current != nil {
		fmt.Printf("  Sampling enabled: Initial=%d, Thereafter=%d\n",
			current.Initial, current.Thereafter)
	}

	// Disable sampling
	logger.SetSampling(nil)
	logger.Info("Sampling disabled - all messages logged")

	fmt.Println()
}

// Section 2: Field validation
func section2FieldValidation() {
	fmt.Println("2. Field Validation")
	fmt.Println("--------------------")

	// Strict snake_case validation
	cfg := dd.DefaultConfig()
	cfg.FieldValidation = dd.StrictSnakeCaseConfig()

	logger, _ := dd.New(cfg)
	defer logger.Close()

	// Valid snake_case fields
	logger.InfoWith("Valid fields",
		dd.String("user_id", "123"),
		dd.String("request_id", "abc"),
		dd.Int("response_code", 200),
	)

	// Invalid fields would be logged in Warn mode or rejected in Strict mode
	// Note: validation warnings go to stderr, not the log output

	// Custom validation config
	customCfg := dd.DefaultConfig()
	customCfg.FieldValidation = &dd.FieldValidationConfig{
		Mode:                     dd.FieldValidationWarn,
		Convention:               dd.NamingConventionCamelCase,
		AllowCommonAbbreviations: true,
	}

	customLogger, _ := dd.New(customCfg)
	defer customLogger.Close()

	customLogger.InfoWith("CamelCase fields",
		dd.String("userId", "123"),
		dd.String("requestId", "abc"),
		dd.Int("responseCode", 200),
	)

	// Validation can be swapped at runtime on an existing logger
	customLogger.SetFieldValidation(dd.StrictSnakeCaseConfig())
	customLogger.InfoWith("Back to snake_case",
		dd.String("user_id", "123"),
	)

	fmt.Println("  Valid: snake_case, camelCase, PascalCase, kebab-case")
	fmt.Println()
}

// Section 3: Dynamic level resolver
func section3LevelResolver() {
	fmt.Println("3. Dynamic Level Resolver")
	fmt.Println("---------------------------")

	// Level resolver allows dynamic level based on runtime conditions.
	// Example: Adjust level based on time of day or system load
	resolver := func(ctx context.Context) dd.LogLevel {
		// In production, you might check:
		// - CPU/memory usage
		// - Error rate
		// - Time of day
		// - Feature flags

		hour := time.Now().Hour()
		if hour >= 22 || hour < 6 {
			// Night time: only warnings and above
			return dd.LevelWarn
		}
		// Day time: debug level
		return dd.LevelDebug
	}

	// Install at construction time via Config.LevelResolver...
	cfg := dd.DefaultConfig()
	cfg.Level = dd.LevelDebug
	cfg.LevelResolver = resolver

	logger, _ := dd.New(cfg)
	defer logger.Close()

	// Log level is determined dynamically for every entry and IsLevelEnabled
	logger.Debug("This may or may not show depending on time")
	logger.Info("Info message")
	logger.Warn("Warning always shows")

	// ...or manage at runtime: SetLevelResolver installs/replaces one,
	// nil restores the static level (Debug, from cfg.Level above)
	logger.SetLevelResolver(nil)
	logger.Debug("Debug with static level (shows)")

	fmt.Println()
}

// Section 4: Custom fatal handler
func section4FatalHandler() {
	fmt.Println("4. Custom Fatal Handler")
	fmt.Println("------------------------")

	// Custom fatal handler for graceful shutdown
	customFatalHandler := func() {
		fmt.Println("  [Custom Fatal Handler] Cleanup before exit...")
		// In production:
		// - Flush buffers
		// - Close connections
		// - Notify monitoring
		// - Then exit
		fmt.Println("  [Custom Fatal Handler] Exiting with code 1")
		// os.Exit(1) // Uncomment for real behavior
	}

	cfg := dd.DefaultConfig()
	cfg.FatalHandler = customFatalHandler

	logger, _ := dd.New(cfg)
	defer logger.Close()

	// Note: We don't call Fatal() here as it would exit the program
	// In production: logger.Fatal("Critical error") would trigger the handler
	fmt.Println("  Fatal handler configured (not triggered in demo)")

	fmt.Println()
}

// Section 5: Debug utilities
func section5DebugUtilities() {
	fmt.Println("5. Debug Utilities")
	fmt.Println("--------------------")

	// Package-level debug functions output DIRECTLY to stdout.
	// WARNING: NO sensitive data filtering — use only for development.
	// NEVER pass passwords, tokens, or secrets to these functions.

	// dd.Text() - Pretty-printed output to stdout
	fmt.Println("dd.Text():")
	dd.Text("Quick debug:", "value", 42, true)
	dd.Text("Complex:", map[string]any{"name": "Alice", "age": 30})

	// dd.Textf() - Formatted output to stdout
	fmt.Println("\ndd.Textf():")
	dd.Textf("User: %s, Age: %d", "Bob", 25)

	// dd.JSON() - Compact JSON to stdout
	fmt.Println("\ndd.JSON():")
	dd.JSON("data", 123, map[string]string{"status": "active"})

	// dd.JSONF() - Formatted JSON to stdout
	fmt.Println("\ndd.JSONF():")
	dd.JSONF("Request from %s", "192.168.1.1")

	// Logger-level debug methods write to configured writers (with filtering)
	logger, _ := dd.New()
	defer logger.Close()

	fmt.Println("\nlogger.Text() and logger.JSON():")
	logger.Text("Processing", "item", 42)
	logger.JSON("result", true, "count", 100)

	// Exit() and Exitf() - Debug output + os.Exit(0)
	// dd.Exit("Program terminated here")
	// dd.Exitf("Fatal error: %s", "critical")

	fmt.Println("\n  WARNING: Package-level dd.Text/JSON/etc. do NOT filter data!")
	fmt.Println("  Logger methods (logger.Text/JSON) write to configured writers with filtering.")
	fmt.Println("  Filtered fmt-style alternatives: dd.Print/Printf (see 07_convenience).")
}

// Section 6: Dynamic-level logging — Log/Logf/LogWith choose the level at
// runtime; package-level WithFields attaches persistent fields to the default
// logger without holding a *Logger.
func section6DynamicLevels() {
	fmt.Println("6. Dynamic-Level Logging")
	fmt.Println("-------------------------")

	logger, _ := dd.New(dd.DevelopmentConfig())
	defer logger.Close()

	// Level chosen at runtime instead of by method name (e.g., from config)
	level := dd.LevelWarn
	logger.Log(level, "Logged via Log() with a runtime level")
	logger.Logf(level, "Logged via Logf() with %s", "a format string")
	logger.LogWith(level, "Logged via LogWith()",
		dd.String("mode", "dynamic"),
	)

	// The same generic-level functions exist at package level
	dd.LogWith(level, "Logged via package-level dd.LogWith()",
		dd.String("layer", "package"),
	)

	// Package-level entries build on the default logger
	dd.WithFields(
		dd.String("service", "batch-job"),
	).Info("Package-level entry with persistent fields")

	fmt.Println()
}
