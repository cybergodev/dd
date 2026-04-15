package dd

// Debug Visualization Functions
//
// This file provides two categories of debug output functions:
//
// 1. Print functions (Print, Println, Printf):
//    - Use the default logger's configured writers
//    - Apply sensitive data filtering based on SecurityConfig
//    - Respect log level settings (uses LevelInfo)
//    - Suitable for development debugging with security awareness
//
// 2. Direct output functions (JSON, Text, Exit, etc.):
//    - Output directly to stdout WITHOUT sensitive data filtering
//    - SECURITY WARNING: Never use with passwords, tokens, or sensitive data
//    - For quick debugging only, not for production use

import (
	"fmt"
	"os"

	"github.com/cybergodev/dd/internal"
)

// Print writes to the default logger's configured writers using LevelInfo.
// This is a convenience function equivalent to Default().Print().
// Applies sensitive data filtering based on SecurityConfig.
func Print(args ...any) {
	Default().Print(args...)
}

// Println writes to the default logger's configured writers with a newline.
// Uses LevelInfo for filtering. Applies sensitive data filtering.
// Note: Behaves identically to Print() because the underlying Log() already adds a newline.
func Println(args ...any) {
	Default().Println(args...)
}

// Printf formats according to a format specifier and writes to the default
// logger's configured writers. Uses LevelInfo for filtering.
// Applies sensitive data filtering based on SecurityConfig.
func Printf(format string, args ...any) {
	Default().Printf(format, args...)
}

// JSON outputs data as compact JSON to stdout with caller info for debugging.
func JSON(data ...any) {
	internal.OutputJSON(os.Stdout, internal.GetCaller(debugVisualizationDepth, false), data...)
}

// JSONF outputs formatted data as compact JSON to stdout with caller info for debugging.
func JSONF(format string, args ...any) {
	formatted := fmt.Sprintf(format, args...)
	internal.OutputJSON(os.Stdout, internal.GetCaller(debugVisualizationDepth, false), formatted)
}

// Text outputs data as pretty-printed format to stdout for debugging.
func Text(data ...any) {
	internal.OutputTextData(os.Stdout, data...)
}

// Textf outputs formatted data as pretty-printed format to stdout for debugging.
func Textf(format string, args ...any) {
	formatted := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintln(os.Stdout, formatted)
}

// Exit outputs data as pretty-printed JSON to stdout and exits with code 0.
func Exit(data ...any) {
	internal.OutputText(os.Stdout, internal.GetCaller(debugVisualizationDepth, false), data...)
	os.Exit(0)
}

// Exitf outputs formatted data to stdout with caller info and exits with code 0.
func Exitf(format string, args ...any) {
	formatted := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(os.Stdout, "%s %s\n", internal.GetCaller(debugVisualizationDepth, false), formatted)
	os.Exit(0)
}
