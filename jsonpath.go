package tapetest

import (
	"fmt"
	"strings"
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
