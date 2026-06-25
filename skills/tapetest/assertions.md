# tapetest — Assertion engine

tapetest shares **one** operator engine across `Expect`, `Json`, `Cookie`, and `Header`.
Learn the operators once; they work everywhere. Implementation: [`assert.go`](../../assert.go).

## Operator matrix

| Operator | Meaning | Arity | Example |
|----------|---------|-------|---------|
| `>` | Greater than | 1 (numeric/time) | `r.Json("price", ">", 100)` |
| `>=` | Greater than or equal | 1 | `r.Json("count", ">=", 5)` |
| `<` | Less than | 1 | `r.Json("age", "<", 18)` |
| `<=` | Less than or equal | 1 | `r.Json("discount", "<=", 0.5)` |
| `==` | Equal (explicit) | 1 | `r.Json("status", "==", "active")` |
| `=` | Equal (shorthand) | 1 | `r.Json("status", "=", "active")` |
| `!=` | Not equal | 1 | `r.Json("error", "!=", "")` |
| `~` | Between (inclusive) | 2 (numeric) | `r.Json("age", "~", 18, 30)` |
| `^` | Contains **all** substrings | n (text) | `r.Json("username", "^", "op", "user")` |
| `!^` | Contains **none** of | n (text) | `r.Json("licenses", "!^", "foss", "pmp")` |
| `*` | Contains **any** of | n (text) | `r.Json("username", "*", "ZZZ", "op")` |

When the first argument is **not** a recognized operator token, it is treated as an
equality check against the expected value.

## Value coercion rules

`evalAssertion` (the shared engine) coerces the *actual* value (always a string internally)
against the *expected* Go value in this priority order:

1. **`time.Time` expected** → actual is parsed as RFC3339 (`time.Parse`) and compared with
   `.Equal()` / `.After()` / `.Before()`. Use this for timestamp fields:
   ```go
   r.Json("created_at", ">=", time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
   ```
2. **Numeric expected** → both sides parsed to `float64` via `strconv.ParseFloat`. Any Go
   numeric type works as the expected operand (`int`, `int64`, `float64`, ...). Note the
   actual string `"131.50"` and the expected `float64(131.5)` compare **equal**.
3. **String expected** → plain string equality (`actual == fmt.Sprintf("%v", expected)`).

### Body assertions (`Expect`)

`Expect` operates on the **raw response body**, trimmed of surrounding whitespace. It
accepts any type: string, int, float, or `time.Time`.

```go
r.Expect("app is working")                                  // exact string
r.Expect(13)                                                // numeric
r.Expect(131.50)                                            // float
r.Expect(">", 131)                                          // numeric operator
r.Expect(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))       // RFC3339 date
r.Expect("^", "ielts", "icdl")                              // contains all words
r.Expect("~", 18, 25)                                       // numeric between
```

### Regex (`Regex`)

Independent of the operator engine — uses Go `regexp`. Compiles per call; an invalid
pattern fails the test via `t.Fatalf`.

```go
r.Regex(`^\d+\.\d{2}$`)   // e.g. "131.50"
r.Regex("is working$")
```

## JSON path navigation (`Json`)

`resolveJSONPath` ([`jsonpath.go`](../../jsonpath.go:10)) splits the path on `.` and walks
`map[string]interface{}` (objects) and `[]interface{}` (arrays). Array access uses a
numeric segment:

```go
r.Json("user.name")            // nested object
r.Json("items.0.name")         // first array element
r.Json("data.items.2.price")   // deep path
```

Rules & gotchas:

- JSON unmarshals into `map[string]interface{}` / `[]interface{}` / `float64` / `string` /
  `bool` / `nil` — there is no schema. A path that doesn't resolve fails the test with
  `t.Errorf` (not `Fatal`), so chained assertions after it still execute.
- `r.Json("user")` with **no expected value** asserts the key **exists** (non-fatal if absent).
- `r.Json("user.age")` with one expected value performs equality/coercion as above.
- Boolean JSON values render to `"true"`/`"false"` strings before comparison, so
  `r.Json("done", true)` works because `fmt.Sprintf("%v", true) == "true"`.
- `nil` JSON values render to `""`; assert absence with `r.Json("avatar", "")`.

### Array length assertions (`JsonCount`)

`JsonCount` ([`response.go`](../../response.go:184)) validates the element count of a
JSON array resolved at a dot-notation path. It shares the same operator engine as
`Json`, applied to the array's length:

```go
r.JsonCount("items")             // array exists and has at least one item
r.JsonCount("items", 2)          // array has exactly 2 items
r.JsonCount("items", ">", 2)     // more than 2 items
r.JsonCount("items", "<=", 5)    // at most 5 items
r.JsonCount("items", "~", 2, 5)  // length is between 2 and 5 (inclusive)
```

- With **no count argument**, it asserts the array is non-empty.
- With a single **numeric** argument, it asserts exact length.
- With an **operator** first argument, the operator is applied to the length.
- A path that doesn't resolve, or resolves to a non-array, fails the test
  (non-fatal `t.Errorf`).

### Status patterns (`Status`)

`Status` accepts an `int` (exact) **or** a 3-char string pattern (`"2xx"`, `"4xx"`, ...),
implemented in [`status.go`](../../status.go:9). The class digit must be `1`–`5` and the
suffix must be `xx`/`XX`. A numeric-looking string like `"200"` is also parsed as exact.

```go
r.Status(200)
r.Status("2xx")   // any success
r.Status("4xx")   // any client error
```

## Common mistakes

- **`Json` numbers are `float64`.** When you need the raw value (not an assertion), use
  `JsonVal("id")` and cast: `int(v.(float64))`. Never assume `int`.
- **Operator arity.** `~` requires exactly 2 numeric bounds; `>`/`<`/`==`/`!=`/`=`/`>=`/`<=`
  require exactly 1. Passing the wrong count produces a descriptive failure message, not a panic.
- **Substring vs. equality.** `^`, `!^`, `*` do **substring** matching (`strings.Contains`),
  not token/word matching. `"user"` matches inside `"superuser"`.
- **`Cookie`/`Header` operators** use the same engine, applied to the cookie/header **value**.
  `r.Header("Content-Type")` with no value asserts existence only.
