# Changelog

All notable changes to the DD logging library will be documented in this file.

## [v1.0.0] - 2025-11-23

### Initial Release

High-performance Go logging library with zero external dependencies.

#### Core Features
- **High Performance**: 190K+ ops/sec simple logging, 140K+ structured logging, 940K+ concurrent
- **Thread-Safe**: Lock-free atomic operations, fully concurrent-safe
- **Zero Dependencies**: Only Go 1.24+ standard library
- **Multiple Formats**: Text (human-readable) and JSON (machine-parseable)
- **Structured Logging**: Type-safe fields with `InfoWith()`, `ErrorWith()`, etc.
- **Log Levels**: Debug, Info, Warn, Error, Fatal with dynamic adjustment

#### Output Management
- **File Rotation**: Auto-rotate by size/time with configurable limits
- **Compression**: Automatic .gz compression of rotated logs
- **Cleanup**: Auto-delete expired log files based on age
- **Multiple Writers**: Console, file, and custom writer support
- **Buffered Writes**: Optional buffering for high-throughput scenarios

#### Security Features
- **Sensitive Data Filtering**: Basic (6 patterns) and Full (12 patterns) modes
  - Credit cards, SSN, passwords, API keys, JWT tokens, AWS keys, etc.
- **Custom Patterns**: Add custom regex patterns for domain-specific filtering
- **Injection Prevention**: Automatic newline/control character sanitization
- **Message Size Limits**: Configurable max message size (default 5MB)
- **Path Traversal Protection**: Secure file path validation

#### Configuration
- **Preset Configs**: `DefaultConfig()`, `DevelopmentConfig()`, `JSONConfig()`
- **Convenience Constructors**: `ToFile()`, `ToConsole()`, `ToJSONFile()`, `ToAll()`
- **Options Pattern**: Flexible `NewWithOptions()` for fine-grained control
- **Chained Configuration**: Fluent API with `WithLevel()`, `WithFormat()`, etc.

#### Advanced Features
- **Dynamic Caller Detection**: Auto-adapt call stack depth for wrapper functions
- **Custom JSON Field Names**: Adapt to different log aggregation systems
- **Custom Fatal Handler**: Control Fatal-level behavior and exit codes
- **Global Default Logger**: Package-level convenience functions
- **Graceful Shutdown**: Proper resource cleanup with timeout handling

#### Performance Optimizations
- Object pools (`sync.Pool`) for buffer reuse
- Pre-allocated buffers to minimize allocations
- Atomic operations instead of mutexes in hot paths
- Single-writer fast path optimization
- Lazy formatting only when needed

---

**Note**: This is the first stable release. Future versions will maintain backward compatibility within the v1.x series.
