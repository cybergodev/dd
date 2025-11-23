//go:build examples

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/cybergodev/dd"
)

func main() {
	fmt.Println("=== DD Cloud & Log Aggregation ===\n ")

	elkStack()
	cloudWatch()
	distributedTracing()

	fmt.Println("\n✅ Examples completed")
}

// 1. ELK Stack - Elasticsearch, Logstash, Kibana
func elkStack() {
	fmt.Println("1. ELK Stack Format")

	// Use @timestamp field name for ELK compatibility
	config := dd.JSONConfig()
	config.JSON.FieldNames = &dd.JSONFieldNames{
		Timestamp: "@timestamp",
		Level:     "level",
		Message:   "message",
	}

	logger, _ := dd.New(config)
	defer logger.Close()

	// Service metadata
	logger.InfoWith("Application started",
		dd.String("service.name", "user-api"),
		dd.String("service.version", "1.2.3"),
		dd.String("environment", "production"),
		dd.Int("pid", os.Getpid()),
	)

	// HTTP request with standard fields
	logger.InfoWith("HTTP request",
		dd.String("http.method", "POST"),
		dd.String("http.url", "/api/users"),
		dd.Int("http.status", 201),
		dd.Float64("response_time_ms", 45.2),
		dd.String("client.ip", "192.168.1.100"),
	)

	// Error with structured fields
	logger.ErrorWith("Database error",
		dd.String("error.type", "ConnectionTimeout"),
		dd.String("db.host", "db.example.com"),
		dd.Int("db.port", 5432),
	)
}

// 2. AWS CloudWatch - ECS and Lambda logging
func cloudWatch() {
	fmt.Println("\n2. AWS CloudWatch Format")

	config := dd.JSONConfig()
	config.WithFile("logs/cloudwatch.log", nil)
	logger, _ := dd.New(config)
	defer logger.Close()

	// ECS container logging
	logger.InfoWith("ECS task started",
		dd.String("aws.region", "us-east-1"),
		dd.String("aws.service", "ecs"),
		dd.String("ecs.cluster", "production"),
		dd.String("ecs.task", "user-api:123"),
		dd.String("container.name", "user-api"),
		dd.String("container.id", "abc123"),
	)

	// Lambda function logging
	logger.InfoWith("Lambda execution",
		dd.String("function_name", "user-processor"),
		dd.String("request_id", "req-abc-123"),
		dd.Int("memory_mb", 512),
		dd.Float64("duration_ms", 1250.5),
		dd.Bool("cold_start", false),
	)
}

// 3. Distributed Tracing - OpenTelemetry, Jaeger, Zipkin
func distributedTracing() {
	fmt.Println("\n3. Distributed Tracing")

	config := dd.JSONConfig()
	config.WithFile("logs/tracing.log", nil)
	logger, _ := dd.New(config)
	defer logger.Close()

	traceID := "1234567890abcdef"
	spanID := "abcdef1234567890"

	// OpenTelemetry format
	logger.InfoWith("Span started",
		dd.String("trace.id", traceID),
		dd.String("span.id", spanID),
		dd.String("span.name", "user.create"),
		dd.String("span.kind", "server"),
		dd.Float64("duration_ms", 100.5),
		dd.String("service.name", "user-api"),
	)

	// Jaeger format
	logger.InfoWith("Database query",
		dd.String("traceID", traceID),
		dd.String("spanID", spanID),
		dd.String("operationName", "db.query"),
		dd.Int64("startTime", time.Now().UnixMicro()),
		dd.Int("duration", 50000),
		dd.String("db.statement", "SELECT * FROM users"),
		dd.String("db.type", "postgresql"),
	)

	// Cross-service correlation
	logger.InfoWith("Service call",
		dd.String("trace_id", traceID),
		dd.String("request_id", "req-abc-123"),
		dd.String("caller", "frontend"),
		dd.String("callee", "user-api"),
		dd.String("operation", "create_user"),
		dd.Bool("success", true),
	)
}
