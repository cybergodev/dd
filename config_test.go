package dd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// CONFIG CREATION TESTS
// ============================================================================

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.Level != LevelInfo {
		t.Errorf("Default level = %v, want %v", config.Level, LevelInfo)
	}
	if config.Format != FormatText {
		t.Errorf("Default format = %v, want %v", config.Format, FormatText)
	}
	if config.TimeFormat != DefaultTimeFormat {
		t.Errorf("Default time format = %v, want %v", config.TimeFormat, DefaultTimeFormat)
	}
	if config.IncludeTime != true {
		t.Error("Default should include time")
	}
	if config.IncludeLevel != true {
		t.Error("Default should include level")
	}
	if config.SecurityConfig == nil {
		t.Error("Default should have security config")
	}
}

func TestDevelopmentConfig(t *testing.T) {
	config := DevelopmentConfig()

	if config.Level != LevelDebug {
		t.Errorf("Dev level = %v, want %v", config.Level, LevelDebug)
	}
	if config.IncludeCaller != true {
		t.Error("Dev config should include caller")
	}
	if config.DynamicCaller != true {
		t.Error("Dev config should enable dynamic caller")
	}
	if config.FullPath != false {
		t.Error("Dev config should use short paths")
	}
}

func TestJSONConfig(t *testing.T) {
	config := JSONConfig()

	if config.Format != FormatJSON {
		t.Errorf("JSON config format = %v, want %v", config.Format, FormatJSON)
	}
	if config.JSON == nil {
		t.Error("JSON config should have JSON options")
	}
	if config.IncludeCaller != true {
		t.Error("JSON config should include caller")
	}
}

// ============================================================================
// CONFIG VALIDATION TESTS
// ============================================================================

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *LoggerConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name:    "valid default",
			config:  DefaultConfig(),
			wantErr: false,
		},
		{
			name: "invalid level",
			config: &LoggerConfig{
				Level:  LogLevel(99),
				Format: FormatText,
			},
			wantErr: true,
		},
		{
			name: "invalid format",
			config: &LoggerConfig{
				Level:  LevelInfo,
				Format: LogFormat(99),
			},
			wantErr: true,
		},
		{
			name: "empty time format with time enabled",
			config: &LoggerConfig{
				Level:       LevelInfo,
				Format:      FormatText,
				IncludeTime: true,
				TimeFormat:  "",
			},
			wantErr: false, // Should apply default
		},
		{
			name: "no writers",
			config: &LoggerConfig{
				Level:   LevelInfo,
				Format:  FormatText,
				Writers: nil,
			},
			wantErr: false, // Should apply default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Check defaults were applied
			if !tt.wantErr && tt.config != nil {
				if tt.config.IncludeTime && tt.config.TimeFormat == "" {
					t.Error("TimeFormat should have default value")
				}
				if len(tt.config.Writers) == 0 {
					t.Error("Writers should have default value")
				}
				if tt.config.SecurityConfig == nil {
					t.Error("SecurityConfig should be initialized")
				}
			}
		})
	}
}

// ============================================================================
// CONFIG CLONING TESTS
// ============================================================================

func TestConfigClone(t *testing.T) {
	original := DefaultConfig()
	original.Level = LevelDebug
	original.SecurityConfig = &SecurityConfig{
		MaxMessageSize:  2048,
		MaxWriters:      50,
		SensitiveFilter: NewBasicSensitiveDataFilter(),
	}

	clone := original.Clone()

	// Verify values are copied
	if clone.Level != original.Level {
		t.Error("Level not cloned correctly")
	}

	// Verify deep copy (modifying clone shouldn't affect original)
	clone.Level = LevelError
	if original.Level == LevelError {
		t.Error("Clone is not independent - level change affected original")
	}

	// Verify filter is cloned
	if clone.SecurityConfig.SensitiveFilter == nil {
		t.Error("SensitiveFilter not cloned")
	}
	clone.SecurityConfig.SensitiveFilter.AddPattern(`test_pattern`)
	if original.SecurityConfig.SensitiveFilter.PatternCount() == clone.SecurityConfig.SensitiveFilter.PatternCount() {
		t.Error("Filter clone is not independent")
	}

	// Verify security config is cloned
	if clone.SecurityConfig == nil {
		t.Error("SecurityConfig not cloned")
	}
	if clone.SecurityConfig.MaxMessageSize != 2048 {
		t.Error("SecurityConfig values not cloned correctly")
	}
}

func TestConfigCloneNil(t *testing.T) {
	var config *LoggerConfig
	clone := config.Clone()

	if clone == nil {
		t.Error("Clone of nil should return default config")
	}
}

func TestConfigCloneWithWriters(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	original := DefaultConfig()
	original.Writers = []io.Writer{&buf1, &buf2}

	clone := original.Clone()

	if len(clone.Writers) != 2 {
		t.Errorf("Writers not cloned, got %d writers", len(clone.Writers))
	}

	// Verify it's a copy, not the same slice
	clone.Writers = append(clone.Writers, &bytes.Buffer{})
	if len(original.Writers) == len(clone.Writers) {
		t.Error("Writers slice is not independent")
	}
}

// ============================================================================
// FLUENT API TESTS
// ============================================================================

func TestConfigFluentAPI(t *testing.T) {
	var buf bytes.Buffer

	config := DefaultConfig().
		WithLevel(LevelDebug).
		WithFormat(FormatJSON).
		WithCaller(true).
		WithDynamicCaller(true).
		WithWriter(&buf).
		EnableFullFiltering()

	if config.Level != LevelDebug {
		t.Error("Chained WithLevel() failed")
	}
	if config.Format != FormatJSON {
		t.Error("Chained WithFormat() failed")
	}
	if !config.IncludeCaller {
		t.Error("Chained WithCaller() failed")
	}
	if !config.DynamicCaller {
		t.Error("Chained WithDynamicCaller() failed")
	}
	if config.SecurityConfig.SensitiveFilter == nil {
		t.Error("Chained EnableFullFiltering() failed")
	}

	found := false
	for _, w := range config.Writers {
		if w == &buf {
			found = true
			break
		}
	}
	if !found {
		t.Error("Chained WithWriter() failed")
	}
}

func TestConfigWithLevel(t *testing.T) {
	config := DefaultConfig().WithLevel(LevelWarn)
	if config.Level != LevelWarn {
		t.Errorf("WithLevel() = %v, want %v", config.Level, LevelWarn)
	}
}

func TestConfigWithFormat(t *testing.T) {
	config := DefaultConfig().WithFormat(FormatJSON)
	if config.Format != FormatJSON {
		t.Errorf("WithFormat() = %v, want %v", config.Format, FormatJSON)
	}
}

func TestConfigWithCaller(t *testing.T) {
	config := DefaultConfig().WithCaller(true)
	if !config.IncludeCaller {
		t.Error("WithCaller(true) should enable caller")
	}

	config = DefaultConfig().WithCaller(false)
	if config.IncludeCaller {
		t.Error("WithCaller(false) should disable caller")
	}
}

func TestConfigWithDynamicCaller(t *testing.T) {
	config := DefaultConfig().WithDynamicCaller(true)
	if !config.DynamicCaller {
		t.Error("WithDynamicCaller(true) should enable dynamic caller")
	}
}

func TestConfigWithWriter(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig().WithWriter(&buf)

	found := false
	for _, w := range config.Writers {
		if w == &buf {
			found = true
			break
		}
	}

	if !found {
		t.Error("WithWriter() should add writer")
	}
}

func TestConfigWithWriterNil(t *testing.T) {
	originalLen := len(DefaultConfig().Writers)
	config := DefaultConfig().WithWriter(nil)

	if len(config.Writers) != originalLen {
		t.Error("WithWriter(nil) should not add writer")
	}
}

// ============================================================================
// FILTER CONFIGURATION TESTS
// ============================================================================

func TestConfigDisableFiltering(t *testing.T) {
	config := DefaultConfig().DisableFiltering()

	if config.SecurityConfig.SensitiveFilter != nil {
		t.Error("DisableFiltering() should set filter to nil")
	}
}

func TestConfigEnableBasicFiltering(t *testing.T) {
	config := DefaultConfig().
		DisableFiltering().
		EnableBasicFiltering()

	if config.SecurityConfig.SensitiveFilter == nil {
		t.Error("EnableBasicFiltering() should set filter")
	}
}

func TestConfigEnableFullFiltering(t *testing.T) {
	config := DefaultConfig().
		DisableFiltering().
		EnableFullFiltering()

	if config.SecurityConfig.SensitiveFilter == nil {
		t.Error("EnableFullFiltering() should set filter")
	}

	// Full filter should have more patterns than basic
	basicFilter := NewBasicSensitiveDataFilter()
	if config.SecurityConfig.SensitiveFilter.PatternCount() <= basicFilter.PatternCount() {
		t.Error("Full filter should have more patterns than basic")
	}
}

func TestConfigWithFilter(t *testing.T) {
	customFilter := NewEmptySensitiveDataFilter()
	customFilter.AddPattern(`custom_pattern`)

	config := DefaultConfig().WithFilter(customFilter)

	if config.SecurityConfig.SensitiveFilter != customFilter {
		t.Error("WithFilter() should set custom filter")
	}
}

// ============================================================================
// FILE CONFIGURATION TESTS
// ============================================================================

func TestConfigWithFileOnly(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := tmpDir + "/test.log"

	config, err := DefaultConfig().WithFileOnly(logFile, FileWriterConfig{})
	if err != nil {
		t.Fatalf("WithFileOnly() error = %v", err)
	}

	if len(config.Writers) != 1 {
		t.Errorf("WithFileOnly() should have 1 writer, got %d", len(config.Writers))
	}

	for _, w := range config.Writers {
		if closer, ok := w.(io.Closer); ok {
			closer.Close()
		}
	}
}

func TestConfigWithFileInvalidPath(t *testing.T) {
	config := DefaultConfig()
	originalLen := len(config.Writers)

	_, err := config.WithFile("\x00invalid", FileWriterConfig{})
	if err == nil {
		t.Error("WithFile() with invalid path should return error")
	}

	if len(config.Writers) != originalLen {
		t.Error("WithFile() with invalid path should not modify config")
	}
}

// ============================================================================
// JSON OPTIONS TESTS
// ============================================================================

func TestDefaultJSONOptions(t *testing.T) {
	opts := DefaultJSONOptions()

	if opts == nil {
		t.Fatal("DefaultJSONOptions() returned nil")
	}

	if opts.PrettyPrint {
		t.Error("Default should not use pretty print")
	}

	if opts.FieldNames == nil {
		t.Error("Default should have field names")
	}
}

func TestJSONFieldNames(t *testing.T) {
	opts := DefaultJSONOptions()

	if opts.FieldNames.Timestamp != "timestamp" {
		t.Errorf("Default timestamp field = %q, want %q", opts.FieldNames.Timestamp, "timestamp")
	}

	if opts.FieldNames.Level != "level" {
		t.Errorf("Default level field = %q, want %q", opts.FieldNames.Level, "level")
	}

	if opts.FieldNames.Message != "message" {
		t.Errorf("Default message field = %q, want %q", opts.FieldNames.Message, "message")
	}

	if opts.FieldNames.Caller != "caller" {
		t.Errorf("Default caller field = %q, want %q", opts.FieldNames.Caller, "caller")
	}

	if opts.FieldNames.Fields != "fields" {
		t.Errorf("Default fields field = %q, want %q", opts.FieldNames.Fields, "fields")
	}
}

func TestJSONConfigWithLogger(t *testing.T) {
	var buf bytes.Buffer

	config := JSONConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Info("test message")

	output := buf.String()

	if !strings.Contains(output, `"message"`) {
		t.Error("Output should contain message field")
	}

	if !strings.Contains(output, `"level"`) {
		t.Error("Output should contain level field")
	}

	if !strings.Contains(output, `"timestamp"`) {
		t.Error("Output should contain timestamp field")
	}
}

func TestJSONConfigCustomFieldNames(t *testing.T) {
	var buf bytes.Buffer

	config := JSONConfig()
	config.JSON.FieldNames = &JSONFieldNames{
		Timestamp: "@timestamp",
		Level:     "severity",
		Message:   "msg",
		Caller:    "source",
		Fields:    "data",
	}
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Info("test message")

	output := buf.String()

	if !strings.Contains(output, `"@timestamp"`) {
		t.Error("Output should contain custom timestamp field")
	}

	if !strings.Contains(output, `"severity"`) {
		t.Error("Output should contain custom level field")
	}

	if !strings.Contains(output, `"msg"`) {
		t.Error("Output should contain custom message field")
	}

	if strings.Contains(output, `"timestamp":`) {
		t.Error("Output should not contain default timestamp field")
	}

	if strings.Contains(output, `"level":`) {
		t.Error("Output should not contain default level field")
	}
}

// ============================================================================
// INTEGRATION TESTS
// ============================================================================

func TestConfigWithLoggerCreation(t *testing.T) {
	var buf bytes.Buffer

	config := &LoggerConfig{
		Level:          LevelDebug,
		Format:         FormatText,
		TimeFormat:     DefaultTimeFormat,
		IncludeTime:    true,
		IncludeLevel:   true,
		Writers:        []io.Writer{&buf},
		SecurityConfig: &SecurityConfig{SensitiveFilter: nil},
	}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Debug("test message")

	if !strings.Contains(buf.String(), "test message") {
		t.Error("Config not applied correctly to logger")
	}
}

func TestConfigSecurityConfigMerge(t *testing.T) {
	filter := NewBasicSensitiveDataFilter()

	config := &LoggerConfig{
		Level:  LevelInfo,
		Format: FormatText,
		SecurityConfig: &SecurityConfig{
			MaxMessageSize:  2048,
			MaxWriters:      50,
			SensitiveFilter: filter,
		},
	}

	err := config.Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if config.SecurityConfig.SensitiveFilter != filter {
		t.Error("SecurityConfig should have the filter")
	}
}

func TestConfigSecurityConfigDefaults(t *testing.T) {
	config := &LoggerConfig{
		Level:  LevelInfo,
		Format: FormatText,
	}

	err := config.Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if config.SecurityConfig == nil {
		t.Fatal("SecurityConfig should be initialized")
	}

	if config.SecurityConfig.MaxMessageSize <= 0 {
		t.Error("SecurityConfig should have default MaxMessageSize")
	}

	if config.SecurityConfig.MaxWriters <= 0 {
		t.Error("SecurityConfig should have default MaxWriters")
	}
}

// ============================================================================
// CONVENIENCE CONFIGURATION TESTS
// ============================================================================

func TestNewWithOptions(t *testing.T) {
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
}

func TestNewWithOptionsLevel(t *testing.T) {
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

func TestNewWithOptionsJSONFormat(t *testing.T) {
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

func TestNewWithOptionsFile(t *testing.T) {
	tmpFile := "test_options_file.log"
	defer os.Remove(tmpFile)

	logger, err := NewWithOptions(Options{
		Console: false,
		File:    tmpFile,
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Info("file test message")

	// Check file was created
	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), "file test message") {
		t.Errorf("Log file should contain message, got: %s", string(content))
	}
}

func TestNewWithOptionsFileConfig(t *testing.T) {
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

func TestNewWithOptionsCaller(t *testing.T) {
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
	// Just check that caller information is present (file name and line number)
	if !strings.Contains(output, ".go:") {
		t.Errorf("Expected caller information in output, got: %s", output)
	}
}

func TestNewWithOptionsFilterBasic(t *testing.T) {
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

func TestNewWithOptionsFilterInvalid(t *testing.T) {
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

// ============================================================================
// CONVENIENCE FUNCTIONS TESTS
// ============================================================================

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

func TestToFileDefaultFilename(t *testing.T) {
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

func TestToConsole(t *testing.T) {
	logger := ToConsole()
	if logger == nil {
		t.Fatal("ToConsole returned nil logger")
	}
	defer logger.Close()

	// Should not panic - we won't actually test the output since it goes to console
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

func TestToFileInvalidPath(t *testing.T) {
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
