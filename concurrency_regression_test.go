package dd

// Concurrency regression tests for the P-002 concurrency audit.
// Each test guards a specific fixed defect; see the referenced file comments
// for the failure mode being prevented.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestRotateHookLoggingNoDeadlock guards against the FileWriter rotation
// deadlock: onRotate (which triggers HookOnRotate) used to fire while
// fw.mu was held, so a hook that logged to the same logger re-entered
// FileWriter.Write and blocked forever on the non-reentrant mutex.
func TestRotateHookLoggingNoDeadlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")

	fw, err := NewFileWriter(path, FileWriterConfig{MaxSizeMB: 1})
	if err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(fw)}
	// Disable message filtering: the 600KB chunk would otherwise be truncated
	// to 256KB by the filter (maxInputLength), so two writes never cross the
	// 1MB rotation threshold and the rotation path is not exercised.
	cfg.Security = &SecurityConfig{SensitiveFilter: nil}
	logger, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	hookFired := make(chan struct{}, 1)
	err = logger.AddHook(HookOnRotate, func(_ context.Context, _ *HookContext) error {
		select {
		case hookFired <- struct{}{}:
		default:
		}
		// The dangerous re-entry: a rotate hook that audit-logs the rotation
		// through the same logger.
		logger.Info("file was rotated")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// "ab " breaks up any base64 run, so the message skips the regex filter
	// entirely (a plain "xxx..." chunk trips the base64 heuristic and the
	// filter dominates the test runtime under -race, masking the deadlock).
	chunk := strings.Repeat("ab ", 200*1024) // ~600KB; 2nd write crosses the 1MB cap

	done := make(chan struct{}, 1)
	go func() {
		logger.Info(chunk)
		logger.Info(chunk) // triggers rotation -> HookOnRotate
		done <- struct{}{}
	}()

	select {
	case <-done:
		select {
		case <-hookFired:
		case <-time.After(2 * time.Second):
			t.Fatal("rotation hook never fired")
		}
	case <-time.After(10 * time.Second):
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		t.Fatalf("deadlock: Write blocked\n%s", buf[:n])
	}
	_ = logger.Close()
}

// TestRotateHookWriterManagementNoDeadlock guards the ABBA variant of the
// same defect: HookOnRotate firing under fw.mu while it calls AddWriter,
// which contends Logger.writersMu — a concurrent Logger.Close holds
// writersMu and waits on fw.mu, closing the cycle.
func TestRotateHookWriterManagementNoDeadlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")

	fw, err := NewFileWriter(path, FileWriterConfig{MaxSizeMB: 1})
	if err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(fw)}
	// See TestRotateHookLoggingNoDeadlock: keep the chunk intact so the
	// rotation threshold is actually crossed.
	cfg.Security = &SecurityConfig{SensitiveFilter: nil}
	logger, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	rotateDone := make(chan struct{})
	err = logger.AddHook(HookOnRotate, func(_ context.Context, _ *HookContext) error {
		defer close(rotateDone)
		// Writer management from within a rotate hook.
		w := &bytes.Buffer{}
		_ = logger.AddWriter(w)
		_ = logger.RemoveWriter(w)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// "ab " avoids the base64 heuristic; see TestRotateHookLoggingNoDeadlock.
	chunk := strings.Repeat("ab ", 200*1024)

	wrote := make(chan struct{}, 1)
	go func() {
		logger.Info(chunk)
		logger.Info(chunk) // rotation -> HookOnRotate -> AddWriter (writersMu)
		wrote <- struct{}{}
	}()

	// Concurrent Close (writersMu -> fw.mu) must not deadlock with the hook.
	closed := make(chan struct{}, 1)
	go func() {
		<-wrote
		_ = logger.Close()
		closed <- struct{}{}
	}()

	select {
	case <-rotateDone:
	case <-time.After(10 * time.Second):
		t.Fatal("rotation hook never completed")
	}
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("deadlock: concurrent Close and HookOnRotate/AddWriter")
	}
}

// closeTrackingWriter records whether Close was called.
type closeTrackingWriter struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	closed bool
}

func (w *closeTrackingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, os.ErrClosed
	}
	return w.buf.Write(p)
}

func (w *closeTrackingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	return nil
}

func (w *closeTrackingWriter) isClosed() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.closed
}

// TestMultiWriterCloseAddWriterRace guards the AddWriter/Close TOCTOU:
// MultiWriter.Close now takes mu when snapshotting the writers, so a writer
// whose AddWriter succeeded is guaranteed to be included in the close pass.
// Without the lock, an Add accepted just before Close's snapshot leaked
// (accepted but never closed).
func TestMultiWriterCloseAddWriterRace(t *testing.T) {
	const iterations = 500
	for i := 0; i < iterations; i++ {
		w := &closeTrackingWriter{}
		mw := NewMultiWriter()

		start := make(chan struct{})
		var addErr error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			addErr = mw.AddWriter(w)
		}()
		go func() {
			defer wg.Done()
			<-start
			_ = mw.Close()
		}()
		close(start)
		wg.Wait()

		if addErr == nil && !w.isClosed() {
			t.Fatalf("iteration %d: AddWriter succeeded but the writer was never closed by Close", i)
		}
	}
}

// TestWaitForGoroutinesConcurrentNoFalseSuccess guards against cross-talk
// between concurrent WaitForGoroutines calls: the abort flag is now
// per-call, so one caller's timeout can no longer wake another caller's
// helper goroutine and make it return true ("all completed") while filter
// goroutines are still active.
func TestWaitForGoroutinesConcurrentNoFalseSuccess(t *testing.T) {
	f := NewSensitiveDataFilter()

	// Simulate two long-running filter goroutines (the async filter path
	// increments this counter around its goroutine).
	f.activeGoroutines.Add(2)

	// The victim wait starts first; give its helper time to enter Cond.Wait.
	victimDone := make(chan bool, 1)
	go func() { victimDone <- f.WaitForGoroutines(500 * time.Millisecond) }()
	time.Sleep(50 * time.Millisecond)

	// A concurrent wait that times out immediately.
	go func() { f.WaitForGoroutines(time.Millisecond) }()

	// The victim must NOT report success while the counter is still 2.
	select {
	case ok := <-victimDone:
		if ok {
			t.Fatal("WaitForGoroutines returned true while goroutines were still active (cross-talk between concurrent waits)")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("victim WaitForGoroutines did not return in time")
	}

	// Drain the fake goroutines and close so the helper goroutine wakes up
	// (Close broadcasts on the cond) instead of leaking.
	f.activeGoroutines.Add(-2)
	_ = f.Close()
}

// gatedChunkWriter records every chunk it receives and, for the first Write
// only, blocks until a second Write has entered — deterministically overlapping
// the write phases of two concurrent log calls.
type gatedChunkWriter struct {
	mu       sync.Mutex
	chunks   []string
	entered  atomic.Int32
	secondIn chan struct{}
	once     sync.Once
}

func (w *gatedChunkWriter) Write(p []byte) (int, error) {
	if w.entered.Add(1) == 1 {
		// First writer waits (bounded) for a concurrent Write so the two log
		// calls overlap — the exact window where split line writes interleave.
		select {
		case <-w.secondIn:
		case <-time.After(2 * time.Second):
		}
	} else {
		w.once.Do(func() { close(w.secondIn) })
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.chunks = append(w.chunks, string(p))
	return len(p), nil
}

// TestConcurrentSingleWriterLineIntegrity guards the single-writer write path:
// writeMessage must deliver the complete line (message + newline) in ONE Write
// call. It previously wrote the message and the newline as two separate calls,
// so two goroutines logging through the same writer could interleave into
// corrupted lines ("msgAmsgB\n\n") — despite the package-wide thread-safety
// guarantee. The gated writer makes the overlap deterministic: with split
// writes, the first recorded chunk lacks its trailing newline.
func TestConcurrentSingleWriterLineIntegrity(t *testing.T) {
	w := &gatedChunkWriter{secondIn: make(chan struct{})}

	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(w)}
	logger, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	const msgA = "message-from-goroutine-A"
	const msgB = "message-from-goroutine-B"
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		logger.Info(msgA)
	}()
	go func() {
		defer wg.Done()
		logger.Info(msgB)
	}()
	wg.Wait()
	_ = logger.Close()

	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.chunks) < 2 {
		t.Fatalf("expected at least 2 recorded writes, got %d: %q", len(w.chunks), w.chunks)
	}
	for i, chunk := range w.chunks {
		if !strings.HasSuffix(chunk, "\n") {
			t.Fatalf("write #%d does not end with a newline (split line write): %q", i, chunk)
		}
	}
	joined := strings.Join(w.chunks, "")
	if !strings.Contains(joined, msgA) || !strings.Contains(joined, msgB) {
		t.Fatalf("both messages must be present in the output: %q", joined)
	}
}

// TestFilterVsCloseCacheRace guards the Filter/Close cache data race: the two
// post-scan cacheResult sites in Filter used to re-read f.cache without the
// lock, racing with Close()'s `f.cache = nil` (written under cacheMu). Filter
// now consults a cachePresent snapshot taken under the lookup's RLock. Inputs
// are unique per call to force cache misses — on a cache hit Filter returns
// from the lookup block and never reaches the raced read.
func TestFilterVsCloseCacheRace(t *testing.T) {
	for round := 0; round < 50; round++ {
		f := NewSensitiveDataFilter()

		var counter atomic.Int64
		start := make(chan struct{})
		var wg sync.WaitGroup

		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				for j := 0; j < 200; j++ {
					n := counter.Add(1)
					// 35 bytes (within cacheInputMaxLen) with '@' and digits:
					// passes couldContainSensitiveData, so the full pattern
					// sweep — and the raced read after it — actually runs.
					f.Filter(fmt.Sprintf("user%d@example.com password=secret%d", n, n))
				}
			}()
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			f.Close()
		}()

		close(start)
		wg.Wait()
	}
}

// TestConcurrentDynamicReconfiguration guards every runtime reconfiguration
// surface racing the hot logging path. Each Set*/Add* API publishes state
// through an atomic pointer (writers, hooks, extractors, sampling, security,
// level resolver, field validation) with copy-on-write under a mutation lock;
// a regression that mutates shared state in place, or publishes without the
// lock, shows up here as a race-detector report while logging runs.
func TestConcurrentDynamicReconfiguration(t *testing.T) {
	for round := 0; round < 20; round++ {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Targets = []OutputTarget{CustomOutput(&buf)}
		logger, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}

		rlCfg := DefaultRateLimitConfig()
		rlCfg.MaxMessagesPerSecond = 0
		rlCfg.MaxBytesPerSecond = 0

		hookReg := NewHookRegistry()
		hookReg.Add(HookAfterLog, func(context.Context, *HookContext) error { return nil })

		start := make(chan struct{})
		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 300; i++ {
				logger.Info("probe msg")
				logger.InfoWith("fields", String("k", fmt.Sprint(i)))
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 150; i++ {
				logger.SetSecurityConfig(&SecurityConfig{SensitiveFilter: nil, RateLimitConfig: rlCfg})
				logger.SetLevel(LevelDebug)
				logger.SetLevel(LevelWarn)
				logger.SetLevelResolver(func(context.Context) LogLevel { return LevelDebug })
				logger.SetLevelResolver(nil)
				logger.SetFieldValidation(StrictSnakeCaseConfig())
				_ = logger.GetFieldValidation()
				logger.SetWriteErrorHandler(func(io.Writer, error) {})
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 150; i++ {
				_ = logger.AddHook(HookOnFilter, func(context.Context, *HookContext) error { return nil })
				_ = logger.SetHooks(hookReg)
				_ = logger.GetHooks()
				_ = logger.AddContextExtractor(func(context.Context) []Field {
					return []Field{String("ctx", "v")}
				})
				_ = logger.SetContextExtractors()
				_ = logger.GetContextExtractors()
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 150; i++ {
				logger.SetSampling(&SamplingConfig{Enabled: true, Initial: 5, Thereafter: 2, Tick: time.Millisecond})
				_ = logger.GetSampling()
				w := &bytes.Buffer{}
				_ = logger.AddWriter(w)
				_ = logger.RemoveWriter(w)
			}
		}()

		close(start)
		wg.Wait()
		_ = logger.Close()
	}
}

// TestFilterPatternMutationVsFilter guards the filter's copy-on-write pattern
// lists: AddPattern/ClearPatterns replace the patternsPtr/gatesPtr slices
// atomically under mu, while Filter reads them lock-free. An in-place append
// or an unlocked store would race the running regex sweep.
func TestFilterPatternMutationVsFilter(t *testing.T) {
	for round := 0; round < 10; round++ {
		f := NewEmptySensitiveDataFilter()

		start := make(chan struct{})
		var wg sync.WaitGroup

		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				<-start
				for j := 0; j < 200; j++ {
					f.Filter(fmt.Sprintf("probe %d input with password=abc%d", id, j))
				}
			}(i)
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 100; i++ {
				_ = f.AddPattern(fmt.Sprintf(`probe%d_\d+`, i%7))
				f.ClearPatterns()
				_ = f.PatternCount()
				_ = f.GetFilterStats()
			}
		}()

		close(start)
		wg.Wait()
		_ = f.Close()
	}
}

// TestAuditLoggerLogStatsCloseOverlap guards the AuditLogger send/close
// exclusion: Log re-checks closed under the read lock and Close takes the
// write lock before signaling done, so no counted event can be lost by a send
// landing after the drain loop exits. Without that discipline a racing Log
// either deadlocks, double-closes done, or reports events as logged that were
// never written.
func TestAuditLoggerLogStatsCloseOverlap(t *testing.T) {
	for round := 0; round < 30; round++ {
		cfg := DefaultAuditConfig()
		cfg.Output = nil // events accepted and counted, no output
		al, err := NewAuditLogger(cfg)
		if err != nil {
			t.Fatal(err)
		}

		start := make(chan struct{})
		var wg sync.WaitGroup

		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				for j := 0; j < 200; j++ {
					al.Log(AuditEvent{Type: AuditEventSecurityViolation, Message: "m", Severity: AuditSeverityInfo})
				}
			}()
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 50; i++ {
				_ = al.Stats()
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_ = al.Close()
		}()

		close(start)
		wg.Wait()

		// Post-Close invariant: every event Stats counted as logged must have
		// fit in the buffer (and therefore been drained before Close returned)
		// — counted-but-lost events are the worst failure mode for an audit
		// trail.
		stats := al.Stats()
		if stats.TotalEvents > int64(stats.BufferSize) {
			t.Fatalf("counted %d events exceed buffer %d", stats.TotalEvents, stats.BufferSize)
		}
	}
}

// TestMultiWriterLifecycleStress guards the MultiWriter copy-on-write writer
// list against Write/AddWriter/RemoveWriter/Close running concurrently: Write
// loads the immutable snapshot lock-free, mutations swap the slice under mu,
// and Close snapshots under the same mu so an accepted writer can never leak.
func TestMultiWriterLifecycleStress(t *testing.T) {
	for round := 0; round < 100; round++ {
		mw := NewMultiWriter(&bytes.Buffer{})

		start := make(chan struct{})
		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 100; i++ {
				_, _ = mw.Write([]byte("probe\n"))
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 50; i++ {
				w := &closeTrackingWriter{}
				_ = mw.AddWriter(w)
				_ = mw.RemoveWriter(w)
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_ = mw.Close()
		}()

		close(start)
		wg.Wait()
	}
}

// TestDefaultLoggerSwapStorm guards the package-level default-logger globals
// (defaultLogger atomic pointer, defaultInitErr atomic.Value, defaultOnce)
// against simultaneous Default()/SetDefault()/InitDefault() traffic —
// including Default()'s CAS install path, which must never overwrite a
// SetDefault-installed logger. The pre-probe default is restored afterwards
// so later tests observe an untouched default logger.
func TestDefaultLoggerSwapStorm(t *testing.T) {
	old := Default()

	var wg sync.WaitGroup
	start := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 30; i++ {
			Info("default probe")
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 10; i++ {
			cfg := DefaultConfig()
			cfg.Targets = []OutputTarget{CustomOutput(&bytes.Buffer{})}
			l, err := New(cfg)
			if err != nil {
				t.Error(err)
				return
			}
			SetDefault(l)
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 10; i++ {
			cfg := DefaultConfig()
			cfg.Targets = []OutputTarget{CustomOutput(&bytes.Buffer{})}
			if err := InitDefault(cfg); err != nil {
				t.Error(err)
				return
			}
		}
	}()

	close(start)
	wg.Wait()

	// Restore the pre-probe default; SetDefault schedules a background close
	// of the last probe logger, which resource_leak_test.go's
	// waitForBackgroundClosers drains if it runs afterwards.
	SetDefault(old)
}
