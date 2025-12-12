package dd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
)

// getCallerInfo returns the caller's file path and line number in the format "./file.go:line".
// It skips the specified number of stack frames to get the actual caller.
func getCallerInfo(skip int) string {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return ""
	}

	rel := filepath.Base(file)
	return fmt.Sprintf("./%s:%d", rel, line)
}

// Json outputs data as compact JSON to console for debugging.
// It marshals the provided data to JSON format without HTML escaping and prints it directly to stdout.
// Supports multiple arguments of any type (including pointers, structs, slices, maps, etc.).
// Multiple arguments are printed on the same line separated by spaces with a newline at the end.
// The output is prefixed with the caller's file path and line number.
func Json(data ...any) {
	outputJSON(getCallerInfo(2), data...)
}

// Jsonf outputs formatted data as compact JSON to console for debugging.
// It formats the string using fmt.Sprintf and then marshals to JSON format.
// The format string and arguments follow the same rules as fmt.Fprintf.
// The output is prefixed with the caller's file path and line number.
func Jsonf(format string, args ...any) {
	formatted := fmt.Sprintf(format, args...)
	outputJSON(getCallerInfo(2), formatted)
}

// Text outputs data as pretty-printed format to console for debugging.
// For simple types (string, number, bool), it prints the raw value without JSON quotes.
// For complex types (struct, slice, map), it marshals to formatted JSON with indentation.
// Supports multiple arguments of any type (including pointers, structs, slices, maps, etc.).
// Multiple arguments are printed on the same line separated by spaces with a newline at the end.
// The output is prefixed with the caller's file path and line number.
func Text(data ...any) {
	outputText(getCallerInfo(2), data...)
}

// Textf outputs formatted data as pretty-printed format to console for debugging.
// It formats the string using fmt.Sprintf and then prints it.
// The format string and arguments follow the same rules as fmt.Fprintf.
// The output is prefixed with the caller's file path and line number.
func Textf(format string, args ...any) {
	formatted := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stdout, "%s %s\n", getCallerInfo(2), formatted)
}

// Exit outputs data as pretty-printed format to console for debugging and then exits the program.
// For simple types (string, number, bool), it prints the raw value without JSON quotes.
// For complex types (struct, slice, map), it marshals to formatted JSON with indentation.
// Supports multiple arguments of any type (including pointers, structs, slices, maps, etc.).
// Multiple arguments are printed on the same line separated by spaces with a newline at the end.
// The output is prefixed with the caller's file path and line number.
// After printing, calls os.Exit(0) to terminate the program.
func Exit(data ...any) {
	outputText(getCallerInfo(2), data...)
	os.Exit(0)
}

// Exitf outputs formatted data as pretty-printed format to console for debugging and then exits the program.
// It formats the string using fmt.Sprintf and then prints it.
// The format string and arguments follow the same rules as fmt.Fprintf.
// The output is prefixed with the caller's file path and line number.
// After printing, calls os.Exit(0) to terminate the program.
func Exitf(format string, args ...any) {
	formatted := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stdout, "%s %s\n", getCallerInfo(2), formatted)
	os.Exit(0)
}

// Json outputs data as compact JSON to console for debugging.
func (l *Logger) Json(data ...any) {
	outputJSON(getCallerInfo(2), data...)
}

// Jsonf outputs formatted data as compact JSON to console for debugging.
func (l *Logger) Jsonf(format string, args ...any) {
	formatted := fmt.Sprintf(format, args...)
	outputJSON(getCallerInfo(2), formatted)
}

// Text outputs data as pretty-printed format to console for debugging.
func (l *Logger) Text(data ...any) {
	outputText(getCallerInfo(2), data...)
}

// Textf outputs formatted data as pretty-printed format to console for debugging.
func (l *Logger) Textf(format string, args ...any) {
	formatted := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stdout, "%s %s\n", getCallerInfo(2), formatted)
}

// Exit outputs data as pretty-printed format to console for debugging and then exits the program.
func (l *Logger) Exit(data ...any) {
	outputText(getCallerInfo(2), data...)
	os.Exit(0)
}

// Exitf outputs formatted data as pretty-printed format to console for debugging and then exits the program.
func (l *Logger) Exitf(format string, args ...any) {
	formatted := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stdout, "%s %s\n", getCallerInfo(2), formatted)
	os.Exit(0)
}

// isSimpleType checks if the value is a simple primitive type that should be printed directly.
func isSimpleType(v any) bool {
	if v == nil {
		return true
	}

	// Check if it's an error type
	if _, ok := v.(error); ok {
		return true
	}

	val := reflect.ValueOf(v)
	kind := val.Kind()

	// Handle pointers by dereferencing
	if kind == reflect.Ptr {
		if val.IsNil() {
			return true
		}
		val = val.Elem()
		kind = val.Kind()
	}

	switch kind {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

// formatSimpleValue formats a simple value for direct output without JSON encoding.
func formatSimpleValue(v any) string {
	if v == nil {
		return "nil"
	}

	// Handle error type specially
	if err, ok := v.(error); ok {
		if err == nil {
			return "nil"
		}
		return err.Error()
	}

	val := reflect.ValueOf(v)

	// Handle pointers by dereferencing
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return "nil"
		}
		val = val.Elem()
	}

	return fmt.Sprintf("%v", val.Interface())
}

// 优化的共享输出实现，减少内存分配和重复代码
var (
	debugBufPool = sync.Pool{
		New: func() any {
			return &bytes.Buffer{}
		},
	}
)

// 共享的 JSON 输出实现
func outputJSON(caller string, data ...any) {
	if len(data) == 0 {
		fmt.Fprintf(os.Stdout, "%s\n", caller)
		return
	}

	fmt.Fprint(os.Stdout, caller)

	buf := debugBufPool.Get().(*bytes.Buffer)
	defer func() {
		buf.Reset()
		debugBufPool.Put(buf)
	}()

	encoder := json.NewEncoder(buf)
	encoder.SetEscapeHTML(false)

	for i, item := range data {
		buf.Reset()

		// Convert error types to strings for JSON encoding
		jsonItem := item
		if err, ok := item.(error); ok {
			if err == nil {
				jsonItem = nil
			} else {
				jsonItem = err.Error()
			}
		}

		if err := encoder.Encode(jsonItem); err != nil {
			fmt.Fprintf(os.Stdout, " [%d] JSON marshal error: %v", i, err)
			continue
		}

		// Remove trailing newline added by Encode
		output := buf.Bytes()
		if len(output) > 0 && output[len(output)-1] == '\n' {
			output = output[:len(output)-1]
		}

		// Print with space separator, add newline only after last item
		if i < len(data)-1 {
			fmt.Fprintf(os.Stdout, " %s", output)
		} else {
			fmt.Fprintf(os.Stdout, " %s\n", output)
		}
	}
}

// 共享的文本输出实现
func outputText(caller string, data ...any) {
	if len(data) == 0 {
		fmt.Fprintf(os.Stdout, "%s\n", caller)
		return
	}

	fmt.Fprint(os.Stdout, caller)

	buf := debugBufPool.Get().(*bytes.Buffer)
	defer func() {
		buf.Reset()
		debugBufPool.Put(buf)
	}()

	encoder := json.NewEncoder(buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")

	for i, item := range data {
		// Check if it's a simple type
		if isSimpleType(item) {
			output := formatSimpleValue(item)
			if i < len(data)-1 {
				fmt.Fprintf(os.Stdout, " %s", output)
			} else {
				fmt.Fprintf(os.Stdout, " %s\n", output)
			}
			continue
		}

		// For complex types, use JSON formatting
		buf.Reset()
		if err := encoder.Encode(item); err != nil {
			fmt.Fprintf(os.Stdout, " [%d] JSON marshal error: %v", i, err)
			continue
		}

		// Remove trailing newline added by Encode
		output := buf.Bytes()
		if len(output) > 0 && output[len(output)-1] == '\n' {
			output = output[:len(output)-1]
		}

		if i < len(data)-1 {
			fmt.Fprintf(os.Stdout, " %s", output)
		} else {
			fmt.Fprintf(os.Stdout, " %s\n", output)
		}
	}
}
