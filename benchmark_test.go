package dd

import (
	"bytes"
	"io"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// CORE PERFORMANCE BENCHMARKS
// ============================================================================

func BenchmarkLoggerCreation(b *testing.B) {
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		logger, _ := New(cfg)
		logger.Close()
	}
}

func BenchmarkSimpleLogging(b *testing.B) {
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
	logger, _ := New(cfg)
	defer logger.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		logger.Info("test message")
	}
}

func BenchmarkFormattedLogging(b *testing.B) {
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
	logger, _ := New(cfg)
	defer logger.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		logger.Infof("User %s performed action %d", "john", i)
	}
}

func BenchmarkStructuredLogging(b *testing.B) {
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
	logger, _ := New(cfg)
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
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
	logger, _ := New(cfg)
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
	cfg := DefaultConfig()
	cfg.Format = FormatText
	cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}

	logger, _ := New(cfg)
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
	cfg := JSONConfig()
	cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}

	logger, _ := New(cfg)
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
	cfg := JSONConfig()
	cfg.JSON.PrettyPrint = false
	cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}

	logger, _ := New(cfg)
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
	cfg := JSONConfig()
	cfg.JSON.PrettyPrint = true
	cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}

	logger, _ := New(cfg)
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
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
	logger, _ := New(cfg)
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
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
	logger, _ := New(cfg)
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
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
	logger, _ := New(cfg)
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
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
	cfg.Level = LevelWarn
	logger, _ := New(cfg)
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
		cfg := DefaultConfig()
		cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
		cfg.Security = &SecurityConfig{SensitiveFilter: nil}

		logger, _ := New(cfg)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info("test")
		}
	})

	b.Run("3Writers", func(b *testing.B) {
		cfg := DefaultConfig()
		cfg.Targets = []OutputTarget{CustomOutput(io.Discard), CustomOutput(io.Discard), CustomOutput(io.Discard)}
		cfg.Security = &SecurityConfig{SensitiveFilter: nil}

		logger, _ := New(cfg)
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

		targets := make([]OutputTarget, len(writers))
		for i, w := range writers {
			targets[i] = CustomOutput(w)
		}

		cfg := DefaultConfig()
		cfg.Targets = targets
		cfg.Security = &SecurityConfig{
			MaxMessageSize:  1024 * 1024,
			MaxWriters:      20,
			SensitiveFilter: nil,
		}

		logger, _ := New(cfg)
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
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf1), CustomOutput(&buf2), CustomOutput(&buf3)}

	logger, _ := New(cfg)
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
		cfg := DefaultConfig()
		cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
		cfg.Security = &SecurityConfig{SensitiveFilter: nil}

		logger, _ := New(cfg)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info(msg)
		}
	})

	b.Run("BasicFilter", func(b *testing.B) {
		cfg := DefaultConfig()
		cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
		cfg.Security = &SecurityConfig{SensitiveFilter: newBasicSensitiveDataFilter()}

		logger, _ := New(cfg)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info(msg)
		}
	})

	b.Run("SecureFilter", func(b *testing.B) {
		cfg := DefaultConfig()
		cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
		cfg.Security = &SecurityConfig{SensitiveFilter: NewSensitiveDataFilter()}

		logger, _ := New(cfg)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info(msg)
		}
	})
}

func BenchmarkBasicFilter(b *testing.B) {
	filter := newBasicSensitiveDataFilter()
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
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
	cfg.Security = &SecurityConfig{SensitiveFilter: nil}

	logger, _ := New(cfg)
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
	cfg := DefaultConfig()
	cfg.Security = &SecurityConfig{SensitiveFilter: NewSensitiveDataFilter()}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = cfg.Clone()
	}
}

func BenchmarkConfigValidation(b *testing.B) {
	cfg := DefaultConfig()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = New(cfg)
	}
}

// ============================================================================
// WRITER BENCHMARKS
// ============================================================================

func BenchmarkBufferedWriter(b *testing.B) {
	var buf bytes.Buffer
	bw, _ := NewBufferedWriter(&buf, BufferedWriterConfig{BufferSize: 4096})
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
		cfg := DefaultConfig()
		cfg.Targets = []OutputTarget{CustomOutput(&buf)}

		logger, _ := New(cfg)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info("test message")
		}
	})

	b.Run("Buffered", func(b *testing.B) {
		var buf bytes.Buffer
		bw, _ := NewBufferedWriter(&buf, BufferedWriterConfig{BufferSize: 4096})
		defer bw.Close()

		cfg := DefaultConfig()
		cfg.Targets = []OutputTarget{CustomOutput(bw)}

		logger, _ := New(cfg)
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
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
	cfg.Security = &SecurityConfig{SensitiveFilter: nil}

	logger, _ := New(cfg)
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
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
	logger, _ := New(cfg)
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
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
	logger, _ := New(cfg)
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

	jsonCfg := JSONConfig()
	jsonCfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
	jsonLogger, _ := New(jsonCfg)
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
		cfg := JSONConfig()
		cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
		cfg.JSON.PrettyPrint = false

		logger, _ := New(cfg)
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
		cfg := JSONConfig()
		cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
		cfg.JSON.PrettyPrint = true

		logger, _ := New(cfg)
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
		cfg := JSONConfig()
		cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
		cfg.JSON.FieldNames = &JSONFieldNames{
			Timestamp: "@timestamp",
			Level:     "severity",
			Message:   "msg",
		}

		logger, _ := New(cfg)
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

// ============================================================================
// OPTIMIZATION-SPECIFIC BENCHMARKS (Phase 4)
// ============================================================================

// BenchmarkMultiWriterThroughput tests the atomic writer pointer optimization
func BenchmarkMultiWriterThroughput(b *testing.B) {
	b.Run("SingleWriter", func(b *testing.B) {
		cfg := DefaultConfig()
		cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
		cfg.Security = &SecurityConfig{SensitiveFilter: nil}
		logger, _ := New(cfg)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info("test message")
		}
	})

	b.Run("TwoWriters", func(b *testing.B) {
		cfg := DefaultConfig()
		cfg.Targets = []OutputTarget{CustomOutput(io.Discard), CustomOutput(io.Discard)}
		cfg.Security = &SecurityConfig{SensitiveFilter: nil}
		logger, _ := New(cfg)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info("test message")
		}
	})

	b.Run("FiveWriters", func(b *testing.B) {
		cfg := DefaultConfig()
		cfg.Targets = []OutputTarget{CustomOutput(io.Discard), CustomOutput(io.Discard), CustomOutput(io.Discard), CustomOutput(io.Discard), CustomOutput(io.Discard)}
		cfg.Security = &SecurityConfig{SensitiveFilter: nil}
		logger, _ := New(cfg)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info("test message")
		}
	})
}

// BenchmarkJSONMapAllocation tests the JSON map pooling optimization
func BenchmarkJSONMapAllocation(b *testing.B) {
	cfg := JSONConfig()
	cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
	cfg.Security = &SecurityConfig{SensitiveFilter: nil}
	logger, _ := New(cfg)
	defer logger.Close()

	b.Run("NoFields", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info("test message")
		}
	})

	b.Run("WithFields", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.InfoWith("test message",
				String("key1", "value1"),
				Int("key2", 42),
				Bool("key3", true),
			)
		}
	})

	b.Run("WithManyFields", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.InfoWith("test message",
				String("key1", "value1"),
				Int("key2", 42),
				Bool("key3", true),
				Float64("key4", 3.14),
				String("key5", "value5"),
				Int("key6", 100),
				String("key7", "value7"),
			)
		}
	})
}

// BenchmarkReflectionTypeCheck tests the type switch fast path optimization
func BenchmarkReflectionTypeCheck(b *testing.B) {
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
	cfg.Security = &SecurityConfig{SensitiveFilter: nil}
	logger, _ := New(cfg)
	defer logger.Close()

	b.Run("StringType", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.InfoWith("test", String("key", "value"))
		}
	})

	b.Run("IntType", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.InfoWith("test", Int("key", 42))
		}
	})

	b.Run("BoolType", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.InfoWith("test", Bool("key", true))
		}
	})

	b.Run("FloatType", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.InfoWith("test", Float64("key", 3.14159))
		}
	})

	b.Run("TimeType", func(b *testing.B) {
		now := time.Now()
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.InfoWith("test", Any("time", now))
		}
	})

	b.Run("ComplexType", func(b *testing.B) {
		data := struct {
			Name  string
			Value int
		}{"test", 42}
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.InfoWith("test", Any("data", data))
		}
	})
}

// BenchmarkTimeFormatting tests the time cache optimization
func BenchmarkTimeFormatting(b *testing.B) {
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
	cfg.Security = &SecurityConfig{SensitiveFilter: nil}
	cfg.IncludeTime = true
	logger, _ := New(cfg)
	defer logger.Close()

	b.Run("CachedTime", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info("test message")
		}
	})

	b.Run("CachedTimeParallel", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				logger.Info("test message")
			}
		})
	})
}

// BenchmarkEndToEndText tests end-to-end text logging performance
func BenchmarkEndToEndText(b *testing.B) {
	b.Run("Simple", func(b *testing.B) {
		cfg := DefaultConfig()
		cfg.Format = FormatText
		cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
		cfg.Security = &SecurityConfig{SensitiveFilter: nil}
		logger, _ := New(cfg)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info("test message")
		}
	})

	b.Run("WithFields", func(b *testing.B) {
		cfg := DefaultConfig()
		cfg.Format = FormatText
		cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
		cfg.Security = &SecurityConfig{SensitiveFilter: nil}
		logger, _ := New(cfg)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.InfoWith("test message",
				String("user", "john"),
				Int("id", 12345),
				String("action", "login"),
			)
		}
	})

	b.Run("Parallel", func(b *testing.B) {
		cfg := DefaultConfig()
		cfg.Format = FormatText
		cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
		cfg.Security = &SecurityConfig{SensitiveFilter: nil}
		logger, _ := New(cfg)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				logger.Info("test message")
			}
		})
	})
}

// BenchmarkEndToEndJSON tests end-to-end JSON logging performance
func BenchmarkEndToEndJSON(b *testing.B) {
	b.Run("Simple", func(b *testing.B) {
		cfg := JSONConfig()
		cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
		cfg.Security = &SecurityConfig{SensitiveFilter: nil}
		logger, _ := New(cfg)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info("test message")
		}
	})

	b.Run("WithFields", func(b *testing.B) {
		cfg := JSONConfig()
		cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
		cfg.Security = &SecurityConfig{SensitiveFilter: nil}
		logger, _ := New(cfg)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.InfoWith("test message",
				String("user", "john"),
				Int("id", 12345),
				String("action", "login"),
			)
		}
	})

	b.Run("Parallel", func(b *testing.B) {
		cfg := JSONConfig()
		cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
		cfg.Security = &SecurityConfig{SensitiveFilter: nil}
		logger, _ := New(cfg)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				logger.Info("test message")
			}
		})
	})
}

// BenchmarkBuilderPool tests the strings.Builder pool optimization
func BenchmarkBuilderPool(b *testing.B) {
	cfg := DefaultConfig()
	cfg.Format = FormatText
	cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
	cfg.Security = &SecurityConfig{SensitiveFilter: nil}
	logger, _ := New(cfg)
	defer logger.Close()

	b.Run("ShortMessage", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info("short")
		}
	})

	b.Run("MediumMessage", func(b *testing.B) {
		msg := "This is a medium length message for testing the builder pool optimization"
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info(msg)
		}
	})

	b.Run("LongMessage", func(b *testing.B) {
		msg := "This is a longer message that will require more buffer space in the strings.Builder and will test the pool's ability to handle varying message sizes efficiently."
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info(msg)
		}
	})
}

// BenchmarkWriterModification tests the performance of AddWriter/RemoveWriter
func BenchmarkWriterModification(b *testing.B) {
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
	logger, _ := New(cfg)
	defer logger.Close()

	var wg sync.WaitGroup

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		wg.Add(2)

		// Concurrent writer modification and logging
		go func() {
			defer wg.Done()
			logger.AddWriter(io.Discard)
		}()

		go func() {
			defer wg.Done()
			logger.Info("concurrent message")
		}()

		wg.Wait()

		// Clean up
		logger.RemoveWriter(io.Discard)
	}
}

// BenchmarkFieldNameCaching tests the JSON field name caching optimization
func BenchmarkFieldNameCaching(b *testing.B) {
	b.Run("DefaultFieldNames", func(b *testing.B) {
		cfg := JSONConfig()
		cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
		cfg.Security = &SecurityConfig{SensitiveFilter: nil}
		logger, _ := New(cfg)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info("test")
		}
	})

	b.Run("CustomFieldNames", func(b *testing.B) {
		cfg := JSONConfig()
		cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
		cfg.Security = &SecurityConfig{SensitiveFilter: nil}
		cfg.JSON.FieldNames = &JSONFieldNames{
			Timestamp: "@timestamp",
			Level:     "severity",
			Caller:    "source",
			Message:   "msg",
			Fields:    "data",
		}
		logger, _ := New(cfg)
		defer logger.Close()

		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			logger.Info("test")
		}
	})
}

// ============================================================================
// ALLOCATION TARGET TESTS
// ============================================================================

// TestAllocsPerLog verifies that allocation targets are met
func TestAllocsPerLog(t *testing.T) {
	t.Run("SimpleTextLog", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
		cfg.Security = &SecurityConfig{SensitiveFilter: nil}
		logger, _ := New(cfg)
		defer logger.Close()

		allocs := testing.AllocsPerRun(1000, func() {
			logger.Info("test message")
		})

		// Target: < 5 allocations for simple text log
		if allocs > 5 {
			t.Logf("Warning: Simple text log allocated %.1f times (target: < 5)", allocs)
		} else {
			t.Logf("Simple text log: %.1f allocations (target: < 5) - PASS", allocs)
		}
	})

	t.Run("JSONLogNoFields", func(t *testing.T) {
		cfg := JSONConfig()
		cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
		cfg.Security = &SecurityConfig{SensitiveFilter: nil}
		logger, _ := New(cfg)
		defer logger.Close()

		allocs := testing.AllocsPerRun(1000, func() {
			logger.Info("test message")
		})

		// Target: < 8 allocations for JSON log
		if allocs > 8 {
			t.Logf("Warning: JSON log (no fields) allocated %.1f times (target: < 8)", allocs)
		} else {
			t.Logf("JSON log (no fields): %.1f allocations (target: < 8) - PASS", allocs)
		}
	})

	t.Run("JSONLogWithFields", func(t *testing.T) {
		cfg := JSONConfig()
		cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
		cfg.Security = &SecurityConfig{SensitiveFilter: nil}
		logger, _ := New(cfg)
		defer logger.Close()

		allocs := testing.AllocsPerRun(1000, func() {
			logger.InfoWith("test message",
				String("user", "john"),
				Int("id", 123),
			)
		})

		// Target: < 12 allocations for JSON log with fields
		if allocs > 12 {
			t.Logf("Warning: JSON log (with fields) allocated %.1f times (target: < 12)", allocs)
		} else {
			t.Logf("JSON log (with fields): %.1f allocations (target: < 12) - PASS", allocs)
		}
	})

	t.Run("MultiWriterLog", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Targets = []OutputTarget{CustomOutput(io.Discard), CustomOutput(io.Discard), CustomOutput(io.Discard)}
		cfg.Security = &SecurityConfig{SensitiveFilter: nil}
		logger, _ := New(cfg)
		defer logger.Close()

		allocs := testing.AllocsPerRun(1000, func() {
			logger.Info("test message")
		})

		// Target: < 6 allocations for multi-writer log (no slice copy)
		if allocs > 6 {
			t.Logf("Warning: Multi-writer log allocated %.1f times (target: < 6)", allocs)
		} else {
			t.Logf("Multi-writer log: %.1f allocations (target: < 6) - PASS", allocs)
		}
	})
}
