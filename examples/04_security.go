//go:build examples

package main

import (
	"fmt"

	"github.com/cybergodev/dd"
)

// Security - Sensitive Data Filtering and Protection
//
// Topics covered:
// 1. Basic filtering (passwords, API keys, credit cards)
// 2. Full filtering (emails, IPs, SSNs, JWTs)
// 3. Custom filtering patterns
// 4. Filter statistics and monitoring
// 5. Disable filtering when needed
//
// Industry-specific presets: HealthcareConfig(), FinancialConfig(), GovernmentConfig()
func main() {
	fmt.Println("=== DD Security Features ===")

	section1BasicFiltering()
	section2FullFiltering()
	section3CustomFiltering()
	section4FilterStats()
	section5DisableFiltering()

	fmt.Println("\n✅ Security examples completed!")
}

// Section 1: Basic filtering (default protection)
func section1BasicFiltering() {
	fmt.Println("1. Basic Filtering")
	fmt.Println("-------------------")

	// Basic filtering: passwords, API keys, credit cards, phones
	cfg := dd.DefaultConfig()
	cfg.Security = dd.DefaultSecurityConfig()

	logger, _ := dd.New(cfg)
	defer logger.Close()

	// These are automatically filtered
	logger.Info("password=secret123")
	logger.Info("api_key=sk-1234567890abcdef")
	logger.Info("credit_card=4532015112830366")

	// Structured logging - key-based filtering
	logger.InfoWith("User login",
		dd.String("username", "john_doe"),
		dd.String("password", "secret123"),  // Filtered by key name
		dd.String("api_key", "sk-abc123"),   // Filtered by key name
		dd.String("token", "bearer-xyz789"), // Filtered by key name
	)

	// Nested values (maps, slices, structs via dd.Any) are filtered recursively
	logger.InfoWith("Nested structures",
		dd.Any("request", map[string]any{
			"path": "/login",
			"body": map[string]any{"user": "john", "password": "secret123"},
		}),
	)

	fmt.Println("✓ Sensitive data automatically filtered")
}

// Section 2: Full filtering (comprehensive protection)
func section2FullFiltering() {
	fmt.Println("2. Full Filtering")
	fmt.Println("------------------")

	// Full filtering includes: emails, IPs, SSNs, JWTs, DB URLs
	cfg := dd.DefaultConfig()
	cfg.Security = dd.DefaultSecureConfig() // Full protection

	logger, _ := dd.New(cfg)
	defer logger.Close()

	// Additional patterns filtered
	logger.Info("email=user@example.com")
	logger.Info("ssn=123-45-6789")
	logger.Info("ip_address=192.168.1.100")
	logger.Info("mysql://user:password@localhost:3306/database")

	// JWT tokens
	jwt := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signature"
	logger.Infof("Authorization: Bearer %s", jwt)

	fmt.Println("✓ Comprehensive filtering applied")
}

// Section 3: Custom filtering patterns
func section3CustomFiltering() {
	fmt.Println("3. Custom Filtering")
	fmt.Println("--------------------")

	// One-shot constructor: patterns are compiled and ReDoS-checked on creation
	filter, err := dd.NewCustomSensitiveDataFilter(
		`(?i)(internal_token[:\s=]+)[^\s]+`,
		`(?i)(session_id[:\s=]+)[^\s]+`,
		`(?i)(company_secret[:\s=]+)[^\s]+`,
	)
	if err != nil {
		fmt.Printf("  Invalid pattern: %v\n", err)
		return
	}

	cfg := dd.DefaultConfig()
	cfg.Security = &dd.SecurityConfig{
		SensitiveFilter: filter,
	}

	logger, _ := dd.New(cfg)
	defer logger.Close()

	logger.Info("internal_token=abc123")       // Filtered
	logger.Info("session_id=xyz789")           // Filtered
	logger.Info("company_secret=confidential") // Filtered
	logger.Info("public_data=visible")         // Not filtered

	fmt.Println("✓ Custom patterns applied")
}

// Section 4: Filter statistics and monitoring
func section4FilterStats() {
	fmt.Println("4. Filter Statistics")
	fmt.Println("---------------------")

	// A SensitiveDataFilter tracks how many messages it filtered and how many
	// values it redacted. When a filter is attached via SecurityConfig, the
	// logger clones it (defensive copy), so the logger's internal counters are
	// not visible on this instance. To observe statistics, drive the filter
	// directly with Filter().
	filter := dd.NewSensitiveDataFilter()

	// Apply filtering directly to accumulate real statistics
	for i := 0; i < 10; i++ {
		_ = filter.Filter(fmt.Sprintf("password=secret%d", i)) // redacted in place
	}

	// Read accumulated statistics
	stats := filter.GetFilterStats()
	fmt.Printf("  Pattern count: %d\n", stats.PatternCount)
	fmt.Printf("  Total filtered: %d\n", stats.TotalFiltered)
	fmt.Printf("  Total redactions: %d\n", stats.TotalRedactions)
	fmt.Printf("  Average latency: %v\n", stats.AverageLatency)
	fmt.Printf("  Enabled: %v\n", stats.Enabled)

	// Temporarily disable filtering (input passes through unchanged)
	filter.Disable()
	fmt.Printf("  While disabled: %q\n", filter.Filter("password=visible_now"))

	// Re-enable filtering
	filter.Enable()

	// Monitor active background goroutines (relevant in high-concurrency scenarios)
	fmt.Printf("  Active filter goroutines: %d\n", filter.ActiveGoroutineCount())

	fmt.Println()
}

// Example: Disable filtering completely (use with caution)
func section5DisableFiltering() {
	fmt.Println("5. Disable Filtering & Industry Presets")
	fmt.Println("-----------------------------------------")

	// No filtering - maximum performance (development only)
	cfg := dd.DefaultConfig()
	cfg.Security = dd.SecurityConfigForLevel(dd.SecurityLevelDevelopment)

	logger, _ := dd.New(cfg)
	defer logger.Close()

	logger.Info("password=raw_password") // Not filtered

	// Industry-specific presets (additional patterns for compliance):
	// - dd.HealthcareConfig()   — HIPAA/PHI: ICD codes, MRN, patient IDs
	// - dd.FinancialConfig()    — PCI-DSS: SWIFT/BIC, IBAN, CVV, account numbers
	// - dd.GovernmentConfig()   — PII: passport numbers, driver's license, SSN variants

	healthcareCfg := dd.DefaultConfig()
	healthcareCfg.Security = dd.HealthcareConfig()
	healthcareLogger, _ := dd.New(healthcareCfg)
	if healthcareLogger != nil {
		defer healthcareLogger.Close()
		healthcareLogger.Info("patient_id=MRN-123456") // Filtered by healthcare patterns
	}

	fmt.Println("  Available presets: HealthcareConfig, FinancialConfig, GovernmentConfig")
	fmt.Println()
}
