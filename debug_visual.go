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

// NOTE: The direct-output JSON helpers below (JSON/JSONF) are deliberately
// duplicated between this package-level form and the (*Logger) methods in
// logger.go — they must NOT delegate to each other. They resolve the caller at a
// FIXED depth (debugVisualizationDepth = 2): the package-level dd.JSON() skips
// [GetCaller -> dd.JSON] to reach user code, while (*Logger).JSON() skips
// [GetCaller -> (*Logger).JSON]. Delegating one layer to the other would insert
// an extra frame and report the internal wrapper instead of the real caller.
//
// Text/Textf do NOT resolve the caller, so they delegate normally to the
// default logger (see below) and are not duplicated.

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
//
// Unlike the JSON helpers above, Text does not resolve the caller, so it safely
// delegates to the default logger's Text (which writes to os.Stdout without
// sensitive-data filtering), keeping a single implementation rather than
// duplicating it at both layers.
func Text(data ...any) {
	Default().Text(data...)
}

// Textf outputs formatted data as pretty-printed format to stdout for debugging.
// Delegates to the default logger's Textf (see Text above).
func Textf(format string, args ...any) {
	Default().Textf(format, args...)
}

// Exit/Exitf are package-level-only debug conveniences with no (*Logger)
// counterpart, by design: they write directly to stdout (ignoring the logger's
// configured writers) and call os.Exit(0). Adding them as Logger methods would
// widen the debug-only API surface without benefit; production code should use
// logger.Info/Error plus explicit shutdown instead.

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
