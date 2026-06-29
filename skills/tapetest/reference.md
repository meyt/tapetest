# tapetest — API Reference

Complete index of exported symbols. Source links point into the library root.

## Constructors (`tapetest.go`)

| Function | Signature | Notes |
|----------|-----------|-------|
| [`HttpClient`](../../tapetest.go:34) | `func HttpClient(t *testing.T, baseURL string) *Client` | Real TCP requests via `http.Client`. |
| [`HandlerClient`](../../tapetest.go:50) | `func HandlerClient(t *testing.T, handler http.Handler) *Client` | Dispatches through `httptest`; catches panics. Fastest. |
| [`EchoClient`](../../tapetest.go:77) | `func EchoClient(t *testing.T, handler http.Handler) *Client` | Alias of `HandlerClient` (Echo v4/v5). |
| [`Echo4Client`](../../tapetest.go:83) | `func Echo4Client(t *testing.T, handler http.Handler) *Client` | Echo v4 alias. |
| [`Echo5Client`](../../tapetest.go:89) | `func Echo5Client(t *testing.T, handler http.Handler) *Client` | Echo v5 alias. |
| [`GinClient`](../../tapetest.go:95) | `func GinClient(t *testing.T, handler http.Handler) *Client` | Gin alias. |

## `Client` methods

### Configuration / shared state

| Method | Signature | Notes |
|--------|-----------|-------|
| [`BaseUrl`](../../tapetest.go:77) | `func (c *Client) BaseUrl(prefix string) *Client` | Prefix prepended once to every path. Chainable. |
| [`Server`](../../tapetest.go:94) | `func (c *Client) Server(name, url string) *Client` | Tags recordings with a service name + relative URL → per-operation `servers` in the OpenAPI doc. Chainable. |
| [`Header`](../../tapetest.go:538) | `func (c *Client) Header(key string, value interface{})` | Shared header; `nil` removes it. |
| [`Cookie`](../../tapetest.go:551) | `func (c *Client) Cookie(key string, value interface{})` | Shared cookie; `nil` removes it. |

### HTTP verbs

All return `*Response`. `body` is marshaled to JSON unless it is a `Form` (or `nil`).

| Method | Signature |
|--------|-----------|
| [`Get`](../../tapetest.go:141) | `func (c *Client) Get(path string, opts ...Option) *Response` |
| [`Post`](../../tapetest.go:152) | `func (c *Client) Post(path string, opts ...Option) *Response` |
| [`Put`](../../tapetest.go:159) | `func (c *Client) Put(path string, opts ...Option) *Response` |
| [`Patch`](../../tapetest.go:166) | `func (c *Client) Patch(path string, opts ...Option) *Response` |
| [`Delete`](../../tapetest.go:171) | `func (c *Client) Delete(path string, opts ...Option) *Response` |
| [`Head`](../../tapetest.go:176) | `func (c *Client) Head(path string, opts ...Option) *Response` |
| [`Request`](../../tapetest.go:181) | `func (c *Client) Request(method, path string, opts ...Option) *Response` |

Every method takes only `path` (and `method` for `Request`) plus variadic
options. The request body is set by passing a `Json` or `Form` value as an
option — there is no dedicated body argument, so options may be supplied in any
order:

	c.Post("/item/:id", Param{"id": "1"}, Json{"name": "widget"})
	c.Put("/todos/1", Json{"title": "Updated"}, Bearer("token"))
	c.Post("/upload", Form{"name": "John"}, File("avatar", "./photo.png"))

## Body types & options (`options.go`)

| Symbol | Kind | Notes |
|--------|------|-------|
| [`Json`](../../options.go:62) | `type Json map[string]interface{}` | Body → `application/json`. Implements `Option`, so pass it as an option to set the body. |
| [`Form`](../../options.go:67) | `type Form map[string]interface{}` | Body → `application/x-www-form-urlencoded`; with `File` → multipart. Implements `Option`. |
| [`Param`](../../options.go:75) | `type Param map[string]interface{}` | Substitutes `:key`/`{key}` in path. Values may be named-typed strings (enums). Implements `Option`. |
| [`Query`](../../options.go:122) | `type Query map[string]interface{}` | Query parameters. Values may be named-typed strings (enums). Implements `Option`. |
| [`Header`](../../options.go:135) | `type Header map[string]interface{}` | Per-request headers. Values are stringified. Implements `Option`. |
| [`File`](../../options.go:152) | `func File(field, path string, contentType ...string) Option` | Upload; switches request to multipart. Optional 3rd arg sets the part's `Content-Type` (default `application/octet-stream`) for MIME-validating servers. |
| [`Timeout`](../../options.go:165) | `func Timeout(d time.Duration) Option` | `HttpClient` only. |
| [`Bearer`](../../options.go:175) | `func Bearer(token string) Option` | Sets `Authorization: Bearer <token>`. |
| [`Cookie`](../../options.go:183) | `func Cookie(key, value string) Option` | Per-request cookie. |
| [`Option`](../../options.go:35) | `interface { apply(*requestConfig) }` | Implement to build custom options. |

## Enum registration (`enum_register.go`)

| Function | Signature | Notes |
|----------|-----------|-------|
| [`RegisterEnum`](../../enum_register.go:40) | `func RegisterEnum[T ~string](values ...T)` | Register allowed values for a named string type. Infers the type via reflection. Calling twice for the same type replaces the previous entry. Safe for concurrent use. |
| [`Enum`](../../enum_register.go:70) | `func Enum[T ~string](values ...T)` | Alias for `RegisterEnum`. |
| [`LookupEnum`](../../enum_register.go:76) | `func LookupEnum(t reflect.Type) []string` | Returns the allowed enum values for the given `reflect.Type`, or nil if not registered. |
| [`GetEnumCache`](../../enum_register.go:93) | `func GetEnumCache() map[string][]string` | Returns a copy of the current enum cache. Useful for debugging when enums aren't being detected. |

## `Response` assertions (`response.go`)

All return `*Response` (chainable). Use `t.Helper()` internally so failures point to your line.

| Method | Signature | Asserts |
|--------|-----------|---------|
| [`Status`](../../response.go:44) | `func (r *Response) Status(code interface{}) *Response` | `int` exact, or `"2xx"`/`"4xx"`/... pattern. |
| [`Reason`](../../response.go:66) | `func (r *Response) Reason(text string) *Response` | Exact status reason text. |
| [`ReasonContains`](../../response.go:77) | `func (r *Response) ReasonContains(text string) *Response` | Reason contains substring. |
| [`Header`](../../response.go:96) | `func (r *Response) Header(key string, value ...interface{}) *Response` | Exists, or value/operator. |
| [`Json`](../../response.go:126) | `func (r *Response) Json(path string, args ...interface{}) *Response` | JSON path exists, or value/operator. |
| [`Cookie`](../../response.go:403) | `func (r *Response) Cookie(key string, value ...interface{}) *Response` | Exists, or value/operator. |
| [`Expect`](../../response.go:202) | `func (r *Response) Expect(args ...interface{}) *Response` | Raw trimmed body: exact or operator. |
| [`Regex`](../../response.go:215) | `func (r *Response) Regex(pattern string) *Response` | Body matches a regexp. |
| [`Error`](../../response.go:178) | `func (r *Response) Error() *Response` | Request errored (network/timeout/panic). |
| [`Stream`](../../response.go:240) | `func (r *Response) Stream(fn func(io.Reader) error) *Response` | Process body via callback. |

### Documentation control

| Method | Signature | Notes |
|--------|-----------|-------|
| [`DocOrder`](../../response.go:181) | `func (r *Response) DocOrder(order interface{}) *Response` | Orders/hides this example in generated docs. `int` (0/+=first, -=last), `nil`=excluded. Chainable. |

## `Response` value accessors

Terminal (return the value, not `*Response`). Use these to drive multi-step tests.

| Method | Signature | Returns |
|--------|-----------|---------|
| [`JsonVal`](../../response.go:258) | `func (r *Response) JsonVal(path ...string) interface{}` | Full parsed JSON or value at path; `nil` if missing/not JSON. |
| [`StatusVal`](../../response.go:276) | `func (r *Response) StatusVal() int` | HTTP status code. |
| [`ReasonVal`](../../response.go:283) | `func (r *Response) ReasonVal() string` | Status reason text. |
| [`CookieVal`](../../response.go:293) | `func (r *Response) CookieVal(key ...string) interface{}` | `[]*http.Cookie` or single value string. |
| [`HeaderVal`](../../response.go:311) | `func (r *Response) HeaderVal(key ...string) interface{}` | `http.Header` or single value string. |
| [`TextVal`](../../response.go:321) | `func (r *Response) TextVal() string` | Raw body text. |

## `ResponseData` (`r.Response`)

| Method | Signature | Notes |
|--------|-----------|-------|
| [`Json`](../../response.go:330) | `func (d *ResponseData) Json() interface{}` | Parsed JSON. |
| [`Text`](../../response.go:339) | `func (d *ResponseData) Text() string` | Body as string. |
| [`Write`](../../response.go:349) | `func (d *ResponseData) Write(path string) string` | Save body to file; dir → auto filename from `Content-Disposition`. Returns saved path. |
| [`Cookies`](../../response.go:432) | `func (d *ResponseData) Cookies() []*http.Cookie` | All response cookies. |

## Recording (`recorder.go`)

Global singleton, mutex-guarded.

| Function | Signature | Notes |
|----------|-----------|-------|
| [`EnableRecording`](../../recorder.go:57) | `func EnableRecording(dir string)` | Turns on capture; resets buffer. |
| [`DisableRecording`](../../recorder.go:66) | `func DisableRecording()` | Turns off capture. |
| [`IsRecordingEnabled`](../../recorder.go:73) | `func IsRecordingEnabled() bool` | Current state. |
| [`FlushRecording`](../../recorder.go:102) | `func FlushRecording() error` | Writes `<dir>/recordings.json`. Call after `m.Run()`. |
| [`LoadRecordings`](../../recorder.go:131) | `func LoadRecordings(dir string) ([]RecordedExchange, error)` | Read recordings back. |
| [`GetRecordings`](../../recorder.go:147) | `func GetRecordings() []RecordedExchange` | In-memory buffer (no flush). |
| [`ClearRecordings`](../../recorder.go:154) | `func ClearRecordings() error` | Wipe buffer + delete file. |
| [`SetLastExchangeDocOrder`](../../recorder.go:105) | `func SetLastExchangeDocOrder(order *int)` | Sets example ordering on the last recorded exchange. `nil`=exclude. Called by `DocOrder`. |

Recorded shapes: [`RecordedRequest`](../../recorder.go:13), [`RecordedResponse`](../../recorder.go:25),
[`RecordedExchange`](../../recorder.go:32).

## Documentation generation (`swagger.go` / `openapi.go`)

| Function / Type | Notes |
|-----------------|-------|
| [`GenerateDocs`](../../swagger.go:371) | `func GenerateDocs(opts GenerateDocsOptions) error` — end-to-end: load recordings + annotations → `openapi.json` + Swagger UI. |
| [`GenerateDocsOptions`](../../swagger.go:327) | All knobs: dirs, title, version, `SourceDir`, `ServerURL`, `ReadableExamples`, swagger assets, `Config`. |
| [`BuildSwaggerConfig`](../../swagger.go:98) | `func BuildSwaggerConfig(values map[string]string) SwaggerUIConfig` — string map → typed config. |
| [`DefaultSwaggerUIConfig`](../../swagger.go:89) | Sensible defaults from `SwaggerUIOptions`. |
| [`SwaggerUIOptions`](../../swagger.go:21) | Full list of supported swagger-ui runtime keys. |
| [`GenerateSwaggerUI`](../../swagger.go:159) | Lower-level: build just the static site from an existing spec. |
| [`GenerateOpenAPIFromRecordings`](../../openapi.go:223) | Lower-level: build just the `OpenAPIDocument`. |
| [`NewOpenAPIDocument`](../../openapi.go:124) | Manual document construction. |
| [`OpenAPIDocument`](../../openapi.go:14) | Root v3 doc type with `AddOperation`, `AddServer`, `WriteJSON`, `ApplyGeneralAPIInfo`. |

Constants: `DefaultSwaggerUICSS`, `DefaultSwaggerUIJS` (unpkg CDN URLs).

## Annotation parsers (`parser.go` / `apimeta.go`)

| Function | Returns |
|----------|---------|
| [`ParseAnnotationsFromDir`](../../parser.go:150) | `[]HandlerAnnotation` — handler `@Summary`/`@Router`/... blocks. |
| [`ParseSecurityDefinitionsFromDir`](../../parser.go:254) | `[]SecurityDefinition`. |
| [`ParseSecurityDefinitionsFromFiles`](../../parser.go:282) | Same, from explicit files. |
| [`ParseGeneralAPIInfoFromDir`](../../apimeta.go:95) | `GeneralAPIInfo` — top-level `@title`/`@host`/`@BasePath`/... |
| [`ParseGeneralAPIInfoFromFiles`](../../apimeta.go:121) | Same, from explicit files. |

Parsed types: [`HandlerAnnotation`](../../parser.go:24), [`SecurityDefinition`](../../parser.go:103)
(with [`ToOpenAPISecurityScheme`](../../openapi.go:338)), [`GeneralAPIInfo`](../../apimeta.go:34)
(with `ServerURL()`, `HasServerInfo()`, `HasAny()`).
