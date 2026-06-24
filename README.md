# tapetest

A dead-simple Go REST API testing library with a fluent Client-based API.
Write tests in seconds, not minutes.

<p align="center"><img src="./tapetest.svg" alt="tapetest logo" height="150px"></p>

## Features

- **Zero dependencies** — only Go standard library
- **Automatic OpenAPI documentation** — generate Swagger UI from your tests [[demo]](https://meyt.github.io/tapetest)
- **Fluent assertions** — clean, chainable API for status, headers, JSON, and more
- **Multiple client types** — test against handlers directly or running servers
- **Framework agnostic** — works with Echo, Gin, Chi, or any `http.Handler`
- **Rich request options** — query params, headers, cookies, file uploads, timeouts
- **Shared state** — persistent cookies and headers across requests
- **Go-swag annotations** — enrich docs with titles, descriptions, tags, and security

## Installation

```bash
go get github.com/meyt/tapetest
```

## Quick Start

```go
import (
    "testing"
    . "github.com/meyt/tapetest"
)

func TestGetUsers(t *testing.T) {
    c := HandlerClient(t, myHandler)
    c.Get("/users").Status(200)
}
```

## Client Creation

```go
c := HandlerClient(t, handler)            // Test against http.Handler (fast)
c := HttpClient(t, "http://localhost:8080") // Test against running server
c := EchoClient(t, echoInstance)          // Echo v4/v5
c := GinClient(t, ginEngine)              // Gin
```

## Base URL

Set a common prefix prepended to every request path — handy when all your
routes live under an API version like `/api/v1`.

```go
c := HandlerClient(t, myHandler)
c.BaseUrl("/api/v1")
c.Get("/users").Status(200)        // -> /api/v1/users
```

## HTTP Methods

```go
c.Get("/todos")
c.Post("/todos", Json{"title": "Buy milk"})
c.Put("/todos/1", Json{"title": "Updated"})
c.Patch("/todos/1", Json{"done": true})
c.Delete("/todos/1")
c.Head("/todos")
c.Request("OPTIONS", "/todos")
```

## Request Options

```go
c.Get("/users", Query("page", "1"))
c.Post("/user", body, Header("Authorization", "Bearer token"))
c.Get("/me", Bearer("my-token"))
c.Get("/profile", Cookie("session_id", "abc123"))
c.Post("/upload", Form{"firstName": "John"}, File("avatar", "./photo.png"))
c.Get("/slow", Timeout(5*time.Second))
```

## Assertions

```go
r := c.Get("/users")

r.Status(200)                    // Exact match or patterns: "2xx", "4xx"
r.Reason("OK")                   // Status reason text
r.Header("Content-Type", "application/json")
r.Cookie("session_id")           // Assert cookie exists
r.Cookie("session_id", "abc123") // Assert cookie value
r.Json("user.name", "John")      // Dot notation
r.Json("user.age", ">", 20)      // with operators: ">", "<=", etc.
r.Json("user.created_at", "<=", time.Now())
r.Expect("app is working")       // Assert raw body text
r.Regex("is working$")           // Assert body matches a regex
r.Error()                        // Assert request resulted in error
```

### Supported Assert Operators

The following operators are shared by `Expect`, `Json`, `Cookie`, and `Header`
assertions, so the same syntax works everywhere:

| Operator | Description | Example | Types Supported |
|----------|-------------|---------|-----------------|
| `>` | Greater than | `r.Json("price", ">", 100)` | numeric, time |
| `>=` | Greater than or equal | `r.Json("count", ">=", 5)` | numeric, time |
| `<` | Less than | `r.Json("age", "<", 18)` | numeric, time |
| `<=` | Less than or equal | `r.Json("discount", "<=", 0.5)` | numeric, time |
| `==` | Equal (explicit) | `r.Json("status", "==", "active")` | all types |
| `=` | Equal (shorthand) | `r.Json("status", "=", "active")` | all types |
| `!=` | Not equal | `r.Json("error", "!=", null)` | all types |
| `~` | Between (inclusive) | `r.Json("age", "~", 18, 30)` | numeric |
| `^` | Contains all of | `r.Expect("^", "ielts", "icdl")` | text |
| `!^` | Contains none of | `r.Expect("!^", "mba", "foss")` | text |
| `*` | Contains any of | `r.Expect("*", "ielts", "mba")` | text |

#### Body Assertions

`Expect` asserts the raw response body as text. The body is trimmed of
surrounding whitespace before comparison. It supports exact matches plus the
full set of operators above.

```go
r.Expect("app is working")                                      // exact string match
r.Expect(13)                                                     // numeric match
r.Expect(131.50)                                                 // numeric (float) match
r.Expect(">", 131.50)                                            // numeric with operator
r.Expect(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))            // date match (RFC3339)
r.Expect(">", time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC))       // date with operator
r.Expect("^", "ielts", "icdl")                                   // must contain these words
r.Expect("!^", "mba", "foss")                                    // must NOT contain these words
r.Expect("~", 18, 25)                                            // numeric between two values
r.Expect("*", "ielts", "icdl", "mba")                            // contains any of these
```

`Regex` asserts the body matches a regular expression:

```go
r.Regex("is working$")
r.Regex(`^\d{3}-\d{4}$`)
```

#### Status Code Patterns

Status assertions support pattern matching:

```go
r.Status(200)      // Exact match
r.Status("2xx")    // Any 2xx status (200, 201, 204, etc.)
r.Status("4xx")    // Any 4xx client error
r.Status("5xx")    // Any 5xx server error
```

#### Cookie Comparisons

Cookie assertions support all the operators listed above, applied to the cookie
value:

```go
r.Cookie("session_id")              // Check cookie exists
r.Cookie("session_id", "abc123")    // Check cookie value
r.Cookie("count", ">", 1)           // Numeric greater than
r.Cookie("count", "<=", 10)         // Numeric less or equal
r.Cookie("count", "==", 5)          // Numeric equal
r.Cookie("count", "~", 1, 10)       // Numeric between 1 and 10
r.Cookie("flags", "^", "vip", "pro") // Value contains all substrings
```

#### JSON Path Navigation

JSON assertions support dot-notation paths with nested objects and array indices:

```go
r.Json("user.name")              // Simple path
r.Json("user.address.city")      // Nested objects
r.Json("items.0.name")           // Array index
r.Json("users.1.email")          // Nested array element
r.Json("data.items.2.price")     // Complex paths
```

## Response Data

```go
r := c.Get("/users")
body := r.Response.Text()        // Raw text
data := r.Response.Json()        // Parsed JSON
headers := r.Response.headers        // Parsed Headers
cookies := r.Response.cookies        // Parsed Cookies 
r.Response.Write("/tmp/file")    // Save to file
```

## Shared Headers and Cookies

```go
c := HandlerClient(t, handler) 

// Shared cookies across all requests
c.Cookie("session_id", "abc123")
c.Get("/profile")
c.Cookie("user_pref", nil)       // Remove shared cookie

// Shared headers across all requests
c.Header("Authorization", "Bearer token123")
c.Get("/profile")
c.Header("Authorization", nil)   // Remove shared header
```

## Automatic Documentation

Generate OpenAPI v3 spec and Swagger UI from your tests with zero hand-written spec files:

```go
func TestMain(m *testing.M) {
    EnableRecording(".tapetest")
    code := m.Run()
    FlushRecording()

    GenerateDocs(GenerateDocsOptions{
        RecordingDir: ".tapetest",
        OutputDir:    "docs",
        Title:        "My API",
        Version:      "1.0.0",
        SourceDir:    ".",
        Config: BuildSwaggerConfig(map[string]string{
            "docExpansion":    "false",
            "tryItOutEnabled": "false",
        }),
    })
    os.Exit(code)
}
```

### Annotations

Add go-swag style comments to your handlers:

```go
// @Title Create todo
// @Description Create a new todo item
// @Tag todos
// @Security UserAuth
// @Method POST
// @Path /todos
func (a *App) createTodo(c echo.Context) error { ... }
```

Multiple `@Security` annotations indicate that any of the specified
authentication methods can access the endpoint.

### Security Definitions

Define security schemes using go-swag style securityDefinitions annotations.
These are typically placed in your main file or a dedicated configuration file:

#### Basic Authentication

```go
// @securityDefinitions.basic BasicAuth
// @description Basic HTTP authentication
```

#### API Key Authentication

```go
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization
// @description API key in header
```

API keys can be placed in headers, query parameters, or cookies:

```go
// @securityDefinitions.apikey QueryApiKey
// @in query
// @name api_key
```

#### Bearer Authentication (JWT)

```go
// @securityDefinitions.bearer BearerAuth
// @description JWT token authentication
// @bearerFormat JWT
```

#### OAuth2 Flows

**Application Flow (Client Credentials):**

```go
// @securityDefinitions.oauth2.application OAuth2App
// @tokenUrl https://example.com/oauth/token
// @scope.read Read access
// @scope.write Write access
```

**Implicit Flow:**

```go
// @securityDefinitions.oauth2.implicit OAuth2Implicit
// @authorizationUrl https://example.com/oauth/authorize
// @scope.profile User profile
// @scope.email User email
```

**Password Flow:**

```go
// @securityDefinitions.oauth2.password OAuth2Password
// @tokenUrl https://example.com/oauth/token
// @scope.all Full access
```

**Authorization Code Flow:**

```go
// @securityDefinitions.oauth2.accessCode OAuth2AccessCode
// @authorizationUrl https://example.com/oauth/authorize
// @tokenUrl https://example.com/oauth/token
// @scope.read Read access
// @scope.write Write access
```

#### OpenID Connect

```go
// @securityDefinitions.openIdConnect OpenID
// @openIdConnectUrl https://example.com/.well-known/openid-configuration
// @description OpenID Connect authentication
```

#### Security Definition Properties

| Property | Description | Applies To |
|----------|-------------|------------|
| `@in` | Location: header, query, or cookie | apiKey |
| `@name` | Header/parameter/cookie name | apiKey |
| `@description` | Human-readable description | All types |
| `@bearerFormat` | Token format (e.g., "JWT") | bearer |
| `@tokenUrl` | OAuth2 token endpoint URL | oauth2 |
| `@authorizationUrl` | OAuth2 authorization endpoint URL | oauth2 (implicit, accessCode) |
| `@openIdConnectUrl` | OpenID Connect discovery URL | openIdConnect |
| `@scope.<name>` | OAuth2 scope name and description | oauth2 |

For complete details on OpenAPI v3 security schemes, see the [official OpenAPI v3 specification](https://swagger.io/specification/#security-scheme-object).

### API Metadata

Define top-level API info in your main file:

```go
// @title        My API
// @version      1.0
// @description  API description
// @host         localhost:8080
// @BasePath     /api/v1
// @schemes      http
```

See [`./demoapp`](demoapp) for a complete working example (`make dev` to run).
