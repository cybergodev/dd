package caller

import (
	"fmt"
	"path/filepath"
	"runtime"
)

// GetCaller returns the caller information at the specified depth
func GetCaller(callerDepth int, fullPath bool) string {
	_, file, line, ok := runtime.Caller(callerDepth)
	if !ok {
		return ""
	}

	if !fullPath {
		file = filepath.Base(file)
	}

	return fmt.Sprintf("%s:%d", file, line)
}
