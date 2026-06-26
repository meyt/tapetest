package tapetest

import (
	"io"
	"net/http"
	"testing"
)

// captureBodyHandler records the request body, Content-Type and resolved path
// for assertion.
func captureBodyHandler(t *testing.T, gotBody, gotCT, gotPath *string) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*gotBody = string(b)
		*gotCT = r.Header.Get("Content-Type")
		*gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})
}

// TestBodyAsOptionMixedOrder verifies the body can be supplied as an option in
// any position alongside other options — no dedicated/positional body argument
// and no nil placeholder required.
func TestBodyAsOptionMixedOrder(t *testing.T) {
	var gotBody, gotCT, gotPath string
	c := HandlerClient(t, captureBodyHandler(t, &gotBody, &gotCT, &gotPath))

	c.Post("/item/:id", Param{"id": "1"}, Json{"name": "widget", "qty": 3})

	if gotPath != "/item/1" {
		t.Errorf("path: want /item/1, got %q", gotPath)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type: want application/json, got %q", gotCT)
	}
	if gotBody != `{"name":"widget","qty":3}` {
		t.Errorf("body: want {\"name\":\"widget\",\"qty\":3}, got %q", gotBody)
	}
}

// TestJsonAsOption verifies a Json body supplied as an option is sent as JSON.
func TestJsonAsOption(t *testing.T) {
	var gotBody, gotCT string
	c := HandlerClient(t, captureBodyHandler(t, &gotBody, &gotCT, new(string)))

	c.Post("/items", Json{"name": "widget", "qty": 3})

	if gotCT != "application/json" {
		t.Errorf("Content-Type: want application/json, got %q", gotCT)
	}
	if gotBody != `{"name":"widget","qty":3}` {
		t.Errorf("body: want {\"name\":\"widget\",\"qty\":3}, got %q", gotBody)
	}
}

// TestJsonOptionLastWins ensures that when multiple Json bodies are supplied as
// options, the last one wins.
func TestJsonOptionLastWins(t *testing.T) {
	var gotBody string
	c := HandlerClient(t, captureBodyHandler(t, &gotBody, new(string), new(string)))

	c.Post("/items", Json{"name": "first"}, Json{"name": "second"})

	if gotBody != `{"name":"second"}` {
		t.Errorf("body: want last option to win, got %q", gotBody)
	}
}

// TestFormAsOption verifies a Form body supplied as an option is sent as
// application/x-www-form-urlencoded.
func TestFormAsOption(t *testing.T) {
	var gotBody, gotCT string
	c := HandlerClient(t, captureBodyHandler(t, &gotBody, &gotCT, new(string)))

	c.Post("/items", Form{"name": "John", "age": 12})

	if gotCT != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type: want application/x-www-form-urlencoded, got %q", gotCT)
	}
	if gotBody != "age=12&name=John" {
		t.Errorf("body: want age=12&name=John, got %q", gotBody)
	}
}

// TestJsonOptionOnAnyVerb verifies that a body can be attached to a body-less
// verb (Request) via the Json option.
func TestJsonOptionOnAnyVerb(t *testing.T) {
	var gotBody, gotCT string
	c := HandlerClient(t, captureBodyHandler(t, &gotBody, &gotCT, new(string)))

	c.Request(http.MethodPut, "/items/1", Json{"name": "widget"})

	if gotCT != "application/json" {
		t.Errorf("Content-Type: want application/json, got %q", gotCT)
	}
	if gotBody != `{"name":"widget"}` {
		t.Errorf("body: want {\"name\":\"widget\"}, got %q", gotBody)
	}
}

// TestNilOptionIsIgnored ensures a nil option (e.g. a leftover nil body) is
// safely skipped instead of panicking.
func TestNilOptionIsIgnored(t *testing.T) {
	var gotBody, gotCT string
	c := HandlerClient(t, captureBodyHandler(t, &gotBody, &gotCT, new(string)))

	c.Post("/items", nil, Json{"name": "widget"})

	if gotBody != `{"name":"widget"}` {
		t.Errorf("body: want {\"name\":\"widget\"}, got %q", gotBody)
	}
}
