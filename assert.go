package tapetest

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// assertionOperators is the set of recognized operator tokens that can be
// passed as the first argument to an assertion method (Expect, Json, Cookie,
// Header). When the first argument is one of these strings, the remaining
// arguments are interpreted as operands for that operator.
var assertionOperators = map[string]bool{
	">":  true,
	">=": true,
	"<":  true,
	"<=": true,
	"==": true,
	"=":  true,
	"!=": true,
	"^":  true, // contains all
	"!^": true, // contains none
	"~":  true, // between
	"*":  true, // any of
}

func isOperatorToken(v interface{}) bool {
	s, ok := v.(string)
	return ok && assertionOperators[s]
}

// parseNumber tries to parse a string into a float64.
func parseNumber(s string) (float64, bool) {
	n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// asNumber returns the float64 value of any numeric Go type.
func asNumber(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint64:
		return float64(n), true
	case uint32:
		return float64(n), true
	}
	return 0, false
}

// evalAssertion evaluates an assertion against an actual string value.
// It is the shared engine behind Expect, Json, Cookie and Header assertions,
// so all of them support the same set of operators.
//
// Returns (passed, failureMessage). On success the failureMessage is empty.
//
// Argument forms:
//
//	()                         -> always true (existence already verified)
//	(expected)                 -> equality (string, numeric, or time)
//	("op", expected)           -> single-operand operators: > >= < <= == = !=
//	("~", low, high)           -> actual is numerically between low and high
//	("^", a, b, ...)           -> actual must contain every listed substring
//	("!^", a, b, ...)          -> actual must contain none of the listed substrings
//	("*", a, b, ...)           -> actual must contain at least one listed substring
func evalAssertion(actual string, args ...interface{}) (bool, string) {
	switch len(args) {
	case 0:
		return true, ""
	case 1:
		if isOperatorToken(args[0]) {
			return false, fmt.Sprintf("operator %q requires additional arguments", args[0])
		}
		if equalValue(actual, args[0]) {
			return true, ""
		}
		return false, fmt.Sprintf("expected %v but got %q", args[0], actual)
	}

	// Two or more arguments: if the first is an operator, dispatch to it.
	if op, ok := args[0].(string); ok && assertionOperators[op] {
		return applyOperator(actual, op, args[1:])
	}

	// First arg is not an operator: fall back to equality against it.
	if equalValue(actual, args[0]) {
		return true, ""
	}
	return false, fmt.Sprintf("expected %v but got %q", args[0], actual)
}

// equalValue checks equality of the actual string against an expected value,
// handling numeric, time, and plain string comparisons.
func equalValue(actual string, expected interface{}) bool {
	if expTime, ok := expected.(time.Time); ok {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(actual))
		if err != nil {
			return false
		}
		return parsed.Equal(expTime)
	}

	if en, ok := asNumber(expected); ok {
		if an, ok := parseNumber(actual); ok {
			return an == en
		}
		return false
	}

	return actual == fmt.Sprintf("%v", expected)
}

// applyOperator evaluates multi/single-operand operators against an actual string.
func applyOperator(actual, op string, args []interface{}) (bool, string) {
	switch op {
	case "^": // must contain all
		for _, a := range args {
			if !strings.Contains(actual, fmt.Sprintf("%v", a)) {
				return false, fmt.Sprintf("expected to contain %q", a)
			}
		}
		return true, ""

	case "!^": // must contain none
		for _, a := range args {
			if strings.Contains(actual, fmt.Sprintf("%v", a)) {
				return false, fmt.Sprintf("expected NOT to contain %q", a)
			}
		}
		return true, ""

	case "*": // any of
		want := make([]string, 0, len(args))
		for _, a := range args {
			if strings.Contains(actual, fmt.Sprintf("%v", a)) {
				return true, ""
			}
			want = append(want, fmt.Sprintf("%v", a))
		}
		return false, fmt.Sprintf("expected to contain any of [%s] but got %q", strings.Join(want, ", "), actual)

	case "~": // between (inclusive)
		if len(args) != 2 {
			return false, fmt.Sprintf("operator %q requires exactly 2 arguments, got %d", op, len(args))
		}
		low, lok := asNumber(args[0])
		high, hok := asNumber(args[1])
		if !lok || !hok {
			return false, fmt.Sprintf("operator %q requires numeric bounds", op)
		}
		an, ok := parseNumber(actual)
		if !ok {
			return false, fmt.Sprintf("value %q is not numeric", actual)
		}
		if an >= low && an <= high {
			return true, ""
		}
		return false, fmt.Sprintf("expected %v to be between %v and %v", an, low, high)

	case ">", ">=", "<", "<=", "==", "=", "!=":
		if len(args) != 1 {
			return false, fmt.Sprintf("operator %q requires exactly 1 argument, got %d", op, len(args))
		}
		return compareValue(actual, op, args[0])
	}

	return false, fmt.Sprintf("unsupported operator %q", op)
}

// compareValue performs a single-operand comparison of an actual string
// against an expected value (numeric, time, or string).
func compareValue(actual, op string, expected interface{}) (bool, string) {
	if an, ok := parseNumber(actual); ok {
		if en, ok2 := asNumber(expected); ok2 {
			return compareNumeric(an, op, en)
		}
	}

	if expTime, ok := expected.(time.Time); ok {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(actual))
		if err != nil {
			return false, fmt.Sprintf("value %q is not a valid RFC3339 time", actual)
		}
		return compareTime(parsed, op, expTime)
	}

	actualStr := actual
	expectedStr := fmt.Sprintf("%v", expected)
	switch op {
	case "==", "=":
		if actualStr == expectedStr {
			return true, ""
		}
	case "!=":
		if actualStr != expectedStr {
			return true, ""
		}
	}
	return false, fmt.Sprintf("comparison %q %s %v failed", actual, op, expected)
}

// compareNumeric compares two floats with a relational operator.
func compareNumeric(a float64, op string, b float64) (bool, string) {
	switch op {
	case ">":
		if a > b {
			return true, ""
		}
	case ">=":
		if a >= b {
			return true, ""
		}
	case "<":
		if a < b {
			return true, ""
		}
	case "<=":
		if a <= b {
			return true, ""
		}
	case "==", "=":
		if a == b {
			return true, ""
		}
	case "!=":
		if a != b {
			return true, ""
		}
	}
	return false, fmt.Sprintf("expected %v %s %v", a, op, b)
}

// compareTime compares two times with a relational operator.
func compareTime(a time.Time, op string, b time.Time) (bool, string) {
	switch op {
	case ">":
		if a.After(b) {
			return true, ""
		}
	case ">=":
		if !a.Before(b) {
			return true, ""
		}
	case "<":
		if a.Before(b) {
			return true, ""
		}
	case "<=":
		if !a.After(b) {
			return true, ""
		}
	case "==", "=":
		if a.Equal(b) {
			return true, ""
		}
	case "!=":
		if !a.Equal(b) {
			return true, ""
		}
	}
	return false, fmt.Sprintf("expected %v %s %v", a, op, b)
}

// matchRegex reports whether the actual string matches the given regular
// expression. Returns (matched, errorFromCompile).
func matchRegex(actual, pattern string) (bool, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, err
	}
	return re.MatchString(actual), nil
}
