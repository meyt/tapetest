package tapetest

import (
	"fmt"
	"strings"
	"time"
)

// resolveJSONPath resolves a dot-notation path in a JSON structure.
// Supports nested objects ("user.name") and array indices ("items.0.name").
func resolveJSONPath(data interface{}, path string) (interface{}, bool) {
	parts := strings.Split(path, ".")
	current := data

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			var ok bool
			current, ok = v[part]
			if !ok {
				return nil, false
			}
		case []interface{}:
			// Support array index: "items.0.name"
			idx := 0
			if _, err := fmt.Sscanf(part, "%d", &idx); err == nil {
				if idx < 0 || idx >= len(v) {
					return nil, false
				}
				current = v[idx]
			} else {
				return nil, false
			}
		default:
			return nil, false
		}
	}

	return current, true
}

// jsonValueEqual compares a JSON value with an expected Go value.
// Handles type coercion for JSON numbers (float64) vs Go int/float.
func jsonValueEqual(actual, expected interface{}) bool {
	switch exp := expected.(type) {
	case int:
		if f, ok := actual.(float64); ok {
			return int(f) == exp
		}
	case int64:
		if f, ok := actual.(float64); ok {
			return int64(f) == exp
		}
	case float64:
		if f, ok := actual.(float64); ok {
			return f == exp
		}
	case bool:
		if b, ok := actual.(bool); ok {
			return b == exp
		}
	case string:
		if s, ok := actual.(string); ok {
			return s == exp
		}
	case time.Time:
		if s, ok := actual.(string); ok {
			parsed, err := time.Parse(time.RFC3339, s)
			if err != nil {
				return false
			}
			return parsed.Equal(exp)
		}
	}
	return fmt.Sprintf("%v", actual) == fmt.Sprintf("%v", expected)
}

// compareValues compares a JSON value with an expected value using an operator.
// Supports numeric, time, and string comparisons.
func compareValues(actual interface{}, operator string, expected interface{}) bool {
	// Try numeric comparison
	actualNum := toFloat64(actual)
	expectedNum := toFloat64(expected)

	if !isNaN(actualNum) && !isNaN(expectedNum) {
		switch operator {
		case ">":
			return actualNum > expectedNum
		case ">=":
			return actualNum >= expectedNum
		case "<":
			return actualNum < expectedNum
		case "<=":
			return actualNum <= expectedNum
		case "==", "=":
			return actualNum == expectedNum
		case "!=":
			return actualNum != expectedNum
		}
	}

	// Try time comparison
	if expTime, ok := expected.(time.Time); ok {
		if s, ok := actual.(string); ok {
			parsed, err := time.Parse(time.RFC3339, s)
			if err != nil {
				return false
			}
			switch operator {
			case ">":
				return parsed.After(expTime)
			case ">=":
				return !parsed.Before(expTime)
			case "<":
				return parsed.Before(expTime)
			case "<=":
				return !parsed.After(expTime)
			case "==", "=":
				return parsed.Equal(expTime)
			case "!=":
				return !parsed.Equal(expTime)
			}
		}
	}

	// String comparison
	actualStr := fmt.Sprintf("%v", actual)
	expectedStr := fmt.Sprintf("%v", expected)
	switch operator {
	case "==", "=":
		return actualStr == expectedStr
	case "!=":
		return actualStr != expectedStr
	}

	return false
}

func toFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case int32:
		return float64(n)
	case uint:
		return float64(n)
	case uint64:
		return float64(n)
	case uint32:
		return float64(n)
	}
	return float64(1<<63 - 1) // NaN sentinel
}

func isNaN(f float64) bool {
	return f == float64(1<<63-1)
}
