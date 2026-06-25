package tapetest

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

type fileUpload struct {
	field       string
	path        string
	contentType string // optional per-part Content-Type; empty => application/octet-stream (default)
}

type requestConfig struct {
	headers  map[string]string
	query    map[string]string
	timeout  time.Duration
	files    []fileUpload
	params   map[string]string // path parameters like :id
	formBody url.Values        // form-encoded body fields
	cookies  map[string]string // cookies for the request
}

func defaultConfig() *requestConfig {
	return &requestConfig{
		headers: make(map[string]string),
		query:   make(map[string]string),
		cookies: make(map[string]string),
	}
}

// Option configures a request. Pass options to Get, Post, Put, Patch, Delete, Head, or Request.
// Built-in options: Query, Header, File, Timeout, Bearer, Param, Form.
type Option interface {
	apply(*requestConfig)
}

// optionFunc wraps a function to implement Option.
type optionFunc func(*requestConfig)

func (f optionFunc) apply(cfg *requestConfig) {
	f(cfg)
}

// --- Body Types ---

// Json is a JSON body type. Use as the body argument in Post, Put, Patch.
//
//	c.Post("/user", Json{"username": "john", "age": 12})
type Json map[string]interface{}

// Form is a form-encoded body type (application/x-www-form-urlencoded).
// When used with File options, automatically switches to multipart/form-data.
//
//	c.Post("/user", Form{"username": "john", "age": 12})
//	c.Post("/user", Form{"username": "john"}, File("avatar", "./avatar.png"))
type Form map[string]interface{}

// --- Path Params ---

// Param replaces path parameters like :id, :name in the URL.
//
//	c.Get("/user/:id/:scope", Param{"id": 12, "scope": "books"})
//	// resolves to /user/12/books
type Param map[string]interface{}

func (p Param) apply(cfg *requestConfig) {
	if cfg.params == nil {
		cfg.params = make(map[string]string)
	}
	for k, v := range p {
		cfg.params[k] = fmt.Sprintf("%v", v)
	}
}

// resolveParams replaces :param and {param} placeholders in the path with actual values.
func resolveParams(path string, params map[string]string) string {
	if params == nil {
		return path
	}
	for k, v := range params {
		path = strings.ReplaceAll(path, ":"+k, v)
		path = strings.ReplaceAll(path, "{"+k+"}", v)
	}
	return path
}

// --- Form handling ---

// Form implements Option to add form fields to multipart requests.
func (f Form) apply(cfg *requestConfig) {
	if cfg.formBody == nil {
		cfg.formBody = make(url.Values)
	}
	for k, v := range f {
		cfg.formBody.Set(k, fmt.Sprintf("%v", v))
	}
}

// --- Functional Options ---

// Query adds a query parameter to the request.
//
//	c.Get("/users", Query("page", "1"), Query("limit", "10"))
func Query(key, value string) Option {
	return optionFunc(func(cfg *requestConfig) {
		cfg.query[key] = value
	})
}

// Header adds a header to the request.
//
//	c.Post("/user", body, Header("Authorization", "Bearer token"))
func Header(key, value string) Option {
	return optionFunc(func(cfg *requestConfig) {
		cfg.headers[key] = value
	})
}

// File adds a file upload to the request (uses multipart/form-data).
//
// By default the uploaded part is sent as application/octet-stream. Pass an
// optional third argument to set the part's Content-Type, which servers that
// validate the MIME type (e.g. image upload validators) rely on instead of
// sniffing the bytes:
//
//	c.Post("/upload", nil, File("avatar", "./photo.png"))
//	c.Post("/upload", nil, File("avatar", "./photo.png", "image/png"))
//	c.Post("/upload", nil, File("doc", "./report.pdf", "application/pdf"))
func File(field, path string, contentType ...string) Option {
	ct := ""
	if len(contentType) > 0 {
		ct = contentType[0]
	}
	return optionFunc(func(cfg *requestConfig) {
		cfg.files = append(cfg.files, fileUpload{field: field, path: path, contentType: ct})
	})
}

// Timeout sets a request timeout. Only effective with HttpClient.
//
//	c.Get("/slow", Timeout(5*time.Second))
func Timeout(d time.Duration) Option {
	return optionFunc(func(cfg *requestConfig) {
		cfg.timeout = d
	})
}

// Bearer sets the Authorization header with a Bearer token.
// Shorthand for Header("Authorization", "Bearer "+token).
//
//	c.Get("/me", Bearer("my-token"))
func Bearer(token string) Option {
	return optionFunc(func(cfg *requestConfig) {
		cfg.headers["Authorization"] = "Bearer " + token
	})
}

// Cookie adds a cookie to the request.
//
//	c.Get("/cart", Cookie("session_id", "abc123"))
func Cookie(key, value string) Option {
	return optionFunc(func(cfg *requestConfig) {
		cfg.cookies[key] = value
	})
}
