package tests

import (
	"net/http"
	"testing"

	. "github.com/meyt/tapetest"
)

// jsonHandler returns an http.Handler that replies with the given JSON body
// and an application/json Content-Type.
func jsonHandler(body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(body))
	})
}

// TestJsonCountNonEmpty verifies that a non-empty array passes the no-arg form.
func TestJsonCountNonEmpty(t *testing.T) {
	c := HandlerClient(t, jsonHandler(`{"items":[1,2,3]}`))
	c.Get("/items").JsonCount("items")
}

// TestJsonCountExact verifies the exact-count form.
func TestJsonCountExact(t *testing.T) {
	c := HandlerClient(t, jsonHandler(`{"items":[1,2,3]}`))
	c.Get("/items").JsonCount("items", 3)
	c.Get("/items").JsonCount("items", "==", 3)
}

// TestJsonCountOperators verifies the comparison-operator forms pass for
// matching counts.
func TestJsonCountOperators(t *testing.T) {
	c := HandlerClient(t, jsonHandler(`{"items":[1,2,3]}`))
	c.Get("/items").JsonCount("items", ">", 2)
	c.Get("/items").JsonCount("items", ">=", 3)
	c.Get("/items").JsonCount("items", "<", 4)
	c.Get("/items").JsonCount("items", "<=", 3)
	c.Get("/items").JsonCount("items", "!=", 5)
	c.Get("/items").JsonCount("items", "~", 2, 5)
}

// TestJsonCountNestedArray verifies dot-notation paths to nested arrays.
func TestJsonCountNestedArray(t *testing.T) {
	body := `{"data":{"tags":["a","b"]}}`
	c := HandlerClient(t, jsonHandler(body))
	c.Get("/items").JsonCount("data.tags")
	c.Get("/items").JsonCount("data.tags", 2)
	c.Get("/items").JsonCount("data.tags", "==", 2)
}

// TestJsonCountViaHandler exercises the full handler-based flow.
func TestJsonCountViaHandler(t *testing.T) {
	c := HandlerClient(t, jsonHandler(`{"items":[1,2,3,4]}`))
	c.Get("/items").JsonCount("items", ">", 3)
}
