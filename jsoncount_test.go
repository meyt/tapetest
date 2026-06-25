package tapetest

import (
	"encoding/json"
	"net/http"
	"testing"
)

func newJSONResponse(t *testing.T, body string) *Response {
	return &Response{
		t:      t,
		code:   200,
		reason: "OK",
		body:   []byte(body),
	}
}

// TestJsonCountNonEmpty verifies that a non-empty array passes the no-arg form.
func TestJsonCountNonEmpty(t *testing.T) {
	r := newJSONResponse(t, `{"items":[1,2,3]}`)
	r.JsonCount("items")
}

// TestJsonCountExact verifies the exact-count form.
func TestJsonCountExact(t *testing.T) {
	r := newJSONResponse(t, `{"items":[1,2,3]}`)
	r.JsonCount("items", 3)
	r.JsonCount("items", "==", 3)
}

// TestJsonCountOperators verifies the comparison-operator forms pass for
// matching counts.
func TestJsonCountOperators(t *testing.T) {
	r := newJSONResponse(t, `{"items":[1,2,3]}`)
	r.JsonCount("items", ">", 2)
	r.JsonCount("items", ">=", 3)
	r.JsonCount("items", "<", 4)
	r.JsonCount("items", "<=", 3)
	r.JsonCount("items", "!=", 5)
	r.JsonCount("items", "~", 2, 5)
}

// TestJsonCountNestedArray verifies dot-notation paths to nested arrays.
func TestJsonCountNestedArray(t *testing.T) {
	body, _ := json.Marshal(map[string]interface{}{
		"data": map[string]interface{}{"tags": []string{"a", "b"}},
	})
	r := newJSONResponse(t, string(body))
	r.JsonCount("data.tags")
	r.JsonCount("data.tags", 2)
	r.JsonCount("data.tags", "==", 2)
}

// TestJsonCountViaHandler exercises the full handler-based flow.
func TestJsonCountViaHandler(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[1,2,3,4]}`))
	})
	c := HandlerClient(t, handler)
	c.Get("/items").JsonCount("items", ">", 3)
}
