package dd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cybergodev/dd/internal/caller"
)

// Json outputs data as compact JSON to console for debugging.
// It marshals the provided data to JSON format without HTML escaping and prints it directly to stdout.
// Supports multiple arguments of any type (including pointers, structs, slices, maps, etc.).
// Multiple arguments are printed on the same line separated by spaces with a newline at the end.
// The output is prefixed with the caller's file path and line number.
func Json(data ...any) {
	outputJSON(caller.GetCaller(DebugVisualizationDepth, false), data...)
}

// Jsonf outputs formatted data as compact JSON to console for debugging.
// It formats the string using fmt.Sprintf and then marshals to JSON format.
// The format string and arguments follow the same rules as fmt.Fprintf.
// The output is prefixed with the caller's file path and line number.
func Jsonf(format string, args ...any) {
	formatted := fmt.Sprintf(format, args...)
	outputJSON(caller.GetCaller(DebugVisualizationDepth, false), formatted)
}

// Text outputs data as pretty-printed format to console for debugging.
// For simple types (string, number, bool), it prints the raw value without JSON quotes.
// For complex types (struct, slice, map), it marshals to formatted JSON with indentation.
// Supports multiple arguments of any type (including pointers, structs, slices, maps, etc.).
// Multiple arguments are printed on the same line separated by spaces with a newline at the end.
// The output is prefixed with the caller's file path and line number.
func Text(data ...any) {
	outputText(caller.GetCaller(DebugVisualizationDepth, false), data...)
}

// Textf outputs formatted data as pretty-printed format to console for debugging.
// It formats the string using fmt.Sprintf and then prints it.
// The format string and arguments follow the same rules as fmt.Fprintf.
// The output is prefixed with the caller's file path and line number.
func Textf(format string, args ...any) {
	formatted := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stdout, "%s %s\n", caller.GetCaller(DebugVisualizationDepth, false), formatted)
}

// Exit outputs data as pretty-printed format to console for debugging and then exits the program.
// For simple types (string, number, bool), it prints the raw value without JSON quotes.
// For complex types (struct, slice, map), it marshals to formatted JSON with indentation.
// Supports multiple arguments of any type (including pointers, structs, slices, maps, etc.).
// Multiple arguments are printed on the same line separated by spaces with a newline at the end.
// The output is prefixed with the caller's file path and line number.
// After printing, calls os.Exit(0) to terminate the program.
func Exit(data ...any) {
	outputText(caller.GetCaller(DebugVisualizationDepth, false), data...)
	os.Exit(0)
}

// Exitf outputs formatted data as pretty-printed format to console for debugging and then exits the program.
// It formats the string using fmt.Sprintf and then prints it.
// The format string and arguments follow the same rules as fmt.Fprintf.
// The output is prefixed with the caller's file path and line number.
// After printing, calls os.Exit(0) to terminate the program.
func Exitf(format string, args ...any) {
	formatted := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stdout, "%s %s\n", caller.GetCaller(DebugVisualizationDepth, false), formatted)
	os.Exit(0)
}

// Json outputs data as compact JSON to console for debugging.
func (l *Logger) Json(data ...any) {
	outputJSON(caller.GetCaller(DebugVisualizationDepth, false), data...)
}

// Jsonf outputs formatted data as compact JSON to console for debugging.
func (l *Logger) Jsonf(format string, args ...any) {
	formatted := fmt.Sprintf(format, args...)
	outputJSON(caller.GetCaller(DebugVisualizationDepth, false), formatted)
}

// Text outputs data as pretty-printed format to console for debugging.
func (l *Logger) Text(data ...any) {
	outputText(caller.GetCaller(DebugVisualizationDepth, false), data...)
}

// Textf outputs formatted data as pretty-printed format to console for debugging.
func (l *Logger) Textf(format string, args ...any) {
	formatted := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stdout, "%s %s\n", caller.GetCaller(DebugVisualizationDepth, false), formatted)
}

// Exit outputs data as pretty-printed format to console for debugging and then exits the program.
func (l *Logger) Exit(data ...any) {
	outputText(caller.GetCaller(DebugVisualizationDepth, false), data...)
	os.Exit(0)
}

// Exitf outputs formatted data as pretty-printed format to console for debugging and then exits the program.
func (l *Logger) Exitf(format string, args ...any) {
	formatted := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stdout, "%s %s\n", caller.GetCaller(DebugVisualizationDepth, false), formatted)
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

// Simplified shared output implementation
var (
	debugBufPool = sync.Pool{
		New: func() any {
			return &bytes.Buffer{}
		},
	}
)

// TypeConverter handles intelligent type conversion for JSON marshaling
type TypeConverter struct {
	visited  map[uintptr]bool // Track visited pointers to handle circular references
	depth    int              // Current recursion depth
	maxDepth int              // Maximum recursion depth
}

// TypeConverter pool for performance optimization
var typeConverterPool = sync.Pool{
	New: func() any {
		return &TypeConverter{
			visited:  make(map[uintptr]bool),
			maxDepth: 10, // Prevent infinite recursion
		}
	},
}

// getTypeConverter gets a TypeConverter from the pool
func getTypeConverter() *TypeConverter {
	tc := typeConverterPool.Get().(*TypeConverter)
	tc.depth = 0
	// Clear the visited map for reuse
	for k := range tc.visited {
		delete(tc.visited, k)
	}
	return tc
}

// putTypeConverter returns a TypeConverter to the pool
func putTypeConverter(tc *TypeConverter) {
	typeConverterPool.Put(tc)
}

// NewTypeConverter creates a new type converter with default settings
func NewTypeConverter() *TypeConverter {
	return &TypeConverter{
		visited:  make(map[uintptr]bool),
		maxDepth: 10, // Prevent infinite recursion
	}
}

// ConvertValue intelligently converts any value to a JSON-marshalable format
func (tc *TypeConverter) ConvertValue(v any) any {
	if v == nil {
		return nil
	}

	// Check recursion depth
	if tc.depth > tc.maxDepth {
		return fmt.Sprintf("<max_depth_exceeded:%d>", tc.maxDepth)
	}
	tc.depth++
	defer func() { tc.depth-- }()

	val := reflect.ValueOf(v)
	return tc.convertReflectValue(val)
}

// convertReflectValue handles the actual conversion logic
func (tc *TypeConverter) convertReflectValue(val reflect.Value) any {
	if !val.IsValid() {
		return nil
	}

	// Handle pointers and check for circular references
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return nil
		}

		// Check for circular references
		ptr := val.Pointer()
		if tc.visited[ptr] {
			return fmt.Sprintf("<circular_ref:0x%x>", ptr)
		}
		tc.visited[ptr] = true
		defer delete(tc.visited, ptr)

		return tc.convertReflectValue(val.Elem())
	}

	// Handle interfaces
	if val.Kind() == reflect.Interface {
		if val.IsNil() {
			return nil
		}
		return tc.convertReflectValue(val.Elem())
	}

	switch val.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return val.Interface()

	case reflect.Func:
		return fmt.Sprintf("<func:%s>", val.Type().String())

	case reflect.Chan:
		return fmt.Sprintf("<chan:%s>", val.Type().String())

	case reflect.UnsafePointer:
		return fmt.Sprintf("<unsafe.Pointer:0x%x>", val.Pointer())

	case reflect.Slice, reflect.Array:
		return tc.convertSliceOrArray(val)

	case reflect.Map:
		return tc.convertMap(val)

	case reflect.Struct:
		return tc.convertStruct(val)

	case reflect.Complex64, reflect.Complex128:
		return fmt.Sprintf("%v", val.Interface())

	default:
		// For other types, try to get a string representation
		if val.CanInterface() {
			iface := val.Interface()

			// Special handling for common types
			switch v := iface.(type) {
			case error:
				if v == nil {
					return nil
				}
				return v.Error()
			case time.Time:
				return v.Format(time.RFC3339)
			case time.Duration:
				return v.String()
			case fmt.Stringer:
				return v.String()
			}

			// Try JSON marshaling as last resort for unknown types
			if data, err := json.Marshal(iface); err == nil {
				var result any
				if json.Unmarshal(data, &result) == nil {
					return result
				}
			}
		}

		return fmt.Sprintf("<%s:%v>", val.Type().String(), val)
	}
}

// convertSliceOrArray converts slices and arrays
func (tc *TypeConverter) convertSliceOrArray(val reflect.Value) any {
	length := val.Len()
	if length == 0 {
		return []any{}
	}

	result := make([]any, length)
	for i := range length {
		result[i] = tc.convertReflectValue(val.Index(i))
	}
	return result
}

// convertMap converts maps
func (tc *TypeConverter) convertMap(val reflect.Value) any {
	if val.IsNil() {
		return nil
	}

	result := make(map[string]any)
	keys := val.MapKeys()

	for _, key := range keys {
		keyStr := tc.convertKeyToString(key)
		value := tc.convertReflectValue(val.MapIndex(key))
		result[keyStr] = value
	}

	return result
}

// convertKeyToString converts map keys to strings
func (tc *TypeConverter) convertKeyToString(key reflect.Value) string {
	switch key.Kind() {
	case reflect.String:
		return key.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(key.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(key.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(key.Float(), 'f', -1, 64)
	case reflect.Bool:
		return strconv.FormatBool(key.Bool())
	default:
		return fmt.Sprintf("%v", key.Interface())
	}
}

// convertStruct converts structs, handling unexported fields and special cases
func (tc *TypeConverter) convertStruct(val reflect.Value) any {
	typ := val.Type()

	// Special handling for common types that should be converted differently
	if val.CanInterface() {
		iface := val.Interface()

		// Handle other special types
		switch v := iface.(type) {
		case error:
			if v == nil {
				return nil
			}
			return v.Error()
		case time.Time:
			return v.Format(time.RFC3339)
		case fmt.Stringer:
			return v.String()
		}
	}

	result := make(map[string]any)

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// Skip function fields entirely
		if field.Kind() == reflect.Func {
			result[fieldType.Name] = fmt.Sprintf("<func:%s>", field.Type().String())
			continue
		}

		// Skip channel fields
		if field.Kind() == reflect.Chan {
			result[fieldType.Name] = fmt.Sprintf("<chan:%s>", field.Type().String())
			continue
		}

		// Skip unexported fields that we can't access safely
		if !field.CanInterface() && !fieldType.IsExported() {
			result[fieldType.Name] = fmt.Sprintf("<unexported:%s>", fieldType.Type.String())
			continue
		}

		// Get field name, check for json tag
		fieldName := fieldType.Name
		if tag := fieldType.Tag.Get("json"); tag != "" {
			if tag == "-" {
				continue // Skip fields marked with json:"-"
			}
			if tagName, _, found := strings.Cut(tag, ","); found {
				fieldName = tagName
			} else {
				fieldName = tag
			}
			if fieldName == "" {
				fieldName = fieldType.Name
			}
		}

		// Convert field value
		fieldValue := tc.convertReflectValue(field)
		result[fieldName] = fieldValue
	}

	return result
}

// Shared JSON output implementation
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

		// Use intelligent type conversion with pooled converter
		converter := getTypeConverter()
		convertedItem := converter.ConvertValue(item)
		putTypeConverter(converter)

		if err := encoder.Encode(convertedItem); err != nil {
			// Fallback: try to get a string representation
			fmt.Fprintf(os.Stdout, " [%d] \"%v\"", i, item)
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

// Shared text output implementation
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

		// For complex types, use intelligent type conversion and JSON formatting
		buf.Reset()
		converter := getTypeConverter()
		convertedItem := converter.ConvertValue(item)
		putTypeConverter(converter)

		if err := encoder.Encode(convertedItem); err != nil {
			// Fallback: try to get a string representation
			fmt.Fprintf(os.Stdout, " [%d] %v", i, item)
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
