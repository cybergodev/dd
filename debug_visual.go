package dd

import (
	"encoding/json"
	"fmt"
	"os"
)

// Json outputs data as compact JSON to console for debugging.
// It marshals the provided data to JSON format and prints it directly to stdout.
func Json(data any) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		fmt.Fprintf(os.Stdout, "JSON marshal error: %v\n", err)
		return
	}
	fmt.Fprintln(os.Stdout, string(jsonData))
}

// Text outputs data as pretty-printed JSON to console for debugging.
// It marshals the provided data to formatted JSON with indentation for better readability.
func Text(data any) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stdout, "JSON marshal error: %v\n", err)
		return
	}
	fmt.Fprintln(os.Stdout, string(jsonData))
}

// Json outputs data as compact JSON to console for debugging.
// It marshals the provided data to JSON format and prints it directly to stdout.
func (l *Logger) Json(data any) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		fmt.Fprintf(os.Stdout, "JSON marshal error: %v\n", err)
		return
	}
	fmt.Fprintln(os.Stdout, string(jsonData))
}

// Text outputs data as pretty-printed JSON to console for debugging.
// It marshals the provided data to formatted JSON with indentation for better readability.
func (l *Logger) Text(data any) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stdout, "JSON marshal error: %v\n", err)
		return
	}
	fmt.Fprintln(os.Stdout, string(jsonData))
}
