package tapetest

import (
	"net/http"
	"testing"
)

// TestClientServerTagsRecordings verifies that the Client.Server builder tags
// every recording with the configured service name and URL, so per-operation
// servers can be emitted for multi-service suites.
func TestClientServerTagsRecordings(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	})

	EnableRecording(".tapetest-test")
	defer func() { _ = ClearRecordings() }()

	c := HandlerClient(t, handler).BaseUrl("/api/v1").Server("Admin API", "/api/v1")
	c.Get("/users")

	recs := GetRecordings()
	if len(recs) != 1 {
		t.Fatalf("expected 1 recording, got %d", len(recs))
	}
	if recs[0].Server != "Admin API" {
		t.Errorf("expected Server=%q, got %q", "Admin API", recs[0].Server)
	}
	if recs[0].ServerURL != "/api/v1" {
		t.Errorf("expected ServerURL=%q, got %q", "/api/v1", recs[0].ServerURL)
	}
}

// TestRecordingsWithoutServerHaveNoServerMetadata ensures a plain client
// (no Server() call) records no server metadata — backwards compatibility.
func TestRecordingsWithoutServerHaveNoServerMetadata(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	EnableRecording(".tapetest-test")
	defer func() { _ = ClearRecordings() }()

	c := HandlerClient(t, handler)
	c.Get("/ping")

	recs := GetRecordings()
	if len(recs) != 1 {
		t.Fatalf("expected 1 recording, got %d", len(recs))
	}
	if recs[0].Server != "" || recs[0].ServerURL != "" {
		t.Errorf("expected empty server metadata, got Server=%q ServerURL=%q",
			recs[0].Server, recs[0].ServerURL)
	}
}

// TestPerOperationServersEmission verifies that recordings carrying service
// metadata produce a per-operation servers array, deduplicated and in
// first-seen order. It models the multi-service scenario from the feature
// request (Admin API + User API on different URLs).
func TestPerOperationServersEmission(t *testing.T) {
	// Two independent services hitting the same logical path template but
	// different base URLs.
	adminHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"service":"admin"}`))
	})
	userHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"service":"user"}`))
	})

	EnableRecording(".tapetest-test")
	defer func() { _ = ClearRecordings() }()

	// Admin API client.
	admin := HandlerClient(t, adminHandler).Server("Admin API", "/api/v1")
	admin.Get("/api/v1/users")

	// User API client (different relative URL).
	user := HandlerClient(t, userHandler).Server("User API", "/api/v2")
	user.Get("/api/v2/users")

	doc := GenerateOpenAPIFromRecordings(
		GetRecordings(),
		nil, nil, "", "Multi-Service API", "1.0.0", false,
	)

	usersOps, ok := doc.Paths["/users"]
	if !ok {
		t.Fatalf("expected a /users path, got paths: %v", doc.Paths)
	}
	op, ok := usersOps["get"]
	if !ok {
		t.Fatalf("expected a GET /users operation, got: %v", usersOps)
	}

	if len(op.Servers) != 2 {
		t.Fatalf("expected 2 per-operation servers, got %d: %+v", len(op.Servers), op.Servers)
	}

	wantURLs := []string{"/api/v1", "/api/v2"}
	wantDescs := []string{"Admin API", "User API"}
	for i, s := range op.Servers {
		if s.URL != wantURLs[i] {
			t.Errorf("server[%d]: expected URL %q, got %q", i, wantURLs[i], s.URL)
		}
		if s.Description != wantDescs[i] {
			t.Errorf("server[%d]: expected description %q, got %q", i, wantDescs[i], s.Description)
		}
	}
}

// TestPerOperationServersDeduplicated verifies that several recordings from
// the same service collapse into a single per-operation server entry.
func TestPerOperationServersDeduplicated(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	})

	EnableRecording(".tapetest-test")
	defer func() { _ = ClearRecordings() }()

	// BaseUrl prefixes each path, so the recorded path is /api/v1/users.
	// The ServerURL /api/v1 is then stripped so the OpenAPI path is /users
	// and Try-it-out resolves /api/v1 + /users.
	c := HandlerClient(t, handler).BaseUrl("/api/v1").Server("Admin API", "/api/v1")
	c.Get("/users")
	c.Get("/users")
	c.Get("/users")

	doc := GenerateOpenAPIFromRecordings(
		GetRecordings(),
		nil, nil, "", "API", "1.0.0", false,
	)

	op := doc.Paths["/users"]["get"]
	if len(op.Servers) != 1 {
		t.Fatalf("expected a single deduplicated server, got %d: %+v", len(op.Servers), op.Servers)
	}
	if op.Servers[0].URL != "/api/v1" || op.Servers[0].Description != "Admin API" {
		t.Errorf("unexpected server: %+v", op.Servers[0])
	}
}

// TestPerOperationServersMixed verifies that a mix of tagged and untagged
// recordings only emits servers for the tagged ones.
func TestPerOperationServersMixed(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	})

	EnableRecording(".tapetest-test")
	defer func() { _ = ClearRecordings() }()

	// Tagged recording.
	tagged := HandlerClient(t, handler).Server("Admin API", "/api/v1")
	tagged.Get("/users")

	// Untagged recording (legacy client, no server info).
	plain := HandlerClient(t, handler)
	plain.Get("/users")

	doc := GenerateOpenAPIFromRecordings(
		GetRecordings(),
		nil, nil, "", "API", "1.0.0", false,
	)

	op := doc.Paths["/users"]["get"]
	if len(op.Servers) != 1 {
		t.Fatalf("expected exactly 1 server (only the tagged recording), got %d: %+v",
			len(op.Servers), op.Servers)
	}
	if op.Servers[0].URL != "/api/v1" {
		t.Errorf("expected server URL /api/v1, got %q", op.Servers[0].URL)
	}
}

// TestPerOperationServersAbsolute verifies that an absolute server URL (e.g.
// "https://user-api.example.com") is emitted verbatim as an operation-level
// server and is NOT stripped from the recorded request path. The path prefix
// comes from BaseUrl in this scenario.
func TestPerOperationServersAbsolute(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	})

	EnableRecording(".tapetest-test")
	defer func() { _ = ClearRecordings() }()

	admin := HandlerClient(t, handler).
		BaseUrl("/api/v1").
		Server("Admin API", "https://admin-api.example.com")
	user := HandlerClient(t, handler).
		BaseUrl("/api/v1").
		Server("User API", "https://user-api.example.com")
	admin.Get("/users")
	user.Get("/users")

	doc := GenerateOpenAPIFromRecordings(
		GetRecordings(),
		nil, nil, "", "Multi-Service API", "1.0.0", false,
	)

	// Paths retain the BaseUrl prefix because absolute server URLs are not
	// stripped; both recordings land under /api/v1/users.
	op, ok := doc.Paths["/api/v1/users"]["get"]
	if !ok {
		t.Fatalf("expected GET /api/v1/users, got paths: %v", doc.Paths)
	}
	if len(op.Servers) != 2 {
		t.Fatalf("expected 2 absolute servers, got %d: %+v", len(op.Servers), op.Servers)
	}
	want := map[string]string{
		"https://admin-api.example.com": "Admin API",
		"https://user-api.example.com":  "User API",
	}
	for _, s := range op.Servers {
		desc, ok := want[s.URL]
		if !ok {
			t.Errorf("unexpected server URL %q", s.URL)
			continue
		}
		if s.Description != desc {
			t.Errorf("server %q: expected description %q, got %q", s.URL, desc, s.Description)
		}
	}
}

// TestIsAbsoluteURL covers the URL-scheme detection used to decide whether a
// server URL is stripped from recorded paths.
func TestIsAbsoluteURL(t *testing.T) {
	cases := map[string]bool{
		"https://example.com":   true,
		"http://localhost:8080": true,
		"HTTPS://Example.com":   true,
		"/api/v1":               false,
		"api/v1":                false,
		"":                      false,
		"ftp://example.com":     false, // only http(s) treated as absolute
	}
	for in, want := range cases {
		if got := isAbsoluteURL(in); got != want {
			t.Errorf("isAbsoluteURL(%q) = %v, want %v", in, got, want)
		}
	}
}
