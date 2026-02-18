package envutil

import (
	"os"
	"strings"
)

func Bool(key string) bool {
	return ParseBool(os.Getenv(key))
}

func ParseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "t", "yes", "y", "on":
		return true
	default:
		return false
	}
}
