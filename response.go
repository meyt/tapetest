package tapetest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Response holds the test response and provides fluent assertion methods.
// All assertion methods return *Response for chaining.
type Response struct {
	t       *testing.T
	code    int
	reason  string
	headers http.Header
	body    []byte
	err     error
	cookies []*http.Cookie

	// Response provides data access methods: Json(), Text(), Write()
	Response *ResponseData
}

// ResponseData provides access to the raw response data.
type ResponseData struct {
	body    []byte
	headers http.Header
	cookies []*http.Cookie
}

// --- Status Assertions ---

// Status asserts the HTTP status code. Accepts int for exact match or string for patterns like "2xx", "4xx".
//
//	r.Status(200)    // exact match
//	r.Status("2xx")  // any 2xx status
func (r *Response) Status(code interface{}) *Response {
	r.t.Helper()

	switch v := code.(type) {
	case int:
		if r.code != v {
			r.t.Errorf("\nexpected status %d but got %d\n  body: %s", v, r.code, string(r.body))
		}
	case string:
		if !matchStatusPattern(r.code, v) {
			r.t.Errorf("\nexpected status %q but got %d\n  body: %s", v, r.code, string(r.body))
		}
	default:
		r.t.Fatalf("tapetest: Status() expects int or string, got %T", code)
	}

	return r
}

// Reason asserts the status reason phrase equals the given text.
//
//	r.Reason("OK")
func (r *Response) Reason(text string) *Response {
	r.t.Helper()
	if r.reason != text {
		r.t.Errorf("\nexpected reason %q but got %q", text, r.reason)
	}
	return r
}

// ReasonContains asserts the status reason phrase contains the given text.
//
//	r.ReasonContains("Created")
func (r *Response) ReasonContains(text string) *Response {
	r.t.Helper()
	if !strings.Contains(r.reason, text) {
		r.t.Errorf("\nexpected reason to contain %q but got %q", text, r.reason)
	}
	return r
}

// --- Header Assertions ---

// Header asserts a response header. With one argument, checks existence.
// With two arguments, checks the value matches.
//
//	r.Header("Content-Type")                        // check exists
//	r.Header("Content-Type", "application/json")    // check value
func (r *Response) Header(key string, value ...string) *Response {
	r.t.Helper()

	actual := r.headers.Get(key)
	if len(value) == 0 {
		// Check existence
		if actual == "" {
			r.t.Errorf("\nexpected header %q to exist but it was not found", key)
		}
	} else {
		// Check value
		if actual != value[0] {
			r.t.Errorf("\nheader %q mismatch:\n  expected: %s\n  actual:   %s", key, value[0], actual)
		}
	}
	return r
}

// --- JSON Assertions ---

// Json asserts on JSON response using dot-notation paths.
//
//	r.Json("user")                     // assert key exists
//	r.Json("user.name", "John")        // assert value equals
//	r.Json("user.age", ">", 18)        // assert with comparison operator
//	r.Json("user.created_at", "<=", time.Now())
func (r *Response) Json(path string, args ...interface{}) *Response {
	r.t.Helper()

	var data interface{}
	if err := json.Unmarshal(r.body, &data); err != nil {
		r.t.Fatalf("tapetest: response is not valid JSON: %v\nbody: %s", err, string(r.body))
		return r
	}

	value, exists := resolveJSONPath(data, path)
	if !exists {
		r.t.Errorf("\nJSON path %q not found in response\n  body: %s", path, string(r.body))
		return r
	}

	switch len(args) {
	case 0:
		// Just check existence — already done above
	case 1:
		// Equality check
		if !jsonValueEqual(value, args[0]) {
			r.t.Errorf("\nJSON %q mismatch:\n  expected: %v\n  actual:   %v", path, args[0], value)
		}
	case 2:
		// Comparison: Json("user.age", ">", 18)
		operator, ok := args[0].(string)
		if !ok {
			r.t.Fatalf("tapetest: Json() comparison operator must be string, got %T", args[0])
			return r
		}
		if !compareValues(value, operator, args[1]) {
			r.t.Errorf("\nJSON %q comparison failed: %v %s %v", path, value, operator, args[1])
		}
	default:
		r.t.Fatalf("tapetest: Json() accepts 0, 1, or 2 extra arguments, got %d", len(args))
	}

	return r
}

// --- Error Assertion ---

// Error asserts that the request resulted in a Go error (network error, timeout, panic, etc.).
//
//	r.Error()
func (r *Response) Error() *Response {
	r.t.Helper()
	if r.err == nil {
		r.t.Errorf("\nexpected an error but got nil (status: %d)", r.code)
	}
	return r
}

// --- Streaming ---

// Stream processes the response body as a stream via a callback function.
// The callback receives an io.Reader for line-by-line or chunked processing.
//
//	r.Stream(func(reader io.Reader) error {
//	    scanner := bufio.NewScanner(reader)
//	    for scanner.Scan() {
//	        fmt.Println(scanner.Text())
//	    }
//	    return nil
//	})
func (r *Response) Stream(fn func(io.Reader) error) *Response {
	r.t.Helper()
	reader := bytes.NewReader(r.body)
	if err := fn(reader); err != nil {
		r.t.Fatalf("tapetest: stream callback error: %v", err)
	}
	return r
}

// --- ResponseData Methods ---

// Json returns the response body parsed as a generic JSON value.
//
//	data := r.Response.Json()
func (d *ResponseData) Json() interface{} {
	var v interface{}
	_ = json.Unmarshal(d.body, &v)
	return v
}

// Text returns the response body as a string.
//
//	body := r.Response.Text()
func (d *ResponseData) Text() string {
	return string(d.body)
}

// Write saves the response body to a file. If path is a directory,
// generates a filename from the Content-Disposition header or request path.
// Returns the full path of the saved file.
//
//	r.Response.Write("/tmp/avatar.png")
//	r.Response.Write("/tmp/")  // auto-generates filename
func (d *ResponseData) Write(path string) string {
	if path == "" {
		path = "."
	}

	// Check if path is a directory
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		// Generate filename
		filename := generateFilename(d.headers)
		path = filepath.Join(path, filename)
	}

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return ""
	}

	if err := os.WriteFile(path, d.body, 0644); err != nil {
		return ""
	}

	return path
}

// generateFilename tries to get a filename from Content-Disposition header,
// falls back to "response.bin".
func generateFilename(headers http.Header) string {
	cd := headers.Get("Content-Disposition")
	if cd != "" {
		if idx := strings.Index(cd, "filename="); idx != -1 {
			name := cd[idx+9:]
			name = strings.Trim(name, `"`)
			if name != "" {
				return name
			}
		}
	}
	return "response.bin"
}

// --- Cookie Assertions ---

// Cookie asserts a response cookie. With one argument, checks existence.
// With two arguments, checks the value matches. Supports operators for numeric values.
//
//	r.Cookie("session_id")                                    // check exists
//	r.Cookie("session_id", "abc123")                           // check value
//	r.Cookie("count", ">", 1)                                  // check numeric greater than
//	r.Cookie("count", "<", 10)                                 // check numeric less than
//	r.Cookie("count", ">=", 1)                                 // check numeric greater or equal
//	r.Cookie("count", "<=", 10)                                // check numeric less or equal
func (r *Response) Cookie(key string, value ...interface{}) *Response {
	r.t.Helper()

	var cookie *http.Cookie
	for _, c := range r.cookies {
		if c.Name == key {
			cookie = c
			break
		}
	}

	if cookie == nil {
		r.t.Errorf("\nexpected cookie %q to exist but it was not found", key)
		return r
	}

	if len(value) > 0 {
		// Check value
		if len(value) == 1 {
			// Simple value check
			if fmt.Sprintf("%v", value[0]) != cookie.Value {
				r.t.Errorf("\ncookie %q value mismatch:\n  expected: %v\n  actual:   %s", key, value[0], cookie.Value)
			}
		} else if len(value) == 2 {
			// Operator check for numeric values
			operator, ok := value[0].(string)
			if !ok {
				r.t.Fatalf("tapetest: Cookie() operator must be a string, got %T", value[0])
			}

			expectedNum, ok := value[1].(int)
			if !ok {
				// Try to convert from string
				if strVal, ok := value[1].(string); ok {
					var err error
					expectedNum, err = strconv.Atoi(strVal)
					if err != nil {
						r.t.Fatalf("tapetest: Cookie() expected value must be a number or numeric string, got %T", value[1])
					}
				} else {
					r.t.Fatalf("tapetest: Cookie() expected value must be a number or numeric string, got %T", value[1])
				}
			}

			actualNum, err := strconv.Atoi(cookie.Value)
			if err != nil {
				r.t.Errorf("\ncookie %q value %q is not a numeric value for comparison", key, cookie.Value)
				return r
			}

			switch operator {
			case ">":
				if !(actualNum > expectedNum) {
					r.t.Errorf("\ncookie %q value %d is not greater than %d", key, actualNum, expectedNum)
				}
			case "<":
				if !(actualNum < expectedNum) {
					r.t.Errorf("\ncookie %q value %d is not less than %d", key, actualNum, expectedNum)
				}
			case ">=":
				if !(actualNum >= expectedNum) {
					r.t.Errorf("\ncookie %q value %d is not greater than or equal to %d", key, actualNum, expectedNum)
				}
			case "<=":
				if !(actualNum <= expectedNum) {
					r.t.Errorf("\ncookie %q value %d is not less than or equal to %d", key, actualNum, expectedNum)
				}
			case "==":
				if actualNum != expectedNum {
					r.t.Errorf("\ncookie %q value %d is not equal to %d", key, actualNum, expectedNum)
				}
			case "!=":
				if actualNum == expectedNum {
					r.t.Errorf("\ncookie %q value %d should not be equal to %d", key, actualNum, expectedNum)
				}
			default:
				r.t.Fatalf("tapetest: Cookie() unsupported operator %q, use >, <, >=, <=, ==, or !=", operator)
			}
		}
	}

	return r
}

// Cookies returns all cookies from the response.
//
//	fmt.Println(r.Response.Cookies)
func (d *ResponseData) Cookies() []*http.Cookie {
	return d.cookies
}
