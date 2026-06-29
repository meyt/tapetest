package tapetest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- Enum types declared at package level ---
//
// These are registered via RegisterEnum in init() below. Test code uses the
// bare constants; the recorder reflects on the named type and consults the
// registry to recover the allowed values.

type GenderType string

const (
	Male   GenderType = "male"
	Female GenderType = "female"
)

type UserProp string

const (
	GenderProp   UserProp = "gender"
	UsernameProp UserProp = "username"
)

type SortField string

const (
	SortName     SortField = "name"
	SortNameDesc SortField = "-name"
	SortCreated  SortField = "created_at"
	SortCreatedD SortField = "-created_at"
)

type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
	RoleGuest Role = "guest"
)

func init() {
	// Package-level enums are registered once at process start.
	RegisterEnum(Male, Female)
	RegisterEnum(GenderProp, UsernameProp)
	RegisterEnum(SortName, SortNameDesc, SortCreated, SortCreatedD)
	RegisterEnum(RoleAdmin, RoleUser, RoleGuest)
}

// TestEnumByNamedType drives the full pipeline using plain Go named types
// and constants. The generated OpenAPI document should carry `enum`
// constraints on the right schemas, and recorded bodies should show the
// bare string values.
func TestEnumByNamedType(t *testing.T) {
	EnableRecording(".tapetest-test")
	defer func() {
		_ = ClearRecordings()
		DisableRecording()
	}()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	c := HandlerClient(t, handler)

	// Exactly the API the user requested:
	c.Get("/admin/address-parts", Query{"sort_by": SortName}).Status(200)
	c.Get("/user/:prop", Param{"prop": UsernameProp}).Status(200)
	c.Patch("/user", Form{"gender": Male}).Status(200)
	c.Post("/user", Json{"role": RoleAdmin}).Status(200)

	annotations := []HandlerAnnotation{
		{Method: "GET", Path: "/admin/address-parts"},
		{Method: "GET", Path: "/user/:prop"},
		{Method: "PATCH", Path: "/user"},
		{Method: "POST", Path: "/user"},
	}

	recordings := GetRecordings()
	doc := GenerateOpenAPIFromRecordings(recordings, annotations, nil, "", "Test API", "1.0", false)
	rendered, _ := json.MarshalIndent(doc, "", "  ")

	// --- query enum ---
	qp := findParam(t, doc, "/admin/address-parts", "get", "query", "sort_by")
	want := []interface{}{"name", "-name", "created_at", "-created_at"}
	if !sameSlice(qp.Schema.Enum, want) {
		t.Errorf("sort_by enum mismatch\ngot:  %v\nwant: %v\ndoc: %s", qp.Schema.Enum, want, rendered)
	}

	// --- path param enum ---
	pp := findParam(t, doc, "/user/{prop}", "get", "path", "prop")
	want = []interface{}{"gender", "username"}
	if !sameSlice(pp.Schema.Enum, want) {
		t.Errorf("prop path enum mismatch\ngot:  %v\nwant: %v\ndoc: %s", pp.Schema.Enum, want, rendered)
	}

	// --- form body enum ---
	patchSchema := doc.Paths["/user"]["patch"].RequestBody.Content["application/x-www-form-urlencoded"].Schema
	if got, want := patchSchema.Properties["gender"].Enum, []interface{}{"male", "female"}; !sameSlice(got, want) {
		t.Errorf("gender form enum mismatch\ngot:  %v\nwant: %v\ndoc: %s", got, want, rendered)
	}
	patchExample := doc.Paths["/user"]["patch"].RequestBody.Content["application/x-www-form-urlencoded"].Examples
	if v := exampleValueJSON(patchExample, "TestEnumByNamedType"); !strings.Contains(v, `"gender":"male"`) {
		t.Errorf("gender form example must serialize as bare string\ngot: %s\ndoc: %s", v, rendered)
	}

	// --- json body enum ---
	postSchema := doc.Paths["/user"]["post"].RequestBody.Content["application/json"].Schema
	if got, want := postSchema.Properties["role"].Enum, []interface{}{"admin", "user", "guest"}; !sameSlice(got, want) {
		t.Errorf("role json enum mismatch\ngot:  %v\nwant: %v\ndoc: %s", got, want, rendered)
	}
}

// TestEnumFunctionLocalType verifies that enum types declared *inside* a test
// function (not at package level) work when registered via RegisterEnum from
// within the same function. This is the most ergonomic pattern for tests —
// the type, its values, and the registration call all live next to the test.
func TestEnumFunctionLocalType(t *testing.T) {
	EnableRecording(".tapetest-local")
	defer func() {
		_ = ClearRecordings()
		DisableRecording()
	}()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	c := HandlerClient(t, handler)

	// Type and consts declared inside the test function — Go allows this.
	type LocalStatus string
	const (
		LocalStatusActive   LocalStatus = "local-active"
		LocalStatusInactive LocalStatus = "local-inactive"
	)
	// Registration must happen in the same scope where the consts are visible.
	RegisterEnum(LocalStatusActive, LocalStatusInactive)

	c.Get("/items", Query{"status": LocalStatusActive}).Status(200)

	annotations := []HandlerAnnotation{
		{Method: "GET", Path: "/items"},
	}

	recordings := GetRecordings()
	doc := GenerateOpenAPIFromRecordings(recordings, annotations, nil, "", "Test API", "1.0", false)
	rendered, _ := json.MarshalIndent(doc, "", "  ")

	qp := findParam(t, doc, "/items", "get", "query", "status")
	want := []interface{}{"local-active", "local-inactive"}
	if !sameSlice(qp.Schema.Enum, want) {
		t.Errorf("function-local enum not detected\ngot:  %v\nwant: %v\ndoc: %s", qp.Schema.Enum, want, rendered)
	}
}

// TestEnumWireBodyIsBareString confirms that named-typed string values are
// sent over the wire as their bare string, not as a struct. We capture the
// raw request bytes via an httptest.Server.
func TestEnumWireBodyIsBareString(t *testing.T) {
	EnableRecording(".tapetest-test-wire")
	defer func() {
		_ = ClearRecordings()
		DisableRecording()
	}()

	var capturedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		capturedBody = string(buf)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := HttpClient(t, srv.URL)
	c.Post("/", Json{"role": RoleAdmin}).Status(200)

	if !strings.Contains(capturedBody, `"role":"admin"`) {
		t.Errorf("wire body did not contain bare enum value\nbody: %s", capturedBody)
	}
	if strings.Contains(capturedBody, "Role") {
		t.Errorf("wire body leaked type name\nbody: %s", capturedBody)
	}
}

// --- helpers ---

func findParam(t *testing.T, doc *OpenAPIDocument, path, method, in, name string) OpenAPIParameter {
	t.Helper()
	ops, ok := doc.Paths[path]
	if !ok {
		t.Fatalf("path %q not in doc; paths=%v", path, pathKeys(doc.Paths))
	}
	op, ok := ops[method]
	if !ok {
		t.Fatalf("method %q not on path %q; methods=%v", method, path, methodKeys(ops))
	}
	for _, p := range op.Parameters {
		if p.In == in && p.Name == name {
			return p
		}
	}
	t.Fatalf("parameter in=%q name=%q not found on %s %s", in, name, method, path)
	return OpenAPIParameter{}
}

func pathKeys(m map[string]map[string]OpenAPIOperation) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func methodKeys(m map[string]OpenAPIOperation) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func sameSlice(a, b []interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// exampleValueJSON returns the JSON-marshaled value of the named example.
// Accesses the unexported fields of OpenAPIExamples directly (test is in the
// same package).
func exampleValueJSON(exs *OpenAPIExamples, name string) string {
	if exs == nil || exs.vals == nil {
		return ""
	}
	ex, ok := exs.vals[name]
	if !ok {
		return ""
	}
	bs, _ := json.Marshal(ex.Value)
	return string(bs)
}
