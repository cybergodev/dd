package dd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cybergodev/dd/internal/caller"
)

// ===== Direct Output Functions (no return value) =====

// Printf formats according to a format specifier and writes to standard output.
// It returns the number of bytes written and any write error encountered.
// This is equivalent to fmt.Printf but uses dd's Text formatting for consistency.
func Printf(format string, args ...any) {
	formatted := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stdout, "%s %s\n", caller.GetCaller(DebugVisualizationDepth, false), formatted)
}

// Print formats using the default formats for its operands and writes to standard output.
// Spaces are added between operands when neither is a string.
// It returns the number of bytes written and any write error encountered.
// This maintains consistency with dd.Text method behavior but without newline.
func Print(args ...any) int {
	n, err := fmt.Fprint(os.Stdout, args...)
	if err != nil {
		fmt.Println(err)
	}
	return n
}

// Println formats using the default formats for its operands and writes to standard output.
// Spaces are always added between operands and a newline is appended.
// It returns the number of bytes written and any write error encountered.
// This maintains consistency with dd.Text method behavior.
func Println(args ...any) {
	outputText(caller.GetCaller(DebugVisualizationDepth, false), args...)
}

// ===== String Return Functions (no output) =====

// Sprintf formats according to a format specifier and returns the resulting string.
func Sprintf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}

// Sprint formats using the default formats for its operands and returns the resulting string.
// Spaces are added between operands when neither is a string.
func Sprint(args ...any) string {
	return fmt.Sprint(args...)
}

// Sprintln formats using the default formats for its operands and returns the resulting string.
// Spaces are always added between operands and a newline is appended.
func Sprintln(args ...any) string {
	return fmt.Sprintln(args...)
}

// ===== Writer Output Functions =====

// Fprintf formats according to a format specifier and writes to w.
// It returns the number of bytes written and any write error encountered.
func Fprintf(w io.Writer, format string, args ...any) int {
	n, err := fmt.Fprintf(w, format, args...)
	if err != nil {
		fmt.Println(err)
	}
	return n
}

// Fprint formats using the default formats for its operands and writes to w.
// Spaces are added between operands when neither is a string.
// It returns the number of bytes written and any write error encountered.
func Fprint(w io.Writer, args ...any) int {
	n, err := fmt.Fprint(w, args...)
	if err != nil {
		fmt.Println(err)
	}
	return n
}

// Fprintln formats using the default formats for its operands and writes to w.
// Spaces are always added between operands and a newline is appended.
// It returns the number of bytes written and any write error encountered.
func Fprintln(w io.Writer, args ...any) int {
	n, err := fmt.Fprintln(w, args...)
	if err != nil {
		fmt.Println(err)
	}
	return n
}

// ===== Input Scanning Functions =====

// Scan scans text read from standard input, storing successive space-separated
// values into successive arguments. Newlines count as space. It returns the
// number of items successfully scanned. If that is less than the number of
// arguments, err will report why.
func Scan(a ...any) int {
	n, err := fmt.Scan(a...)
	if err != nil {
		fmt.Println(err)
	}
	return n
}

// Scanf scans text read from standard input, storing successive space-separated
// values into successive arguments as determined by the format. It returns the
// number of items successfully scanned. If that is less than the number of
// arguments, err will report why.
func Scanf(format string, a ...any) int {
	n, err := fmt.Scanf(format, a...)
	if err != nil {
		fmt.Println(err)
	}
	return n
}

// Scanln is similar to Scan, but stops scanning at a newline and after the final
// item there must be a newline or EOF.
func Scanln(a ...any) int {
	n, err := fmt.Scanln(a...)
	if err != nil {
		fmt.Println(err)
	}
	return n
}

// ===== Reader Input Functions =====

// Fscan scans text read from r, storing successive space-separated values into
// successive arguments. Newlines count as space. It returns the number of items
// successfully scanned. If that is less than the number of arguments, err will
// report why.
func Fscan(r io.Reader, a ...any) int {
	n, err := fmt.Fscan(r, a...)
	if err != nil {
		fmt.Println(err)
	}
	return n
}

// Fscanf scans text read from r, storing successive space-separated values into
// successive arguments as determined by the format. It returns the number of
// items successfully scanned. If that is less than the number of arguments,
// err will report why.
func Fscanf(r io.Reader, format string, a ...any) int {
	n, err := fmt.Fscanf(r, format, a...)
	if err != nil {
		fmt.Println(err)
	}
	return n
}

// Fscanln is similar to Fscan, but stops scanning at a newline and after the
// final item there must be a newline or EOF.
func Fscanln(r io.Reader, a ...any) int {
	n, err := fmt.Fscanln(r, a...)
	if err != nil {
		fmt.Println(err)
	}
	return n
}

// ===== String Input Functions =====

// Sscan scans the argument string, storing successive space-separated values
// into successive arguments. Newlines count as space. It returns the number
// of items successfully scanned. If that is less than the number of arguments,
// err will report why.
func Sscan(str string, a ...any) int {
	n, err := fmt.Sscan(str, a...)
	if err != nil {
		fmt.Println(err)
	}
	return n
}

// Sscanf scans the argument string, storing successive space-separated values
// into successive arguments as determined by the format. It returns the number
// of items successfully scanned. If that is less than the number of arguments,
// err will report why.
func Sscanf(str string, format string, a ...any) int {
	n, err := fmt.Sscanf(str, format, a...)
	if err != nil {
		fmt.Println(err)
	}
	return n
}

// Sscanln is similar to Sscan, but stops scanning at a newline and after the
// final item there must be a newline or EOF.
func Sscanln(str string, a ...any) int {
	n, err := fmt.Sscanln(str, a...)
	if err != nil {
		fmt.Println(err)
	}
	return n
}

// ===== Error Formatting Function =====

// NewError formats according to a format specifier and returns the string as a
// value that satisfies error. If the format specifier includes a %w verb with
// an error operand, the returned error will implement an Unwrap method
// returning the operand. It is invalid to include more than one %w verb or to
// supply it with an operand that does not implement the error interface. The
// %w verb is otherwise a synonym for %v.
// This is equivalent to fmt.Errorf but with a different name to avoid conflict
// with the existing logging Errorf function.
func NewError(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

// ===== Additional Utility Functions =====

// AppendFormat appends the formatted string to dst and returns the extended buffer.
// This is equivalent to fmt.Appendf but with a more descriptive name.
func AppendFormat(dst []byte, format string, args ...any) []byte {
	return fmt.Appendf(dst, format, args...)
}

// Append formats using the default formats for its operands, appends the result to dst,
// and returns the extended buffer.
func Append(dst []byte, args ...any) []byte {
	return fmt.Append(dst, args...)
}

// Appendln formats using the default formats for its operands, appends the result to dst,
// and returns the extended buffer. A newline is appended to the result.
func Appendln(dst []byte, args ...any) []byte {
	return fmt.Appendln(dst, args...)
}

// ===== Enhanced Functions with dd Integration =====

// PrintfWith formats according to a format specifier, writes to standard output,
// and also logs the message using dd's structured logging if a logger is available.
// This provides both immediate output and logging capability.
func PrintfWith(format string, args ...any) int {
	formatted := fmt.Sprintf(format, args...)

	// Output to stdout
	n, err := fmt.Fprint(os.Stdout, formatted)
	if err != nil {
		fmt.Println(err)
	}

	// Also log using dd if default logger is available
	if logger := Default(); logger != nil {
		// Remove trailing newline for logging consistency
		logMsg := strings.TrimSuffix(formatted, "\n")
		logger.Info(logMsg)
	}

	return n
}

// PrintlnWith formats using the default formats, writes to standard output with newline,
// and also logs the message using dd's structured logging if a logger is available.
func PrintlnWith(args ...any) int {
	// Output to stdout
	n, err := fmt.Fprintln(os.Stdout, args...)
	if err != nil {
		fmt.Println(err)
	}

	// Also log using dd if default logger is available
	if logger := Default(); logger != nil {
		logger.Info(fmt.Sprint(args...))
	}

	return n
}

// NewErrorWith creates an error and also logs it using dd's error logging if available.
// This provides both error creation and automatic error logging.
func NewErrorWith(format string, args ...any) error {
	err := fmt.Errorf(format, args...)

	// Also log using dd if default logger is available
	if logger := Default(); logger != nil {
		logger.Error(err.Error())
	}

	return err
}
