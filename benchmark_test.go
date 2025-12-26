package dd

import (
	"bytes"
	"io"
	"testing"
)

// ============================================================================
// CORE PERFORMANCE BENCHMARKS
// ============================================================================

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

// ============================================================================
// FORMAT PERFORMANCE BENCHMARKS
// ============================================================================

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
// FIELD PERFORMANCE BENCHMARKS
// ============================================================================

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

// ============================================================================
// LEVEL AND WRITER BENCHMARKS
// ============================================================================

func BenchmarkLogLevels(b *testing.B) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}
	logger, _ := New(config)
	defer logger.Close()

	b.Run("Debug", func(b *testing.B) {
		logger.SetLevel(LevelDebug)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Debug("test")
		}
	})

	b.Run("Info", func(b *testing.B) {
		logger.SetLevel(LevelInfo)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info("test")
		}
	})

	b.Run("Warn", func(b *testing.B) {
		logger.SetLevel(LevelWarn)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Warn("test")
		}
	})

	b.Run("Error", func(b *testing.B) {
		logger.SetLevel(LevelError)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Error("test")
		}
	})
}

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
// SECURITY PERFORMANCE BENCHMARKS
// ============================================================================

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

func BenchmarkBasicFilter(b *testing.B) {
	filter := NewBasicSensitiveDataFilter()
	message := "User password: secret123 and card 4532015112830366"

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = filter.Filter(message)
	}
}

func BenchmarkSecureFilter(b *testing.B) {
	filter := NewSensitiveDataFilter()
	message := "User password: secret123 and card 4532015112830366"

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = filter.Filter(message)
	}
}

// ============================================================================
// MESSAGE SIZE BENCHMARKS
// ============================================================================

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

// ============================================================================
// CONFIGURATION BENCHMARKS
// ============================================================================

func BenchmarkConfigClone(b *testing.B) {
	config := DefaultConfig()
	config.SecurityConfig = &SecurityConfig{SensitiveFilter: NewSensitiveDataFilter()}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = config.Clone()
	}
}

func BenchmarkConfigValidation(b *testing.B) {
	config := DefaultConfig()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = config.Validate()
	}
}

// ============================================================================
// WRITER BENCHMARKS
// ============================================================================

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

// ============================================================================
// CONCURRENCY BENCHMARKS
// ============================================================================

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

// ============================================================================
// MEMORY ALLOCATION BENCHMARKS
// ============================================================================

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

// ============================================================================
// JSON OPTIONS BENCHMARKS
// ============================================================================

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
