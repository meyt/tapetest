package tapetest

import (
	"fmt"
	"strings"
)

// matchStatusPattern checks if a status code matches a pattern like "2xx", "4xx", etc.
func matchStatusPattern(code int, pattern string) bool {
	pattern = strings.TrimSpace(pattern)

	if len(pattern) != 3 {
		return false
	}

	classDigit := pattern[0]
	suffix := pattern[1:]

	if suffix != "xx" && suffix != "XX" {
		// Not a pattern, try exact match as string
		var pCode int
		if _, err := fmt.Sscanf(pattern, "%d", &pCode); err == nil {
			return code == pCode
		}
		return false
	}

	var classBase int
	switch classDigit {
	case '1', '2', '3', '4', '5':
		classBase = int(classDigit-'0') * 100
	default:
		return false
	}

	return code >= classBase && code < classBase+100
}
