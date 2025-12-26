package logformat

import (
	"fmt"
	"strings"
	"time"

	"github.com/cybergodev/dd/internal/caller"
	"github.com/cybergodev/dd/internal/types"
)

func FormatMessage(
	level types.LogLevel,
	includeTime bool,
	timeFormat string,
	includeLevel bool,
	includeCaller bool,
	callerDepth int,
	fullPath bool,
	args ...any,
) string {
	capacity := 1 // message always included
	if includeTime {
		capacity++
	}
	if includeLevel {
		capacity++
	}
	if includeCaller {
		capacity++
	}

	parts := make([]string, 0, capacity)

	if includeTime {
		parts = append(parts, "["+time.Now().Format(timeFormat)+"]")
	}

	if includeLevel {
		parts = append(parts, "["+level.String()+"]")
	}

	if includeCaller {
		if callerInfo := caller.GetCaller(callerDepth, fullPath); callerInfo != "" {
			parts = append(parts, callerInfo)
		}
	}

	var message string
	argCount := len(args)
	switch argCount {
	case 0:
		message = ""
	case 1:
		if s, ok := args[0].(string); ok {
			message = s
		} else {
			message = fmt.Sprint(args[0])
		}
	default:
		message = strings.TrimSuffix(fmt.Sprintln(args...), "\n")
	}
	parts = append(parts, message)

	return strings.Join(parts, " ")
}
