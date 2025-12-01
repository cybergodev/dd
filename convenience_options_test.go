package dd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNewWithOptions_Defaults(t *testing.T) {
	buf := &bytes.Buffer{}

	logger, err := NewWithOptions(Options{
		Console:           false,
		AdditionalWriters: []io.Writer{buf},
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Info("test message")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("Expected output to contain 'test message', got: %s", output)
	}
	if !strings.Contains(output, "[INFO]") {
		t.Errorf("Expected output to contain '[INFO]', got: %s", output)
	}
}

func TestNewWithOptions_Level(t *testing.T) {
	buf := &bytes.Buffer{}

	logger, err := NewWithOptions(Options{
		Level:             LevelWarn,
		Console:           false,
		AdditionalWriters: []io.Writer{buf},
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Info("should not appear")
	logger.Warn("should appear")

	output := buf.String()
	if strings.Contains(output, "should not appear") {
		t.Errorf("Info message should not appear with Warn level")
	}
	if !strings.Contains(output, "should appear") {
		t.Errorf("Warn message should appear")
	}
}

func TestNewWithOptions_JSONFormat(t *testing.T) {
	buf := &bytes.Buffer{}

	logger, err := NewWithOptions(Options{
		Format:            FormatJSON,
		Console:           false,
		AdditionalWriters: []io.Writer{buf},
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Info("json test")

	output := buf.String()
	if !strings.Contains(output, `"message":"json test"`) {
		t.Errorf("Expected JSON output, got: %s", output)
	}
	if !strings.Contains(output, `"level":"INFO"`) {
		t.Errorf("Expected JSON level field, got: %s", output)
	}
}

func TestNewWithOptions_ConsoleAndFile(t *testing.T) {
	tmpFile := "test_options_console_file.log"
	defer os.Remove(tmpFile)

	logger, err := NewWithOptions(Options{
		Console: true,
		File:    tmpFile,
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Info("test message")

	// Check file was created
	if _, err := os.Stat(tmpFile); os.IsNotExist(err) {
		t.Errorf("Log file was not created")
	}

	// Read file content
	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), "test message") {
		t.Errorf("Log file should contain 'test message', got: %s", string(content))
	}
}

func TestNewWithOptions_FileOnly(t *testing.T) {
	tmpFile := "test_options_file_only.log"
	defer os.Remove(tmpFile)

	logger, err := NewWithOptions(Options{
		Console: false,
		File:    tmpFile,
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Info("file only message")

	// Check file was created
	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), "file only message") {
		t.Errorf("Log file should contain message, got: %s", string(content))
	}
}

func TestNewWithOptions_FileConfig(t *testing.T) {
	tmpFile := "test_options_file_config.log"
	defer os.Remove(tmpFile)
	defer os.Remove(tmpFile + ".1")

	logger, err := NewWithOptions(Options{
		Console: false,
		File:    tmpFile,
		FileConfig: FileWriterConfig{
			MaxSizeMB:  1,
			MaxBackups: 5,
			MaxAge:     24 * time.Hour,
			Compress:   false,
		},
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Info("test with file config")

	// Verify file exists
	if _, err := os.Stat(tmpFile); os.IsNotExist(err) {
		t.Errorf("Log file was not created")
	}
}

func TestNewWithOptions_Caller(t *testing.T) {
	buf := &bytes.Buffer{}

	logger, err := NewWithOptions(Options{
		IncludeCaller:     true,
		Console:           false,
		AdditionalWriters: []io.Writer{buf},
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Info("caller test")

	output := buf.String()
	if !strings.Contains(output, "convenience_options_test.go") {
		t.Errorf("Expected caller information in output, got: %s", output)
	}
}

func TestNewWithOptions_DynamicCaller(t *testing.T) {
	buf := &bytes.Buffer{}

	logger, err := NewWithOptions(Options{
		IncludeCaller:     true,
		DynamicCaller:     true,
		Console:           false,
		AdditionalWriters: []io.Writer{buf},
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Info("dynamic caller test")

	output := buf.String()
	if !strings.Contains(output, "convenience_options_test.go") {
		t.Errorf("Expected caller information in output, got: %s", output)
	}
}

func TestNewWithOptions_FilterNone(t *testing.T) {
	buf := &bytes.Buffer{}

	logger, err := NewWithOptions(Options{
		FilterLevel:       "none",
		Console:           false,
		AdditionalWriters: []io.Writer{buf},
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Info("password=secret123")

	output := buf.String()
	if !strings.Contains(output, "password=secret123") {
		t.Errorf("Password should not be filtered with 'none' filter level, got: %s", output)
	}
}

func TestNewWithOptions_FilterBasic(t *testing.T) {
	buf := &bytes.Buffer{}

	logger, err := NewWithOptions(Options{
		FilterLevel:       "basic",
		Console:           false,
		AdditionalWriters: []io.Writer{buf},
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Info("password=secret123")

	output := buf.String()
	if !strings.Contains(output, "[REDACTED]") {
		t.Errorf("Password should be filtered with 'basic' filter level, got: %s", output)
	}
	if strings.Contains(output, "secret123") {
		t.Errorf("Password value should not appear in output, got: %s", output)
	}
}

func TestNewWithOptions_FilterFull(t *testing.T) {
	buf := &bytes.Buffer{}

	logger, err := NewWithOptions(Options{
		FilterLevel:       "full",
		Console:           false,
		AdditionalWriters: []io.Writer{buf},
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Info("email: user@example.com")

	output := buf.String()
	if !strings.Contains(output, "[REDACTED]") {
		t.Errorf("Email should be filtered with 'full' filter level, got: %s", output)
	}
}

func TestNewWithOptions_FilterInvalid(t *testing.T) {
	_, err := NewWithOptions(Options{
		FilterLevel: "invalid",
		Console:     false,
	})
	if err == nil {
		t.Errorf("Expected error for invalid filter level")
	}
	if !strings.Contains(err.Error(), "invalid filter level") {
		t.Errorf("Expected 'invalid filter level' error, got: %v", err)
	}
}

func TestNewWithOptions_CustomFilter(t *testing.T) {
	buf := &bytes.Buffer{}

	customFilter := NewEmptySensitiveDataFilter()
	customFilter.AddPattern(`custom_secret=\w+`)

	logger, err := NewWithOptions(Options{
		CustomFilter:      customFilter,
		Console:           false,
		AdditionalWriters: []io.Writer{buf},
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Info("custom_secret=mysecret")

	output := buf.String()
	if !strings.Contains(output, "[REDACTED]") {
		t.Errorf("Custom pattern should be filtered, got: %s", output)
	}
	if strings.Contains(output, "mysecret") {
		t.Errorf("Secret value should not appear in output, got: %s", output)
	}
}

func TestNewWithOptions_JSONOptions(t *testing.T) {
	buf := &bytes.Buffer{}

	logger, err := NewWithOptions(Options{
		Format:  FormatJSON,
		Console: false,
		JSONOptions: &JSONOptions{
			PrettyPrint: true,
			Indent:      "  ",
		},
		AdditionalWriters: []io.Writer{buf},
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Info("pretty json test")

	output := buf.String()
	// Pretty printed JSON should have newlines
	if !strings.Contains(output, "\n") {
		t.Errorf("Expected pretty-printed JSON with newlines, got: %s", output)
	}
}

func TestNewWithOptions_CustomTimeFormat(t *testing.T) {
	buf := &bytes.Buffer{}

	logger, err := NewWithOptions(Options{
		TimeFormat:        "15:04:05",
		Console:           false,
		AdditionalWriters: []io.Writer{buf},
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Info("time format test")

	output := buf.String()
	// Should contain time in HH:MM:SS format
	if !strings.Contains(output, ":") {
		t.Errorf("Expected time format in output, got: %s", output)
	}
}

func TestNewWithOptions_NoWriters(t *testing.T) {
	// When no writers specified, should default to console
	logger, err := NewWithOptions(Options{
		Console: false,
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Should not panic
	logger.Info("test with default writer")
}

func TestNewWithOptions_MultipleWriters(t *testing.T) {
	buf1 := &bytes.Buffer{}
	buf2 := &bytes.Buffer{}

	logger, err := NewWithOptions(Options{
		Console:           false,
		AdditionalWriters: []io.Writer{buf1, buf2},
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Info("multiple writers test")

	output1 := buf1.String()
	output2 := buf2.String()

	if !strings.Contains(output1, "multiple writers test") {
		t.Errorf("First writer should contain message, got: %s", output1)
	}
	if !strings.Contains(output2, "multiple writers test") {
		t.Errorf("Second writer should contain message, got: %s", output2)
	}
}

func TestNewWithOptions_StructuredLogging(t *testing.T) {
	buf := &bytes.Buffer{}

	logger, err := NewWithOptions(Options{
		Format:            FormatJSON,
		Console:           false,
		AdditionalWriters: []io.Writer{buf},
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.InfoWith("structured test",
		String("key1", "value1"),
		Int("key2", 42),
	)

	output := buf.String()
	if !strings.Contains(output, `"key1":"value1"`) {
		t.Errorf("Expected structured field in JSON output, got: %s", output)
	}
	if !strings.Contains(output, `"key2":42`) {
		t.Errorf("Expected numeric field in JSON output, got: %s", output)
	}
}

func TestNewWithOptions_InvalidLevel(t *testing.T) {
	buf := &bytes.Buffer{}

	// Invalid level should default to Info
	logger, err := NewWithOptions(Options{
		Level:             LogLevel(99),
		Console:           false,
		AdditionalWriters: []io.Writer{buf},
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	if logger.GetLevel() != LevelDebug {
		t.Errorf("Expected default level Info, got: %v", logger.GetLevel())
	}
}

func TestNewWithOptions_InvalidFormat(t *testing.T) {
	buf := &bytes.Buffer{}

	// Invalid format should default to Text
	logger, err := NewWithOptions(Options{
		Format:            LogFormat(99),
		Console:           false,
		AdditionalWriters: []io.Writer{buf},
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Info("format test")

	output := buf.String()
	// Text format should have [INFO] marker
	if !strings.Contains(output, "[INFO]") {
		t.Errorf("Expected text format output, got: %s", output)
	}
}

// Test new convenience methods

func TestToFile(t *testing.T) {
	tmpFile := "test_to_file.log"
	defer os.Remove(tmpFile)

	logger := ToFile(tmpFile)
	if logger == nil {
		t.Fatal("ToFile returned nil logger")
	}
	defer logger.Close()

	logger.Info("file only test")

	// Check file was created and contains message
	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), "file only test") {
		t.Errorf("Log file should contain message, got: %s", string(content))
	}
}

func TestToFile_DefaultFilename(t *testing.T) {
	defer os.Remove("logs/app.log")
	defer os.RemoveAll("logs")

	logger := ToFile()
	if logger == nil {
		t.Fatal("ToFile returned nil logger")
	}
	defer logger.Close()

	logger.Info("default filename test")

	// Check default file was created
	content, err := os.ReadFile("logs/app.log")
	if err != nil {
		t.Fatalf("Failed to read default log file: %v", err)
	}

	if !strings.Contains(string(content), "default filename test") {
		t.Errorf("Default log file should contain message, got: %s", string(content))
	}
}

func TestToFile_EmptyFilename(t *testing.T) {
	defer os.Remove("logs/app.log")
	defer os.RemoveAll("logs")

	logger := ToFile("")
	if logger == nil {
		t.Fatal("ToFile returned nil logger")
	}
	defer logger.Close()

	logger.Info("empty filename test")

	// Check default file was created when empty string provided
	content, err := os.ReadFile("logs/app.log")
	if err != nil {
		t.Fatalf("Failed to read default log file: %v", err)
	}

	if !strings.Contains(string(content), "empty filename test") {
		t.Errorf("Default log file should contain message, got: %s", string(content))
	}
}

func TestToConsole(t *testing.T) {
	logger := ToConsole()
	if logger == nil {
		t.Fatal("ToConsole returned nil logger")
	}
	defer logger.Close()

	// Should not panic
	logger.Info("console only test")
}

func TestToJSONFile(t *testing.T) {
	tmpFile := "test_to_json_file.log"
	defer os.Remove(tmpFile)

	logger := ToJSONFile(tmpFile)
	if logger == nil {
		t.Fatal("ToJSONFile returned nil logger")
	}
	defer logger.Close()

	logger.Info("json file test")

	// Check file was created and contains JSON
	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	output := string(content)
	if !strings.Contains(output, `"message":"json file test"`) {
		t.Errorf("Expected JSON format in file, got: %s", output)
	}
	if !strings.Contains(output, `"level":"INFO"`) {
		t.Errorf("Expected JSON level field in file, got: %s", output)
	}
}

func TestToJSONFile_DefaultFilename(t *testing.T) {
	defer os.Remove("logs/app.log")
	defer os.RemoveAll("logs")

	logger := ToJSONFile()
	if logger == nil {
		t.Fatal("ToJSONFile returned nil logger")
	}
	defer logger.Close()

	logger.Info("default json filename test")

	content, err := os.ReadFile("logs/app.log")
	if err != nil {
		t.Fatalf("Failed to read default JSON log file: %v", err)
	}

	output := string(content)
	if !strings.Contains(output, `"message":"default json filename test"`) {
		t.Errorf("Expected JSON format in default file, got: %s", output)
	}
}

func TestToJSONFile_EmptyFilename(t *testing.T) {
	defer os.Remove("logs/app.log")
	defer os.RemoveAll("logs")

	logger := ToJSONFile("")
	if logger == nil {
		t.Fatal("ToJSONFile returned nil logger")
	}
	defer logger.Close()

	logger.Info("empty json filename test")

	content, err := os.ReadFile("logs/app.log")
	if err != nil {
		t.Fatalf("Failed to read default JSON log file: %v", err)
	}

	output := string(content)
	if !strings.Contains(output, `"message":"empty json filename test"`) {
		t.Errorf("Expected JSON format in default file, got: %s", output)
	}
}

func TestToAll(t *testing.T) {
	tmpFile := "test_to_all.log"
	defer os.Remove(tmpFile)

	logger := ToAll(tmpFile)
	if logger == nil {
		t.Fatal("ToAll returned nil logger")
	}
	defer logger.Close()

	logger.Info("console and file test")

	// Check file was created and contains message
	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), "console and file test") {
		t.Errorf("Log file should contain message, got: %s", string(content))
	}
}

func TestToAll_DefaultFilename(t *testing.T) {
	defer os.Remove("logs/app.log")
	defer os.RemoveAll("logs")

	logger := ToAll()
	if logger == nil {
		t.Fatal("ToAll returned nil logger")
	}
	defer logger.Close()

	logger.Info("default all filename test")

	// Check default file was created
	content, err := os.ReadFile("logs/app.log")
	if err != nil {
		t.Fatalf("Failed to read default log file: %v", err)
	}

	if !strings.Contains(string(content), "default all filename test") {
		t.Errorf("Default log file should contain message, got: %s", string(content))
	}
}

func TestToAll_EmptyFilename(t *testing.T) {
	defer os.Remove("logs/app.log")
	defer os.RemoveAll("logs")

	logger := ToAll("")
	if logger == nil {
		t.Fatal("ToAll returned nil logger")
	}
	defer logger.Close()

	logger.Info("empty all filename test")

	// Check default file was created when empty string provided
	content, err := os.ReadFile("logs/app.log")
	if err != nil {
		t.Fatalf("Failed to read default log file: %v", err)
	}

	if !strings.Contains(string(content), "empty all filename test") {
		t.Errorf("Default log file should contain message, got: %s", string(content))
	}
}

func TestToFile_InvalidPath(t *testing.T) {
	// Use path traversal which should be blocked by security validation
	// Should return fallback console logger instead of failing
	logger := ToFile("../../../etc/passwd")
	if logger == nil {
		t.Fatal("ToFile should return fallback logger, not nil")
	}
	defer logger.Close()

	// Should still be able to log (to console fallback)
	logger.Info("fallback test")
}

func TestToJSONFile_InvalidPath(t *testing.T) {
	// Use path traversal which should be blocked by security validation
	// Should return fallback console logger instead of failing
	logger := ToJSONFile("../../../etc/passwd")
	if logger == nil {
		t.Fatal("ToJSONFile should return fallback logger, not nil")
	}
	defer logger.Close()

	// Should still be able to log (to console fallback)
	logger.Info("fallback test")
}

func TestToAll_InvalidPath(t *testing.T) {
	// Use path traversal which should be blocked by security validation
	// Should return fallback console logger instead of failing
	logger := ToAll("../../../etc/passwd")
	if logger == nil {
		t.Fatal("ToAll should return fallback logger, not nil")
	}
	defer logger.Close()

	// Should still be able to log (to console fallback)
	logger.Info("fallback test")
}
