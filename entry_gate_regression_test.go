package dd

import (
	"bytes"
	"sync/atomic"
	"testing"
)

// This file pins the gate semantics shared by the two logging layers
// ((*Logger) methods and LoggerEntry/package-level functions).

// TestEntryArgFamilyConsumesSingleSamplingSlot pins that one entry-layer log
// call consumes exactly one sampling slot. entryLogDispatch/entryLogfDispatch
// used to call shouldLog and then route through logWithLazyMessage, which
// gated again — every entry arg-family log advanced the sampling counter (and
// the rate limiter's message budget) twice, halving the effective budgets of
// entry methods relative to the (*Logger) families.
func TestEntryArgFamilyConsumesSingleSamplingSlot(t *testing.T) {
	// Initial=4, Thereafter=0: the first 4 gated calls pass, then everything
	// is dropped. A double-gated path exhausts the budget after 2 messages.
	newLogged := func(t *testing.T, log func(l *Logger)) int {
		t.Helper()
		rec := NewLoggerRecorder()
		logger, err := rec.NewLogger()
		if err != nil {
			t.Fatalf("NewLogger: %v", err)
		}
		defer logger.Close()

		logger.SetSampling(&SamplingConfig{Enabled: true, Initial: 4, Thereafter: 0})
		log(logger)
		return rec.Count()
	}

	if n := newLogged(t, func(l *Logger) {
		for range 3 {
			l.Info("msg") // control: single-gated path
		}
	}); n != 3 {
		t.Errorf("(*Logger).Info: %d/3 entries recorded, want 3", n)
	}

	loggers := map[string]func(*Logger){
		"entry.Info": func(l *Logger) {
			e := l.WithField("k", "v")
			for range 3 {
				e.Info("msg")
			}
		},
		"entry.Infof": func(l *Logger) {
			e := l.WithField("k", "v")
			for range 3 {
				e.Infof("msg %d", 1)
			}
		},
		"entry.Printf": func(l *Logger) {
			e := l.WithField("k", "v")
			for range 3 {
				e.Printf("msg %d", 1)
			}
		},
	}
	for name, log := range loggers {
		if n := newLogged(t, log); n != 3 {
			t.Errorf("%s: %d/3 entries recorded, want 3 (sampling budget consumed more than once per entry)", name, n)
		}
	}
}

// TestEntryArgFamilyConsumesSingleRateLimitSlot is the rate-limiter twin of
// TestEntryArgFamilyConsumesSingleSamplingSlot: with a 2-message/second budget
// and no burst, three entry logs must yield exactly two entries — the same
// result as three (*Logger) logs. (The double-gated path dropped the second
// message because each log spent two message slots.)
func TestEntryArgFamilyConsumesSingleRateLimitSlot(t *testing.T) {
	newLogger := func(t *testing.T) (*Logger, *LoggerRecorder) {
		t.Helper()
		rec := NewLoggerRecorder()
		logger, err := rec.NewLogger()
		if err != nil {
			t.Fatalf("NewLogger: %v", err)
		}
		logger.SetSecurityConfig(&SecurityConfig{
			SensitiveFilter: NewEmptySensitiveDataFilter(),
			RateLimitConfig: &RateLimitConfig{
				MaxMessagesPerSecond: 2,
				BurstSize:            0,
				MaxBytesPerSecond:    0,
			},
		})
		return logger, rec
	}

	l1, rec1 := newLogger(t)
	defer l1.Close()
	for range 3 {
		l1.Info("msg")
	}
	if n := rec1.Count(); n != 2 {
		t.Errorf("(*Logger).Info under 2-msg/s limit: %d entries recorded, want 2", n)
	}

	l2, rec2 := newLogger(t)
	defer l2.Close()
	e := l2.WithField("k", "v")
	for range 3 {
		e.Info("msg")
	}
	if n := rec2.Count(); n != 2 {
		t.Errorf("entry.Info under 2-msg/s limit: %d entries recorded, want 2", n)
	}
}

// TestFatalIsNeverDroppedByGate pins that a Fatal log always reaches
// handleFatal. shouldLog used to route Fatal through shouldSample() (and the
// closed check), so a sampling config of Initial=0/Thereafter=0 — or a closed
// logger — silently discarded the Fatal entry and the fatal handler never
// ran: the process kept running despite a documented must-exit contract.
func TestFatalIsNeverDroppedByGate(t *testing.T) {
	newFatalLogger := func(t *testing.T) (*Logger, *atomic.Bool) {
		t.Helper()
		called := &atomic.Bool{}
		logger, err := New(Config{
			Targets:      []OutputTarget{CustomOutput(&bytes.Buffer{})},
			FatalHandler: func() { called.Store(true) },
		})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		// Drop-everything sampling: without the Fatal exemption this discards
		// the entry before handleFatal.
		logger.SetSampling(&SamplingConfig{Enabled: true, Initial: 0, Thereafter: 0})
		return logger, called
	}

	t.Run("sampling cannot drop Fatal", func(t *testing.T) {
		logger, called := newFatalLogger(t)
		defer logger.Close()
		logger.Fatal("must terminate")
		if !called.Load() {
			t.Error("fatal handler did not run: Fatal was dropped by the sampling gate")
		}
	})

	t.Run("closed logger cannot drop Fatal", func(t *testing.T) {
		logger, called := newFatalLogger(t)
		if err := logger.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		logger.Fatal("must terminate")
		if !called.Load() {
			t.Error("fatal handler did not run: Fatal was dropped by the closed gate")
		}
	})
}
