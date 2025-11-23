package dd

import (
	"bytes"
	"io"
	"testing"
)

// ============================================================================
// BENCHMARK TESTS - Core Logging Performance
// ============================================================================

// BenchmarkLoggerCreation benchmarks logger creation
func BenchmarkLoggerCreation(b *testing.B) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		logger, _ := New(config)
		logger.Close()
	}
}

// BenchmarkSimpleLogging benchmarks simple logging
func BenchmarkSimpleLogging(b *testing.B) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}
	logger, _ := New(config)
	defer logger.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		logger.Info("test message")
	}
}

// BenchmarkFormattedLogging benchmarks formatted logging
func BenchmarkFormattedLogging(b *testing.B) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}
	logger, _ := New(config)
	defer logger.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		logger.Infof("User %s performed action %d", "john", i)
	}
}

// BenchmarkStructuredLogging benchmarks structured logging
func BenchmarkStructuredLogging(b *testing.B) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}
	logger, _ := New(config)
	defer logger.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		logger.InfoWith("User action",
			String("user", "john"),
			Int("action_id", i),
			Bool("success", true),
		)
	}
}

// BenchmarkStructuredLoggingComplex benchmarks complex structured logging
func BenchmarkStructuredLoggingComplex(b *testing.B) {
	logger, _ := New(nil)
	defer logger.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		logger.InfoWith("Complex event",
			String("user", "john_doe"),
			Int("user_id", 12345),
			Int("timestamp", 1234567890),
			Float64("amount", 99.99),
			Bool("verified", true),
			String("ip", "192.168.1.1"),
			String("action", "purchase"),
			Any("metadata", map[string]string{"key": "value"}),
		)
	}
}

// BenchmarkConcurrentLogging benchmarks concurrent logging
func BenchmarkConcurrentLogging(b *testing.B) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}
	logger, _ := New(config)
	defer logger.Close()

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info("concurrent message")
		}
	})
}

// BenchmarkConcurrentStructuredLogging benchmarks concurrent structured logging
func BenchmarkConcurrentStructuredLogging(b *testing.B) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}
	logger, _ := New(config)
	defer logger.Close()

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.InfoWith("concurrent",
				String("key", "value"),
				Int("num", 42),
			)
		}
	})
}

// BenchmarkLevelCheck benchmarks log level checking
func BenchmarkLevelCheck(b *testing.B) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}
	config.Level = LevelWarn
	logger, _ := New(config)
	defer logger.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// This should be filtered out
		logger.Debug("debug message")
	}
}

// BenchmarkMultipleWriters benchmarks logging with multiple writers
func BenchmarkMultipleWriters(b *testing.B) {
	var buf1, buf2, buf3 bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf1, &buf2, &buf3}

	logger, _ := New(config)
	defer logger.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		logger.Info("test message")
	}
}

// ============================================================================
// BENCHMARK TESTS - Format Performance
// ============================================================================

// BenchmarkTextFormat benchmarks text format
func BenchmarkTextFormat(b *testing.B) {
	config := DefaultConfig()
	config.Format = FormatText
	config.Writers = []io.Writer{io.Discard}

	logger, _ := New(config)
	defer logger.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		logger.InfoWith("test",
			String("key1", "value1"),
			Int("key2", 42),
		)
	}
}

// BenchmarkJSONFormat benchmarks JSON format
func BenchmarkJSONFormat(b *testing.B) {
	config := JSONConfig()
	config.Writers = []io.Writer{io.Discard}

	logger, _ := New(config)
	defer logger.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		logger.InfoWith("test",
			String("key1", "value1"),
			Int("key2", 42),
		)
	}
}

// BenchmarkJSONCompact benchmarks compact JSON
func BenchmarkJSONCompact(b *testing.B) {
	config := JSONConfig()
	config.JSON.PrettyPrint = false
	config.Writers = []io.Writer{io.Discard}

	logger, _ := New(config)
	defer logger.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		logger.InfoWith("test",
			String("key1", "value1"),
			Int("key2", 42),
			Float64("key3", 3.14),
			Bool("key4", true),
		)
	}
}

// BenchmarkJSONPretty benchmarks pretty JSON
func BenchmarkJSONPretty(b *testing.B) {
	config := JSONConfig()
	config.JSON.PrettyPrint = true
	config.Writers = []io.Writer{io.Discard}

	logger, _ := New(config)
	defer logger.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		logger.InfoWith("test",
			String("key1", "value1"),
			Int("key2", 42),
			Float64("key3", 3.14),
			Bool("key4", true),
		)
	}
}

// ============================================================================
// BENCHMARK TESTS - Field Operations
// ============================================================================

// BenchmarkFieldCreation benchmarks field creation
func BenchmarkFieldCreation(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = String("key", "value")
		_ = Int("num", 42)
		_ = Bool("flag", true)
		_ = Float64("pi", 3.14)
	}
}

// Note: formatFields is an internal function
// Field formatting performance is tested via structured logging benchmarks

// ============================================================================
// BENCHMARK TESTS - Security Features
// ============================================================================

// BenchmarkLoggingWithBasicFilter benchmarks logging with basic filter
func BenchmarkLoggingWithBasicFilter(b *testing.B) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}
	config.SecurityConfig = &SecurityConfig{SensitiveFilter: NewBasicSensitiveDataFilter()}

	logger, _ := New(config)
	defer logger.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		logger.Info("User password: secret123 and card 4532015112830366")
	}
}

// BenchmarkLoggingWithSecureFilter benchmarks logging with secure filter
func BenchmarkLoggingWithSecureFilter(b *testing.B) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}
	config.SecurityConfig = &SecurityConfig{SensitiveFilter: NewSensitiveDataFilter()}

	logger, _ := New(config)
	defer logger.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		logger.Info("User password: secret123 and card 4532015112830366")
	}
}

// BenchmarkLoggingWithoutFilter benchmarks logging without filter
func BenchmarkLoggingWithoutFilter(b *testing.B) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}
	config.SecurityConfig = &SecurityConfig{SensitiveFilter: nil}

	logger, _ := New(config)
	defer logger.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		logger.Info("User password: secret123 and card 4532015112830366")
	}
}

// BenchmarkBasicFilter benchmarks basic filter directly
func BenchmarkBasicFilter(b *testing.B) {
	filter := NewBasicSensitiveDataFilter()
	message := "User password: secret123 and card 4532015112830366"

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = filter.Filter(message)
	}
}

// BenchmarkSecureFilter benchmarks secure filter directly
func BenchmarkSecureFilter(b *testing.B) {
	filter := NewSensitiveDataFilter()
	message := "User password: secret123 and card 4532015112830366"

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = filter.Filter(message)
	}
}

// Note: sanitizeMessage and sanitizeFieldKey are internal functions
// Their performance is tested via end-to-end logging benchmarks

// ============================================================================
// BENCHMARK TESTS - Hooks
// ============================================================================

// ============================================================================
// BENCHMARK TESTS - Configuration
// ============================================================================

// BenchmarkConfigClone benchmarks configuration cloning
func BenchmarkConfigClone(b *testing.B) {
	config := DefaultConfig()
	config.SecurityConfig = &SecurityConfig{SensitiveFilter: NewSensitiveDataFilter()}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = config.Clone()
	}
}

// BenchmarkConfigValidation benchmarks configuration validation
func BenchmarkConfigValidation(b *testing.B) {
	config := DefaultConfig()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = config.Validate()
	}
}

// ============================================================================
// BENCHMARK TESTS - Writers
// ============================================================================

// BenchmarkBufferedWriter benchmarks buffered writer
func BenchmarkBufferedWriter(b *testing.B) {
	var buf bytes.Buffer
	bw, _ := NewBufferedWriter(&buf, 4096)
	defer bw.Close()

	data := []byte("test message\n")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		bw.Write(data)
	}
}

// BenchmarkMultiWriter benchmarks multi writer
func BenchmarkMultiWriter(b *testing.B) {
	var buf1, buf2, buf3 bytes.Buffer
	mw := NewMultiWriter(&buf1, &buf2, &buf3)

	data := []byte("test message\n")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		mw.Write(data)
	}
}

// ============================================================================
// BENCHMARK TESTS - Memory Allocation
// ============================================================================

// BenchmarkMemoryAllocation benchmarks memory allocation for different logging types
func BenchmarkMemoryAllocation(b *testing.B) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}
	logger, _ := New(config)
	defer logger.Close()

	b.Run("Simple", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info("test message")
		}
	})

	b.Run("Structured", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.InfoWith("test",
				String("key", "value"),
				Int("num", 42),
			)
		}
	})

	jsonConfig := JSONConfig()
	jsonConfig.Writers = []io.Writer{io.Discard}
	jsonLogger, _ := New(jsonConfig)
	defer jsonLogger.Close()

	b.Run("JSON", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			jsonLogger.InfoWith("test",
				String("key", "value"),
				Int("num", 42),
			)
		}
	})
}
