package main

// This example demonstrates the Exit() and Exitf() methods
// These methods output debug information and then terminate the program with os.Exit(0)
//
// Usage: Uncomment ONE example below to test
// Note: The program will terminate immediately after the Exit/Exitf call

func main() {
	println("=== Exit() and Exitf() Methods Demo ===")
	println("These methods print debug output and then call os.Exit(0)")
	println()

	// Example 1: Exit with single value
	// Outputs: "Program terminated here"
	// Then exits with code 0
	// dd.Exit("Program terminated here")

	// Example 2: Exit with multiple values
	// Outputs: "Error: File not found code: 404"
	// Then exits with code 0
	// dd.Exit("Error:", "File not found", "code:", 404)

	// Example 3: Exitf with formatted output
	// Outputs: "Fatal error: Division by zero at line 42"
	// Then exits with code 0
	// dd.Exitf("Fatal error: %s at line %d", "Division by zero", 42)

	// Example 4: Exit with complex data
	// Outputs formatted JSON for complex types
	// dd.Exit("Config error:", map[string]int{"port": 8080, "timeout": 30})

	// Example 5: Logger Exit method
	// logger := dd.ToConsole()
	// defer logger.Close() // Note: defer won't execute because Exit() terminates
	// logger.Exit("Logger exit:", "Shutting down gracefully")

	// Example 6: Logger Exitf method
	// logger := dd.ToConsole()
	// logger.Exitf("Application terminated: %s (code: %d)", "Critical error", 1)

	println("=== Instructions ===")
	println("1. Uncomment ONE Exit/Exitf call above")
	println("2. Run: go run -tags examples examples/17_exit_debugging.go")
	println("3. Observe the output before program termination")
	println()
	println("Note: Any code after Exit/Exitf will NOT execute")
	println("      The program terminates immediately with exit code 0")
}
