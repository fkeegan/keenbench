package logging

import (
	"encoding/json"
	"fmt"
	"strings"
)

var secretKeys = map[string]bool{
	"api_key":               true,
	"apikey":                true,
	"authorization":         true,
	"openai_api_key":        true,
	"keenbench_openai_api_key": true,
	"token":                 true,
	"secret":                true,
}

func RedactValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "bearer ") {
		return "Bearer " + mask(trimmed[7:])
	}
	return mask(trimmed)
}

func RedactAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, val := range typed {
			if isSecretKey(key) {
				out[key] = RedactValue(fmt.Sprint(val))
				continue
			}
			out[key] = RedactAny(val)
		}
		return out
	case map[string]string:
		out := make(map[string]string, len(typed))
		for key, val := range typed {
			if isSecretKey(key) {
				out[key] = RedactValue(val)
				continue
			}
			out[key] = val
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, val := range typed {
			out[i] = RedactAny(val)
		}
		return out
	case []string:
		out := make([]string, len(typed))
		copy(out, typed)
		return out
	default:
		return value
	}
}

func RedactJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return strings.TrimSpace(string(raw))
	}
	return RedactAny(payload)
}

func isSecretKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	return secretKeys[lower]
}

func mask(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "****"
	}
	return "****" + value[len(value)-4:]
}
