package main

import (
	"fmt"
	"strings"

	"github.com/cybergodev/dd"
)

// Security - Sensitive Data Filtering, Security Levels, and Flood Protection
//
// Topics covered:
// 1. Basic filtering (default protection)
// 2. Security levels (Basic, Standard, Strict, Paranoid)
// 3. Industry presets (Healthcare, Financial, Government)
// 4. Custom filtering patterns
// 5. Rate limiting and message size limits
// 6. Filter statistics and monitoring
// 7. Disable filtering when needed
//
// NOTE: constructor errors are ignored (logger, _) for brevity in these
// examples; see 07_convenience for error-handling patterns.
func main() {
	fmt.Println("=== DD Security Features ===")

	section1BasicFiltering()
	section2SecurityLevels()
	section3IndustryPresets()
	section4CustomFiltering()
	section5FloodProtection()
	section6FilterStats()
	section7DisableFiltering()

	fmt.Println("\n✅ Security examples completed!")
}

// Section 1: Basic filtering (default protection)
func section1BasicFiltering() {
	fmt.Println("1. Basic Filtering")
	fmt.Println("-------------------")

	// DefaultSecurityConfig: passwords, API keys, credit cards, phones
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
	fmt.Println()
}

// Section 2: Security levels - SecurityConfigForLevel selects filtering
// strength: Development < Basic < Standard < Strict < Paranoid.
func section2SecurityLevels() {
	fmt.Println("2. Security Levels")
	fmt.Println("-------------------")

	// One message, four strengths. Standard replaces the deprecated
	// DefaultSecureConfig(); DefaultSecurityConfig() equals Basic.
	sample := "email=admin@corp.example.com user_id=usr8f3a2b confidential=project-x"

	levels := []struct {
		level dd.SecurityLevel
	}{
		{dd.SecurityLevelBasic},
		{dd.SecurityLevelStandard},
		{dd.SecurityLevelStrict},
		{dd.SecurityLevelParanoid},
	}

	for _, l := range levels {
		filter := dd.SecurityConfigForLevel(l.level).SensitiveFilter
		fmt.Printf("  %-9s %s\n", l.level.String()+":", filter.Filter(sample))
	}

	// Security can also be swapped on an existing logger at runtime
	// (SetSecurityConfig clones the config; GetSecurityConfig returns a copy)
	logger, _ := dd.New()
	defer logger.Close()
	logger.Info(sample)                                                           // default = Basic
	logger.SetSecurityConfig(dd.SecurityConfigForLevel(dd.SecurityLevelStandard)) // upgrade live
	logger.Info(sample)                                                           // now Standard

	fmt.Println("  Basic: credentials/cards only (fewest false positives)")
	fmt.Println("  Standard: + emails, IPs, JWTs, connection strings")
	fmt.Println("  Strict/Paranoid: + internal IDs, confidential labels")
	fmt.Println()
}

// Section 3: Industry presets - built-in compliance pattern packs on top of
// the full (Standard) filter.
func section3IndustryPresets() {
	fmt.Println("3. Industry Presets")
	fmt.Println("--------------------")

	presets := []struct {
		name   string
		config *dd.SecurityConfig
		sample string
	}{
		{"Healthcare (HIPAA/PHI)", dd.HealthcareConfig(), "mrn=MRN123456 diagnosis=E11.9"},
		{"Financial (PCI-DSS)", dd.FinancialConfig(), "iban=GB29NWBK60161331926819 cvv=123"},
		{"Government (PII)", dd.GovernmentConfig(), "passport_no=123456789 driver_license=D1234567"},
	}

	for _, p := range presets {
		demoPreset(p.name, p.config, p.sample)
	}

	fmt.Println()
}

// demoPreset logs one sample through a logger configured with an industry
// preset. A helper function keeps each preset's logger lifetime scoped to one
// call (a defer inside the loop above would pile up open loggers).
func demoPreset(name string, config *dd.SecurityConfig, sample string) {
	cfg := dd.DefaultConfig()
	cfg.Security = config
	logger, _ := dd.New(cfg)
	defer logger.Close()
	fmt.Printf("  %-24s %s -> ", name, sample)
	logger.Info(sample)
}

// Section 4: Custom filtering patterns
func section4CustomFiltering() {
	fmt.Println("4. Custom Filtering")
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
	fmt.Println()
}

// captureWriter records how many lines were written and the last line's size,
// so the demo can show exactly what survived the rate/size gates.
type captureWriter struct {
	lines   int
	lastLen int
}

func (w *captureWriter) Write(p []byte) (int, error) {
	w.lines++
	w.lastLen = len(p)
	return len(p), nil
}

// Section 5: Flood protection - per-second rate limits and message size caps
// configured through SecurityConfig.
func section5FloodProtection() {
	fmt.Println("5. Rate Limiting & Message Size")
	fmt.Println("--------------------------------")

	// Rate limiting: DefaultRateLimitConfig() then adjust the budgets.
	// 5 messages/second + burst of 5; bytes limit disabled to isolate the demo.
	rateLimit := dd.DefaultRateLimitConfig()
	rateLimit.MaxMessagesPerSecond = 5
	rateLimit.BurstSize = 5
	rateLimit.MaxBytesPerSecond = 0

	rateCfg := dd.DefaultConfig()
	rateCfg.Targets = []dd.OutputTarget{dd.CustomOutput(&captureWriter{})}
	rateCfg.Security = &dd.SecurityConfig{RateLimitConfig: rateLimit}

	rateLogger, _ := dd.New(rateCfg)
	rateOut := rateCfg.Targets[0].Writer.(*captureWriter)
	defer rateLogger.Close()

	// Burst 50 messages in one second: ~10 pass (5/s budget + 5 burst tokens)
	const total = 50
	for i := 0; i < total; i++ {
		rateLogger.Infof("burst message %02d", i)
	}
	fmt.Printf("  Rate limit: %d sent, %d written (budget 5/s + burst 5)\n",
		total, rateOut.lines)

	// Size limit: a separate logger (the rate budget above is exhausted);
	// oversized entries are truncated to MaxMessageSize instead of dropped
	sizeCfg := dd.DefaultConfig()
	sizeCfg.Targets = []dd.OutputTarget{dd.CustomOutput(&captureWriter{})}
	sizeCfg.Security = &dd.SecurityConfig{MaxMessageSize: 120}

	sizeLogger, _ := dd.New(sizeCfg)
	sizeOut := sizeCfg.Targets[0].Writer.(*captureWriter)
	defer sizeLogger.Close()

	sizeLogger.Info(strings.Repeat("x", 300))
	fmt.Printf("  Size limit: 300-char message -> %d-byte line (cap 120)\n",
		sizeOut.lastLen)

	fmt.Println()
}

// Section 6: Filter statistics and monitoring
func section6FilterStats() {
	fmt.Println("6. Filter Statistics")
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

// Section 7: Disable filtering completely (use with caution)
func section7DisableFiltering() {
	fmt.Println("7. Disable Filtering")
	fmt.Println("---------------------")

	// No filtering - maximum performance (development only)
	cfg := dd.DefaultConfig()
	cfg.Security = dd.SecurityConfigForLevel(dd.SecurityLevelDevelopment)

	logger, _ := dd.New(cfg)
	defer logger.Close()

	logger.Info("password=raw_password") // Not filtered

	fmt.Println("  SecurityLevelDevelopment: filtering off, development only")
	fmt.Println()
}
