package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

// --- General API Info (go-swag style, declared once) ---
//
// These map to the OpenAPI v3 `info` object and to a server entry built
// from the host and base-path directives below.

// @title Swagger Example API
// @version 1.0
// @description This is a sample server celler server.
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.url http://www.swagger.io/support
// @contact.email support@swagger.io

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @host localhost:8080
// @BasePath /
// @schemes http

// --- Security Definitions (go-swag style, declared once) ---
//
// These map to OpenAPI v3 components.securitySchemes and are referenced by
// handler @Security annotations below.

// @securityDefinitions.apikey UserAuth
// @in header
// @name Authorization
// @description User JWT token. this is demo for openapi@3 apikey scheme header so Type "Bearer " before the token.

// @securityDefinitions.bearer AdminAuth
// @bearerFormat JWT
// @description Admin JWT token.

// --- Models ---

type Todo struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Active   bool   `json:"active"`
	Avatar   string `json:"avatar"`
}

type Admin struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Token    string `json:"token"`
}

type SearchResult struct {
	Query   string  `json:"query"`
	Results []*Todo `json:"results"`
	Count   int     `json:"count"`
}

// --- App ---

type App struct {
	Echo        *echo.Echo
	todos       map[int]*Todo
	users       map[int]*User
	admins      map[int]*Admin
	userTokens  map[string]int // token -> user ID
	adminTokens map[string]int // token -> admin ID
	mu          sync.RWMutex
	nextID      int
}

func NewApp() *App {
	e := echo.New()
	e.HideBanner = true

	app := &App{
		Echo:        e,
		todos:       make(map[int]*Todo),
		users:       make(map[int]*User),
		admins:      make(map[int]*Admin),
		userTokens:  make(map[string]int),
		adminTokens: make(map[string]int),
		nextID:      1,
	}

	// Initialize default admin with ID 0 (separate from user/todo IDs)
	admin := &Admin{
		ID:       0,
		Username: "admin",
		Email:    "admin@example.com",
		Token:    "admin-token-123",
	}
	app.admins[0] = admin
	app.adminTokens["admin-token-123"] = 0

	// Health
	e.GET("/health", app.health)

	// Todos CRUD
	e.GET("/todos", app.listTodos)
	e.POST("/todos", app.createTodo)
	e.GET("/todos/:id", app.getTodo)
	e.PUT("/todos/:id", app.updateTodo)
	e.PATCH("/todos/:id", app.patchTodo)
	e.DELETE("/todos/:id", app.deleteTodo)
	e.GET("/todos/search", app.searchTodos)

	// Users
	e.POST("/users", app.createUser)
	e.GET("/users/:id", app.getUser)
	e.GET("/users/:id/profile", app.getUserProfile)
	e.PATCH("/users/:id", app.patchUser)
	e.DELETE("/users/:id", app.deleteUser)

	// Admin
	e.POST("/admin/login", app.adminLogin)

	// Auth
	e.POST("/login", app.login)

	// HEAD/OPTIONS
	e.HEAD("/todos", app.listTodos)

	// Cookie examples
	e.GET("/add-cookie", app.addCookie)
	e.GET("/set-cookies", app.setCookies)

	// Docs - serve generated swagger-ui documentation
	e.Static("/docs", "docs")

	return app
}

// --- Health ---

// @Title Health check
// @Description Returns the API health status
// @Tag health
// @Method GET
// @Path /health
func (a *App) health(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// --- Todos ---

// @Title List todos
// @Description Returns all todo items
// @Tag todos
// @Method GET
// @Path /todos
func (a *App) listTodos(c echo.Context) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	todos := make([]*Todo, 0, len(a.todos))
	for _, t := range a.todos {
		todos = append(todos, t)
	}
	return c.JSON(http.StatusOK, todos)
}

// @Title Create todo
// @Description Create a new todo item
// @Tag todos
// @Method POST
// @Path /todos
func (a *App) createTodo(c echo.Context) error {
	var todo Todo
	if err := c.Bind(&todo); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if todo.Title == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "title is required"})
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	todo.ID = a.nextID
	a.nextID++
	a.todos[todo.ID] = &todo

	return c.JSON(http.StatusCreated, &todo)
}

// @Title Get todo
// @Description Get a todo item by ID
// @Tag todos
// @Method GET
// @Path /todos/:id
func (a *App) getTodo(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	todo, ok := a.todos[id]
	if !ok {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "todo not found"})
	}
	return c.JSON(http.StatusOK, todo)
}

// @Title Update todo
// @Description Update a todo item by ID
// @Tag todos
// @Method PUT
// @Path /todos/:id
func (a *App) updateTodo(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	todo, ok := a.todos[id]
	if !ok {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "todo not found"})
	}

	var update Todo
	if err := c.Bind(&update); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	todo.Title = update.Title
	todo.Done = update.Done

	return c.JSON(http.StatusOK, todo)
}

// @Title Patch todo
// @Description Partially update a todo item by ID
// @Tag todos
// @Method PATCH
// @Path /todos/:id
func (a *App) patchTodo(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	todo, ok := a.todos[id]
	if !ok {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "todo not found"})
	}

	var patch Todo
	if err := c.Bind(&patch); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if patch.Title != "" {
		todo.Title = patch.Title
	}
	todo.Done = patch.Done

	return c.JSON(http.StatusOK, todo)
}

// @Title Delete todo
// @Description Delete a todo item by ID
// @Tag todos
// @Method DELETE
// @Path /todos/:id
func (a *App) deleteTodo(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.todos[id]; !ok {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "todo not found"})
	}

	delete(a.todos, id)
	return c.NoContent(http.StatusNoContent)
}

// @Title Search todos
// @Description Search todos by title
// @Tag todos
// @Method GET
// @Path /todos/search
func (a *App) searchTodos(c echo.Context) error {
	query := c.QueryParam("q")
	if query == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "query parameter 'q' is required"})
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	var results []*Todo
	for _, t := range a.todos {
		if strings.Contains(strings.ToLower(t.Title), strings.ToLower(query)) {
			results = append(results, t)
		}
	}

	return c.JSON(http.StatusOK, &SearchResult{
		Query:   query,
		Results: results,
		Count:   len(results),
	})
}

// --- Users ---

// @Title Create user
// @Description Create a new user
// @Tag users
// @Security UserAuth
// @Security AdminAuth
// @Method POST
// @Path /users
func (a *App) createUser(c echo.Context) error {
	var user User
	if err := c.Bind(&user); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if user.Username == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "username is required"})
	}
	if user.Email == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "email is required"})
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Check duplicate username
	for _, u := range a.users {
		if u.Username == user.Username {
			return c.JSON(http.StatusConflict, map[string]string{"error": "username already exists"})
		}
	}

	user.ID = a.nextID
	a.nextID++
	user.Active = true
	a.users[user.ID] = &user

	return c.JSON(http.StatusCreated, &user)
}

// @Title Get user
// @Description Get a user by ID
// @Tag users
// @Security UserAuth
// @Security AdminAuth
// @Method GET
// @Path /users/:id
func (a *App) getUser(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	user, ok := a.users[id]
	if !ok {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "user not found"})
	}
	return c.JSON(http.StatusOK, user)
}

// @Title Get user profile
// @Description Get extended user profile with todo count
// @Tag users
// @Method GET
// @Path /users/:id/profile
func (a *App) getUserProfile(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	user, ok := a.users[id]
	if !ok {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "user not found"})
	}

	todoCount := 0
	for _, t := range a.todos {
		if strings.Contains(strings.ToLower(t.Title), strings.ToLower(user.Username)) {
			todoCount++
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"id":         user.ID,
		"username":   user.Username,
		"email":      user.Email,
		"role":       user.Role,
		"active":     user.Active,
		"todo_count": todoCount,
	})
}

// @Title Patch user
// @Description Partially update a user by ID (supports JSON and multipart/form-data)
// @Tag users
// @Method PATCH
// @Path /users/:id
func (a *App) patchUser(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	user, ok := a.users[id]
	if !ok {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "user not found"})
	}

	// Check if this is a multipart/form-data request
	contentType := c.Request().Header.Get("Content-Type")
	if contentType != "" && strings.Contains(contentType, "multipart/form-data") {
		// Handle multipart/form-data - supports name, username, and avatar file
		if name := c.FormValue("name"); name != "" {
			user.Username = name
		}
		if username := c.FormValue("username"); username != "" {
			user.Username = username
		}

		// Handle avatar file upload
		file, err := c.FormFile("avatar")
		if err == nil {
			// Open the uploaded file
			src, err := file.Open()
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to open uploaded file"})
			}
			defer src.Close()

			// Create uploads directory if it doesn't exist
			uploadsDir := "uploads"
			if err := os.MkdirAll(uploadsDir, 0755); err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create uploads directory"})
			}

			// Generate timestamp for unique filename
			timestamp := time.Now().Format("20060102-150405")
			ext := filepath.Ext(file.Filename)
			filename := fmt.Sprintf("%s-%s%s", file.Filename[:len(file.Filename)-len(ext)], timestamp, ext)
			dstPath := filepath.Join(uploadsDir, filename)

			// Create destination file
			dst, err := os.Create(dstPath)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create destination file"})
			}
			defer dst.Close()

			// Copy uploaded file to destination
			if _, err := io.Copy(dst, src); err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save uploaded file"})
			}

			// Store the relative path in user avatar field
			user.Avatar = filepath.Join("uploads", filename)
		}

		return c.JSON(http.StatusOK, user)
	}

	// Handle JSON request - only updates name/username, not avatar
	var update struct {
		Username string `json:"username"`
		Name     string `json:"name"`
	}
	if err := c.Bind(&update); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if update.Name != "" {
		user.Username = update.Name
	}
	if update.Username != "" {
		user.Username = update.Username
	}

	return c.JSON(http.StatusOK, user)
}

// @Title Delete user
// @Description Delete a user by ID (admin only)
// @Tag users
// @Security AdminAuth
// @Method DELETE
// @Path /users/:id
func (a *App) deleteUser(c echo.Context) error {
	// Check admin token
	token := c.Request().Header.Get("Authorization")
	if !strings.HasPrefix(token, "Bearer ") {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "missing authorization header"})
	}
	token = strings.TrimPrefix(token, "Bearer ")

	a.mu.RLock()
	_, isAdmin := a.adminTokens[token]
	a.mu.RUnlock()

	if !isAdmin {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "admin access required"})
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.users[id]; !ok {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "user not found"})
	}

	delete(a.users, id)
	return c.NoContent(http.StatusNoContent)
}

// --- Admin ---

// @Title Admin login
// @Description Authenticate an admin with username and password (different from user login)
// @Tag admin
// @Method POST
// @Path /admin/login
func (a *App) adminLogin(c echo.Context) error {
	username := c.FormValue("username")
	password := c.FormValue("password")

	if username == "" || password == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "username and password required"})
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	for _, admin := range a.admins {
		if admin.Username == username && password == "admin-secret-"+admin.Username {
			return c.JSON(http.StatusOK, map[string]interface{}{
				"token":   admin.Token,
				"user":    admin.Username,
				"role":    "admin",
				"adminId": admin.ID,
			})
		}
	}

	return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid admin credentials"})
}

// --- Cookie Examples ---

// @Title Add cookie
// @Description Demonstrates cookie handling - sets cookies in response
// @Tag cookies
// @Method GET
// @Path /add-cookie
func (a *App) addCookie(c echo.Context) error {
	// Set various cookies to demonstrate cookie support
	c.SetCookie(&http.Cookie{
		Name:  "we-added-cookie",
		Value: "true",
		Path:  "/",
	})

	c.SetCookie(&http.Cookie{
		Name:  "cookies-count",
		Value: "1",
		Path:  "/",
	})

	c.SetCookie(&http.Cookie{
		Name:  "ad-uid",
		Value: "abc123",
		Path:  "/",
	})

	return c.JSON(http.StatusOK, map[string]string{
		"message": "cookies have been set",
	})
}

// @Title Set cookies
// @Description Demonstrates cookie handling with incrementing counter
// @Tag cookies
// @Method GET
// @Path /set-cookies
func (a *App) setCookies(c echo.Context) error {
	// Get existing cookies count
	countStr := c.Request().Header.Get("Cookie")
	count := 0
	if strings.Contains(countStr, "cookies-count=") {
		parts := strings.Split(countStr, "cookies-count=")
		if len(parts) > 1 {
			nextPart := strings.Split(parts[1], ";")[0]
			if parsed, err := strconv.Atoi(nextPart); err == nil {
				count = parsed
			}
		}
	}
	count++

	c.SetCookie(&http.Cookie{
		Name:  "cookies-count",
		Value: strconv.Itoa(count),
		Path:  "/",
	})

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "cookies updated",
		"count":   count,
	})
}

// --- Auth ---

// @Title Login
// @Description Authenticate a user with username and password
// @Tag auth
// @Method POST
// @Path /login
func (a *App) login(c echo.Context) error {
	username := c.FormValue("username")
	password := c.FormValue("password")

	if username == "" || password == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "username and password required"})
	}

	// Regular user login - just a demo for testing
	// In real app, this would check against user database

	// This is for backwards compatibility with existing tests
	if username == "admin" && password == "secret" {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"token": "admin-token-123",
			"user":  username,
			"role":  "admin",
		})
	}

	// Regular user login
	if username == "user" && password == "password" {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"token": "user-token-456",
			"user":  username,
			"role":  "user",
		})
	}

	return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
}

func main() {
	app := NewApp()
	app.Echo.Logger.Fatal(app.Echo.Start(":8080"))
}
