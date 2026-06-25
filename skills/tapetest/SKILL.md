---
name: tapetest
description: |
  Use when writing or reviewing Go test code (files ending in _test.go) that uses the
  `github.com/meyt/tapetest` REST API testing library — i.e. a fluent `Client` that
  issues HTTP requests against an `http.Handler` or live server and chains assertions
  (`Status`, `Json`, `Expect`, `Cookie`, `Header`, `Regex`), records request/response
  exchanges, and auto-generates an OpenAPI v3 spec + Swagger UI from those tests.
  Triggers: importing `. "github.com/meyt/tapetest"`, calling `HandlerClient`/`HttpClient`/
  `EchoClient`/`GinClient`, `GenerateDocs`, `EnableRecording`, or go-swag annotation
  comments (`@Title`, `@Path`, `@Security`, `@securityDefinitions.*`, `@BasePath`).
  Do NOT load for unrelated Go testing (stdlib `net/http/httptest` alone, testify, gomock).
---

# tapetest — Go REST API testing + OpenAPI generation

`tapetest` is a zero-dependency Go library that turns `*_test.go` files into both an
exhaustive API test suite **and** the source of truth for OpenAPI documentation. You
write fluent request/assert chains; tapetest records each exchange and can emit a
ready-to-serve Swagger UI site with zero hand-written spec files.

## When to use this skill

Load this skill for any of these tasks:

- Writing `*_test.go` that imports `github.com/meyt/tapetest` (note the dot-import idiom).
- Generating OpenAPI v3 / Swagger UI docs from test recordings (`GenerateDocs`).
- Authoring go-swag style annotations (`@Title`, `@Path`, `@Method`, `@Tag`, `@Security`,
  `@securityDefinitions.*`, `@title`/`@host`/`@BasePath`).
- Debugging tapetest assertion failures, status-pattern matching, or JSON path resolution.

## Core mental model

1. **One `Client` per test.** Construct it with a constructor, then call HTTP methods on it.
2. **Every HTTP method returns a `*Response`.** The `*Response` exposes fluent assertion
   methods that return `*Response` again (chainable) and value accessors (terminal).
3. **Two execution backends, transparent.** `HandlerClient` dispatches through
   `httptest` (no network, catches handler panics); `HttpClient` makes real TCP requests.
   The exact same assertion API works for both.
4. **Bodies are typed.** `Json` bodies are marshaled to JSON automatically; `Form` bodies
   become `application/x-www-form-urlencoded`; passing any `File(...)` option upgrades a
   request to `multipart/form-data`. You never build `io.Reader`/`Content-Type` manually.
5. **Recording is global and opt-in.** `EnableRecording(dir)` flips a process-wide flag.
   Each request is captured after it runs; `FlushRecording()` writes `recordings.json`.
6. **Docs are generated in `TestMain` after `m.Run()`.** `GenerateDocs` reads recordings +
   source annotations and writes `openapi.json` + `index.html`.

## Canonical boilerplate

### Minimal test

```go
package myapp_test

import (
	"testing"

	. "github.com/meyt/tapetest"
)

func TestGetUsers(t *testing.T) {
	c := HandlerClient(t, myHandler)
	c.Get("/users").Status(200)
}
```

### TestMain: tests + recording + docs pipeline

This is the standard orchestration pattern developers assemble manually. Use it verbatim
— the order is load-bearing (`EnableRecording` before `m.Run`, `FlushRecording` before
`GenerateDocs`, capture `code` before `os.Exit`).

```go
func TestMain(m *testing.M) {
	EnableRecording(".tapetest")
	code := m.Run()
	if err := FlushRecording(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: flush recordings: %v\n", err)
	}
	if err := GenerateDocs(GenerateDocsOptions{
		RecordingDir:     ".tapetest",
		OutputDir:        "docs",
		Title:            "My API",
		Version:          "1.0.0",
		SourceDir:        ".", // parse go-swag annotations; omit to skip
		ReadableExamples: true,
		Config: BuildSwaggerConfig(map[string]string{
			"docExpansion":    "none",
			"tryItOutEnabled": "false",
		}),
	}); err != nil {
		fmt.Fprintf(os.Stderr, "warning: generate docs: %v\n", err)
	}
	os.Exit(code)
}
```

A deterministic shell helper that drives this whole pipeline lives in
[`run-and-docs.sh`](run-and-docs.sh) — prefer it over re-typing the steps.

## Client construction

```go
c := HandlerClient(t, handler)               // http.Handler (Echo/Gin/Chi/stdlib)
c := HttpClient(t, "http://localhost:8080")  // live server
c := EchoClient(t, echoInstance)             // alias of HandlerClient
c := GinClient(t, ginEngine)                 // alias of HandlerClient
```

`BaseUrl(prefix)` sets a common prefix prepended to **every** request path. It returns the
client for chaining at construction time:

```go
c := HandlerClient(t, app).BaseUrl("/api/v1")
c.Get("/users") // -> /api/v1/users
```

## Request building

HTTP methods (`Get`, `Post`, `Put`, `Patch`, `Delete`, `Head`, `Request`) take a path,
an optional body (`Json`/`Form`/struct/map), and variadic [`Option`](../../options.go:35)
values. Path parameters (`:id`, `{id}`) are substituted from a [`Param`](../../options.go:66)
map — prefer this over `fmt.Sprintf` so templates line up with annotation `@Path` values.

```go
c.Post("/todos", Json{"title": "Buy milk"})
c.Get("/todos/:id", Param{"id": 42})
c.Get("/users", Query("page", "1"), Query("limit", "10"), Bearer(tok))
c.Post("/upload", Form{"firstName": "John"}, File("avatar", "./photo.png"))
c.Get("/slow", Timeout(5*time.Second))
```

Built-in options: `Query`, `Header`, `Cookie`, `Bearer`, `File`, `Timeout`, `Param`.

## Assertions (summary)

All assertion methods live on `*Response`, return `*Response`, and share one operator
engine (`>`, `>=`, `<`, `<=`, `==`, `=`, `!=`, `~` between, `^` contains-all, `!^`
contains-none, `*` contains-any). Status also accepts patterns like `"2xx"`.

```go
r := c.Get("/users/1")
r.Status(200).
	Json("username", "johndoe").
	Json("id", ">", 0).
	Json("id", "~", 1, 100).
	Json("tags", "^", "vip", "pro").
	Header("Content-Type", "application/json")
```

For the full operator matrix and edge cases (numeric vs. time vs. string coercion,
RFC3339 date comparison, JSON array indexing), read [`assertions.md`](assertions.md).

## Value extraction (for multi-step flows)

Assertion methods are for pass/fail checks; when you need a value to feed the next
request (e.g. a created resource's ID or an auth token), use the `*Val()` accessors or
`r.Response.Json()`:

```go
r := c.Post("/users", Json{"username": "jane", "email": "j@x.com"})
r.Status(201)

// Extract the id to use in subsequent requests.
id := r.JsonVal("id")            // float64 (JSON numbers decode as float64)
c.Delete("/users/:id", Param{"id": int(id.(float64))}).Status(204)
```

## Shared state across requests

Client-level `Header(k,v)` and `Cookie(k,v)` persist for all subsequent requests on that
client. Pass `nil` as the value to remove one.

```go
c.Header("Authorization", "Bearer "+token) // applies to every following request
c.Cookie("session_id", "abc123")
c.Header("Authorization", nil)             // remove
```

## Architectural rules & gotchas

- **Dot-import is idiomatic.** `import . "github.com/meyt/tapetest"` lets you write
  `Json`, `Form`, `Param`, `Query`, etc. unqualified — matching all docs and examples.
- **One client per test is safe; sharing a client across goroutines is not.** The
  `Client`'s shared-header/cookie maps are plain `map`s with no lock. Do not reuse a
  single `Client` concurrently across goroutines; build one per test/subtest.
- **`HandlerClient` catches handler panics** and surfaces them as the response's error
  (assertable via `r.Error()`). It does **not** fail the test by itself.
- **`Timeout` only affects `HttpClient`.** A handler run through `httptest` cannot time out.
- **JSON numbers are `float64`.** When extracting IDs via `JsonVal`, cast to `float64`
  then to `int`. For comparisons, pass Go numeric types directly to operators — tapetest
  parses both sides as `float64`.
- **`BaseUrl` is prepended exactly once** and is seen by the recording layer, so recorded
  paths include the prefix unless `@BasePath` strips it during doc generation.
- **Recording is a global singleton** guarded by a mutex (`recorder.go`). `EnableRecording`
  resets in-memory recordings; call it once at the start of `TestMain`.
- **`t.Helper()` is called everywhere**, so failures point at **your** test line, not
  tapetest internals.

## Where to look next

- [`reference.md`](reference.md) — complete method/type index with signatures.
- [`assertions.md`](assertions.md) — full operator semantics and JSON-path rules.
- [`openapi-docs.md`](openapi-docs.md) — annotations, security definitions, `GenerateDocs`.
- [`run-and-docs.sh`](run-and-docs.sh) — deterministic test + docs pipeline script.
