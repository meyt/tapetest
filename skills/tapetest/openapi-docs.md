# tapetest — OpenAPI / Swagger UI generation

tapetest generates documentation **from your tests**: every recorded request/response becomes
an OpenAPI example, and go-swag-style source annotations enrich the spec (titles, tags,
security). No separate CLI or hand-written YAML.

## End-to-end: `GenerateDocs`

`GenerateDocs(opts)` ([`swagger.go:371`](../../swagger.go:371)) does, in order:

1. `LoadRecordings(RecordingDir)` → recorded exchanges.
2. If `SourceDir != ""`: parse handler `@*` annotations, `@securityDefinitions.*`, and
   general API info (`@title`, `@host`, `@BasePath`, ...) from non-test `.go` files.
3. `GenerateOpenAPIFromRecordings(...)` → match each recording to an annotation template,
   group by path+method, build operations. Each test case becomes an **example**.
4. `doc.ApplyGeneralAPIInfo(...)` — source annotations override flag defaults for `info`.
5. Add server URL: explicit `ServerURL` wins, else `@host`+`@BasePath`+`@schemes`.
6. Write `OutputDir/openapi.json`.
7. `GenerateSwaggerUI(...)` → write `OutputDir/index.html` (+ copied assets).

It prints a summary (`N endpoints documented from M recordings`) and returns `nil` on
success. Required fields: `RecordingDir`, `OutputDir`.

```go
GenerateDocs(GenerateDocsOptions{
	RecordingDir:     ".tapetest",
	OutputDir:        "docs",
	Title:            "My API",
	Version:          "1.0.0",
	SourceDir:        ".",          // omit to skip annotations entirely
	ReadableExamples: true,         // "TestCreateTodo" -> "Create Todo"
	Config: BuildSwaggerConfig(map[string]string{
		"docExpansion":    "none",
		"tryItOutEnabled": "false",
	}),
})
```

### `GenerateDocsOptions` fields

| Field | Purpose |
|-------|---------|
| `RecordingDir`, `OutputDir` | Required paths. |
| `Title`, `Version` | Default OpenAPI `info.title`/`info.version` (overridden by `@title`/`@version`). |
| `SourceDir` | Parsed for annotations; empty disables annotation parsing. |
| `ServerURL` | Explicit server entry; else built from `@host`/`@BasePath`/`@schemes`. |
| `ReadableExamples` | Humanize test names in example summaries. |
| `SwaggerUICSS`, `SwaggerUIJS` | Local path or URL; default to unpkg CDN (`DefaultSwaggerUICSS`/`DefaultSwaggerUIJS`). |
| `CSSFiles`, `JSFiles` | Custom local files (copied in) or URLs injected into the UI. |
| `Config` | `SwaggerUIConfig`; `nil` map → `DefaultSwaggerUIConfig()`. |

## Path matching (recordings ↔ annotations)

Recorded request paths include the `BaseUrl` prefix. `@BasePath` (general API info) is
**stripped** before matching so `/api/v1/users/1` aligns with a template `/users/:id`.
Matching is segment-by-segment ([`openapi.go:701`](../../openapi.go:701)):

- `:param` and `{param}` segments are wildcards (captured into OpenAPI path params).
- Static segments must match literally.
- **Static-first ordering**: annotations are sorted so `/todos/search` matches before
  `/todos/:id` — preventing the parameterized route from swallowing literal sub-paths.

If no annotation matches, the recorded path is used verbatim (no path params, no metadata).

## Handler annotations

Place above a handler function. Requires **both** `@Method` and `@Path` to be emitted.

```go
// @Title Create user
// @Description Create a new user
// @Tag users
// @Security UserAuth
// @Security AdminAuth
// @Method POST
// @Path /users
func (a *App) createUser(c echo.Context) error { ... }
```

| Key | Effect | Multiplicity |
|-----|--------|--------------|
| `@Title` | `operation.summary` | one |
| `@Description` | `operation.description` | one |
| `@Tag` | adds to `operation.tags` | many |
| `@Method` | HTTP verb (uppercased) | one (required) |
| `@Path` | template (`:id`/`{id}`) | one (required) |
| `@Security` | adds a security requirement | many (OR-semantics: any scheme suffices) |

Parsed by [`ParseAnnotationsFromDir`](../../parser.go:150). Only non-`_test.go` files are scanned.

## Security definitions (`@securityDefinitions.*`)

Declared once per project (typically in `main.go`), mapped to
`components.securitySchemes`. All directives are matched **case-insensitively** (go-swag parity).

```go
// @securityDefinitions.apikey UserAuth
// @in header
// @name Authorization
// @description User JWT token.

// @securityDefinitions.bearer AdminAuth
// @bearerFormat JWT
// @description Admin JWT token.
```

Supported scheme markers and their go-swag → OpenAPI v3 flow mapping:

| Directive | Type | Notes |
|-----------|------|-------|
| `.basic <Name>` | `http` + `basic` | |
| `.apikey <Name>` | `apiKey` | needs `@in` + `@name`. |
| `.bearer <Name>` | `http` + `bearer` | `@bearerFormat` defaults to `JWT`. |
| `.oauth2.application <Name>` | `oauth2` | flow → `clientCredentials`; `@tokenUrl`. |
| `.oauth2.implicit <Name>` | `oauth2` | flow → `implicit`; `@authorizationUrl`. |
| `.oauth2.password <Name>` | `oauth2` | flow → `password`; `@tokenUrl`. |
| `.oauth2.accessCode <Name>` | `oauth2` | flow → `authorizationCode`; `@authorizationUrl` + `@tokenUrl`. |
| `.openIdConnect <Name>` | `openIdConnect` | `@openIdConnectUrl`. |

Sub-properties: `@in` (header/query/cookie), `@name`, `@description`, `@tokenUrl`,
`@authorizationUrl`, `@bearerFormat`, `@openIdConnectUrl`, `@scope.<name> <desc>` (oauth2).

> **Fallback:** if any operation references a security scheme but **no** definitions are
> declared, tapetest emits a default `UserAuth` JWT bearer scheme so the spec stays valid.

## General API info (`@title`, `@host`, ...)

Top-level metadata, merged across files (first non-empty value wins). A comment group is
treated as general info only if it contains a "primary" directive (`title`, `version`,
`host`, `basePath`, `contact.*`, `license.*`, ...) — this prevents stealing `@description`
lines that belong to a `@securityDefinitions` block.

```go
// @title        My API
// @version      1.0
// @description  API description
// @host         localhost:8080
// @BasePath     /api/v1
// @schemes      http
```

`GeneralAPIInfo.ServerURL()` combines scheme+host+basePath into one server entry.

## Multipart & form request bodies

`buildRequestBody` groups recordings by normalized content type, so a single endpoint that
accepts both `application/json` and `multipart/form-data` gets **multiple** media-type
entries (swagger-ui shows a selector). Recorded file uploads become
`{ type: string, format: binary }` properties → file-chooser inputs in "Try it out".

## Example ordering & exclusion (`DocOrder`)

Each recorded exchange normally becomes an example. Call [`DocOrder`](../../response.go:181)
on the `*Response` to control where the example lands — or hide it entirely. The argument is
`interface{}` but in practice you pass an `int` literal or `nil`:

```go
c.Get("/todos").DocOrder(0)   // first example
c.Get("/todos").DocOrder(nil) // excluded from the docs
c.Get("/todos").DocOrder(-1)  // last example
```

| Argument | Tier | Result |
|----------|------|--------|
| `0`, `1`, `2`, ... | first (ascending) | `0` → `1` → `2` … ; `0` is the topmost example |
| *(not called)* | middle | natural recording order |
| `-1`, `-2`, ... | last (ascending) | more-negative precedes `-1`, so `-1` is the very last |
| `nil` | — | **excluded** — the exchange is not emitted at all |

Behaviour notes:

- `DocOrder` is recorded on the **most recent exchange** via
  [`SetLastExchangeDocOrder`](../../recorder.go:105). It applies per-recorded-request, so a
  test that hits the same endpoint several times only orders its own example(s).
- Sorting is **stable**: ties (same value, or all "natural") keep their original recording
  order.
- Endpoint-level effect: filtering runs at the endpoint-group level
  ([`orderedRecordings`](../../openapi.go:751)). If **every** recording for an endpoint is
  excluded via `DocOrder(nil)`, the endpoint is omitted from the spec entirely.
- Ordering is applied consistently to **response** examples and to all **request-body**
  media types (JSON, form, multipart), and the spec JSON itself preserves insertion order
  ([`OpenAPIExamples`](../../openapi.go:88) marshals keys in order) so swagger-ui renders
  them in the intended sequence.
- `DocOrder` returns the `*Response` and is chainable with assertions.

## Swagger UI runtime config

`BuildSwaggerConfig(map[string]string)` types string values (from flags/env) into the
swagger-ui config. `OptionToFlag("deepLinking")` → `deep-linking`;
`OptionToEnv("deepLinking")` → `TAPETEST_DEEP_LINKING`. Supported keys are listed in
[`SwaggerUIOptions`](../../swagger.go:21) (booleans, strings, ints).

## Lower-level building blocks

- Need just the spec object? `GenerateOpenAPIFromRecordings(...)` → `*OpenAPIDocument`,
  then `doc.WriteJSON(path)`.
- Need just the static site from an existing spec? `GenerateSwaggerUI(outputDir, specFile, config, css, js, cssURL, jsURL)`.
- Building a spec by hand? `NewOpenAPIDocument(title, version)`, `doc.AddOperation`,
  `doc.AddServer`, `doc.AddServer`.

## Verifying output

After generation, assert the docs landed correctly in your test/CI:

```bash
test -f docs/openapi.json            # spec exists
test -f docs/index.html             # swagger UI exists
grep -q '"openapi":"3.0.3"' docs/openapi.json
```

Or drive the whole thing deterministically with [`run-and-docs.sh`](run-and-docs.sh).
