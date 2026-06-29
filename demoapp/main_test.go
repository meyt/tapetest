package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	. "github.com/meyt/tapetest"
)

// TestMain enables recording for all tests, flushes to .tapetest/ after
// completion, and then regenerates the OpenAPI docs from the freshly recorded
// exchanges + the go-swag annotations in the demo app source.
func TestMain(m *testing.M) {
	EnableRecording(".tapetest")
	code := m.Run()
	if err := FlushRecording(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to flush recordings: %v\n", err)
	}

	// Generate the docs after all tests have run and recordings are flushed.
	if err := GenerateDocs(GenerateDocsOptions{
		RecordingDir:     ".tapetest",
		OutputDir:        "docs",
		Title:            "Demo API",
		Version:          "1.0.0",
		SourceDir:        ".",
		ReadableExamples: true,
		Config: BuildSwaggerConfig(map[string]string{
			"docExpansion":    "false",
			"tryItOutEnabled": "false",
		}),
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to generate docs: %v\n", err)
	}

	os.Exit(code)
}

// setup creates a fresh Client for each test. All demo app routes are served
// under the /api/v1 prefix, so the client's BaseUrl is set once here and every
// subsequent request path is resolved relative to it.
//
// Server tags every recording with its service name and relative URL, so the
// generated OpenAPI document emits a per-operation servers entry. Swagger UI's
// "Try it out" then routes each request to /api/v1 regardless of where the
// docs are hosted. See the "Multi-service suites" section of the README.
func setup(t *testing.T) *Client {
	return EchoClient(t, NewApp().Echo).BaseUrl("/api/v1").Server("Demo API", "/api/v1")
}

// ============================================================
// Health
// ============================================================

// TestHealth checks the health endpoint status, body, status-pattern
// matching and raw response data access (Text/Json).
func TestHealth(t *testing.T) {
	c := setup(t)
	r := c.Get("/health")
	r.Status(200).
		Json("status", "ok")

	// Status code can also be asserted with a pattern.
	c.Get("/health").Status("2xx")

	// Raw response data access.
	body := r.Response.Text()
	fmt.Printf("Response body: %s\n", body)

	data := r.Response.Json()
	if data == nil {
		t.Error("expected non-nil JSON data")
	}
}

// ============================================================
// Todos - List
// ============================================================

// TestListTodos verifies that listing todos returns an empty list by default
// and that custom request headers and bearer tokens are accepted.
func TestListTodos(t *testing.T) {
	c := setup(t)
	r := c.Get("/todos",
		Header{"X-Custom": "test-value"},
		Bearer("my-token"),
	)
	r.Status(200)
}

// TestListTodosHead verifies the HEAD variant of the list endpoint.
func TestListTodosHead(t *testing.T) {
	c := setup(t)
	r := c.Head("/todos")
	r.Status(200)
}

// ============================================================
// Todos - Create
// ============================================================

// TestCreateTodo creates a todo and asserts the returned fields, the status
// reason text and a status-pattern match.
func TestCreateTodo(t *testing.T) {
	c := setup(t)
	r := c.Post("/todos", Json{
		"title": "Buy milk",
	}).DocOrder(0) // show as the first example in the docs
	r.Status(201).
		ReasonContains("Created").
		Json("title", "Buy milk").
		Json("done", false).
		Json("id")
}

func TestCreateTodoWithDone(t *testing.T) {
	c := setup(t)
	r := c.Post("/todos", Json{
		"title": "Already done",
		"done":  true,
	}).DocOrder(-1) // show as the last example in the docs
	r.Status(201).
		Json("title", "Already done").
		Json("done", true)
}

func TestCreateTodoValidation(t *testing.T) {
	c := setup(t)
	r := c.Post("/todos", Json{
		"title": "",
	}).DocOrder(nil) // exclude this validation-error example from the docs
	r.Status(400).
		Json("error", "title is required")
}

// ============================================================
// Todos - Get
// ============================================================

// TestGetTodo fetches a single todo and asserts its fields plus value-range
// comparisons on the id.
func TestGetTodo(t *testing.T) {
	c := setup(t)
	c.Post("/todos", Json{"title": "Test todo"}).Status(201)

	r := c.Get("/todos/1")
	r.Status(200).
		Json("id", 1).
		Json("id", ">", 0).
		Json("id", "<=", 100).
		Json("title", "Test todo").
		Json("done", false)
}

func TestGetTodoNotFound(t *testing.T) {
	c := setup(t)
	r := c.Get("/todos/999")
	r.Status(404).
		Json("error", "todo not found")
}

// ============================================================
// Todos - Update (PUT)
// ============================================================

func TestUpdateTodo(t *testing.T) {
	c := setup(t)
	c.Post("/todos", Json{"title": "Original"}).Status(201)

	r := c.Put("/todos/1", Json{
		"title": "Updated",
		"done":  true,
	})
	r.Status(200).
		Json("id", 1).
		Json("title", "Updated").
		Json("done", true)
}

func TestUpdateTodoNotFound(t *testing.T) {
	c := setup(t)
	r := c.Put("/todos/999", Json{"title": "Nope"})
	r.Status(404)
}

// ============================================================
// Todos - Patch
// ============================================================

func TestPatchTodo(t *testing.T) {
	c := setup(t)
	c.Post("/todos", Json{"title": "Patch me"}).Status(201)

	r := c.Patch("/todos/1", Json{"done": true})
	r.Status(200).
		Json("title", "Patch me").
		Json("done", true)
}

// ============================================================
// Todos - Delete
// ============================================================

func TestDeleteTodo(t *testing.T) {
	c := setup(t)
	c.Post("/todos", Json{"title": "To delete"}).Status(201)

	c.Delete("/todos/1").Status(204)
	c.Get("/todos/1").Status(404)
}

func TestDeleteTodoNotFound(t *testing.T) {
	c := setup(t)
	c.Delete("/todos/999").Status(404)
}

// ============================================================
// Todos - Search (Query params)
// ============================================================

func TestSearchTodos(t *testing.T) {
	c := setup(t)
	c.Post("/todos", Json{"title": "Buy groceries"}).Status(201)
	c.Post("/todos", Json{"title": "Clean house"}).Status(201)
	c.Post("/todos", Json{"title": "Buy presents"}).Status(201)

	r := c.Get("/todos/search", Query{"q": "buy"})
	r.Status(200).
		Json("query", "buy").
		Json("count", ">", 0)

	// Second request with two query params - the 'status' param should also
	// appear in swagger-ui
	c.Post("/todos", Json{"title": "Buy books", "done": true}).Status(201)

	type StatusType string

	const (
		Pending  StatusType = "pending-status"
		Active   StatusType = "active-status"
		Inactive StatusType = "inactive-status"
	)
	Enum(Pending, Active, Inactive)
	r2 := c.Get("/todos/search", Query{"q": "buy", "status": Active})
	r2.Status(200).
		Json("query", "buy").
		Json("count", ">", 0)

	c.Post("/todos/search", Query{"must-not-documented-in-the-get-method": "some value"}).Status(400)
}

func TestSearchTodosNoQuery(t *testing.T) {
	c := setup(t)
	r := c.Get("/todos/search")
	r.Status(400)
}

// ============================================================
// Todos - Path Params
// ============================================================

func TestGetTodoByColonParam(t *testing.T) {
	c := setup(t)
	c.Post("/todos", Json{"title": "Param test"}).Status(201)

	r := c.Get("/todos/:id", Param{"id": 1})
	r.Status(200).
		Json("title", "Param test")
}

func TestGetTodoByBraceParam(t *testing.T) {
	c := setup(t)
	c.Post("/todos", Json{"title": "Brace test"}).Status(201)

	r := c.Get("/todos/{id}", Param{"id": 1})
	r.Status(200).
		Json("title", "Brace test")
}

// ============================================================
// Todos - Full CRUD Workflow
// ============================================================

func TestTodoCRUDWorkflow(t *testing.T) {
	c := setup(t)

	// 1. Create
	r := c.Post("/todos", Json{"title": "Workflow item"})
	r.Status(201).Json("title", "Workflow item")

	// 2. Read
	r = c.Get("/todos/1")
	r.Status(200).Json("title", "Workflow item").Json("done", false)

	// 3. Update
	r = c.Put("/todos/1", Json{"title": "Updated item", "done": true})
	r.Status(200).Json("title", "Updated item").Json("done", true)

	// 4. List
	c.Get("/todos").Status(200)

	// 5. Delete
	c.Delete("/todos/1").Status(204)

	// 6. Verify deleted
	c.Get("/todos/1").Status(404)
}

// ============================================================
// Users - Create
// ============================================================

func TestCreateUser(t *testing.T) {
	c := setup(t)
	r := c.Post("/users", Json{
		"username": "johndoe",
		"email":    "john@example.com",
		"role":     "user",
	}).DocOrder(0) // show as the first example in the docs
	r.Status(201).
		Json("username", "johndoe").
		Json("email", "john@example.com").
		Json("role", "user").
		Json("active", true)
}

func TestCreateUserNoUsername(t *testing.T) {
	c := setup(t)
	r := c.Post("/users", Json{
		"email": "john@example.com",
	}).DocOrder(nil) // exclude this validation-error example from the docs
	r.Status(400).
		Json("error", "username is required")
}

func TestCreateUserNoEmail(t *testing.T) {
	c := setup(t)
	r := c.Post("/users", Json{
		"username": "johndoe",
	})
	r.Status(400).
		Json("error", "email is required")
}

func TestCreateUserDuplicate(t *testing.T) {
	c := setup(t)
	c.Post("/users", Json{
		"username": "johndoe",
		"email":    "john@example.com",
	}).Status(201)

	r := c.Post("/users", Json{
		"username": "johndoe",
		"email":    "john2@example.com",
	})
	r.Status(409)
}

// ============================================================
// Users - Get
// ============================================================

// TestGetUser fetches a user and asserts fields, plus Json operators for
// contains-all (^), between (~) and any-of (*) matching.
func TestGetUser(t *testing.T) {
	c := setup(t)
	c.Post("/users", Json{
		"username": "opuser",
		"email":    "op@example.com",
	}).Status(201)

	r := c.Get("/users/1")
	r.Status(200).
		Json("username", "^", "op", "user"). // contains all
		Json("id", "~", 1, 100).             // between
		Json("username", "*", "ZZZ", "op")   // any of
}

func TestGetUserNotFound(t *testing.T) {
	c := setup(t)
	r := c.Get("/users/999")
	r.Status(404)
}

// ============================================================
// Users - Profile (nested path params)
// ============================================================

func TestGetUserProfile(t *testing.T) {
	c := setup(t)
	c.Post("/users", Json{
		"username": "profileuser",
		"email":    "profile@example.com",
	}).Status(201)

	r := c.Get("/users/1/profile")
	r.Status(200).
		Json("username", "profileuser").
		Json("todo_count", ">", -1)
}

// ============================================================
// Users - Patch (JSON)
// ============================================================

func TestPatchUserJson(t *testing.T) {
	c := setup(t)
	c.Post("/users", Json{
		"username": "originaluser",
		"email":    "original@example.com",
	}).Status(201)

	// Update user with JSON - only name/username, not avatar
	r := c.Patch("/users/1", Json{
		"username": "updateduser",
	})
	r.Status(200).
		Json("id", 1).
		Json("username", "updateduser").
		Json("email", "original@example.com").
		Json("avatar", "") // avatar should remain empty
}

// ============================================================
// Users - Patch (Multipart with File)
// ============================================================

func TestPatchUserMultipart(t *testing.T) {
	c := setup(t)
	c.Post("/users", Json{
		"username": "multipartuser",
		"email":    "multipart@example.com",
	}).Status(201)

	// Update user with multipart form - name, username, and avatar file
	r := c.Patch("/users/1",
		Form{
			"username": "updatedmultipart",
		},
		File("avatar", "sample.jpg"),
	)
	r.Status(200).
		Json("id", 1).
		Json("username", "updatedmultipart").
		Json("email", "multipart@example.com")

	// Get the avatar path from response
	data := r.Response.Json()
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		t.Fatal("expected JSON response to be a map")
	}
	avatar, ok := dataMap["avatar"].(string)
	if !ok || avatar == "" {
		t.Fatal("expected avatar to be set")
	}

	// Verify the file path format: uploads/sample-<timestamp>.jpg
	if !strings.Contains(avatar, "uploads/sample-") || !strings.HasSuffix(avatar, ".jpg") {
		t.Errorf("expected avatar path to match 'uploads/sample-<timestamp>.jpg', got: %s", avatar)
	}

	// Verify the file exists
	if _, err := os.Stat(avatar); os.IsNotExist(err) {
		t.Errorf("uploaded file does not exist: %s", avatar)
	}
}

// ============================================================
// Users - Scalar fields (plain-text body assertions)
// ============================================================

// TestHealthText asserts exact and regex matching against a plain-text body.
func TestHealthText(t *testing.T) {
	c := setup(t)
	c.Get("/health-text").Status(200).
		Expect("app is working").
		Regex("is working")
}

// TestUserAgeAndBalance asserts numeric values on a user's scalar endpoints,
// including comparison operators, regex matching and response header
// matching on the Content-Type.
func TestUserAgeAndBalance(t *testing.T) {
	c := setup(t)
	c.Post("/users", Json{"username": "num", "email": "num@example.com"}).Status(201)

	// Age
	c.Get("/users/1/age").Status(200).Expect(13) // exact numeric

	// Balance
	c.Get("/users/1/balance").Status(200).Expect(131.50)   // exact float
	c.Get("/users/1/balance").Status(200).Expect(">", 131) // greater than
	c.Get("/users/1/balance").Status(200).Expect("<", 132) // less than
	c.Get("/users/1/balance").Status(200).Regex(`^\d+\.\d{2}$`)

	// Content-Type header operators on a plain-text response.
	r := c.Get("/users/1/balance")
	r.Status(200).
		Header("Content-Type", "^", "text/plain").   // contains all
		Header("Content-Type", "*", "json", "plain") // any of
}

// TestUserCreatedAt asserts date matching and comparisons on the
// created_at scalar endpoint.
func TestUserCreatedAt(t *testing.T) {
	c := setup(t)
	c.Post("/users", Json{"username": "dt", "email": "dt@example.com"}).Status(201)

	want := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	c.Get("/users/1/created_at").Status(200).Expect(want) // date match
	c.Get("/users/1/created_at").Status(200).Expect(">", time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC))
	c.Get("/users/1/created_at").Status(200).Expect("<", time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC))
}

// TestUserLicenses asserts contains-all (^), contains-none (!^),
// contains-any (*) and numeric between (~) operators.
func TestUserLicenses(t *testing.T) {
	c := setup(t)
	c.Post("/users", Json{"username": "lic", "email": "lic@example.com"}).Status(201)

	// must contain all of these words
	c.Get("/users/1/licenses").Status(200).Expect("^", "ielts", "icdl")
	// must contain none of these words
	c.Get("/users/1/licenses").Status(200).Expect("!^", "foss", "pmp")
	// contains any of these words
	c.Get("/users/1/licenses").Status(200).Expect("*", "ielts", "icdl", "xyz")
	// numeric between
	c.Get("/users/1/age").Status(200).Expect("~", 10, 20)
}

// ============================================================
// Users - Delete (Admin authorization)
// ============================================================

func TestAdminLogin(t *testing.T) {
	c := setup(t)

	// Admin login with different credentials than user login
	r := c.Post("/admin/login", Form{"username": "admin", "password": "admin-secret-admin"})
	r.Status(200)
	r.Json("role", "admin")
	r.Json("token", "admin-token-123")
	r.Json("adminId", 0)
}

func TestAdminDeleteUser(t *testing.T) {
	c := setup(t)

	// First create a user
	r := c.Post("/users", Json{"username": "john_doe", "email": "john@example.com"})
	r.Status(201)

	// Extract user ID from response
	data := r.Response.Json()
	dataMap := data.(map[string]interface{})
	userId := dataMap["id"].(float64)

	// Try to delete user without admin token (should fail)
	r = c.Delete("/users/:id", Param{"id": int(userId)})
	r.Status(401) // Unauthorized

	// Login as admin
	r = c.Post("/admin/login", Form{"username": "admin", "password": "admin-secret-admin"})
	r.Status(200)

	// Extract admin token
	data = r.Response.Json()
	dataMap = data.(map[string]interface{})
	adminToken := dataMap["token"].(string)

	// Delete user with admin token (should succeed)
	r = c.Delete("/users/:id", Param{"id": int(userId)}, Header{"Authorization": "Bearer " + adminToken})
	r.Status(204) // No Content

	// Verify user is deleted
	r = c.Get("/users/:id", Param{"id": int(userId)})
	r.Status(404) // Not Found
}

func TestAdminCannotDeleteWithUserToken(t *testing.T) {
	c := setup(t)

	// Create a user
	r := c.Post("/users", Json{"username": "jane_doe", "email": "jane@example.com"})
	r.Status(201)

	// Extract user ID from response
	data := r.Response.Json()
	dataMap := data.(map[string]interface{})
	userId := dataMap["id"].(float64)

	// Login as regular user (different endpoint than admin)
	r = c.Post("/login", Form{"username": "user", "password": "password"})
	r.Status(200)

	// Extract user token
	data = r.Response.Json()
	dataMap = data.(map[string]interface{})
	userToken := dataMap["token"].(string)

	// Try to delete user with regular user token (should fail - admin only)
	r = c.Delete("/users/:id", Param{"id": int(userId)}, Header{"Authorization": "Bearer " + userToken})
	r.Status(403) // Forbidden - admin access required
}

// ============================================================
// Auth - Login (Form body)
// ============================================================

func TestFormLogin(t *testing.T) {
	c := setup(t)
	r := c.Post("/login", Form{
		"username": "admin",
		"password": "secret",
	})
	r.Status(200).
		Json("token", "admin-token-123").
		Json("user", "admin").
		Json("role", "admin")
}

func TestFormLoginInvalid(t *testing.T) {
	c := setup(t)
	r := c.Post("/login", Form{
		"username": "admin",
		"password": "wrong",
	})
	r.Status(401).
		Json("error", "invalid credentials")
}

func TestFormLoginMissingFields(t *testing.T) {
	c := setup(t)
	r := c.Post("/login", Form{"username": ""})
	r.Status(400)
}

// ============================================================
// Cookies
// ============================================================

// TestCookieBasics verifies cookies set by the endpoint and demonstrates the
// presence, exact-value and any-of/between operators on cookies.
func TestCookieBasics(t *testing.T) {
	c := setup(t)

	// Get endpoint that sets cookies
	r := c.Get("/add-cookie")
	r.Status(200).Json("message", "cookies have been set")

	// Check that cookies were set
	r.Cookie("we-added-cookie")         // just makes sure the cookie exists
	r.Cookie("we-added-cookie", "true") // check value
	r.Cookie("cookies-count", "1")      // check exact value

	// Cookie operators
	r.Cookie("cookies-count", "~", 1, 10)           // between
	r.Cookie("we-added-cookie", "*", "ZZZ", "true") // any of

	// Print all cookies
	fmt.Println("Response cookies:")
	for _, cookie := range r.Response.Cookies() {
		fmt.Printf("  %s: %s\n", cookie.Name, cookie.Value)
	}
}

// TestCookieNumericOperators demonstrates the full set of numeric comparison
// operators against cookie values.
func TestCookieNumericOperators(t *testing.T) {
	c := setup(t)

	// Get endpoint that sets cookies
	r := c.Get("/add-cookie")
	r.Status(200)

	// Check cookie with numeric comparison
	r.Cookie("cookies-count", ">", 0)  // value should be greater than 0
	r.Cookie("cookies-count", ">=", 1) // value should be greater or equal to 1
	r.Cookie("cookies-count", "<", 10) // value should be less than 10
	r.Cookie("cookies-count", "<=", 1) // value should be less or equal to 1
	r.Cookie("cookies-count", "==", 1) // value should be equal to 1
	r.Cookie("cookies-count", "!=", 0) // value should not be equal to 0
}

func TestSharedHeadersAndCookies(t *testing.T) {
	c := setup(t)

	// Set shared header and cookies
	c.Header("Authorization", "Bearer token-123")
	c.Header("X-Custom-Header", "custom-value")
	c.Cookie("ad-uid", "abc123")
	c.Cookie("session_id", "xyz789")

	// These headers and cookies will be included in all subsequent requests
	r := c.Get("/health")
	r.Status(200)

	// Remove shared header
	c.Header("Authorization", nil)

	// Remove shared cookie
	c.Cookie("ad-uid", nil)

	// Now only session_id cookie will be sent
	r = c.Get("/health")
	r.Status(200)
}

func TestCookieIncrement(t *testing.T) {
	c := setup(t)

	// First call sets count to 1
	r := c.Get("/set-cookies")
	r.Status(200)
	r.Cookie("cookies-count", "1")

	// Get all cookies from response
	cookies := r.Response.Cookies()

	// Find the cookies-count cookie
	var countValue string
	for _, cookie := range cookies {
		if cookie.Name == "cookies-count" {
			countValue = cookie.Value
			break
		}
	}

	if countValue != "" {
		// Use the cookie value in the next request
		r = c.Get("/set-cookies", Cookie("cookies-count", countValue))
		r.Status(200)
		r.Cookie("cookies-count", ">", 1) // Should be incremented to 2
	}
}
