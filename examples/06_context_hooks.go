//go:build examples

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/cybergodev/dd"
)

// Context & Hooks - Tracing Integration and Lifecycle Events
//
// Topics covered:
// 1. Type-safe context keys (trace_id, span_id, request_id)
// 2. Context extraction via configured extractors
// 3. Custom context extractors
// 4. Hook system for lifecycle events
// 5. OpenTelemetry-style integration
func main() {
	fmt.Println("=== DD Context & Hooks ===")

	section1ContextKeys()
	section2ContextLogging()
	section3CustomExtractors()
	section4Hooks()
	section5RequestScopedLogging()

	fmt.Println("\n✅ Context & Hooks examples completed!")
}

// Section 1: Type-safe context keys
func section1ContextKeys() {
	fmt.Println("1. Context Keys")
	fmt.Println("----------------")

	ctx := context.Background()

	// Add tracing metadata to context
	ctx = dd.WithTraceID(ctx, "trace-abc123")
	ctx = dd.WithSpanID(ctx, "span-def456")
	ctx = dd.WithRequestID(ctx, "req-789xyz")

	// Retrieve values
	traceID := dd.GetTraceID(ctx)
	spanID := dd.GetSpanID(ctx)
	requestID := dd.GetRequestID(ctx)

	fmt.Printf("  Trace ID: %s\n", traceID)
	fmt.Printf("  Span ID: %s\n", spanID)
	fmt.Printf("  Request ID: %s\n", requestID)

	fmt.Println()
}

// Section 2: Context-aware logging with WithFields pattern
func section2ContextLogging() {
	fmt.Println("2. Context-Aware Logging (WithFields Pattern)")
	fmt.Println("-----------------------------------------------")

	logger, _ := dd.New()
	defer logger.Close()

	// Create context with trace info
	ctx := dd.WithTraceID(context.Background(), "trace-123")
	ctx = dd.WithSpanID(ctx, "span-456")

	// Pattern 1: Extract context fields and pass to WithFields
	// This is the recommended way to include context data in logs
	entry := logger.WithFields(
		dd.String("trace_id", dd.GetTraceID(ctx)),
		dd.String("span_id", dd.GetSpanID(ctx)),
	)
	entry.InfoWith("Processing request",
		dd.String("user", "alice"),
	)

	// Pattern 2: Use helper function for extraction
	traceFields := extractTraceFields(ctx)
	logger.InfoWith("User action", append(traceFields,
		dd.String("action", "login"),
		dd.String("user", "alice"),
	)...)

	fmt.Println("✓ Trace IDs included via WithFields pattern")
}

// extractTraceFields is a helper to extract trace context as fields
func extractTraceFields(ctx context.Context) []dd.Field {
	var fields []dd.Field
	if traceID := dd.GetTraceID(ctx); traceID != "" {
		fields = append(fields, dd.String("trace_id", traceID))
	}
	if spanID := dd.GetSpanID(ctx); spanID != "" {
		fields = append(fields, dd.String("span_id", spanID))
	}
	if requestID := dd.GetRequestID(ctx); requestID != "" {
		fields = append(fields, dd.String("request_id", requestID))
	}
	return fields
}

// Section 3: Context extractor pattern
func section3CustomExtractors() {
	fmt.Println("3. Context Extractors")
	fmt.Println("-----------------------")

	// Define custom context extractors for application-specific values
	tenantExtractor := func(ctx context.Context) []dd.Field {
		var fields []dd.Field
		if v := ctx.Value("tenant_id"); v != nil {
			if s, ok := v.(string); ok {
				fields = append(fields, dd.String("tenant_id", s))
			}
		}
		return fields
	}

	userExtractor := func(ctx context.Context) []dd.Field {
		if v := ctx.Value("user_id"); v != nil {
			if i, ok := v.(int); ok {
				return []dd.Field{dd.Int("user_id", i)}
			}
		}
		return nil
	}

	// Pass extractors via Config for automatic context field extraction
	cfg := dd.DefaultConfig()
	cfg.ContextExtractors = []dd.ContextExtractor{tenantExtractor, userExtractor}
	logger, _ := dd.New(cfg)
	defer logger.Close()

	// Create context with application-specific values
	ctx := context.WithValue(context.Background(), "tenant_id", "tenant-abc")
	ctx = context.WithValue(ctx, "user_id", 12345)

	// Context fields are extracted automatically when logging
	logger.InfoWith("Request with extracted context",
		dd.String("action", "data_access"),
	)

	// Combine with built-in trace helpers for full request context
	ctx = dd.WithTraceID(ctx, "trace-xyz")
	ctx = dd.WithSpanID(ctx, "span-789")

	logger.InfoWith("Full request context",
		dd.String("trace_id", dd.GetTraceID(ctx)),
		dd.String("span_id", dd.GetSpanID(ctx)),
		dd.String("action", "update"),
	)

	fmt.Println("  Tip: Use Config.ContextExtractors to register custom extractors,")
	fmt.Println("  then combine with WithTraceID/WithSpanID for full request context.")
	fmt.Println()
}

// Section 4: Hook system
func section4Hooks() {
	fmt.Println("4. Hook System")
	fmt.Println("---------------")

	// Create hook registry with HooksConfig (struct-based configuration)
	hooks := dd.NewHooksFromConfig(dd.HooksConfig{
		BeforeLog: []dd.Hook{
			func(ctx context.Context, hctx *dd.HookContext) error {
				fmt.Printf("  [BeforeLog] Level: %s, Msg: %s\n",
					hctx.Level.String(), hctx.Message)
				return nil
			},
		},
		AfterLog: []dd.Hook{
			func(ctx context.Context, hctx *dd.HookContext) error {
				fmt.Printf("  [AfterLog] Completed: %s\n", hctx.Message)
				return nil
			},
		},
		OnError: []dd.Hook{
			func(ctx context.Context, hctx *dd.HookContext) error {
				fmt.Printf("  [OnError] %v\n", hctx.Error)
				return nil
			},
		},
	})

	cfg := dd.DefaultConfig()
	cfg.Format = dd.FormatJSON
	cfg.Hooks = hooks

	logger, _ := dd.New(cfg)
	defer logger.Close()

	// Log messages trigger hooks
	logger.Info("This triggers BeforeLog and AfterLog hooks")

	// Add hooks at runtime
	logger.AddHook(dd.HookOnFilter, func(ctx context.Context, hctx *dd.HookContext) error {
		fmt.Printf("  [OnFilter] Data was filtered\n")
		return nil
	})

	fmt.Println()
}

// OpenTelemetry Integration Reference (see source code comments)
// This shows how to integrate with OpenTelemetry (requires opentelemetry-go package):
//
//	import "go.opentelemetry.io/otel/trace"
//
//	otelExtractor := func(ctx context.Context) []dd.Field {
//	    span := trace.SpanFromContext(ctx)
//	    if !span.SpanContext().IsValid() {
//	        return nil
//	    }
//	    return []dd.Field{
//	        dd.String("trace_id", span.SpanContext().TraceID().String()),
//	        dd.String("span_id", span.SpanContext().SpanID().String()),
//	        dd.Bool("sampled", span.SpanContext().IsSampled()),
//	    }
//	}

// Section 5: Request-scoped logging pattern
func section5RequestScopedLogging() {
	fmt.Println("5. Request-Scoped Logging")
	fmt.Println("---------------------------")

	cfg := dd.DefaultConfig()
	cfg.Format = dd.FormatJSON
	cfg.File = &dd.FileConfig{Path: "logs/requests.log"}

	logger, _ := dd.New(cfg)
	defer logger.Close()

	// Simulated HTTP handler with context extraction
	handler := func(ctx context.Context, path string) {
		ctx = dd.WithRequestID(ctx, fmt.Sprintf("req-%d", time.Now().UnixNano()))
		ctx = dd.WithTraceID(ctx, "trace-from-header")

		// Create a request-scoped logger with context fields
		reqLogger := logger.WithFields(
			dd.String("trace_id", dd.GetTraceID(ctx)),
			dd.String("request_id", dd.GetRequestID(ctx)),
			dd.String("path", path),
		)

		reqLogger.Info("Request started")

		// Business logic...

		reqLogger.InfoWith("Request completed",
			dd.Int("status", 200),
			dd.Duration("duration", 50*time.Millisecond),
		)
	}

	handler(context.Background(), "/api/users")
	fmt.Println("✓ Request-scoped logging pattern demonstrated")
}
