package dd

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// ============================================================================
// HOOK REGISTRY TESTS (merged from hooks_test.go)
// ============================================================================

func TestNewHookRegistry(t *testing.T) {
	registry := NewHookRegistry()
	if registry == nil {
		t.Fatal("expected non-nil registry")
	}
	if registry.count() != 0 {
		t.Errorf("expected empty registry, got %d hooks", registry.count())
	}
}

func TestHookRegistry_Add(t *testing.T) {
	registry := NewHookRegistry()

	hook := func(ctx context.Context, hc *HookContext) error {
		return nil
	}
	registry.Add(HookBeforeLog, hook)

	if registry.count() != 1 {
		t.Errorf("expected 1 hook, got %d", registry.count())
	}
	if registry.countFor(HookBeforeLog) != 1 {
		t.Errorf("expected 1 BeforeLog hook, got %d", registry.countFor(HookBeforeLog))
	}

	// Test adding nil hook (should be ignored)
	registry.Add(HookBeforeLog, nil)
	if registry.count() != 1 {
		t.Errorf("expected 1 hook after adding nil, got %d", registry.count())
	}

	// Add hook for different event
	registry.Add(HookAfterLog, hook)
	if registry.count() != 2 {
		t.Errorf("expected 2 hooks, got %d", registry.count())
	}
}

func TestHookRegistry_Remove(t *testing.T) {
	registry := NewHookRegistry()
	hook := func(ctx context.Context, hc *HookContext) error {
		return nil
	}
	registry.Add(HookBeforeLog, hook)
	registry.Add(HookAfterLog, hook)

	if registry.count() != 2 {
		t.Fatalf("expected 2 hooks, got %d", registry.count())
	}

	registry.Remove(HookBeforeLog)
	if registry.count() != 1 {
		t.Errorf("expected 1 hook after remove, got %d", registry.count())
	}
	if registry.countFor(HookBeforeLog) != 0 {
		t.Errorf("expected 0 BeforeLog hooks, got %d", registry.countFor(HookBeforeLog))
	}
}

func TestHookRegistry_Trigger(t *testing.T) {
	t.Run("no hooks", func(t *testing.T) {
		registry := NewHookRegistry()
		hookCtx := &HookContext{Event: HookBeforeLog}
		err := registry.Trigger(context.Background(), HookBeforeLog, hookCtx)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("single hook success", func(t *testing.T) {
		registry := NewHookRegistry()
		called := false
		registry.Add(HookBeforeLog, func(ctx context.Context, hc *HookContext) error {
			called = true
			return nil
		})

		hookCtx := &HookContext{Event: HookBeforeLog}
		err := registry.Trigger(context.Background(), HookBeforeLog, hookCtx)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if !called {
			t.Error("expected hook to be called")
		}
	})

	t.Run("multiple hooks in order", func(t *testing.T) {
		registry := NewHookRegistry()
		var order []int

		registry.Add(HookBeforeLog, func(ctx context.Context, hc *HookContext) error {
			order = append(order, 1)
			return nil
		})
		registry.Add(HookBeforeLog, func(ctx context.Context, hc *HookContext) error {
			order = append(order, 2)
			return nil
		})

		hookCtx := &HookContext{Event: HookBeforeLog}
		err := registry.Trigger(context.Background(), HookBeforeLog, hookCtx)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if len(order) != 2 || order[0] != 1 || order[1] != 2 {
			t.Errorf("expected hooks to be called in order [1, 2], got %v", order)
		}
	})

	t.Run("hook returns error", func(t *testing.T) {
		registry := NewHookRegistry()
		expectedErr := errors.New("hook error")

		registry.Add(HookBeforeLog, func(ctx context.Context, hc *HookContext) error {
			return expectedErr
		})

		hookCtx := &HookContext{Event: HookBeforeLog}
		err := registry.Trigger(context.Background(), HookBeforeLog, hookCtx)
		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})

	t.Run("hook error stops execution", func(t *testing.T) {
		registry := NewHookRegistry()
		secondCalled := false

		registry.Add(HookBeforeLog, func(ctx context.Context, hc *HookContext) error {
			return errors.New("stop")
		})
		registry.Add(HookBeforeLog, func(ctx context.Context, hc *HookContext) error {
			secondCalled = true
			return nil
		})

		hookCtx := &HookContext{Event: HookBeforeLog}
		_ = registry.Trigger(context.Background(), HookBeforeLog, hookCtx)

		if secondCalled {
			t.Error("expected second hook not to be called after first returns error")
		}
	})

	t.Run("nil registry", func(t *testing.T) {
		var registry *HookRegistry
		hookCtx := &HookContext{Event: HookBeforeLog}
		err := registry.Trigger(context.Background(), HookBeforeLog, hookCtx)
		if err != nil {
			t.Errorf("expected no error for nil registry, got %v", err)
		}
	})
}

func TestHookRegistry_Clone(t *testing.T) {
	registry := NewHookRegistry()
	registry.Add(HookBeforeLog, func(ctx context.Context, hc *HookContext) error {
		return nil
	})

	clone := registry.clone()
	if clone == nil {
		t.Fatal("expected non-nil clone")
	}

	if clone.count() != registry.count() {
		t.Errorf("clone count mismatch: got %d, want %d", clone.count(), registry.count())
	}

	// Modify original and verify clone is independent
	registry.Add(HookAfterLog, func(ctx context.Context, hc *HookContext) error {
		return nil
	})

	if registry.count() == clone.count() {
		t.Error("clone should be independent of original")
	}

	// Test nil registry
	var nilRegistry *HookRegistry
	if nilRegistry.clone() != nil {
		t.Error("expected nil for nil registry clone")
	}
}

func TestHookRegistry_Clear(t *testing.T) {
	registry := NewHookRegistry()
	registry.Add(HookBeforeLog, func(ctx context.Context, hc *HookContext) error {
		return nil
	})
	registry.Add(HookAfterLog, func(ctx context.Context, hc *HookContext) error {
		return nil
	})

	if registry.count() != 2 {
		t.Fatalf("expected 2 hooks, got %d", registry.count())
	}

	registry.Clear()
	if registry.count() != 0 {
		t.Errorf("expected 0 hooks after clear, got %d", registry.count())
	}
}

func TestHookRegistry_ClearFor(t *testing.T) {
	registry := NewHookRegistry()
	registry.Add(HookBeforeLog, func(ctx context.Context, hc *HookContext) error {
		return nil
	})
	registry.Add(HookAfterLog, func(ctx context.Context, hc *HookContext) error {
		return nil
	})

	registry.ClearFor(HookBeforeLog)
	if registry.count() != 1 {
		t.Errorf("expected 1 hook after ClearFor, got %d", registry.count())
	}
	if registry.countFor(HookBeforeLog) != 0 {
		t.Errorf("expected 0 BeforeLog hooks, got %d", registry.countFor(HookBeforeLog))
	}
}

func TestHookEvent_String(t *testing.T) {
	assertEnumStringer(t, "HookEvent", []stringerCase[HookEvent]{
		{HookBeforeLog, "BeforeLog"},
		{HookAfterLog, "AfterLog"},
		{HookOnFilter, "OnFilter"},
		{HookOnRotate, "OnRotate"},
		{HookOnClose, "OnClose"},
		{HookOnError, "OnError"},
		{HookEvent(999), "Unknown"},
	}, HookEvent.String)
}

func TestHooksConfig(t *testing.T) {
	registry := NewHooksFromConfig(HooksConfig{
		BeforeLog: []Hook{func(ctx context.Context, hc *HookContext) error { return nil }},
		AfterLog:  []Hook{func(ctx context.Context, hc *HookContext) error { return nil }},
		OnFilter:  []Hook{func(ctx context.Context, hc *HookContext) error { return nil }},
		OnRotate:  []Hook{func(ctx context.Context, hc *HookContext) error { return nil }},
		OnClose:   []Hook{func(ctx context.Context, hc *HookContext) error { return nil }},
		OnError:   []Hook{func(ctx context.Context, hc *HookContext) error { return nil }},
	})

	if registry == nil {
		t.Fatal("expected non-nil registry")
	}
	if registry.count() != 6 {
		t.Errorf("expected 6 hooks, got %d", registry.count())
	}

	// Verify each event has one hook
	events := []HookEvent{HookBeforeLog, HookAfterLog, HookOnFilter, HookOnRotate, HookOnClose, HookOnError}
	for _, event := range events {
		if registry.countFor(event) != 1 {
			t.Errorf("expected 1 hook for %v, got %d", event, registry.countFor(event))
		}
	}
}

func TestHookRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewHookRegistry()

	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent adds
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			registry.Add(HookBeforeLog, func(ctx context.Context, hc *HookContext) error {
				return nil
			})
		}()
	}

	// Concurrent triggers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hookCtx := &HookContext{Event: HookBeforeLog}
			_ = registry.Trigger(context.Background(), HookBeforeLog, hookCtx)
		}()
	}

	// Concurrent count
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = registry.count()
		}()
	}

	wg.Wait()

	if registry.countFor(HookBeforeLog) != numGoroutines {
		t.Errorf("expected %d BeforeLog hooks, got %d", numGoroutines, registry.countFor(HookBeforeLog))
	}
}

func TestHookRegistry_NilContext(t *testing.T) {
	registry := NewHookRegistry()
	called := false

	registry.Add(HookBeforeLog, func(ctx context.Context, hc *HookContext) error {
		called = true
		return nil
	})

	hookCtx := &HookContext{Event: HookBeforeLog}
	err := registry.Trigger(nil, HookBeforeLog, hookCtx)

	// The hook should be called even with nil context (hooks handle nil context)
	if !called {
		t.Error("expected hook to be called even with nil context")
	}
	_ = err // We don't check the error as behavior with nil context may vary
}

func TestHookRegistry_PanicRecovery(t *testing.T) {
	registry := NewHookRegistry()

	// Add a hook that panics
	registry.Add(HookBeforeLog, func(ctx context.Context, hc *HookContext) error {
		panic("intentional test panic")
	})

	hookCtx := &HookContext{Event: HookBeforeLog}

	// Trigger should not panic - it should recover and return an error
	err := registry.Trigger(context.Background(), HookBeforeLog, hookCtx)

	// Should return an error from the panic
	if err == nil {
		t.Error("expected error from panicked hook")
	}

	// Verify error message contains panic info
	if err != nil && err.Error() == "" {
		t.Error("error message should not be empty")
	}
}

func TestHookRegistry_PanicRecovery_ContinuesWithErrorHandler(t *testing.T) {
	secondHookCalled := false
	var recordedErrors []error

	registry := NewHookRegistry()
	registry.SetErrorHandler(func(event HookEvent, hc *HookContext, err error) {
		recordedErrors = append(recordedErrors, err)
	})

	// Add a hook that panics
	registry.Add(HookBeforeLog, func(ctx context.Context, hc *HookContext) error {
		panic("intentional test panic")
	})

	// Add a hook that should be called after the panic (because we have an error handler)
	registry.Add(HookBeforeLog, func(ctx context.Context, hc *HookContext) error {
		secondHookCalled = true
		return nil
	})

	hookCtx := &HookContext{Event: HookBeforeLog}

	// Trigger should not panic - it should recover and continue
	err := registry.Trigger(context.Background(), HookBeforeLog, hookCtx)

	// Should return an error from the panic
	if err == nil {
		t.Error("expected error from panicked hook")
	}

	// Error should have been recorded
	if len(recordedErrors) != 1 {
		t.Errorf("expected 1 recorded error, got %d", len(recordedErrors))
	}

	// Second hook should be called (because we have an error handler)
	if !secondHookCalled {
		t.Error("expected second hook to be called after panic recovery when error handler is set")
	}
}

// ============================================================================
// CONTEXT EXTRACTOR REGISTRY TESTS (merged from context_extractor_test.go)
// ============================================================================

func TestExtractorRegistry_New(t *testing.T) {
	registry := newContextExtractorRegistry()
	if registry == nil {
		t.Fatal("expected non-nil registry")
	}
	if registry.count() != 0 {
		t.Errorf("expected empty registry, got %d extractors", registry.count())
	}
}

func TestExtractorRegistry_Add(t *testing.T) {
	registry := newContextExtractorRegistry()

	// Test adding extractor
	extractor := func(ctx context.Context) []Field {
		return []Field{String("test", "value")}
	}
	registry.Add(extractor)

	if registry.count() != 1 {
		t.Errorf("expected 1 extractor, got %d", registry.count())
	}

	// Test adding nil extractor (should be ignored)
	registry.Add(nil)
	if registry.count() != 1 {
		t.Errorf("expected 1 extractor after adding nil, got %d", registry.count())
	}
}

func TestExtractorRegistry_Extract(t *testing.T) {
	t.Run("empty registry", func(t *testing.T) {
		registry := newContextExtractorRegistry()
		ctx := context.Background()
		fields := registry.Extract(ctx)
		if fields != nil {
			t.Errorf("expected nil fields from empty registry, got %v", fields)
		}
	})

	t.Run("single extractor", func(t *testing.T) {
		registry := newContextExtractorRegistry()
		registry.Add(func(ctx context.Context) []Field {
			return []Field{String("key1", "value1")}
		})

		ctx := context.Background()
		fields := registry.Extract(ctx)

		if len(fields) != 1 {
			t.Fatalf("expected 1 field, got %d", len(fields))
		}
		if fields[0].Key != "key1" || fields[0].Value != "value1" {
			t.Errorf("unexpected field: %v", fields[0])
		}
	})

	t.Run("multiple extractors", func(t *testing.T) {
		registry := newContextExtractorRegistry()
		registry.Add(func(ctx context.Context) []Field {
			return []Field{String("key1", "value1")}
		})
		registry.Add(func(ctx context.Context) []Field {
			return []Field{String("key2", "value2")}
		})

		ctx := context.Background()
		fields := registry.Extract(ctx)

		if len(fields) != 2 {
			t.Fatalf("expected 2 fields, got %d", len(fields))
		}
	})

	t.Run("extractor returns nil", func(t *testing.T) {
		registry := newContextExtractorRegistry()
		registry.Add(func(ctx context.Context) []Field {
			return nil
		})
		registry.Add(func(ctx context.Context) []Field {
			return []Field{String("key1", "value1")}
		})

		ctx := context.Background()
		fields := registry.Extract(ctx)

		if len(fields) != 1 {
			t.Fatalf("expected 1 field, got %d", len(fields))
		}
	})

	t.Run("nil context with nil-safe extractor", func(t *testing.T) {
		registry := newContextExtractorRegistry()
		registry.Add(func(ctx context.Context) []Field {
			if ctx == nil {
				return nil
			}
			return []Field{String("key1", "value1")}
		})

		fields := registry.Extract(nil)
		if fields != nil {
			t.Errorf("expected nil fields for nil context with nil-safe extractor, got %v", fields)
		}
	})
}

func TestExtractorRegistry_Clone(t *testing.T) {
	registry := newContextExtractorRegistry()
	registry.Add(func(ctx context.Context) []Field {
		return []Field{String("key1", "value1")}
	})

	clone := registry.clone()
	if clone == nil {
		t.Fatal("expected non-nil clone")
	}

	if clone.count() != registry.count() {
		t.Errorf("clone count mismatch: got %d, want %d", clone.count(), registry.count())
	}

	// Modify original and verify clone is independent
	registry.Add(func(ctx context.Context) []Field {
		return []Field{String("key2", "value2")}
	})

	if registry.count() == clone.count() {
		t.Error("clone should be independent of original")
	}

	// Test nil registry
	var nilRegistry *contextExtractorRegistry
	if nilRegistry.clone() != nil {
		t.Error("expected nil for nil registry clone")
	}
}

func TestExtractorRegistry_Clear(t *testing.T) {
	registry := newContextExtractorRegistry()
	registry.Add(func(ctx context.Context) []Field {
		return []Field{String("key1", "value1")}
	})

	if registry.count() != 1 {
		t.Fatalf("expected 1 extractor, got %d", registry.count())
	}

	registry.clear()
	if registry.count() != 0 {
		t.Errorf("expected 0 extractors after clear, got %d", registry.count())
	}
}

func TestExtractorRegistry_ConcurrentAccess(t *testing.T) {
	registry := newContextExtractorRegistry()

	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent adds
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			registry.Add(func(ctx context.Context) []Field {
				return []Field{Int("index", idx)}
			})
		}(i)
	}

	// Concurrent extracts
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = registry.Extract(context.Background())
		}()
	}

	// Concurrent count
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = registry.count()
		}()
	}

	wg.Wait()

	if registry.count() != numGoroutines {
		t.Errorf("expected %d extractors, got %d", numGoroutines, registry.count())
	}
}

func TestExtractorRegistry_PanicRecovery(t *testing.T) {
	registry := newContextExtractorRegistry()

	// Add an extractor that panics
	registry.Add(func(ctx context.Context) []Field {
		panic("intentional test panic")
	})

	// Add an extractor that should still be called
	registry.Add(func(ctx context.Context) []Field {
		return []Field{String("key1", "value1")}
	})

	ctx := context.Background()

	// Extract should not panic - it should recover and continue
	fields := registry.Extract(ctx)

	// Should still get fields from the second extractor
	if len(fields) != 1 {
		t.Errorf("expected 1 field from non-panicking extractor, got %d", len(fields))
	}

	if fields[0].Key != "key1" || fields[0].Value != "value1" {
		t.Errorf("unexpected field: %v", fields[0])
	}
}

func TestExtractorRegistry_MultiplePanics(t *testing.T) {
	registry := newContextExtractorRegistry()

	// Add multiple extractors that panic
	registry.Add(func(ctx context.Context) []Field {
		panic("panic 1")
	})
	registry.Add(func(ctx context.Context) []Field {
		panic("panic 2")
	})

	// Add a working extractor
	registry.Add(func(ctx context.Context) []Field {
		return []Field{String("success", "true")}
	})

	ctx := context.Background()

	// Extract should not panic
	fields := registry.Extract(ctx)

	// Should still get fields from the working extractor
	if len(fields) != 1 {
		t.Errorf("expected 1 field, got %d", len(fields))
	}
}
