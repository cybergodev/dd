package dd

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// HookEvent represents the type of event that triggers a hook.
type HookEvent int

const (
	// HookBeforeLog is triggered before a log message is written.
	// Hooks can modify fields or abort logging by returning an error.
	HookBeforeLog HookEvent = iota

	// HookAfterLog is triggered after a log message is successfully written.
	HookAfterLog

	// HookOnFilter is triggered when sensitive data is filtered.
	HookOnFilter

	// HookOnRotate is triggered when a log file is rotated.
	HookOnRotate

	// HookOnClose is triggered when the logger is closed.
	HookOnClose

	// HookOnError is triggered when a write error occurs.
	HookOnError
)

// String returns the string representation of the hook event.
func (e HookEvent) String() string {
	switch e {
	case HookBeforeLog:
		return "BeforeLog"
	case HookAfterLog:
		return "AfterLog"
	case HookOnFilter:
		return "OnFilter"
	case HookOnRotate:
		return "OnRotate"
	case HookOnClose:
		return "OnClose"
	case HookOnError:
		return "OnError"
	default:
		return "Unknown"
	}
}

// HookContext provides contextual information for hook execution.
type HookContext struct {
	// Event is the type of hook event being triggered.
	Event HookEvent

	// Level is the log level for log-related events.
	Level LogLevel

	// Message is the log message (may be empty for non-log events).
	Message string

	// Fields are the structured fields attached to the log entry (after filtering).
	Fields []Field

	// OriginalFields are the fields before sensitive data filtering.
	// This allows hooks to access the original values if needed.
	OriginalFields []Field

	// Error contains any error that occurred (for OnError events).
	Error error

	// Timestamp is when the event occurred.
	Timestamp time.Time

	// Writer is the target writer (for write-related events).
	Writer io.Writer

	// Additional metadata can be stored here.
	Metadata map[string]any
}

// Hook is a function that is called during logging lifecycle events.
// If a BeforeLog hook returns an error, the log entry is not written.
// For other events, the error is logged but does not prevent the operation.
type Hook func(ctx context.Context, hookCtx *HookContext) error

// HookErrorHandler handles errors that occur during hook execution.
// This allows custom error handling strategies such as logging, metrics,
// or ignoring errors for non-critical hooks.
//
// Parameters:
//   - event: The hook event type that triggered the error
//   - hookCtx: The context provided to the hook
//   - err: The error returned by the hook
//
// The handler should not panic. If it does, the panic will be recovered
// and logged to stderr.
type HookErrorHandler func(event HookEvent, hookCtx *HookContext, err error)

// HookRegistry manages a collection of hooks organized by event type.
// It is thread-safe and supports dynamic hook registration.
//
// Error Handling Behavior:
//   - By default, Trigger returns the first error from a hook and stops execution
//   - If an error handler is set via SetErrorHandler, all hooks are executed
//     regardless of errors, and errors are passed to the handler
//   - For BeforeLog events, an error still prevents the log from being written
//     even with an error handler set
type HookRegistry struct {
	mu           sync.RWMutex
	hooks        map[HookEvent][]Hook
	errorHandler HookErrorHandler
}

// NewHookRegistry creates a new empty hook registry.
func NewHookRegistry() *HookRegistry {
	return &HookRegistry{
		hooks: make(map[HookEvent][]Hook),
	}
}

// SetErrorHandler sets the error handler for this registry.
// Pass nil to remove the error handler and restore default behavior.
func (r *HookRegistry) SetErrorHandler(handler HookErrorHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errorHandler = handler
}

// Add registers a hook for a specific event type.
// If the hook is nil, it is ignored.
// Multiple hooks can be registered for the same event.
// Hooks are executed in the order they were added.
func (r *HookRegistry) Add(event HookEvent, hook Hook) {
	if hook == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hooks[event] = append(r.hooks[event], hook)
}

// Remove removes all hooks for a specific event type.
func (r *HookRegistry) Remove(event HookEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.hooks, event)
}

// Trigger executes all hooks registered for the given event.
//
// Error Handling Behavior:
//   - If no error handler is set (default): hooks are executed in order;
//     if any hook returns an error or panics, execution stops and that error is returned.
//   - If an error handler is set: all hooks are executed regardless of errors or panics;
//     each error is passed to the error handler, and the first error is returned.
//
// For BeforeLog events, an error prevents the log from being written
// regardless of whether an error handler is set.
//
// Panic Recovery: If a hook panics, the panic is recovered and converted to an error.
// This ensures that a misbehaving hook cannot crash the application.
// A panicking error handler is likewise recovered and returned as an error.
func (r *HookRegistry) Trigger(ctx context.Context, event HookEvent, hookCtx *HookContext) (err error) {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	hooks := r.hooks[event]
	handler := r.errorHandler
	r.mu.RUnlock()

	if len(hooks) == 0 {
		return nil
	}

	var firstErr error

	for _, hook := range hooks {
		// Execute hook with panic recovery
		hookErr := r.executeHookWithRecovery(ctx, hook, hookCtx, event)
		if hookErr != nil {
			if handler != nil {
				// Call the error handler and continue to next hook.
				// SEC-003: the handler is user code and is invoked from paths
				// that run outside logCoreWithDepth's recover (Close, rotation
				// callbacks, pre-core field processing), so it needs its own
				// backstop — this is the behavior HookErrorHandler's doc
				// already promises.
				if firstErr == nil {
					if handlerErr := executeHandlerWithRecovery(handler, event, hookCtx, hookErr); handlerErr != nil {
						firstErr = handlerErr
					} else {
						firstErr = hookErr
					}
				} else {
					_ = executeHandlerWithRecovery(handler, event, hookCtx, hookErr)
				}
			} else {
				// Default behavior: stop on first error (including panic)
				return hookErr
			}
		}
	}

	return firstErr
}

// executeHandlerWithRecovery executes a hook error handler with panic recovery.
// If the handler panics, the panic is recovered, logged to stderr, and returned
// as an error — mirroring executeHookWithRecovery so a misbehaving error handler
// cannot crash the application.
func executeHandlerWithRecovery(handler HookErrorHandler, event HookEvent, hookCtx *HookContext, hookErr error) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			panicErr := fmt.Errorf("hook error handler panic for event %s: %v (while handling: %w)", event, rec, hookErr)
			fmt.Fprintf(os.Stderr, "dd: %v\n", panicErr)
			err = panicErr
		}
	}()

	handler(event, hookCtx, hookErr)
	return nil
}

// executeHookWithRecovery executes a hook with panic recovery.
// If the hook panics, the panic is recovered, logged to stderr, and converted to an error.
func (r *HookRegistry) executeHookWithRecovery(ctx context.Context, hook Hook, hookCtx *HookContext, event HookEvent) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			// Convert panic to error
			panicErr := fmt.Errorf("hook panic for event %s: %v", event, rec)
			// Log to stderr as a fallback
			fmt.Fprintf(os.Stderr, "dd: %v\n", panicErr)
			err = panicErr
		}
	}()

	return hook(ctx, hookCtx)
}

// clone creates a copy of the registry with the same hooks and error handler.
// The hooks themselves are shared (functions are not copied).
func (r *HookRegistry) clone() *HookRegistry {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	clone := &HookRegistry{
		hooks:        make(map[HookEvent][]Hook, len(r.hooks)),
		errorHandler: r.errorHandler,
	}

	for event, hooks := range r.hooks {
		clone.hooks[event] = append([]Hook(nil), hooks...)
	}

	return clone
}

// count returns the total number of registered hooks.
func (r *HookRegistry) count() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, hooks := range r.hooks {
		count += len(hooks)
	}
	return count
}

// countFor returns the number of hooks registered for a specific event.
func (r *HookRegistry) countFor(event HookEvent) int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.hooks[event])
}

// Clear removes all registered hooks.
func (r *HookRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hooks = make(map[HookEvent][]Hook)
}

// ClearFor removes all hooks for a specific event type.
//
// Deprecated: ClearFor is a duplicate of Remove (byte-for-byte the same
// operation) kept only for backward compatibility; use Remove, which pairs
// with Add like every other registry in this package.
func (r *HookRegistry) ClearFor(event HookEvent) {
	r.Remove(event)
}

// HooksConfig provides a struct-based configuration for creating hook registries.
// This follows the project's design guidelines favoring struct-based configuration
// over fluent API patterns.
//
// Example:
//
//	cfg := HooksConfig{
//	    BeforeLog: []Hook{myBeforeHook},
//	    AfterLog:  []Hook{myAfterHook},
//	    ErrorHandler: func(event HookEvent, hookCtx *HookContext, err error) {
//	        log.Printf("hook error: %v", err)
//	    },
//	}
//	registry := NewHooksFromConfig(cfg)
type HooksConfig struct {
	// BeforeLog hooks are called before a log message is written.
	BeforeLog []Hook
	// AfterLog hooks are called after a log message is successfully written.
	AfterLog []Hook
	// OnFilter hooks are called when sensitive data is filtered.
	OnFilter []Hook
	// OnRotate hooks are called when a log file is rotated.
	OnRotate []Hook
	// OnClose hooks are called when the logger is closed.
	OnClose []Hook
	// OnError hooks are called when a write error occurs.
	OnError []Hook
	// ErrorHandler handles errors that occur during hook execution.
	ErrorHandler HookErrorHandler
}

// NewHooksFromConfig creates a HookRegistry from the configuration.
// This is the recommended way to create a hook registry with multiple hooks.
func NewHooksFromConfig(cfg HooksConfig) *HookRegistry {
	registry := NewHookRegistry()
	if cfg.ErrorHandler != nil {
		registry.SetErrorHandler(cfg.ErrorHandler)
	}
	for _, hook := range cfg.BeforeLog {
		registry.Add(HookBeforeLog, hook)
	}
	for _, hook := range cfg.AfterLog {
		registry.Add(HookAfterLog, hook)
	}
	for _, hook := range cfg.OnFilter {
		registry.Add(HookOnFilter, hook)
	}
	for _, hook := range cfg.OnRotate {
		registry.Add(HookOnRotate, hook)
	}
	for _, hook := range cfg.OnClose {
		registry.Add(HookOnClose, hook)
	}
	for _, hook := range cfg.OnError {
		registry.Add(HookOnError, hook)
	}
	return registry
}
