package tapetest

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestDocOrder(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	})

	EnableRecording(".tapetest-test")
	defer func() { _ = ClearRecordings() }()

	c := HandlerClient(t, handler)

	// First recorded, but should be LAST in the docs.
	c.Get("/todos").DocOrder(-1)
	// Recorded second, but should be FIRST in the docs.
	c.Get("/todos").DocOrder(0)
	// Recorded third, excluded entirely.
	c.Get("/todos").DocOrder(nil)
	// Recorded fourth, natural order (middle).
	c.Get("/todos")

	recs := orderedRecordings(GetRecordings())

	if len(recs) != 3 {
		t.Fatalf("expected 3 recordings after excluding 1, got %d", len(recs))
	}

	// Expect: [DocOrder=0, unset, DocOrder=-1]
	want := []*int{ptrInt(0), nil, ptrInt(-1)}
	for i, want := range want {
		got := recs[i].DocOrder
		if (got == nil) != (want == nil) {
			t.Fatalf("order[%d]: docOrder presence mismatch: got %v, want %v", i, got, want)
		}
		if got != nil && *got != *want {
			t.Fatalf("order[%d]: docOrder got %d, want %d", i, *got, *want)
		}
	}

	for _, r := range recs {
		if r.ExcludeFromDocs {
			t.Fatalf("excluded recording should not appear")
		}
	}
}

func ptrInt(v int) *int { return &v }

// Ensure ordered examples survive JSON marshalling in insertion order.
func TestOpenAPIExamplesMarshalOrder(t *testing.T) {
	ex := NewOpenAPIExamples()
	ex.Set("zebra", OpenAPIExample{Summary: "z"})
	ex.Set("alpha", OpenAPIExample{Summary: "a"})
	ex.Set("mike", OpenAPIExample{Summary: "m"})

	out, err := json.Marshal(ex)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	want := `"zebra":{"summary":"z"},"alpha":{"summary":"a"},"mike":{"summary":"m"}`
	if !strings.Contains(got, want) {
		t.Fatalf("expected insertion order in JSON, got: %s", got)
	}
}
