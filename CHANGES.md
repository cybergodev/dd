# Changelog

All notable changes to the cybergodev/dd library will be documented in this file.

---

## v1.3.3 - Security Hardening, Correctness & Hot-Path Performance (2026-09-05)

### Fixed
- Symlink redirection attack on the log path: the documented protection was dead code (fstat follows symlinks) — now enforced via pre-open lstat and post-open identity re-check, so `ErrSymlinkNotAllowed` is actually returned
- Two redaction patterns never compiled (PEM private key, GCP service-account key exceeded Go's regex repeat limit) — both key types passed through unredacted
- Letter-only sensitive values (SWIFT, MRN, driver's license, connection strings, fingerprints) bypassed the redaction pre-gate; separator-less keyword forms (`dlnumber`, `socialinsurancenumber`, `fpid`) slipped past the pattern gates
- Structured log calls with map/slice/func-typed fields crashed the process when sensitive-data filtering was enabled
- Recursive field filtering failed OPEN on a panicking `String()`/`Error()` — sibling secret fields now stay redacted (fail-closed)
- Quoted text-format field values emitted raw control bytes — newline-splitting consumers could be fed forged log lines
- `IntegritySigner.Verify` panicked (slice bounds) when `SignaturePrefix` itself contained `]`
- Every audit-written signed event failed `Verify`/`VerifyAuditEvent` — the space separator leaked into the re-signed payload
- Strict/paranoid/industry presets silently ran ungated (gate-slice misalignment): redaction kept, the performance guarantee lost
- Large messages (>32 KB) were wholesale replaced with `[REDACTED]` on the fixed 50 ms filter deadline — the timeout now scales with input length
- A timed-out `WaitForGoroutines` permanently disabled sensitive-data filtering for all later log calls
- Panicking user callbacks no longer crash the process: log-arg `Stringer`/`error` values, `Err*` field constructors, hook error handler, write-error handler, `LevelResolver`, writer `Flush`/`Close`, and all background goroutines (autoflush, cleanup, compression, audit worker)

### Performance
- Steady-state logging performs zero library-side allocations (pooled line buffers end-to-end; SimpleLogging/ConcurrentLogging 1 → 0 allocs/op)
- Caller capture moved to fixed-skip entry dispatchers: SimpleLogging −68%, StructuredLogging −50%, TextFormat −48% ns/op
- Sound pattern gates skip provably unmatchable regexes: FormattedLogging 35.7µs → 1.9µs (−95%)
- Second-byte keyword index and fused token scan: FormattedLogging −12%, filter cache-hit path −21%, token-scan CPU share halved

### Changed
- Symlinked log paths now actually fail with `ErrSymlinkNotAllowed` at open/rotate (documented contract, previously unenforced)
- `LevelResolver` is now also invoked by every `IsLevelEnabled` check — resolvers run more frequently than before
- Invalid `AuditConfig`s now fail `Config.Validate`/`New` instead of being silently dropped (audit no longer silently disabled)
- `ClearFor` marked deprecated (exact alias of `Remove`)

---

## v1.3.2 - Hot-Path Performance & Redaction Safety (2026-06-16)

### Fixed
- Sensitive-data filter cache was dead in production — `SensitiveDataFilter.clone()` never initialized it, so every flagged message re-ran all 67 patterns; caching is now restored with no security boundary weakened
- Byte rate limiting (`MaxBytesPerSecond`) now actually takes effect — the hot path previously skipped the byte gate
- Fatal-level messages now bypass rate limiting so a fatal log is never silently dropped and the process always exits
- Large-input (>32 KB) sensitive-data redaction no longer corrupts output at chunk boundaries (single-scan `filterWithContext`)
- Data race on the rate limiter under concurrent `SetSecurityConfig` — the field is now an `atomic.Pointer`
- `sync.WaitGroup` Add-after-Wait race in `FileWriter.Close` during concurrent rotation
- `HookOnFilter` hooks now fire (previously registerable but silent no-ops); they carry only the field key, never the redacted value
- `FileWriter.SetOnRotateCallback` no longer races active rotations (callback read/written under the lock)
- `BufferedWriter.Write` returns `os.ErrClosed` after `Close()`, matching `FileWriter`
- Single-writer newline write routed through `safeWrite`, preserving the Fatal-exit and `AfterLog` hook guarantees
- `applySizeLimit` output no longer exceeds `MaxMessageSize`
- `SecureBuffer.Grow` no longer panics when `len < cap`
- `IntegrityConfig.MarshalJSON` handles a nil receiver without panicking
- Rate-limit byte gate rolls back the message-count slot on rejection (dropped messages no longer leak the quota)
- ReDoS detection no longer mis-flags bounded `{n,m}` quantifier ranges
- Multi-byte leading-rune handling in camelCase/PascalCase field-key validation

### Performance
- Caller resolution captures the stack once per log line: `SimpleLogging` −59%, `EndToEndText/Simple` −63%, `JSONFormat` −43% ns/op; allocations unchanged on the fieldless path
- Filter cache restored + `MatchString` gate before `ReplaceAllString`: digit-bearing messages −95% B/op and −97% allocs/op; filtered JSON −85% B/op, −97% allocs/op
- `processFields` is copy-on-write — no field-slice allocation when nothing is redacted (the common case)

### Changed
- `Config.Clone()` delegates the Audit copy to `AuditConfig.Clone()` (removes drift risk if `AuditConfig` gains a field)
- Package-level `dd.Text`/`dd.Textf` delegate to `Default()` — byte-identical behavior, removes the only unjustified duplication in the two-layer API
- Corrected Print-family and Text/JSON doc comments to distinguish filtered vs. raw-stdout behavior
- doc.go graceful-shutdown example now compiles; README perf-table corrected (Structured logging 5→4 allocs/op)
- Internal cleanup: consolidated field-validation case helpers, shared `callerForPC`, unified hex lookup table, removed a dead rate-limiter field

### Removed
- Unused `SanitizeUnicodeControlChars` and its helpers (`internal/` package — not a public-API change)
- Dead `LoggerRecorder.buf` field and other unused internal symbols / test-only helpers

---

## v1.3.1 - Production Safety & Quality Fixes (2026-05-07)

### Fixed
- Zero sensitive key material in `NewIntegritySigner` to prevent memory exposure
- Panic-safe buffer zeroing for all `IntegritySigner` pool operations
- UTF-8 corruption in size-limited output — truncate at valid rune boundaries
- Symlink TOCTOU during log rotation prevented with exclusive file creation (`O_EXCL`)
- Leading space in `FormatFields` when first field has empty key
- Nil pointer dereference in `FileWriter.Write` during concurrent `Close()`
- Byte count accounting in `RateLimiter` — rejected messages no longer inflate counter
- Signature bytes zeroed before pool return, preventing data leakage through reuse
- Nil receiver guard on `LoggerEntry` log methods to prevent panics
- 5 unchecked type assertions in atomic loads replaced with safe comma-ok pattern
- Panic recovery in `Shutdown()` and `handleFatal()` goroutines to prevent process crash
- Element count limit (10000) in `FilterValueRecursive` prevents memory exhaustion
- Abbreviation suffix matching now validates prefix convention

### Changed
- `RateLimiter` integrated into Logger pipeline via `SecurityConfig.RateLimitConfig`
- `AuditLogger` integrated into Logger pipeline via `Config.Audit` for automatic event emission
- Audit events emitted for sensitive data redactions and rate-limit actions
- `FileWriter` triggers `HookOnRotate` callback after successful rotation
- `SecurityConfig` and `Config` extended for rate limiting and audit configuration

### Added
- `SetOnRotateCallback()` method on `FileWriter` for rotation notification
- `OpenFileExclusive()` for symlink-safe file creation
- 29 new boundary and coverage tests; 8 caller tests consolidated to table-driven format

---

## v1.3.0 - API Unification, Performance & Quality (2026-04-16)

### Breaking Changes
- Removed `FileConfig` struct, `Config.Output`, `Config.Outputs`, `Config.File` — use `Config.Targets` with `OutputTarget` helpers
- Removed convenience functions (`ToFile`, `ToConsole`, `ToAll`, `ToAllJSON`, `ToWriter`, `ToWriters`, `ToJSONFile`) — use `New(Config)` with `Targets`
- Removed all `*WithConfig` constructors — inlined into main constructors (`New`, `NewFileWriter`, `NewBufferedWriter`, `NewAuditLogger`, `NewIntegritySigner`)
- Removed `DefaultIntegrityConfig()` (panicked on entropy failure) — use `DefaultIntegrityConfigSafe()`
- Privatized 49 internal identifiers (`ErrCode*` constants, `NewError`, `WrapError`, hook helpers, registry methods)
- Privatized `ContextExtractorRegistry` → use `Config.ContextExtractors` and `AddContextExtractor()`
- Privatized `WaitForBackgroundCloses()` → internal coordination only
- `New(cfgs ...*Config)` → `New(cfg ...Config)` (value type, no pointer)
- `NewAuditLogger` returns `(*AuditLogger, error)` instead of `*AuditLogger`
- `LoggerRecorder.NewLogger` returns `(*Logger, error)` instead of `*Logger`
- `NewFileWriter(path, cfg ...FileWriterConfig)` → `NewFileWriter(path, cfg FileWriterConfig)` (non-variadic)
- `NewBufferedWriter(w, bufferSizes ...int)` → `NewBufferedWriter(w, cfg BufferedWriterConfig)` (Config struct)
- `NewIntegritySigner(cfg ...IntegrityConfig)` → `NewIntegritySigner(cfg IntegrityConfig)` (non-variadic)
- Config functions return values: `DefaultConfig()`, `Clone()`, `DefaultAuditConfig()` etc. no longer return pointers
- Exported `Config.Validate()` replaces private `Config.validate()`
- Removed `validateErrorCodeMapping()` (was a no-op)
- Removed `namedErr()` deprecated alias — use `ErrWithKey()`

### Added
- `OutputTarget` struct with `ConsoleOutput()`, `FileOutput(path)`, `CustomOutput(w)` helpers
- `OutputType` enum (`OutputConsole`, `OutputFile`, `OutputCustom`)
- `BufferedWriterConfig` struct with `DefaultBufferedWriterConfig()` and `Validate()`
- `FileWriterConfig.Validate()` public validation method
- `AuditConfig.Validate()` and `IntegrityConfig.Validate()` methods
- `Config.Targets []OutputTarget` field for unified output configuration
- `OutputTarget.resolve()` method converts `OutputTarget` to `io.Writer`
- Panic recovery guards in all public logging paths (`logCoreWithDepth`, `FilterValueRecursive`, `safeWrite`)
- Nil-receiver guards on `ClearPatterns()` and all public `Logger` methods
- Pre-compiled security patterns at init time — eliminates runtime panics in `SecurityConfigForLevel()`
- `WaitForBackgroundCloses(timeout)` for background close goroutine coordination
- Godoc comments on all exported types and methods (`Logger`, `LogLevel`, `FileWriter`, `MultiWriter`, `SensitiveDataFilter`, etc.)
- `examples/11_testing.go` — LoggerRecorder usage example
- `resource_leak_test.go` — 7 resource leak verification tests
- `boundary_test.go` — 50+ boundary condition test cases
- `loadHooks()`, `loadContextExtractors()`, `loadSamplingState()` typed accessors for cleaner atomic access

### Changed
- Context extractors now invoked in logging pipeline (previously stored but never called)
- `Logger.Close()` calls `WaitForFilterGoroutines()` before closing writers
- `SensitiveDataFilter.Close()` releases cache map on shutdown
- `AuditLogger.Close()` clears statistics map on shutdown
- HMAC hasher pool replaced with direct `hmac.New()` per operation (fixes nil-key reuse)
- `SensitiveDataFilter.Clone()` now initializes all fields including `goroutineCond` and cache
- Error sentinels (`ErrSymlinkNotAllowed`, `ErrHardlinkNotAllowed`, `ErrOverlongEncoding`) now match via `errors.Is()`
- `MultiWriter.Close()` idempotent via `atomic.Bool` guard
- `FileWriter` closed-flag prevents `Write()` after `Close()`
- Standard streams cached at init time (fixes race with concurrent `os.Stdout` modification)
- Hash seed initialization uses `sync.Once` for thread safety
- `incrementTypeCount()` counter race fixed — new counters start at 0
- `shouldLog()` simplified — eliminated duplicated level-check branches
- `Doc.go` package examples updated to use `Targets` API
- All test and example files updated to use `Targets` instead of deprecated `Output`/`File` fields

### Fixed
- `time.Time` values converted to empty map `{}` by security filter — now preserved as RFC3339 timestamps
- Context extractor fields bypassed sensitive data filtering (security gap)
- `Shutdown()` closed writers after canceling context (reversed order — deadline semantics broken)
- `LoggerRecorder` double-entry bug from whitespace-only writes on single-writer fast path
- `SanitizeControlChars` incomplete 0xEF BOM handling (only appended first byte)
- `mergeFieldSlices` exceeded 2× field limit (only capped `newFields`, not `existingFields`)
- `SecureBuffer.Grow` incomplete zeroing during reallocation (only zeroed up to len, not cap)
- `IntegritySigner` mutated caller's `SignaturePrefix` via non-deep-copied config
- `SensitiveDataFilter.Clone()` nil pointer panic on `WaitForGoroutines` (zero-value `sync.Cond`)
- Divide-by-zero in `handleRateLimited()` when `SamplingRate <= 0`
- Redundant full-string regex pass in `filterInChunksWithContext` (overlap chunking already covers boundaries)
- `shouldSample()` race condition — counter reset and increment now atomic within same mutex scope
- `LoggerEntry` methods panicked on nil logger receiver
- `NewLogger`/`NewLoggerWithConfig` silently swallowed errors, could return nil `*Logger`
- README: incorrect API references (wrong function names, missing error returns, nonexistent types)
- Example code: missing `defer Close()`, silently discarded errors, wrong API usage
- `LoadOrStore` race path ineffectual assignment in `caller.go`
- Multiple lint issues (errcheck, QF1001, QF1008, SA9003)

### Performance
- Single-pass `SanitizeControlChars` — +37% faster for large messages
- Direct field writing to text builder — eliminates 1 string allocation per log with fields
- `formatJSONDirect()` fast path — eliminates 2 map allocations for simple JSON logs
- Zero-copy string-to-bytes via `unsafe.Slice` for single-writer path — eliminates pool Get/Put
- Pooled buffers: `sanitizeBufferPool`, `chunkFilterBufPool`, `signDataPool`, `auditEncoderPool`
- Type-switch fast paths for common map key types (avoids `fmt.Sprintf` for string/int/float/bool)
- Bulk scan + `WriteString` for non-escaped JSON strings with lookup-table escape detection
- Pre-allocated slices in error paths and writer operations
- 65 of 82 benchmarks improved, 0 meaningful regressions
- JSON/Simple: up to −80% allocations (5→1), −27% memory, −20% latency
- Text/WithFields: −25% allocations, −14% memory, −8% latency

### Removed
- `convenience.go` file (single `DefaultLogPath` constant merged into `constants.go`)
- `validateErrorCodeMapping()` no-op function
- `namedErr()` deprecated function (replaced by `ErrWithKey()`)
- `structured.go` deprecated `NamedErr` function
- `DefaultLogPath` from `convenience.go` → moved to `constants.go`
- 7+ duplicate test functions (~190 lines)
- ~65 lines of dead code and unused helpers

---

## v1.2.2 - Security Hardening & API Cleanup (2026-03-21)

### Breaking Changes
- Removed all `*Ctx` methods from `Logger`, `LoggerEntry`, and package-level functions; use `ContextExtractors` with `*With()` methods instead
- Removed all `Must*` methods/functions to enforce explicit error handling (`Must`, `MustNew`, `MustToFile`, `MustAddHook`, etc.)
- Removed `HookBuilder` fluent API; use struct-based `HooksConfig` with `NewHooksFromConfig()` instead
- Removed `ContextLogger` interface from public API

### Security
- Filter cache now limits to ≤128 bytes with full input verification to prevent hash collision attacks
- Fixed weak HMAC verification in `verifyLegacy()` that truncated signature comparison
- Fixed time cache race condition using atomic Compare-And-Swap for timestamp formatting
- JSON encoder now escapes `<`, `>`, `&` as `\u003c`, `\u003e`, `\u0026` to prevent XSS in HTML contexts
- Pooled buffers now zero sensitive data before return to prevent memory residue
- Added JSON depth limit (100 levels) to prevent stack overflow from deeply nested structures

### Added
- `HooksConfig` struct and `NewHooksFromConfig()` for struct-based hook configuration
- `FilterStats.CacheHits` and `CacheMiss` fields for cache effectiveness monitoring
- `PERFORMANCE_OPTIMIZATION.md` with detailed pprof analysis and optimization recommendations
- `visitedMapPool` for memory reuse in recursive sensitive data filtering

### Changed
- Logger implementation split into 11 focused module files for maintainability (~1759 lines)
- `WaitForGoroutines()` uses `sync.Cond` instead of busy-wait for CPU efficiency
- `SetSampling()` now copies input config to avoid mutating caller's data
- `HookRegistry.Clone()` correctly copies `errorHandler` field
- `SensitiveDataFilter.Clone()` documentation clarifies shared patterns slice behavior
- `NamedErr()` deprecated; use `ErrWithKey()` instead
- Cache TTL uses `time.Time` instead of Unix timestamp for accurate calculation

### Fixed
- BufferWriter.Close() nil pointer dereference when checking for io.Closer
- Out-of-bounds access in `validateNoADS()` URL scheme validation
- Fallback logger missing `sampling` state initialization
- Security pattern false positives for routing numbers and SWIFT/BIC codes
- Configuration validation side effect that modified input config
- SecureBuffer.Grow incomplete zeroing during reallocation
- Caller depth overflow with protection limit of 1000 levels

### Performance
- HookContext lazy allocation saves ~145 bytes per log call when no hooks registered
- Small field merge uses linear search (≤4 new, ≤8 existing) to avoid map allocation
- Direct caller path extraction avoids string allocation from filepath.Base
- Pre-computed `commonSuffixes` eliminates repeated slice allocation
- Buffer pool capacities increased: textBuilder 1024→2048, argsBuilder 256→512, FieldBuilder 384→768
- Overall memory reduction: SimpleLogging -19%, StructuredLogging -12%, ConcurrentLogging -19%

### Removed
- `NewFullSensitiveDataFilter()` deprecated alias; use `NewSensitiveDataFilter()` directly
- Unused `filteredFieldsPool` variable and `bytesToString()` function

---

## v1.2.1 - Performance & Quality Improvements (2026-03-05)

### Added
- Package-level `Print()`, `Println()`, `Printf()` functions for quick console output
- `Print()`, `Println()`, `Printf()` methods for `LoggerEntry` (created via `WithFields()`)
- `FatalWith()` package-level function to match `Logger.FatalWith()` method
- `sync.Pool` optimizations for caller depth detection, JSON encoding, and field filtering

### Changed
- `DefaultConfig()` now enables `DynamicCaller: true` by default for accurate caller locations
- JSON encoding uses fast path for simple types, avoiding reflection overhead
- Integrity signature format enhanced to include timestamp and sequence for replay attack prevention

### Fixed
- Dynamic caller detection now correctly shows user code location instead of internal logger code
- Caller depth calculation offset for accurate file:line display
- Integrity signature verification now validates all signed data components
- Sensitive data detection at truncation boundaries to prevent leakage
- Redaction count accuracy after truncation
- FileWriter rotation error recovery when `OpenFile` fails after successful rename
- Package-level Print functions documentation (LevelInfo not LevelDebug)

### Performance
- JSON format: 19-28% faster, 47-67% fewer allocations
- Buffer pooling reduces GC pressure for high-frequency logging

### Testing
- Internal package coverage: 47.5% → 84.9% (+37.4%)
- Main package coverage: 73.7% → 78.5% (+4.8%)
- 35+ new test cases with table-driven approach

---

## v1.2.0 - Enhance Security & Performance & API Unification (2026-03-01)

### Added
- Audit logging system with async event processing and integrity verification
- HMAC-based log integrity signing and verification (`IntegritySigner`)
- Rate limiting for log flooding prevention (token bucket algorithm)
- Secure memory types: `SecureBuffer`, `SecureString`, `SecureBytes` with `WipeBytes()`
- Context extractors for custom context field extraction (`ContextExtractorRegistry`)
- Lifecycle hook system with `HookBeforeLog`, `HookAfterLog`, `HookOnClose`, `HookOnError`
- Dynamic level resolver for runtime log level adjustment based on context
- Field key validation with naming convention support (snake_case, camelCase, PascalCase, kebab-case)
- Enterprise security presets: `HealthcareConfig()`, `FinancialConfig()`, `GovernmentConfig()`
- Convenience constructors: `ToFile()`, `ToJSONFile()`, `ToConsole()`, `ToAll()`, `MustVal[T]()`
- `WithFields()` / `WithField()` for field inheritance and chaining
- Log sampling support with Initial/Thereafter/Tick configuration
- `GetFilterStats()` for filter performance monitoring
- `IsLevelEnabled()` and `IsXxxEnabled()` convenience methods
- Package-level context-aware logging functions (`DebugCtx`, `InfoCtx`, `WarnCtx`, `ErrorCtx`)
- `Shutdown(ctx)` method for graceful shutdown with timeout support
- `DefaultIntegrityConfigSafe()` panic-free alternative

### Changed
- **BREAKING**: Removed `NewConfig()` - use `DefaultConfig()` instead
- **BREAKING**: Removed `SecureConfig()` - use `DefaultSecureConfig()` instead
- **BREAKING**: Removed `DefaultSecurityConfigDisabled()` - use `SecurityConfigForLevel(SecurityLevelDevelopment)`
- **BREAKING**: Removed `Config.Build()` / `Config.MustBuild()` - use `dd.New(cfg)` / `dd.Must(cfg)`
- **BREAKING**: Removed functional options API (`options.go`) - use struct-based `Config`
- **BREAKING**: Removed `FilterLevel` enum - use filter constructors directly
- Security filtering now enabled by default in all configurations
- `SensitiveDataFilter.Clone()` shares immutable patterns slice (56% memory reduction)
- Multi-writer uses atomic pointer for lock-free read access (15-25% improvement)
- Time formatting uses lock-free atomic pointer cache (30-40% less contention)
- Default integrity key uses cryptographically secure random generation
- Optional parameters: `NewFileWriter(path)`, `NewBufferedWriter(w)`, `NewAuditLogger()`, `NewIntegritySigner()`

### Fixed
- Critical: `LoggerEntry.Logf` format strings were not being formatted
- Race condition in `incrementTypeCount` with concurrent audit logging
- Race condition in security filter cache access outside mutex
- `MultiWriter.Close()` now skips closing standard streams (stdout/stderr/stdin)
- CRLF injection vulnerability - newline/carriage return now escaped
- Message pool memory leak with oversized buffers
- Hook registry race condition in concurrent `AddHook` calls
- Nil context handling in level resolver
- Australia ABN pattern false positives on 11-digit numbers
- NPI pattern false positives - now requires context keywords

### Security
- UTF-8 overlong encoding detection to prevent path traversal bypass
- Hardlink detection to prevent log output redirection attacks
- Windows device name and ADS (Alternate Data Streams) validation
- Log4Shell detection with Unicode escape sequence support
- C1 control character handling (U+0080-U+009F)
- Bounded regex quantifiers to prevent ReDoS attacks (max 1000)
- Circular reference detection in recursive field filtering
- Recursion depth limit (100) to prevent stack overflow
- Panic recovery in hooks and context extractors
- Goroutine leak protection with concurrent filter limit (100)
- IPv6 address filtering added

### Performance
- SimpleLogging: 27.6% faster (232ns → 168ns)
- StructuredLogging: 52.9% faster (2646ns → 1246ns)
- JSONFormat: 34.3% faster (6394ns → 4201ns)
- ConfigClone: 56% less memory (1072B → 472B)
- Pattern matching uses binary search (9.9% CPU reduction)
- Field slice copy only when hooks registered
- `clear()` builtin replaces `delete` loops for map clearing

### Removed
- `WipeString` function (no-op for immutable Go strings)
- `options.go` file with all functional options
- Deprecated constructors: `NewWithOptions()`, `FileLogger()`, `ConsoleLogger()`, `JSONFileLogger()`, `MultiLogger()`
- `FilterLevel` type and constants (`FilterNone`, `FilterBasic`, `FilterFull`)

---

## v1.1.1 - Critical Bug Fixes & API Refinement (2026-01-22)

### Fixed
- **Race Condition**: Fixed concurrent initialization issue in Default() logger using sync.Once pattern
- **Error Handling**: Convenience functions (ToFile, ToJSONFile, ToConsole, ToAll) now panic on initialization failure instead of silently failing
- **Memory Leak**: Fixed silent error handling in CleanupOldFiles, now properly reports cleanup errors
- **Test Race Condition**: Fixed data race in concurrent writer test using mutex-protected buffer
- **Documentation Accuracy**: Corrected function names (Json → JSON, Jsonf → JSONF) and added behavior warnings

### Changed
- **Text/Textf Functions**: Removed caller information for cleaner output (focused on data content only)
- **Other Debug Functions**: JSON, JSONF, Exit, Exitf retain caller information as before

### Test Results
- All tests pass ✓
- Race detector clean ✓
- No regressions introduced ✓

---

## v1.1.0 - [Stable version] Comprehensive Testing, Documentation & Quality Enhancement (2026-01-16)

### Added
  - All config chain methods (WithLevel, WithFormat, WithDynamicCaller, filtering methods)
  - All logger instance methods (Print, Println, Printf, Text, Textf, Json, Jsonf)
  - Enhanced fmt package functions (NewErrorWith, PrintfWith, PrintlnWith)
  - Security filter control (Enable/Disable/IsEnabled)
  - Complex type formatting (slices, maps, nested structures)
  - File rotation and compression triggers
  - JSON options customization
  - Dynamic caller detection
  - Edge cases and error handling

- **Log Level Alignment**: Improved visual organization of text log output
  - Fixed-width padding for log levels (DEBUG, INFO, WARN, ERROR, FATAL)
  - Consistent spacing between timestamp and level
  - Cleaner, more organized log appearance
  - Easier log scanning and parsing

- **Logger Instance fmt Methods**: Print(), Println(), Printf() on logger instances
  - Consistent with package-level functions
  - All include caller information
  - Full feature parity between instance and package-level APIs

### Changed
- **API Consistency**: Print() now an alias for Println() (both add spaces and newlines)
  - Simplifies API by eliminating confusion
  - Prioritizes developer convenience over strict fmt compatibility
  - Better matches common usage patterns

- **Enhanced fmt Package**: All fmt replacement methods now include caller information
  - Printf() at both package and instance level
  - Consistent debugging experience across all console output
  - Better traceability for all output methods

- **Logger Instance Methods**: Text/Json/Textf/Jsonf now output directly to stdout
  - Consistent behavior with package-level functions
  - Include caller information
  - Unified debugging experience

- **Log Output Format**: Timestamp and level wrapped in brackets
  - `[2026-01-16T17:40:46+08:00  INFO]` format
  - Better visual separation of metadata and message
  - Easier parsing and log analysis

### Fixed
- **Documentation Accuracy**: Comprehensive README.md verification and corrections
  - Added missing API entries (Json, Jsonf, Text, Textf, Exit, Exitf)
  - Updated structured logging example with proper field types

- **Code Quality**: Comprehensive optimization and refactoring
  - Removed over-engineering and redundant code (~100 lines eliminated)
  - Fixed TOCTOU vulnerability in symlink validation
  - Fixed resource leaks in compression (proper defer usage)
  - Added ReDoS protection (regex complexity validation)
  - Improved error handling in SetLevel()
  - Consolidated duplicate security patterns

- **Performance**: Optimizations while maintaining all functionality
  - Lock-free Default() initialization
  - Better string building with strings.Builder
  - Simplified TypeConverter (removed pool overhead)
  - Direct pattern application for security filters

- **Security**: Critical vulnerability fixes
  - TOCTOU attack prevention in file operations
  - ReDoS attack prevention with pattern validation
  - Resource leak elimination in compression
  - Better symlink validation

### Removed
- **Redundant Code**: ~65 lines of unused helper functions
- **Excessive Comments**: Cleaned up obvious/redundant comments
- **Duplicate Implementations**: Consolidated pattern definitions

### Improved
- **Code Organization**: Better separation of concerns
  - Validation separated from default value application
  - Simplified backup path building logic

- **Maintainability**: Centralized pattern registry
  - Single source of truth for security patterns
  - Easier to add/modify patterns
  - Better DRY principle adherence

### Performance Impact
- **Test Coverage**:  77% (+10%)
- **Code Quality**: Significantly improved with comprehensive test suite
- **Backward Compatibility**: 100% maintained (except Print() behavior change)
- **Documentation**: 100% accurate (verified against implementation)

---