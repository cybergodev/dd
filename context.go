package dd

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
)

// ============================================================================
// Context Keys
// ============================================================================

// ContextKey is a type-safe key for context values.
// Using a custom type prevents key collisions with other packages
// that may also store values in context.
//
// Example:
//
//	ctx := dd.WithTraceID(context.Background(), "abc123")
//	traceID := dd.GetTraceID(ctx)
type ContextKey string

const (
	// ContextKeyTraceID is the context key for trace ID.
	// This key is used by default context extractors to retrieve
	// the trace ID from context.
	ContextKeyTraceID ContextKey = "trace_id"

	// ContextKeySpanID is the context key for span ID.
	// This key is used by default context extractors to retrieve
	// the span ID from context.
	ContextKeySpanID ContextKey = "span_id"

	// ContextKeyRequestID is the context key for request ID.
	// This key is used by default context extractors to retrieve
	// the request ID from context.
	ContextKeyRequestID ContextKey = "request_id"
)

// WithTraceID adds a trace ID to the context.
// This is the type-safe way to store trace IDs that will be
// automatically extracted by the logger's context extractors.
//
// Example:
//
//	ctx := dd.WithTraceID(context.Background(), "trace-123")
//	logger.InfoWith("processing request", dd.String("trace_id", dd.GetTraceID(ctx)))
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, ContextKeyTraceID, traceID)
}

// WithSpanID adds a span ID to the context.
// This is the type-safe way to store span IDs that will be
// automatically extracted by the logger's context extractors.
//
// Example:
//
//	ctx := dd.WithSpanID(context.Background(), "span-456")
//	logger.InfoWith("processing request", dd.String("span_id", dd.GetSpanID(ctx)))
func WithSpanID(ctx context.Context, spanID string) context.Context {
	return context.WithValue(ctx, ContextKeySpanID, spanID)
}

// WithRequestID adds a request ID to the context.
// This is the type-safe way to store request IDs that will be
// automatically extracted by the logger's context extractors.
//
// Example:
//
//	ctx := dd.WithRequestID(context.Background(), "req-789")
//	logger.InfoWith("processing request", dd.String("request_id", dd.GetRequestID(ctx)))
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, ContextKeyRequestID, requestID)
}

// getContextString retrieves a string value from context by key.
// This is an internal helper to reduce code duplication in getter functions.
func getContextString(ctx context.Context, key ContextKey) string {
	if v := ctx.Value(key); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetTraceID retrieves the trace ID from the context.
// Returns an empty string if no trace ID is found.
//
// Example:
//
//	traceID := dd.GetTraceID(ctx)
//	if traceID != "" {
//	    // Trace ID is present
//	}
func GetTraceID(ctx context.Context) string {
	return getContextString(ctx, ContextKeyTraceID)
}

// GetSpanID retrieves the span ID from the context.
// Returns an empty string if no span ID is found.
//
// Example:
//
//	spanID := dd.GetSpanID(ctx)
//	if spanID != "" {
//	    // Span ID is present
//	}
func GetSpanID(ctx context.Context) string {
	return getContextString(ctx, ContextKeySpanID)
}

// GetRequestID retrieves the request ID from the context.
// Returns an empty string if no request ID is found.
//
// Example:
//
//	requestID := dd.GetRequestID(ctx)
//	if requestID != "" {
//	    // Request ID is present
//	}
func GetRequestID(ctx context.Context) string {
	return getContextString(ctx, ContextKeyRequestID)
}

// ============================================================================
// Context Extractors
// ============================================================================

// ContextExtractor is a function that extracts logging fields from a context.
// This allows integration with various tracing frameworks (OpenTelemetry, Jaeger, etc.)
// by providing custom field extraction logic.
//
// Example:
//
//	// OpenTelemetry trace extractor
//	otelExtractor := func(ctx context.Context) []Field {
//	    span := trace.SpanFromContext(ctx)
//	    if !span.SpanContext().IsValid() {
//	        return nil
//	    }
//	    return []Field{
//	        String("trace_id", span.SpanContext().TraceID().String()),
//	        String("span_id", span.SpanContext().SpanID().String()),
//	    }
//	}
type ContextExtractor func(ctx context.Context) []Field

// contextExtractorRegistry manages a collection of context extractors.
// It is thread-safe and supports dynamic addition of extractors.
// Uses atomic.Pointer for lock-free reads.
type contextExtractorRegistry struct {
	extractorsPtr atomic.Pointer[[]ContextExtractor]
	mu            sync.Mutex // protects modification operations
}

// newContextExtractorRegistry creates a new empty extractor registry.
func newContextExtractorRegistry() *contextExtractorRegistry {
	r := &contextExtractorRegistry{}
	emptySlice := make([]ContextExtractor, 0)
	r.extractorsPtr.Store(&emptySlice)
	return r
}

// Add adds a context extractor to the registry.
// If the extractor is nil, it is ignored.
// This method is thread-safe.
func (r *contextExtractorRegistry) Add(extractor ContextExtractor) {
	if extractor == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	// Load current extractors
	currentPtr := r.extractorsPtr.Load()
	current := *currentPtr

	// Create new slice with the extractor added
	newExtractors := make([]ContextExtractor, len(current)+1)
	copy(newExtractors, current)
	newExtractors[len(current)] = extractor

	// Atomically swap the pointer
	r.extractorsPtr.Store(&newExtractors)
}

// Extract executes all registered extractors and returns the combined fields.
// Extractors are called in the order they were added.
// This method is thread-safe and uses lock-free reads.
//
// Panic Recovery: If an extractor panics, the panic is recovered, logged to stderr,
// and the extractor is skipped. This ensures that a misbehaving extractor cannot
// crash the application.
func (r *contextExtractorRegistry) Extract(ctx context.Context) []Field {
	if r == nil {
		return nil
	}

	// Lock-free read
	extractorsPtr := r.extractorsPtr.Load()
	if extractorsPtr == nil {
		return nil
	}
	extractors := *extractorsPtr

	if len(extractors) == 0 {
		return nil
	}

	// Pre-allocate result slice with estimated capacity (1 field per extractor is typical)
	var fields []Field
	for _, extractor := range extractors {
		// Execute extractor with panic recovery
		extracted := r.executeExtractorWithRecovery(ctx, extractor)
		if len(extracted) > 0 {
			if fields == nil {
				// Lazy allocation: only allocate when we have actual results
				fields = make([]Field, 0, len(extractors))
			}
			fields = append(fields, extracted...)
		}
	}

	return fields
}

// executeExtractorWithRecovery executes an extractor with panic recovery.
// If the extractor panics, the panic is recovered, logged to stderr, and nil is returned.
func (r *contextExtractorRegistry) executeExtractorWithRecovery(ctx context.Context, extractor ContextExtractor) (fields []Field) {
	defer func() {
		if rec := recover(); rec != nil {
			// Log panic to stderr
			fmt.Fprintf(os.Stderr, "dd: context extractor panic: %v\n", rec)
			fields = nil
		}
	}()

	return extractor(ctx)
}

// clone creates a copy of the registry with the same extractors.
// The extractors themselves are shared (functions are not copied).
func (r *contextExtractorRegistry) clone() *contextExtractorRegistry {
	if r == nil {
		return nil
	}

	extractorsPtr := r.extractorsPtr.Load()
	if extractorsPtr == nil {
		return newContextExtractorRegistry()
	}
	extractors := *extractorsPtr

	clone := &contextExtractorRegistry{}
	clonedSlice := make([]ContextExtractor, len(extractors))
	copy(clonedSlice, extractors)
	clone.extractorsPtr.Store(&clonedSlice)

	return clone
}

// count returns the number of registered extractors.
func (r *contextExtractorRegistry) count() int {
	if r == nil {
		return 0
	}
	extractorsPtr := r.extractorsPtr.Load()
	if extractorsPtr == nil {
		return 0
	}
	return len(*extractorsPtr)
}

// clear removes all registered extractors.
func (r *contextExtractorRegistry) clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	emptySlice := make([]ContextExtractor, 0)
	r.extractorsPtr.Store(&emptySlice)
}

// Singleton default registry
var (
	defaultRegistry     *contextExtractorRegistry
	defaultRegistryOnce sync.Once
)

// defaultContextExtractorRegistry returns a singleton registry with the default extractors.
// The default extractors extract trace_id, span_id, and request_id from context values.
// This function is thread-safe and uses sync.Once for initialization.
func defaultContextExtractorRegistry() *contextExtractorRegistry {
	defaultRegistryOnce.Do(func() {
		registry := newContextExtractorRegistry()
		registry.Add(defaultTraceIDExtractor)
		registry.Add(defaultSpanIDExtractor)
		registry.Add(defaultRequestIDExtractor)
		defaultRegistry = registry
	})
	return defaultRegistry
}

// createDefaultExtractor creates a context extractor for a specific key.
// This factory function reduces code duplication by extracting the common
// extraction logic used by trace_id, span_id, and request_id extractors.
//
// Parameters:
//   - key: The type-safe ContextKey to use for extraction
//   - fieldName: The field name to use in the returned Field (e.g., "trace_id")
//
// The extractor supports both type-safe ContextKey and string keys for backward compatibility.
func createDefaultExtractor(key ContextKey, fieldName string) ContextExtractor {
	return func(ctx context.Context) []Field {
		if ctx == nil {
			return nil
		}
		// Try type-safe key first
		if v := ctx.Value(key); v != nil {
			return []Field{String(fieldName, stringValue(v))}
		}
		// Fall back to string key for backward compatibility
		if v := ctx.Value(string(key)); v != nil {
			return []Field{String(fieldName, stringValue(v))}
		}
		return nil
	}
}

// Default extractors created using the factory function.
// These extract trace_id, span_id, and request_id from context values.
var (
	defaultTraceIDExtractor   = createDefaultExtractor(ContextKeyTraceID, "trace_id")
	defaultSpanIDExtractor    = createDefaultExtractor(ContextKeySpanID, "span_id")
	defaultRequestIDExtractor = createDefaultExtractor(ContextKeyRequestID, "request_id")
)

// stringValue converts any value to its string representation.
// Delegates to internal.MapKeyToString to avoid duplicating the type-switch logic.
func stringValue(v any) string {
	if v == nil {
		return ""
	}
	// MapKeyToString takes a reflect.Value; for simple cases, use a type switch
	switch val := v.(type) {
	case string:
		return val
	case bool:
		if val {
			return "true"
		}
		return "false"
	case fmt.Stringer:
		return val.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}
