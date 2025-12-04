package dd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// Json outputs data as compact JSON to console for debugging.
// It marshals the provided data to JSON format without HTML escaping and prints it directly to stdout.
func Json(data any) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(data); err != nil {
		fmt.Fprintf(os.Stdout, "JSON marshal error: %v\n", err)
		return
	}

	// Remove trailing newline added by Encode
	output := buf.Bytes()
	if len(output) > 0 && output[len(output)-1] == '\n' {
		output = output[:len(output)-1]
	}
	fmt.Fprintln(os.Stdout, string(output))
}

// Text outputs data as pretty-printed JSON to console for debugging.
// It marshals the provided data to formatted JSON with indentation without HTML escaping for better readability.
func Text(data any) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(data); err != nil {
		fmt.Fprintf(os.Stdout, "JSON marshal error: %v\n", err)
		return
	}

	// Remove trailing newline added by Encode
	output := buf.Bytes()
	if len(output) > 0 && output[len(output)-1] == '\n' {
		output = output[:len(output)-1]
	}
	fmt.Fprintln(os.Stdout, string(output))
}

// Json outputs data as compact JSON to console for debugging.
// It marshals the provided data to JSON format without HTML escaping and prints it directly to stdout.
func (l *Logger) Json(data any) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(data); err != nil {
		fmt.Fprintf(os.Stdout, "JSON marshal error: %v\n", err)
		return
	}

	// Remove trailing newline added by Encode
	output := buf.Bytes()
	if len(output) > 0 && output[len(output)-1] == '\n' {
		output = output[:len(output)-1]
	}
	fmt.Fprintln(os.Stdout, string(output))
}

// Text outputs data as pretty-printed JSON to console for debugging.
// It marshals the provided data to formatted JSON with indentation without HTML escaping for better readability.
func (l *Logger) Text(data any) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(data); err != nil {
		fmt.Fprintf(os.Stdout, "JSON marshal error: %v\n", err)
		return
	}

	// Remove trailing newline added by Encode
	output := buf.Bytes()
	if len(output) > 0 && output[len(output)-1] == '\n' {
		output = output[:len(output)-1]
	}
	fmt.Fprintln(os.Stdout, string(output))
}
