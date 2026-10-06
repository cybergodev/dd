package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cybergodev/dd"
)

// Context & Hooks - Tracing Integration and Lifecycle Events
//
// Topics covered:
// 1. Type-safe context keys (trace_id, span_id, request_id)
// 2. Request-scoped logging via the WithFields pattern
// 3. Context extractors (global fields only!)
// 4. Hook system: BeforeLog/AfterLog, error handling, entry suppression
// 5. OnRotate hook (fired by actual file rotation)
//
// NOTE: constructor errors are ignored (logger, _) for brevity in these
// examples; see 07_convenience for error-handling patterns.
func main() {
	fmt.Println("=== DD Context & Hooks ===")

	section1ContextKeys()
	section2ContextLogging()
	section3CustomExtractors()
	section4Hooks()
	section5RotationHook()
	section6RequestScopedLogging()

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
	fmt.Println()
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
//
// IMPORTANT: Context extractors registered via Config are called with
// context.Background() since logging methods do not accept a context parameter.
// Use extractors for GLOBAL/static data (hostname, PID, deployment info).
// For REQUEST-SCOPED data, use the WithFields pattern shown in sections 2 & 6.
func section3CustomExtractors() {
	fmt.Println("3. Context Extractors (Global Fields)")
	fmt.Println("--------------------------------------")

	// Context extractors add GLOBAL fields to every log entry automatically.
	// They receive context.Background() since log methods don't accept context.
	hostnameExtractor := func(ctx context.Context) []dd.Field {
		hostname, _ := os.Hostname()
		return []dd.Field{dd.String("hostname", hostname)}
	}

	deployExtractor := func(ctx context.Context) []dd.Field {
		// Static environment info — available without context
		return []dd.Field{
			dd.String("service", "user-api"),
			dd.String("env", "production"),
		}
	}

	cfg := dd.DefaultConfig()
	cfg.ContextExtractors = []dd.ContextExtractor{hostnameExtractor, deployExtractor}
	logger, _ := dd.New(cfg)
	defer logger.Close()

	// Every log entry automatically includes hostname, service, and env
	logger.InfoWith("Request processed",
		dd.String("action", "data_access"),
	)

	// Extractors can also be added at runtime (the registry is cloned, so
	// this is safe concurrent with logging)
	logger.AddContextExtractor(func(ctx context.Context) []dd.Field {
		return []dd.Field{dd.Int("pid", os.Getpid())}
	})
	logger.Info("Runtime-added extractor: pid attached automatically")

	// For REQUEST-SCOPED data (trace_id, user_id, etc.), use WithFields:
	ctx := dd.WithTraceID(context.Background(), "trace-xyz")
	ctx = dd.WithSpanID(ctx, "span-789")

	logger.WithFields(
		dd.String("trace_id", dd.GetTraceID(ctx)),
		dd.String("span_id", dd.GetSpanID(ctx)),
	).InfoWith("Request with trace context",
		dd.String("action", "update"),
	)

	fmt.Println("  Global extractors: hostname, service, env (auto-added)")
	fmt.Println("  (extractor output is security-filtered too — a hostname matching")
	fmt.Println("   a sensitive pattern renders as [REDACTED])")
	fmt.Println("  Request-scoped: trace_id, span_id (via WithFields)")
	fmt.Println()
}

// Section 4: Hook system - lifecycle events, error handling, suppression
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
			// A BeforeLog hook that returns an error DROPS the entry —
			// use for policy enforcement (e.g., suppress debug in prod)
			func(ctx context.Context, hctx *dd.HookContext) error {
				if hctx.Level == dd.LevelDebug {
					return fmt.Errorf("debug suppressed by policy")
				}
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
	cfg.Level = dd.LevelDebug // let Debug entries past the level gate...
	cfg.Format = dd.FormatJSON
	cfg.Hooks = hooks

	logger, _ := dd.New(cfg)
	defer logger.Close()

	// Log messages trigger hooks
	logger.Info("This triggers BeforeLog and AfterLog hooks")

	// ...so the policy hook below is what drops this one (no output,
	// no AfterLog — BeforeLog errors abort the entry)
	logger.Debug("This entry is DROPPED by the policy BeforeLog hook")

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

// Section 5: OnRotate hook - fired automatically when a FileWriter target
// exceeds MaxSizeMB (rotation itself is demonstrated in 05_writers).
func section5RotationHook() {
	fmt.Println("5. OnRotate Hook (Live Rotation)")
	fmt.Println("---------------------------------")

	cfg := dd.DefaultConfig()
	fileTarget := dd.FileOutput("logs/hook-rotate.log")
	fileTarget.MaxSizeMB = 1  // smallest rotation unit
	fileTarget.MaxBackups = 2 // cap demo leftovers across repeated runs
	cfg.Targets = []dd.OutputTarget{fileTarget}

	logger, _ := dd.New(cfg)
	defer logger.Close()

	// The hook fires with the rotated file's path in Metadata
	logger.AddHook(dd.HookOnRotate, func(ctx context.Context, hctx *dd.HookContext) error {
		fmt.Printf("  [OnRotate] rotated: %v\n", hctx.Metadata["path"])
		return nil
	})

	// ~600-byte lines x 2200 = ~1.3MB -> one rotation at ~1MB
	payload := strings.Repeat("r", 512)
	for i := 0; i < 2200; i++ {
		logger.Infof("hook rotation %04d %s", i, payload)
	}

	fmt.Println("✓ OnRotate hook fired when the file rotated")
	fmt.Println()
}

// Section 6: Request-scoped logging pattern
func section6RequestScopedLogging() {
	fmt.Println("6. Request-Scoped Logging")
	fmt.Println("---------------------------")

	cfg := dd.DefaultConfig()
	cfg.Format = dd.FormatJSON
	cfg.Targets = []dd.OutputTarget{dd.FileOutput("logs/requests.log")}

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
