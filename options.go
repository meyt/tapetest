package tapetest

import (
	"fmt"
	"strings"
	"time"
)

type fileUpload struct {
	field       string
	path        string
	contentType string // optional per-part Content-Type; empty => application/octet-stream (default)
}

type requestConfig struct {
	headers map[string]string
	query   map[string]interface{} // query params; values may be named-typed strings (enums)
	timeout time.Duration
	files   []fileUpload
	params  map[string]interface{} // path params; values may be named-typed strings (enums)
	cookies map[string]string
	body    interface{} // request body set via a Json/Form option
}

func defaultConfig() *requestConfig {
	return &requestConfig{
		headers: make(map[string]string),
		query:   make(map[string]interface{}),
		cookies: make(map[string]string),
	}
}

// Option configures a request. Pass options to Get, Post, Put, Patch, Delete, Head, or Request.
// Built-in options: Query, Header, File, Timeout, Bearer, Param, Json, Form.
type Option interface {
	apply(*requestConfig)
}

// optionFunc wraps a function to implement Option.
type optionFunc func(*requestConfig)

func (f optionFunc) apply(cfg *requestConfig) {
	f(cfg)
}

// --- Body Types ---
//
// Json and Form are the request body types. They implement Option, so the body
// is configured just like any other option — alongside Query, Header, Param,
// etc., in any order — with no dedicated body argument:
//
//      c.Post("/user", Json{"username": "john", "age": 12})
//      c.Post("/user", Json{"username": "john"}, Bearer("token"))
//      c.Post("/item/:id", Param{"id": "1"}, Json{"name": "widget"})

// Json is a JSON body type. Pass it as an option to Post, Put, Patch (or any
// verb) to send a JSON request body.
type Json map[string]interface{}

// Form is a form-encoded body type (application/x-www-form-urlencoded).
// When used with File options, automatically switches to multipart/form-data.
// Pass it as an option like Json.
type Form map[string]interface{}

// --- Path Params ---

// Param replaces path parameters like :id, :name in the URL.
//
//	c.Get("/user/:id/:scope", Param{"id": 12, "scope": "books"})
//	// resolves to /user/12/books
type Param map[string]interface{}

func (p Param) apply(cfg *requestConfig) {
	if cfg.params == nil {
		cfg.params = make(map[string]interface{})
	}
	for k, v := range p {
		cfg.params[k] = v
	}
}

// resolveParams replaces :param and {param} placeholders in the path with
// actual values. Values are stringified via fmt.Sprintf("%v", v) so named
// string types (e.g. GenderType) are converted to their underlying value.
func resolveParams(path string, params map[string]interface{}) string {
	if params == nil {
		return path
	}
	for k, v := range params {
		s := fmt.Sprintf("%v", v)
		path = strings.ReplaceAll(path, ":"+k, s)
		path = strings.ReplaceAll(path, "{"+k+"}", s)
	}
	return path
}

// --- Body handling ---

// Json implements Option so it can set the request body (marshaled to JSON)
// just like any other option.
func (j Json) apply(cfg *requestConfig) {
	cfg.body = j
}

// Form implements Option so it can set the request body. It is sent as
// application/x-www-form-urlencoded, or as multipart/form-data fields when
// paired with File options.
func (f Form) apply(cfg *requestConfig) {
	cfg.body = f
}

// --- Query ---

// Query is a query-parameter body type, mirroring Form/Json/Param. Pass it as
// an option to Get/Post/etc. to set query parameters. Values may be plain
// strings, integers, or named-typed strings (enums); the recorder reflects on
// named types and consults the enum registry (see RegisterEnum) to emit
// `enum` constraints in the generated OpenAPI document.
//
//	c.Get("/users", Query{"page": "1", "limit": 10})
//	c.Get("/admin/address-parts", Query{"sort_by": SortName})
type Query map[string]interface{}

func (q Query) apply(cfg *requestConfig) {
	if cfg.query == nil {
		cfg.query = make(map[string]interface{})
	}
	for k, v := range q {
		cfg.query[k] = v
	}
}

// --- Functional Options ---

// Header adds a header to the request.
//
//	c.Post("/user", Json{"name": "john"}, Header("Authorization", "Bearer token"))
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
//	c.Post("/upload", File("avatar", "./photo.png"))
//	c.Post("/upload", File("avatar", "./photo.png", "image/png"))
//	c.Post("/upload", File("doc", "./report.pdf", "application/pdf"))
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
