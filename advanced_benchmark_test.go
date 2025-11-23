package dd

import (
	"bytes"
	"io"
	"testing"
)

// ============================================================================
// ADVANCED BENCHMARK TESTS - Detailed Performance Analysis
// ============================================================================

// BenchmarkLogLevels benchmarks different log levels
func BenchmarkLogLevels(b *testing.B) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}

	logger, _ := New(config)
	defer logger.Close()

	b.Run("Debug", func(b *testing.B) {
		logger.SetLevel(LevelDebug)
		b.ResetTimer()
		b.ReportAllocs()
		for b.Loop() {
			logger.Debug("test")
		}
	})

	b.Run("Info", func(b *testing.B) {
		logger.SetLevel(LevelInfo)
		b.ResetTimer()
		b.ReportAllocs()
		for b.Loop() {
			logger.Info("test")
		}
	})

	b.Run("Warn", func(b *testing.B) {
		logger.SetLevel(LevelWarn)
		b.ResetTimer()
		b.ReportAllocs()
		for b.Loop() {
			logger.Warn("test")
		}
	})

	b.Run("Error", func(b *testing.B) {
		logger.SetLevel(LevelError)
		b.ResetTimer()
		b.ReportAllocs()
		for b.Loop() {
			logger.Error("test")
		}
	})
}

// BenchmarkFieldTypes benchmarks different field types
func BenchmarkFieldTypes(b *testing.B) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}

	logger, _ := New(config)
	defer logger.Close()

	b.Run("String", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.InfoWith("test", String("key", "value"))
		}
	})

	b.Run("Int", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.InfoWith("test", Int("key", 42))
		}
	})

	b.Run("Int64", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.InfoWith("test", Int64("key", int64(1234567890)))
		}
	})

	b.Run("Float64", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.InfoWith("test", Float64("key", 3.14159))
		}
	})

	b.Run("Bool", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.InfoWith("test", Bool("key", true))
		}
	})

	b.Run("Any", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.InfoWith("test", Any("key", []int{1, 2, 3}))
		}
	})
}

// BenchmarkMultipleFields benchmarks logging with multiple fields
func BenchmarkMultipleFields(b *testing.B) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}

	logger, _ := New(config)
	defer logger.Close()

	b.Run("1Field", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.InfoWith("test", String("k1", "v1"))
		}
	})

	b.Run("3Fields", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.InfoWith("test",
				String("k1", "v1"),
				Int("k2", 42),
				Bool("k3", true),
			)
		}
	})

	b.Run("5Fields", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.InfoWith("test",
				String("k1", "v1"),
				Int("k2", 42),
				Bool("k3", true),
				Float64("k4", 3.14),
				String("k5", "v5"),
			)
		}
	})

	b.Run("10Fields", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.InfoWith("test",
				String("k1", "v1"),
				Int("k2", 42),
				Bool("k3", true),
				Float64("k4", 3.14),
				String("k5", "v5"),
				Int("k6", 6),
				String("k7", "v7"),
				Bool("k8", false),
				Int64("k9", int64(999)),
				String("k10", "v10"),
			)
		}
	})
}

// BenchmarkMessageSizes benchmarks different message sizes
func BenchmarkMessageSizes(b *testing.B) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}
	config.SecurityConfig = &SecurityConfig{SensitiveFilter: nil}

	logger, _ := New(config)
	defer logger.Close()

	b.Run("Small_10B", func(b *testing.B) {
		msg := "small msg"
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info(msg)
		}
	})

	b.Run("Medium_100B", func(b *testing.B) {
		msg := string(make([]byte, 100))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info(msg)
		}
	})

	b.Run("Large_1KB", func(b *testing.B) {
		msg := string(make([]byte, 1024))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info(msg)
		}
	})

	b.Run("VeryLarge_10KB", func(b *testing.B) {
		msg := string(make([]byte, 10*1024))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info(msg)
		}
	})
}

// BenchmarkWriterCount benchmarks different writer counts
func BenchmarkWriterCount(b *testing.B) {
	b.Run("1Writer", func(b *testing.B) {
		config := DefaultConfig()
		config.Writers = []io.Writer{io.Discard}
		config.SecurityConfig = &SecurityConfig{SensitiveFilter: nil}

		logger, _ := New(config)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info("test")
		}
	})

	b.Run("3Writers", func(b *testing.B) {
		config := DefaultConfig()
		config.Writers = []io.Writer{io.Discard, io.Discard, io.Discard}
		config.SecurityConfig = &SecurityConfig{SensitiveFilter: nil}

		logger, _ := New(config)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info("test")
		}
	})

	b.Run("10Writers", func(b *testing.B) {
		writers := make([]io.Writer, 10)
		for i := range writers {
			writers[i] = io.Discard
		}

		config := DefaultConfig()
		config.Writers = writers
		config.SecurityConfig = &SecurityConfig{SensitiveFilter: nil}
		config.SecurityConfig = &SecurityConfig{
			MaxMessageSize:  1024 * 1024,
			MaxWriters:      20,
			SensitiveFilter: nil,
		}

		logger, _ := New(config)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info("test")
		}
	})
}

// BenchmarkFilterComparison benchmarks different filter configurations
func BenchmarkFilterComparison(b *testing.B) {
	msg := "User password: secret123 and card 4532015112830366"

	b.Run("NoFilter", func(b *testing.B) {
		config := DefaultConfig()
		config.Writers = []io.Writer{io.Discard}
		config.SecurityConfig = &SecurityConfig{SensitiveFilter: nil}

		logger, _ := New(config)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info(msg)
		}
	})

	b.Run("BasicFilter", func(b *testing.B) {
		config := DefaultConfig()
		config.Writers = []io.Writer{io.Discard}
		config.SecurityConfig = &SecurityConfig{SensitiveFilter: NewBasicSensitiveDataFilter()}

		logger, _ := New(config)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info(msg)
		}
	})

	b.Run("SecureFilter", func(b *testing.B) {
		config := DefaultConfig()
		config.Writers = []io.Writer{io.Discard}
		config.SecurityConfig = &SecurityConfig{SensitiveFilter: NewSensitiveDataFilter()}

		logger, _ := New(config)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info(msg)
		}
	})
}

// BenchmarkJSONOptions benchmarks different JSON configurations
func BenchmarkJSONOptions(b *testing.B) {
	b.Run("CompactJSON", func(b *testing.B) {
		config := JSONConfig()
		config.Writers = []io.Writer{io.Discard}
		config.JSON.PrettyPrint = false

		logger, _ := New(config)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.InfoWith("test",
				String("k1", "v1"),
				Int("k2", 42),
			)
		}
	})

	b.Run("PrettyJSON", func(b *testing.B) {
		config := JSONConfig()
		config.Writers = []io.Writer{io.Discard}
		config.JSON.PrettyPrint = true

		logger, _ := New(config)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.InfoWith("test",
				String("k1", "v1"),
				Int("k2", 42),
			)
		}
	})

	b.Run("CustomFieldNames", func(b *testing.B) {
		config := JSONConfig()
		config.Writers = []io.Writer{io.Discard}
		config.JSON.FieldNames = &JSONFieldNames{
			Timestamp: "@timestamp",
			Level:     "severity",
			Message:   "msg",
		}

		logger, _ := New(config)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.InfoWith("test",
				String("k1", "v1"),
				Int("k2", 42),
			)
		}
	})
}

// BenchmarkConcurrencyLevels benchmarks different concurrency levels
func BenchmarkConcurrencyLevels(b *testing.B) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}
	config.SecurityConfig = &SecurityConfig{SensitiveFilter: nil}

	logger, _ := New(config)
	defer logger.Close()

	b.Run("Sequential", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info("test")
		}
	})

	b.Run("Parallel", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				logger.Info("test")
			}
		})
	})
}

// BenchmarkHookOverhead benchmarks hook overhead
func BenchmarkHookOverhead(b *testing.B) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}

	b.Run("NoHooks", func(b *testing.B) {
		logger, _ := New(config)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info("test")
		}
	})

}

// BenchmarkBufferedVsUnbuffered benchmarks buffered vs unbuffered writing
func BenchmarkBufferedVsUnbuffered(b *testing.B) {
	b.Run("Unbuffered", func(b *testing.B) {
		var buf bytes.Buffer
		config := DefaultConfig()
		config.Writers = []io.Writer{&buf}

		logger, _ := New(config)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info("test message")
		}
	})

	b.Run("Buffered", func(b *testing.B) {
		var buf bytes.Buffer
		bw, _ := NewBufferedWriter(&buf, 4096)
		defer bw.Close()

		config := DefaultConfig()
		config.Writers = []io.Writer{bw}

		logger, _ := New(config)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info("test message")
		}
	})
}
