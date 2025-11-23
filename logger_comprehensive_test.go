package dd

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestLoggerExtremeConditions tests logger behavior under extreme conditions
func TestLoggerExtremeConditions(t *testing.T) {
	t.Run("very_large_message", func(t *testing.T) {
		var buf bytes.Buffer
		config := DefaultConfig()
		config.Writers = []io.Writer{&buf}
		config.SecurityConfig = &SecurityConfig{
			MaxMessageSize: 10 * 1024 * 1024, // 10MB
		}

		logger, err := New(config)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}
		defer logger.Close()

		// Create 5MB message
		largeMsg := strings.Repeat("A", 5*1024*1024)
		logger.Info(largeMsg)

		// Should handle large message without panic
		if buf.Len() == 0 {
			t.Error("Logger should handle large messages")
		}
	})

	t.Run("many_fields", func(t *testing.T) {
		var buf bytes.Buffer
		config := JSONConfig()
		config.Writers = []io.Writer{&buf}

		logger, err := New(config)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}
		defer logger.Close()

		fields := make([]Field, 1000)
		for i := 0; i < 1000; i++ {
			fields[i] = Int("field"+string(rune(i)), i)
		}

		logger.InfoWith("test message", fields...)

		// Should handle many fields without panic
		if buf.Len() == 0 {
			t.Error("Logger should handle many fields")
		}
	})

	t.Run("high_concurrency", func(t *testing.T) {
		config := DefaultConfig()
		config.Writers = []io.Writer{io.Discard}

		logger, err := New(config)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}
		defer logger.Close()

		var wg sync.WaitGroup
		numGoroutines := 1000
		messagesPerGoroutine := 100

		start := time.Now()

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < messagesPerGoroutine; j++ {
					logger.Infof("goroutine %d message %d", id, j)
				}
			}(i)
		}

		wg.Wait()
		duration := time.Since(start)

		if duration > 30*time.Second {
			t.Errorf("High concurrency test took too long: %v", duration)
		}
	})

	t.Run("rapid_level_changes", func(t *testing.T) {
		var buf bytes.Buffer
		config := DefaultConfig()
		config.Writers = []io.Writer{&buf}

		logger, err := New(config)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}
		defer logger.Close()

		var wg sync.WaitGroup

		// Rapidly change levels
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10000; i++ {
				logger.SetLevel(LogLevel(i % 5))
			}
		}()

		// Log while levels are changing
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10000; i++ {
				logger.Info("test message")
			}
		}()

		wg.Wait()

		// Should not panic
	})
}

// TestLoggerMemoryBehavior tests memory-related behavior
func TestLoggerMemoryBehavior(t *testing.T) {
	t.Run("no_memory_leak_on_repeated_creation", func(t *testing.T) {
		// Create and close many loggers
		for i := 0; i < 1000; i++ {
			logger, err := New(DefaultConfig())
			if err != nil {
				t.Fatalf("Failed to create logger: %v", err)
			}
			logger.Info("test message")
			logger.Close()
		}

		// Should complete without excessive memory growth
		// (actual memory leak detection would require runtime.MemStats)
	})

	t.Run("buffer_pool_reuse", func(t *testing.T) {
		var buf bytes.Buffer
		config := DefaultConfig()
		config.Writers = []io.Writer{&buf}

		logger, err := New(config)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}
		defer logger.Close()

		// Log many messages to trigger buffer pool reuse
		for i := 0; i < 10000; i++ {
			logger.Info("test message")
		}

		// Should complete efficiently
		if buf.Len() == 0 {
			t.Error("Should have output")
		}
	})
}

// TestLoggerEdgeCases tests edge cases and corner scenarios
func TestLoggerEdgeCases(t *testing.T) {
	t.Run("empty_message", func(t *testing.T) {
		var buf bytes.Buffer
		config := DefaultConfig()
		config.Writers = []io.Writer{&buf}

		logger, err := New(config)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}
		defer logger.Close()

		logger.Info("")

		// Should handle empty message
		if buf.Len() == 0 {
			t.Error("Should log empty message")
		}
	})

	t.Run("nil_fields", func(t *testing.T) {
		var buf bytes.Buffer
		config := DefaultConfig()
		config.Writers = []io.Writer{&buf}

		logger, err := New(config)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}
		defer logger.Close()

		logger.InfoWith("test", Any("key", nil))

		// Should handle nil field value
		if buf.Len() == 0 {
			t.Error("Should log message with nil field")
		}
	})

	t.Run("special_characters_in_message", func(t *testing.T) {
		var buf bytes.Buffer
		config := DefaultConfig()
		config.Writers = []io.Writer{&buf}

		logger, err := New(config)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}
		defer logger.Close()

		// Test various special characters
		logger.Info("test\nmessage\rwith\tspecial\x00chars")

		output := buf.String()

		// Should sanitize special characters
		if strings.Contains(output, "\x00") {
			t.Error("Should sanitize null bytes")
		}
	})

	t.Run("unicode_in_message", func(t *testing.T) {
		var buf bytes.Buffer
		config := DefaultConfig()
		config.Writers = []io.Writer{&buf}

		logger, err := New(config)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}
		defer logger.Close()

		logger.Info("测试消息 🚀 тест")

		// Should handle unicode
		if buf.Len() == 0 {
			t.Error("Should log unicode message")
		}
	})

	t.Run("very_long_field_key", func(t *testing.T) {
		var buf bytes.Buffer
		config := DefaultConfig()
		config.Writers = []io.Writer{&buf}

		logger, err := New(config)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}
		defer logger.Close()

		longKey := strings.Repeat("a", 1000)
		logger.InfoWith("test", String(longKey, "value"))

		// Should handle long field keys
		if buf.Len() == 0 {
			t.Error("Should log message with long field key")
		}
	})
}

// TestLoggerStateTransitions tests state transition scenarios
func TestLoggerStateTransitions(t *testing.T) {
	t.Run("log_after_close", func(t *testing.T) {
		var buf bytes.Buffer
		config := DefaultConfig()
		config.Writers = []io.Writer{&buf}

		logger, err := New(config)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}

		logger.Info("before close")
		initialLen := buf.Len()

		logger.Close()
		logger.Info("after close")

		// Should not log after close
		if buf.Len() != initialLen {
			t.Error("Should not log after close")
		}
	})

	t.Run("add_writer_after_close", func(t *testing.T) {
		logger, err := New(DefaultConfig())
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}

		logger.Close()

		var buf bytes.Buffer
		err = logger.AddWriter(&buf)

		// Should return error
		if err == nil {
			t.Error("Should return error when adding writer after close")
		}
	})

	t.Run("multiple_closes", func(t *testing.T) {
		logger, err := New(DefaultConfig())
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}

		// Multiple closes should not panic
		logger.Close()
		logger.Close()
		logger.Close()
	})
}

// TestLoggerConcurrentOperations tests various concurrent operations
func TestLoggerConcurrentOperations(t *testing.T) {
	t.Run("concurrent_add_remove_writers", func(t *testing.T) {
		logger, err := New(DefaultConfig())
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}
		defer logger.Close()

		var wg sync.WaitGroup
		numOps := 100

		// Concurrent add
		for i := 0; i < numOps; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				var buf bytes.Buffer
				logger.AddWriter(&buf)
			}()
		}

		// Concurrent remove
		for i := 0; i < numOps; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				var buf bytes.Buffer
				logger.RemoveWriter(&buf)
			}()
		}

		// Concurrent log
		for i := 0; i < numOps; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				logger.Info("test message")
			}()
		}

		wg.Wait()

		// Should not panic or deadlock
	})

	t.Run("concurrent_security_config_changes", func(t *testing.T) {
		logger, err := New(DefaultConfig())
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}
		defer logger.Close()

		var wg sync.WaitGroup

		// Concurrent set
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				config := &SecurityConfig{
					MaxMessageSize: 1024 * id,
					MaxWriters:     10 + id,
				}
				logger.SetSecurityConfig(config)
			}(i)
		}

		// Concurrent get
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = logger.GetSecurityConfig()
			}()
		}

		// Concurrent log
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				logger.Info("test message")
			}()
		}

		wg.Wait()

		// Should not panic or race
	})
}

// Test helper types

type panicWriter struct{}

func (pw *panicWriter) Write(p []byte) (int, error) {
	panic("writer panic")
}

type failingWriter struct{}

func (fw *failingWriter) Write(p []byte) (int, error) {
	return 0, errors.New("write failed")
}

type slowWriter struct {
	delay time.Duration
}

func (sw *slowWriter) Write(p []byte) (int, error) {
	time.Sleep(sw.delay)
	return len(p), nil
}

type countingWriter struct {
	count atomic.Int64
}

func (cw *countingWriter) Write(p []byte) (int, error) {
	cw.count.Add(1)
	return len(p), nil
}
