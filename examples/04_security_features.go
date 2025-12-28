//go:build examples

package main

import (
	"fmt"

	"github.com/cybergodev/dd"
)

func main() {
	fmt.Println("DD Logger - Security Features Examples")
	fmt.Println("======================================")

	example1DefaultFiltering()
	example2ComprehensiveFiltering()
	example3DisableFiltering()
	example4CustomFiltering()
	example5FieldLevelFiltering()
	example6LogInjectionProtection()
	example7MessageSizeLimit()
	example8MultiLayerSecurity()
	example9CreditCardFiltering()
	example10PasswordTokenFiltering()
	example11DatabaseConnectionFiltering()
	example12JWTFiltering()
	example13PrivateKeyFiltering()
	example14CloudCredentialsFiltering()
	example15RealWorldScenario()

	fmt.Println("\nAll examples completed!")
	fmt.Println("\nSecurity Tips:")
	fmt.Println("1. Sensitive data filtering is disabled by default, enable manually when handling sensitive data")
	fmt.Println("2. Use EnableBasicFiltering() for basic filtering (recommended)")
	fmt.Println("3. Use EnableFullFiltering() for comprehensive filtering (more protection)")
	fmt.Println("4. Custom filtering rules can be added")
	fmt.Println("5. Log injection protection is always enabled, no configuration needed")
	fmt.Println("6. Regularly review log content to ensure no sensitive data leakage")
}

// Example 1: Enable basic security filtering
func example1DefaultFiltering() {
	fmt.Println("\n=== Example 1: Enable Basic Security Filtering ===")

	// Sensitive data filtering is disabled by default, must be manually enabled
	config := dd.DefaultConfig().EnableBasicFiltering()
	logger, _ := dd.New(config)
	defer logger.Close()

	fmt.Println("Basic sensitive data filtering enabled")

	// These sensitive information will be automatically filtered
	logger.Info("password=secret123")
	logger.Info("api_key=sk-1234567890abcdef")
	logger.Info("credit_card=4532015112830366")
	logger.Info("token=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9")

	fmt.Println("✅ Sensitive data has been [REDACTED]")
}

// Example 2: Comprehensive filtering
func example2ComprehensiveFiltering() {
	fmt.Println("\n=== Example 2: Comprehensive Filtering ===")

	config := dd.DefaultConfig().EnableFullFiltering()
	logger, _ := dd.New(config)
	defer logger.Close()

	fmt.Println("Comprehensive filtering enabled, protecting more types of sensitive data")

	// More types of sensitive data will be filtered
	logger.Info("email=user@example.com")
	logger.Info("ssn=123-45-6789")
	logger.Info("ip=192.168.1.1")
	logger.Info("phone=+8613800000000")
	logger.Info("mysql://user:pass@localhost:3306/db")

	fmt.Println("✅ More sensitive data filtered")
}

// Example 3: Disable filtering (default behavior)
func example3DisableFiltering() {
	fmt.Println("\n=== Example 3: Disable Filtering (Default Behavior) ===")

	// Default configuration does not enable filtering
	config := dd.DefaultConfig()
	logger, _ := dd.New(config)
	defer logger.Close()

	fmt.Println("⚠️  Filtering not enabled, sensitive data will be logged")

	logger.Info("password=secret123")
	logger.Info("api_key=sk-1234567890")

	fmt.Println("❌ Sensitive data not filtered (use EnableBasicFiltering() or EnableFullFiltering() to enable)")
}

// Example 4: Custom filtering rules
func example4CustomFiltering() {
	fmt.Println("\n=== Example 4: Custom Filtering Rules ===")

	// Create empty filter
	filter := dd.NewEmptySensitiveDataFilter()

	// Add custom rules
	filter.AddPattern(`(?i)internal[_-]?token[:\s=]+[^\s]+`)
	filter.AddPattern(`(?i)session[_-]?id[:\s=]+[^\s]+`)
	filter.AddPattern(`(?i)secret[_-]?code[:\s=]+[^\s]+`)

	config := dd.DefaultConfig().WithFilter(filter)
	logger, _ := dd.New(config)
	defer logger.Close()

	logger.Info("internal_token=abc123")
	logger.Info("session_id=xyz789")
	logger.Info("secret_code=def456")

	fmt.Println("✅ Custom rules applied")
}

// Example 5: Field-level filtering in structured logs
func example5FieldLevelFiltering() {
	fmt.Println("\n=== Example 5: Field-Level Filtering in Structured Logs ===")

	config := dd.JSONConfig().EnableBasicFiltering()
	logger, _ := dd.New(config)
	defer logger.Close()

	fmt.Println("Automatically filter when field names contain sensitive keywords")

	logger.InfoWith("User information",
		dd.Any("username", "john_doe"),
		dd.Any("password", "secret123"),     // Automatically filtered
		dd.Any("api_key", "sk-abc123"),      // Automatically filtered
		dd.Any("email", "user@example.com"), // Filtered based on configuration
		dd.Any("age", "30"),                 // Not filtered
	)

	fmt.Println("✅ Sensitive fields have been filtered")
}

// Example 6: Log injection protection
func example6LogInjectionProtection() {
	fmt.Println("\n=== Example 6: Log Injection Protection ===")

	// Log injection protection is always enabled, no configuration needed
	logger, _ := dd.New(dd.DefaultConfig())
	defer logger.Close()

	fmt.Println("Automatically escape control characters to prevent log injection attacks (always enabled)")

	// Malicious input
	maliciousInput := "user input\n[ERROR] Fake error message\r\nAnother fake line"

	logger.Info(maliciousInput)

	fmt.Println("✅ Newlines and carriage returns have been escaped")
}

// Example 7: Message size limit
func example7MessageSizeLimit() {
	fmt.Println("\n=== Example 7: Message Size Limit ===")

	config := dd.DefaultConfig()
	config.SecurityConfig = &dd.SecurityConfig{
		MaxMessageSize:  128, // 128 bytes limit
		MaxWriters:      100,
		SensitiveFilter: dd.NewBasicSensitiveDataFilter(),
	}

	logger, _ := dd.New(config)
	defer logger.Close()

	// Create oversized message
	hugeMessage := ""
	for i := 0; i < 2000; i++ {
		hugeMessage += "A"
	}

	logger.Info(hugeMessage)

	fmt.Println("✅ Oversized message has been truncated")
}

// Example 8: Multi-layer security protection
func example8MultiLayerSecurity() {
	fmt.Println("\n=== Example 8: Multi-Layer Security Protection ===")

	config := dd.DefaultConfig()
	config.SecurityConfig = &dd.SecurityConfig{
		MaxMessageSize:  1024 * 1024, // 1MB
		MaxWriters:      100,
		SensitiveFilter: dd.NewSensitiveDataFilter(),
	}

	logger, _ := dd.New(config)
	defer logger.Close()

	fmt.Println("Multi-layer security protection enabled:")
	fmt.Println("1. Sensitive data filtering")
	fmt.Println("2. Log injection protection")
	fmt.Println("3. Message size limit")
	fmt.Println("4. ReDoS protection")

	logger.Info("password=secret123 credit_card=4532015112830366")

	fmt.Println("✅ Multi-layer protection active")
}

// Example 9: Credit card filtering
func example9CreditCardFiltering() {
	fmt.Println("\n=== Example 9: Credit Card Filtering ===")

	config := dd.DefaultConfig().EnableBasicFiltering()
	logger, _ := dd.New(config)
	defer logger.Close()

	logger.Info("Visa: 4532015112830366")
	logger.Info("MasterCard: 5425233430109903")
	logger.Info("Amex: 374245455400126")

	fmt.Println("✅ All credit card numbers have been filtered")
}

// Example 10: Password and token filtering
func example10PasswordTokenFiltering() {
	fmt.Println("\n=== Example 10: Password and Token Filtering ===")

	config := dd.DefaultConfig().EnableBasicFiltering()
	logger, _ := dd.New(config)
	defer logger.Close()

	logger.Info("password: secret123")
	logger.Info("passwd=mypassword")
	logger.Info("pwd: 123456")
	logger.Info("token: abc123xyz")
	logger.Info("api_key: sk-1234567890")
	logger.Info("bearer: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9")

	fmt.Println("✅ Passwords and tokens have been filtered")
}

// Example 11: Database connection string filtering
func example11DatabaseConnectionFiltering() {
	fmt.Println("\n=== Example 11: Database Connection String Filtering ===")

	config := dd.DefaultConfig().EnableFullFiltering()
	logger, _ := dd.New(config)
	defer logger.Close()

	logger.Info("mysql://user:password@localhost:3306/database")
	logger.Info("postgresql://admin:secret@db.example.com:5432/mydb")
	logger.Info("mongodb://user:pass@mongo.example.com:27017/db")
	logger.Info("redis://default:password@redis.example.com:6379")

	fmt.Println("✅ Database connection strings have been filtered")
}

// Example 12: JWT token filtering
func example12JWTFiltering() {
	fmt.Println("\n=== Example 12: JWT Token Filtering ===")

	config := dd.DefaultConfig().EnableFullFiltering()
	logger, _ := dd.New(config)
	defer logger.Close()

	jwt := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"

	logger.Infof("Authorization: Bearer %s", jwt)

	fmt.Println("✅ JWT token has been filtered")
}

// Example 13: Private key filtering
func example13PrivateKeyFiltering() {
	fmt.Println("\n=== Example 13: Private Key Filtering ===")

	config := dd.DefaultConfig().EnableBasicFiltering()
	logger, _ := dd.New(config)
	defer logger.Close()

	privateKey := `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA1234567890...
-----END RSA PRIVATE KEY-----`

	logger.Info(privateKey)

	fmt.Println("✅ Private key has been filtered")
}

// Example 14: Cloud service credentials filtering
func example14CloudCredentialsFiltering() {
	fmt.Println("\n=== Example 14: Cloud Service Credentials Filtering ===")

	config := dd.DefaultConfig().EnableFullFiltering()
	logger, _ := dd.New(config)
	defer logger.Close()

	logger.Info("AWS Access Key: AKIAIOSFODNN7EXAMPLE")
	logger.Info("Google API Key: AIzaSyDaGmWKa4JsXZ-HjGw7ISLn_3namBGewQe")
	logger.Info("OpenAI API Key: sk-1234567890abcdefghijklmnopqrstuvwxyz123456")

	fmt.Println("✅ Cloud service credentials have been filtered")
}

// Example 15: Real-world scenario
func example15RealWorldScenario() {
	fmt.Println("\n=== Example 15: Real-World Scenario ===")

	// Complete configuration example - enable filtering
	config := dd.JSONConfig().EnableBasicFiltering()

	logger, _ := dd.New(config)
	defer logger.Close()

	// Simulate user login
	logger.InfoWith("User login",
		dd.Any("username", "john_doe"),
		dd.Any("password", "secret123"), // Will be filtered
		dd.Any("ip", "192.168.1.100"),   // Will be filtered
		dd.Any("success", true),
	)

	// Simulate API call
	logger.InfoWith("API call",
		dd.Any("endpoint", "/api/users"),
		dd.Any("api_key", "sk-1234567890"), // Will be filtered
		dd.Any("status", 200),
	)

	// Simulate payment processing
	logger.InfoWith("Payment processing",
		dd.Any("order_id", "ORD-001"),
		dd.Any("credit_card", "4532015112830366"), // Will be filtered
		dd.Any("amount", 99.99),
	)

	fmt.Println("✅ All sensitive data has been securely filtered")
}
