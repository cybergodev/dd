# Development Change Log

This document tracks all development changes made to the cybergodev/dd logging library.

## 2025-12-28

### Version: Development
### Type: Bug Fix
### Affected Files: 
- `constants.go`

### Summary: 
Fixed caller detection in debug visualization methods (dd.Text(), dd.Json(), dd.Exit())

### Details:
The debug visualization methods were incorrectly showing `asm_amd64.s:1700` instead of the actual caller location where the methods were invoked. This was caused by an incorrect `DebugVisualizationDepth` constant value.

**Root Cause:**
- The `DebugVisualizationDepth` was set to 4, which was too deep in the call stack
- This caused `runtime.Caller()` to retrieve assembly-level caller information instead of the user's source code location

**Solution:**
- Changed `DebugVisualizationDepth` from 4 to 2 in `constants.go`
- This ensures that when `dd.Text()`, `dd.Json()`, `dd.Exit()`, etc. are called, they correctly show the file and line number where the method was invoked

**Verification:**
- Tested with `dev_test/main.go` - now correctly shows `main.go:XX` instead of `asm_amd64.s:1700`
- All existing tests continue to pass
- No breaking changes to the API

**Impact:**
- Improved developer experience for debugging
- Accurate caller information for all debug visualization methods
- No performance impact
- Maintains backward compatibility