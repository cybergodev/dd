package dd

type LogFormat int8

const (
	FormatText LogFormat = iota
	FormatJSON
)

func (f LogFormat) String() string {
	switch f {
	case FormatText:
		return "text"
	case FormatJSON:
		return "json"
	default:
		return "unknown"
	}
}
