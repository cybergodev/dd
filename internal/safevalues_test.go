package internal

import (
	"errors"
	"strings"
	"testing"
)

// panickingError's Error method always panics — a stand-in for third-party
// error types whose formatting code may be broken.
type panickingError struct{}

func (panickingError) Error() string { panic("Error method exploded") }

// panickingStringer's String method always panics.
type panickingStringer struct{}

func (panickingStringer) String() string { panic("String method exploded") }

// TestSafeErrorString verifies SEC-003 panic containment: a panicking Error
// method surfaces as an fmt-style placeholder instead of escaping to the
// caller, while healthy errors pass through unchanged.
func TestSafeErrorString(t *testing.T) {
	if got, want := SafeErrorString(errors.New("boom")), "boom"; got != want {
		t.Errorf("SafeErrorString(healthy) = %q, want %q", got, want)
	}

	got := SafeErrorString(panickingError{})
	if !strings.Contains(got, "PANIC=Error method") {
		t.Errorf("SafeErrorString(panicking) = %q, want placeholder containing %q", got, "PANIC=Error method")
	}
}

// TestSafeStringerString is the Stringer counterpart of TestSafeErrorString.
func TestSafeStringerString(t *testing.T) {
	if got, want := SafeStringerString(stringerStub("fine")), "fine"; got != want {
		t.Errorf("SafeStringerString(healthy) = %q, want %q", got, want)
	}

	got := SafeStringerString(panickingStringer{})
	if !strings.Contains(got, "PANIC=String method") {
		t.Errorf("SafeStringerString(panicking) = %q, want placeholder containing %q", got, "PANIC=String method")
	}
}
