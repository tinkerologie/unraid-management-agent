package lib

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

// sensitiveFields contains field names that should always be redacted (case-insensitive match).
var sensitiveFields = []string{
	"password",
	"token",
	"secret",
	"credential",
	"api_key",
	"apikey",
	"secret_key",
	"secretkey",
	"auth_key",
	"authkey",
	"private_key",
	"privatekey",
	"access_key",
	"accesskey",
}

// Compiled regex patterns for sensitive data detection in string values.
var (
	passwordPattern  = regexp.MustCompile(`(?i)(password\s*[=:]\s*)(\S+)`)
	bearerPattern    = regexp.MustCompile(`(?i)(Bearer\s+)(\S+)`)
	shoutrrrPattern  = regexp.MustCompile(`(?i)((?:ntfy|gotify|discord|slack|telegram|pushover|smtp|teams|matrix|generic)://)([^\s]+)`)
	webhookPattern   = regexp.MustCompile(`(?i)(https?://[^\s]*?(?:/webhook/|[?&]token=|[?&]key=))([^\s&]+)`)
	csrfTokenPattern = regexp.MustCompile(`(?i)(csrf_token\s*[=:]\s*)(\S+)`)
)

// Redact applies all redaction rules to a string value, replacing sensitive data with [REDACTED].
func Redact(value string) string {
	if value == "" {
		return value
	}

	value = passwordPattern.ReplaceAllString(value, "${1}[REDACTED]")
	value = bearerPattern.ReplaceAllString(value, "${1}[REDACTED]")
	value = shoutrrrPattern.ReplaceAllString(value, "${1}[REDACTED]")
	value = webhookPattern.ReplaceAllString(value, "${1}[REDACTED]")
	value = csrfTokenPattern.ReplaceAllString(value, "${1}[REDACTED]")

	return value
}

// isSensitiveField checks if a field name matches any known sensitive field pattern.
func isSensitiveField(name string) bool {
	lower := strings.ToLower(name)
	for _, field := range sensitiveFields {
		if strings.Contains(lower, field) {
			return true
		}
	}
	return false
}

// RedactMap recursively redacts sensitive values in a map.
// String values matching sensitive patterns are redacted.
// Values of keys whose names match sensitiveFields are fully replaced with "[REDACTED]".
func RedactMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}

	result := make(map[string]any, len(m))
	for k, v := range m {
		if isSensitiveField(k) {
			result[k] = "[REDACTED]"
			continue
		}

		switch val := v.(type) {
		case string:
			result[k] = Redact(val)
		case map[string]any:
			result[k] = RedactMap(val)
		case []any:
			result[k] = redactSlice(val)
		default:
			result[k] = v
		}
	}
	return result
}

// redactSlice recursively redacts values in a slice.
func redactSlice(s []any) []any {
	result := make([]any, len(s))
	for i, v := range s {
		switch val := v.(type) {
		case string:
			result[i] = Redact(val)
		case map[string]any:
			result[i] = RedactMap(val)
		case []any:
			result[i] = redactSlice(val)
		default:
			result[i] = v
		}
	}
	return result
}

// RedactStruct uses reflection to redact sensitive fields in structs.
// Fields whose names match sensitiveFields are replaced with "[REDACTED]".
// Nested structs and slices are handled recursively.
// Returns a new map[string]any with redacted values.
func RedactStruct(v any) any {
	if v == nil {
		return nil
	}

	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return nil
		}
		val = val.Elem()
	}

	switch val.Kind() {
	case reflect.Struct:
		return redactStructValue(val)
	case reflect.Map:
		return redactMapValue(val)
	case reflect.Slice:
		return redactSliceValue(val)
	case reflect.String:
		return Redact(val.String())
	default:
		return v
	}
}

func redactStructValue(val reflect.Value) map[string]any {
	t := val.Type()
	result := make(map[string]any, t.NumField())

	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		fieldVal := val.Field(i)

		// Use JSON tag name if available, otherwise use the field name
		name := field.Name
		if tag := field.Tag.Get("json"); tag != "" {
			parts := strings.SplitN(tag, ",", 2)
			// Skip fields with json:"-" — they should be omitted entirely
			if parts[0] == "-" {
				continue
			}
			if parts[0] != "" {
				name = parts[0]
			}
		}

		if isSensitiveField(name) {
			result[name] = "[REDACTED]"
			continue
		}

		result[name] = RedactStruct(fieldVal.Interface())
	}

	return result
}

func redactMapValue(val reflect.Value) map[string]any {
	result := make(map[string]any, val.Len())
	for _, key := range val.MapKeys() {
		k := fmt.Sprintf("%v", key.Interface())
		if isSensitiveField(k) {
			result[k] = "[REDACTED]"
			continue
		}
		result[k] = RedactStruct(val.MapIndex(key).Interface())
	}
	return result
}

func redactSliceValue(val reflect.Value) []any {
	result := make([]any, val.Len())
	for i := range val.Len() {
		result[i] = RedactStruct(val.Index(i).Interface())
	}
	return result
}
