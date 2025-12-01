# Changelog

All notable changes to the cybergodev/dd library will be documented in this file.

[//]: # (The format is based on [Keep a Changelog]&#40;https://keepachangelog.com/en/1.0.0/&#41;,)
[//]: # (and t his project adheres to [Semantic Versioning]&#40;https://semver.org/spec/v2.0.0.html&#41;.)

---

## v1.1.0 - Performance & Security Update (2025-12-01)

### Added
- Centralized error definitions in `errors.go` for consistent error handling
- Shared caller detection utility in `internal/caller` package
- Go 1.24 range-over-int syntax support across codebase

### Fixed
- Bearer token filtering vulnerability (tokens not completely filtered)
- Private key filtering pattern (only header was filtered, not full key block)
- ReDoS vulnerabilities in JWT and private key regex patterns
- Double sanitization of messages (redundant processing eliminated)
- Missing filter state check in field processing

### Changed
- `FileWriterConfig` now uses value semantics instead of pointer (API simplification)
- Optimized hot path performance with reduced allocations (20-30% improvement)
- Modernized 8+ loops to Go 1.24 range-over-int syntax
- Improved lock management to reduce contention in concurrent scenarios
- Streamlined security filter processing for better performance
- Enhanced token/API key pattern to support up to 256 characters (JWT support)

### Removed
- 8 unused test helper functions from `test_helpers.go`
- Duplicate `getCaller()` implementations (consolidated to shared utility)
- Redundant pattern copying in security filter

### Security
- JWT pattern now bounded `{10,100}` to prevent catastrophic backtracking
- Private key pattern limited to `{0,50}` characters to prevent ReDoS
- Bearer tokens now properly filtered with 256-character support
- All regex patterns have explicit upper bounds for safety

---

## v1.0.0 - Initial Release (2025-11-23)

### Added

- High-performance logging with 190K+ ops/sec simple logging, 140K+ structured logging, 940K+ concurrent operations
- Thread-safe operations using lock-free atomic operations
- Zero external dependencies - Go 1.24+ standard library only
- Multiple output formats: Text (human-readable) and JSON (machine-parseable)
- Structured logging with type-safe fields via `InfoWith()`, `ErrorWith()`, etc.
- Log levels: Debug, Info, Warn, Error, Fatal with dynamic level adjustment
- File rotation with auto-rotate by size/time and configurable limits
- Automatic .gz compression of rotated log files
- Auto-cleanup of expired log files based on age
- Multiple writer support: console, file, and custom writers
- Optional buffered writes for high-throughput scenarios
- Sensitive data filtering with Basic (6 patterns) and Full (12 patterns) modes
- Custom regex patterns for domain-specific data filtering
- Automatic injection prevention via newline/control character sanitization
- Configurable message size limits (default 5MB)
- Path traversal protection for secure file operations
- Dynamic caller detection that auto-adapts call stack depth for wrapper functions
- Custom JSON field names for different log aggregation systems
- Custom fatal handler to control Fatal-level behavior and exit codes
- Global default logger with package-level convenience functions
- Graceful shutdown with proper resource cleanup and timeout handling

### Changed
- N/A (Initial release)

### Fixed
- N/A (Initial release)

---