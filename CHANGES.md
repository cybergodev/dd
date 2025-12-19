# Changelog

All notable changes to the cybergodev/dd library will be documented in this file.

[//]: # (The format is based on [Keep a Changelog]&#40;https://keepachangelog.com/en/1.0.0/&#41;,)
[//]: # (and t his project adheres to [Semantic Versioning]&#40;https://semver.org/spec/v2.0.0.html&#41;.)

---

## v1.0.4 - Intelligent Type Conversion & Deep Optimization (2025-12-19)

### Added
- Intelligent type conversion system for `dd.Json()`, `dd.Jsonf()`, `dd.Text()`, `dd.Textf()`, `dd.Exit()`, `dd.Exitf()` methods
- TypeConverter with support for all Go types including complex scenarios (function types, channels, circular references)
- Enhanced type recognition: simple types display directly, complex types use JSON formatting, special types get safe conversion
- Circular reference detection to prevent infinite loops during type conversion
- Object pooling for type converters to reduce memory allocations

### Changed
- **Performance**: Deep optimization across logger.go, security.go, structured.go, debug_visual.go, convenience.go, config.go
- **Code Quality**: Comprehensive improvements to code quality, reliability, stability, and performance
- **Type Handling**: Perfect handling of function types, channel types, and circular references
- **Backward Compatibility**: All existing functionality preserved with no API changes, enhanced output quality

### Fixed
- JSON marshal errors for complex and unserializable types
- Handling of function types, channels, and other special Go types
- Memory efficiency in type conversion operations

### Performance
- Reduced memory allocations through object pooling for type converters
- Efficient circular reference detection algorithms
- Optimized type conversion reuse patterns
- Enhanced overall system performance and stability

---

## v1.0.3 - Major Performance & Security Optimization (2025-12-12)

### Added
- Object pooling for string builders and buffers to reduce memory allocations
- Fast-path field processing with batch operations for structured logging
- Character validation lookup table for improved field key validation performance
- Timeout protection for security filters to prevent DoS attacks
- Chunked processing for large inputs in security filters

### Changed
- **Performance**: Restructured Logger memory layout with atomic fields first for better cache performance
- **Performance**: Optimized message writing with single-writer fast path and concurrent multi-writer support
- **Performance**: Simplified caller depth detection, removed unreliable dynamic detection
- **Performance**: Reduced function call overhead in field processing with batch operations
- **Security**: Fixed regex DoS vulnerabilities using atomic groups and strict boundaries
- **Security**: Enhanced sensitive data filter patterns to prevent catastrophic backtracking
- **Code Quality**: Eliminated code duplication, especially in debug visualization
- **Code Quality**: Unified error handling patterns across all modules
- **Code Quality**: Modernized to Go 1.22+ range-over-int syntax
- **Reliability**: Improved fallback logger error handling to ensure always-available logging

### Fixed
- Regex DoS vulnerabilities in private key and JWT token filtering
- Test output noise and console spam during test execution
- Debug visualization caller path formatting for consistent test results
- Dynamic caller detection test stability issues

### Performance
- Simple logging: ~335 ns/op, 200 B/op, 7 allocs/op
- Structured logging: ~102K ns/op, 1533 B/op, 23 allocs/op
- Significant reduction in memory allocations and improved processing speed
- Test coverage: 84.6%

### Security
- Enhanced input validation and boundary checks
- Improved timeout handling for large input processing
- Strengthened regex patterns against ReDoS attacks
- Better resource management and cleanup

---

## v1.0.2 - Debug Visualization (2025-12-04)

### Added
- `dd.Json()` method for compact JSON output of data structures
- `dd.Text()` method for pretty-printed JSON output of data structures
- Debug visualization available as both package-level functions and Logger methods
- Example demonstrating debug data visualization patterns

---

## v1.0.1 - Performance & Security Update (2025-12-01)

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