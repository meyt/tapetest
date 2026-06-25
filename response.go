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

// Header asserts a response header. With no value, checks existence.
// With a value, checks equality or evaluates an operator expression.
//
// All operators supported by Expect/Json are available here too.
//
//	r.Header("Content-Type")                        // check exists
//	r.Header("Content-Type", "application/json")    // check value
//	r.Header("X-Count", ">", 5)                     // numeric operator
//	r.Header("X-Tags", "^", "vip", "pro")           // contains all
func (r *Response) Header(key string, value ...interface{}) *Response {
	r.t.Helper()

	actual := r.headers.Get(key)
	if len(value) == 0 {
		if actual == "" {
			r.t.Errorf("\nexpected header %q to exist but it was not found", key)
		}
		return r
	}

	if ok, msg := evalAssertion(actual, value...); !ok {
		r.t.Errorf("\nheader %q assertion failed:\n  %s\n  value: %s", key, msg, actual)
	}
	return r
}

// --- JSON Assertions ---

// Json asserts on JSON response using dot-notation paths.
//
// All operators supported by Expect are available here too (applied to the
// resolved JSON value rendered as a string).
//
//	r.Json("user")                          // assert key exists
//	r.Json("user.name", "John")             // assert value equals
//	r.Json("user.age", ">", 18)             // assert with comparison operator
//	r.Json("user.created_at", "<=", time.Now())
//	r.Json("user.tags", "^", "vip", "pro")  // value contains all substrings
//	r.Json("user.age", "~", 18, 30)         // value is between 18 and 30
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

	if len(args) == 0 {
		return r // existence already verified above
	}

	actual := jsonValueToString(value)
	if ok, msg := evalAssertion(actual, args...); !ok {
		r.t.Errorf("\nJSON %q assertion failed:\n  %s\n  value: %v", path, msg, value)
	}
	return r
}

// jsonValueToString renders a resolved JSON value to a string suitable for the
// shared assertion engine. Numbers keep their natural representation so that
// numeric parsing and equality still work.
func jsonValueToString(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		if v {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// --- JSON Count Assertion ---

// JsonCount validates the number of elements in a JSON array found at the given
// dot-notation path. It accepts the same comparison operators as the other
// assertions.
//
//	r.JsonCount("items")              // array exists and has at least one item
//	r.JsonCount("items", 2)           // array has exactly 2 items
//	r.JsonCount("items", ">", 2)      // array has more than 2 items
//	r.JsonCount("items", "<=", 5)     // array has at most 5 items
//	r.JsonCount("items", "~", 2, 5)   // array length is between 2 and 5
func (r *Response) JsonCount(path string, args ...interface{}) *Response {
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

	arr, ok := value.([]interface{})
	if !ok {
		r.t.Errorf("\nJSON path %q is not an array\n  body: %s", path, string(r.body))
		return r
	}

	// With no arguments: ensure the array has at least one item.
	if len(args) == 0 {
		if len(arr) == 0 {
			r.t.Errorf("\nJSON path %q is an empty array\n  body: %s", path, string(r.body))
		}
		return r
	}

	actual := strconv.Itoa(len(arr))
	if ok, msg := evalAssertion(actual, args...); !ok {
		r.t.Errorf("\nJSON count %q assertion failed:\n  %s\n  actual count: %d", path, msg, len(arr))
	}
	return r
}

// --- Error Assertion ---

// DocOrder controls how this request's example is prioritized in the generated
// Swagger documentation. It accepts either an int or nil:
//
//	c.Get("/todos").DocOrder(0)   // show as the first example
//	c.Get("/todos").DocOrder(nil) // do not include this example in the docs
//	c.Get("/todos").DocOrder(-1)  // show as the last example
//
// When not called, the example keeps its natural recording order.
// DocOrder returns the Response so it can be chained with assertions.
func (r *Response) DocOrder(order interface{}) *Response {
	switch v := order.(type) {
	case nil:
		SetLastExchangeDocOrder(nil)
	case int:
		SetLastExchangeDocOrder(&v)
	case int8:
		i := int(v)
		SetLastExchangeDocOrder(&i)
	case int16:
		i := int(v)
		SetLastExchangeDocOrder(&i)
	case int32:
		i := int(v)
		SetLastExchangeDocOrder(&i)
	case int64:
		i := int(v)
		SetLastExchangeDocOrder(&i)
	case uint:
		i := int(v)
		SetLastExchangeDocOrder(&i)
	}
	return r
}

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

// --- Body Assertions ---

// Expect asserts the response body as text. The body is trimmed of surrounding
// whitespace before comparison. It supports exact matches and the full set of
// operators (see Expect operators in the README).
//
//	r.Expect("app is working")                     // exact string match
//	r.Expect(13)                                    // numeric match
//	r.Expect(131.50)                                // numeric (float) match
//	r.Expect(">", 131.50)                           // numeric with operator
//	r.Expect(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)) // date match (RFC3339)
//	r.Expect(">", time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC))
//	r.Expect("^", "ielts", "icdl")                  // must contain all of these
//	r.Expect("!^", "mba", "foss")                   // must contain none of these
//	r.Expect("~", 18, 25)                           // numeric between two values
//	r.Expect("*", "ielts", "icdl", "mba")           // contains any of these
func (r *Response) Expect(args ...interface{}) *Response {
	r.t.Helper()
	body := strings.TrimSpace(string(r.body))
	if ok, msg := evalAssertion(body, args...); !ok {
		r.t.Errorf("\nbody assertion failed:\n  %s\n  body: %s", msg, string(r.body))
	}
	return r
}

// Regex asserts that the response body matches a regular expression.
//
//	r.Regex("is working$")
//	r.Regex(`^\d{3}-\d{4}$`)
func (r *Response) Regex(pattern string) *Response {
	r.t.Helper()
	matched, err := matchRegex(string(r.body), pattern)
	if err != nil {
		r.t.Fatalf("tapetest: invalid regex %q: %v", pattern, err)
		return r
	}
	if !matched {
		r.t.Errorf("\nbody does not match regex %q\n  body: %s", pattern, string(r.body))
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

// --- Value Accessors ---

// JsonVal returns a value from the JSON response body.
// With no argument it returns the full parsed JSON object.
// With a dot-notation path it returns the value at that path.
//
//	jsonFull := r.JsonVal()                // full JSON object
//	jsonValue := r.JsonVal("user.age")    // nested value
//	jsonItem := r.JsonVal("items.0.name") // array index
func (r *Response) JsonVal(path ...string) interface{} {
	var data interface{}
	if err := json.Unmarshal(r.body, &data); err != nil {
		return nil
	}
	if len(path) == 0 || path[0] == "" {
		return data
	}
	value, exists := resolveJSONPath(data, path[0])
	if !exists {
		return nil
	}
	return value
}

// StatusVal returns the HTTP status code.
//
//	statusCode := r.StatusVal()
func (r *Response) StatusVal() int {
	return r.code
}

// ReasonVal returns the status reason text.
//
//	statusReason := r.ReasonVal()
func (r *Response) ReasonVal() string {
	return r.reason
}

// CookieVal returns response cookie data.
// With no argument it returns all cookies.
// With a name it returns that single cookie's value as a string.
//
//	cookieFull := r.CookieVal()        // all cookies ([]*http.Cookie)
//	cookieValue := r.CookieVal("count") // single cookie value (string)
func (r *Response) CookieVal(key ...string) interface{} {
	if len(key) == 0 || key[0] == "" {
		return r.cookies
	}
	for _, c := range r.cookies {
		if c.Name == key[0] {
			return c.Value
		}
	}
	return ""
}

// HeaderVal returns response header data.
// With no argument it returns all headers.
// With a key it returns that single header's value as a string.
//
//	headerFull := r.HeaderVal()                 // all headers (http.Header)
//	headerValue := r.HeaderVal("Authorization") // single header value (string)
func (r *Response) HeaderVal(key ...string) interface{} {
	if len(key) == 0 || key[0] == "" {
		return r.headers
	}
	return r.headers.Get(key[0])
}

// TextVal returns the response body as raw text.
//
//	bodyText := r.TextVal()
func (r *Response) TextVal() string {
	return string(r.body)
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

// Cookie asserts a response cookie. With no value, checks existence.
// With a value, checks equality or evaluates an operator expression.
//
// All operators supported by Expect/Json are available here too.
//
//	r.Cookie("session_id")                  // check exists
//	r.Cookie("session_id", "abc123")        // check value
//	r.Cookie("count", ">", 1)               // numeric greater than
//	r.Cookie("count", "~", 1, 10)           // numeric between 1 and 10
//	r.Cookie("flags", "^", "opt-in", "vip") // value contains all
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

	if len(value) == 0 {
		return r // existence already verified above
	}

	if ok, msg := evalAssertion(cookie.Value, value...); !ok {
		r.t.Errorf("\ncookie %q assertion failed:\n  %s\n  value: %s", key, msg, cookie.Value)
	}
	return r
}

// Cookies returns all cookies from the response.
//
//	fmt.Println(r.Response.Cookies)
func (d *ResponseData) Cookies() []*http.Cookie {
	return d.cookies
}
