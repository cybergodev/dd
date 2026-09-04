package internal

import "fmt"

// SEC-003: panic-safe renderers for user-supplied error and fmt.Stringer values.
//
// Error and String methods are third-party code invoked from dd's formatting
// paths. Some of those paths run BEFORE logCoreWithDepth's deferred recover
// (FormatArgsToString from Log/Logf) or outside it entirely (the debug Text/JSON
// helpers), so a panicking method would escape the public API and crash the
// caller. fmt recovers such panics itself ("%!v(PANIC=String method: ...)");
// these helpers give dd's direct .Error()/.String() calls the same guarantee.
// Only the user-controlled branches pay for the deferred recover — stdlib
// types like time.Duration keep their direct calls.

// SafeErrorString returns err.Error(), converting a panic raised by the Error
// method into an fmt-style placeholder (see package comment). err must be non-nil.
func SafeErrorString(err error) (s string) {
	defer func() {
		if r := recover(); r != nil {
			s = fmt.Sprintf("%%!v(PANIC=Error method: %v)", r)
		}
	}()
	return err.Error()
}

// SafeStringerString returns v.String(), converting a panic raised by the
// String method into an fmt-style placeholder (see package comment).
func SafeStringerString(v fmt.Stringer) (s string) {
	defer func() {
		if r := recover(); r != nil {
			s = fmt.Sprintf("%%!v(PANIC=String method: %v)", r)
		}
	}()
	return v.String()
}
