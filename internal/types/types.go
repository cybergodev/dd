package types

// LogLevel represents the severity level of a log message
type LogLevel int8

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// JSONFieldNames allows customization of JSON field names
type JSONFieldNames struct {
	Timestamp string
	Level     string
	Caller    string
	Message   string
	Fields    string
}

// JSONOptions holds JSON-specific configuration options
type JSONOptions struct {
	PrettyPrint bool
	Indent      string
	FieldNames  *JSONFieldNames
}
