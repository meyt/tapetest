package tapetest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Client is the main testing client. Create one per test using
// HttpClient (for live servers) or HandlerClient (for http.Handler).
type Client struct {
	t       *testing.T
	handler http.Handler
	baseURL string
	client  *http.Client

	// Shared headers and cookies (persist across requests)
	sharedHeaders map[string]string
	sharedCookies map[string]string

	// Per-service server metadata tagged onto every recording made by this
	// client, so the generated OpenAPI document can emit per-operation
	// servers and Swagger UI's "Try it out" routes each endpoint correctly.
	serverName string
	serverURL  string
}

// HttpClient creates a Client that makes real HTTP requests to the given base URL.
//
//	c := HttpClient(t, "http://localhost:8080/api/v1")
func HttpClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	return &Client{
		t:             t,
		baseURL:       baseURL,
		client:        &http.Client{},
		sharedHeaders: make(map[string]string),
		sharedCookies: make(map[string]string),
	}
}

// HandlerClient creates a Client that tests against an http.Handler directly.
// Uses httptest under the hood — no network overhead, blazing fast.
// Works with Echo, Gin, Chi, and any framework implementing http.Handler.
//
//	c := HandlerClient(t, echoInstance)
func HandlerClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	return &Client{
		t:             t,
		handler:       handler,
		sharedHeaders: make(map[string]string),
		sharedCookies: make(map[string]string),
	}
}

// BaseUrl sets a URL prefix that is prepended to every request path.
// Useful when all routes share a common prefix (e.g. an API version).
// It returns the Client itself so it can be chained at construction time.
//
//	c := HandlerClient(t, myHandler).BaseUrl("/api/v1")
//	c.Get("/users").Status(200)
//
// or set after construction:
//
//	c := HandlerClient(t, myHandler)
//	c.BaseUrl("/api/v1")
func (c *Client) BaseUrl(prefix string) *Client {
	c.baseURL = prefix
	return c
}

// Server tags every recording made by this client with the given service
// name and URL, so the generated OpenAPI document emits a per-operation
// servers entry. This lets Swagger UI's "Try it out" route each endpoint to
// its own backend when a test suite covers several services deployed on
// different URLs.
//
// The URL may be relative or absolute:
//
//   - Relative (e.g. "/api/v1"): Swagger UI resolves it against wherever the
//     docs are hosted, so "Try it out" works without further configuration.
//     The URL is also stripped from recorded request paths, making the OpenAPI
//     path relative to the operation-level server.
//   - Absolute (e.g. "https://user-api.example.com"): "Try it out" sends
//     requests directly to that host. Keep BaseUrl for the path prefix, so the
//     server is scheme+host only (e.g. "https://user-api.example.com" with
//     BaseUrl("/api/v1")).
//
// Relative — portable across environments:
//
//	c := EchoClient(t, adminApp.Echo).BaseUrl("/api/v1").Server("Admin API", "/api/v1")
//
// Absolute — per-service hosts:
//
//	c := EchoClient(t, adminApp.Echo).BaseUrl("/api/v1").Server("Admin API", "https://admin-api.example.com")
//	c := EchoClient(t, userApp.Echo).BaseUrl("/api/v1").Server("User API", "https://user-api.example.com")
func (c *Client) Server(name, url string) *Client {
	c.serverName = name
	c.serverURL = url
	return c
}

// EchoClient is an alias for HandlerClient. *echo.Echo implements http.Handler.
func EchoClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	return HandlerClient(t, handler)
}

// Echo4Client is an alias for HandlerClient for Echo v4.
func Echo4Client(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	return HandlerClient(t, handler)
}

// Echo5Client is an alias for HandlerClient for Echo v5.
func Echo5Client(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	return HandlerClient(t, handler)
}

// GinClient is an alias for HandlerClient. *gin.Engine implements http.Handler.
func GinClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	return HandlerClient(t, handler)
}

// --- HTTP Methods ---

// Get sends a GET request.
func (c *Client) Get(path string, opts ...Option) *Response {
	return c.do(http.MethodGet, path, nil, opts...)
}

// Post sends a POST request with the given body (marshaled to JSON automatically).
func (c *Client) Post(path string, body interface{}, opts ...Option) *Response {
	return c.do(http.MethodPost, path, body, opts...)
}

// Put sends a PUT request with the given body.
func (c *Client) Put(path string, body interface{}, opts ...Option) *Response {
	return c.do(http.MethodPut, path, body, opts...)
}

// Patch sends a PATCH request with the given body.
func (c *Client) Patch(path string, body interface{}, opts ...Option) *Response {
	return c.do(http.MethodPatch, path, body, opts...)
}

// Delete sends a DELETE request.
func (c *Client) Delete(path string, opts ...Option) *Response {
	return c.do(http.MethodDelete, path, nil, opts...)
}

// Head sends a HEAD request.
func (c *Client) Head(path string, opts ...Option) *Response {
	return c.do(http.MethodHead, path, nil, opts...)
}

// Request sends a request with a custom HTTP method.
func (c *Client) Request(method, path string, opts ...Option) *Response {
	return c.do(method, path, nil, opts...)
}

// --- Internal ---

func (c *Client) do(method, path string, body interface{}, opts ...Option) *Response {
	c.t.Helper()

	cfg := defaultConfig()

	// Apply shared headers first
	for k, v := range c.sharedHeaders {
		cfg.headers[k] = v
	}

	// Apply shared cookies first
	for k, v := range c.sharedCookies {
		cfg.cookies[k] = v
	}

	// Apply request-specific options (can override shared)
	for _, opt := range opts {
		opt.apply(cfg)
	}

	// Resolve path parameters like :id, :name
	path = resolveParams(path, cfg.params)

	// Prepend the configured base URL (set via BaseUrl) once, so that
	// every downstream path (handler/server/multipart/form) and the
	// recording layer all see the full, final path.
	path = c.baseURL + path

	// Handle file uploads or form with files via multipart
	if len(cfg.files) > 0 {
		resp := c.doMultipart(method, path, body, cfg)
		c.recordExchange(method, path, body, cfg, resp)
		return resp
	}

	// Handle form body (application/x-www-form-urlencoded)
	if _, ok := body.(Form); ok {
		resp := c.doForm(method, path, body, cfg)
		c.recordExchange(method, path, body, cfg, resp)
		return resp
	}

	// Marshal body to JSON (default for maps, structs, Json type, etc.)
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("tapetest: failed to marshal body: %v", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
		if _, ok := cfg.headers["Content-Type"]; !ok {
			cfg.headers["Content-Type"] = "application/json"
		}
	}

	var resp *Response
	if c.handler != nil {
		resp = c.doHandler(method, path, bodyReader, cfg)
	} else {
		resp = c.doServer(method, path, bodyReader, cfg)
	}
	c.recordExchange(method, path, body, cfg, resp)
	return resp
}

// doForm sends a request with form-encoded body.
func (c *Client) doForm(method, path string, body interface{}, cfg *requestConfig) *Response {
	c.t.Helper()

	form := body.(Form)
	values := url.Values{}
	for k, v := range form {
		values.Set(k, fmt.Sprintf("%v", v))
	}

	bodyReader := strings.NewReader(values.Encode())
	if _, ok := cfg.headers["Content-Type"]; !ok {
		cfg.headers["Content-Type"] = "application/x-www-form-urlencoded"
	}

	if c.handler != nil {
		return c.doHandler(method, path, bodyReader, cfg)
	}
	return c.doServer(method, path, bodyReader, cfg)
}

func (c *Client) doHandler(method, path string, body io.Reader, cfg *requestConfig) *Response {
	c.t.Helper()

	req := httptest.NewRequest(method, path, body)
	for k, v := range cfg.headers {
		req.Header.Set(k, v)
	}

	// Add cookies to request
	if len(cfg.cookies) > 0 {
		var cookieStrings []string
		for k, v := range cfg.cookies {
			cookieStrings = append(cookieStrings, fmt.Sprintf("%s=%s", k, v))
		}
		req.Header.Set("Cookie", strings.Join(cookieStrings, "; "))
	}

	q := req.URL.Query()
	for k, v := range cfg.query {
		q.Set(k, v)
	}
	req.URL.RawQuery = q.Encode()

	rec := httptest.NewRecorder()

	// Catch handler panics
	var handlerErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				handlerErr = fmt.Errorf("handler panicked: %v", r)
			}
		}()
		c.handler.ServeHTTP(rec, req)
	}()

	// Parse cookies from response
	var cookies []*http.Cookie
	for _, cookie := range rec.Result().Cookies() {
		cookies = append(cookies, cookie)
	}

	return &Response{
		t:       c.t,
		code:    rec.Code,
		reason:  http.StatusText(rec.Code),
		headers: rec.Header(),
		body:    rec.Body.Bytes(),
		err:     handlerErr,
		cookies: cookies,
		Response: &ResponseData{
			body:    rec.Body.Bytes(),
			headers: rec.Header(),
			cookies: cookies,
		},
	}
}

func (c *Client) doServer(method, path string, body io.Reader, cfg *requestConfig) *Response {
	c.t.Helper()

	// c.baseURL was already prepended to path in do(), so use it directly.
	fullURL := path
	if len(cfg.query) > 0 {
		q := url.Values{}
		for k, v := range cfg.query {
			q.Set(k, v)
		}
		fullURL += "?" + q.Encode()
	}

	req, err := http.NewRequest(method, fullURL, body)
	if err != nil {
		return &Response{
			t:        c.t,
			err:      err,
			Response: &ResponseData{},
		}
	}

	for k, v := range cfg.headers {
		req.Header.Set(k, v)
	}

	// Add cookies to request
	if len(cfg.cookies) > 0 {
		var cookieStrings []string
		for k, v := range cfg.cookies {
			cookieStrings = append(cookieStrings, fmt.Sprintf("%s=%s", k, v))
		}
		req.Header.Set("Cookie", strings.Join(cookieStrings, "; "))
	}

	client := c.client
	if client == nil {
		client = http.DefaultClient
	}
	if cfg.timeout > 0 {
		client = &http.Client{Timeout: cfg.timeout}
	}

	httpResp, err := client.Do(req)
	if err != nil {
		return &Response{
			t:        c.t,
			err:      err,
			Response: &ResponseData{},
		}
	}
	defer httpResp.Body.Close()

	bodyBytes, _ := io.ReadAll(httpResp.Body)

	return &Response{
		t:       c.t,
		code:    httpResp.StatusCode,
		reason:  httpResp.Status,
		headers: httpResp.Header,
		body:    bodyBytes,
		Response: &ResponseData{
			body:    bodyBytes,
			headers: httpResp.Header,
			cookies: httpResp.Cookies(),
		},
		cookies: httpResp.Cookies(),
	}
}

func (c *Client) doMultipart(method, path string, body interface{}, cfg *requestConfig) *Response {
	c.t.Helper()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add files
	for _, f := range cfg.files {
		file, err := os.Open(f.path)
		if err != nil {
			c.t.Fatalf("tapetest: failed to open file %s: %v", f.path, err)
		}
		part, err := writer.CreateFormFile(f.field, filepath.Base(f.path))
		if err != nil {
			file.Close()
			c.t.Fatalf("tapetest: failed to create form file: %v", err)
		}
		if _, err := io.Copy(part, file); err != nil {
			file.Close()
			c.t.Fatalf("tapetest: failed to read file: %v", err)
		}
		file.Close()
	}

	// Add form body fields from Form type
	if form, ok := body.(Form); ok {
		for k, v := range form {
			_ = writer.WriteField(k, fmt.Sprintf("%v", v))
		}
	} else if body != nil {
		// Fallback: add map[string]interface{} fields
		if m, ok := body.(map[string]interface{}); ok {
			for k, v := range m {
				_ = writer.WriteField(k, fmt.Sprintf("%v", v))
			}
		}
	}

	// Add form fields from config (set via Form.apply as Option)
	for k, vs := range cfg.formBody {
		for _, v := range vs {
			_ = writer.WriteField(k, v)
		}
	}

	writer.Close()
	cfg.headers["Content-Type"] = writer.FormDataContentType()

	bodyReader := bytes.NewReader(buf.Bytes())

	if c.handler != nil {
		return c.doHandler(method, path, bodyReader, cfg)
	}
	return c.doServer(method, path, bodyReader, cfg)
}

// --- Recording ---

// recordExchange captures the request/response for documentation generation.
func (c *Client) recordExchange(method, path string, body interface{}, cfg *requestConfig, resp *Response) {
	if !IsRecordingEnabled() {
		return
	}

	// Parse request body
	var reqBody interface{}
	if body != nil {
		switch b := body.(type) {
		case Form:
			reqBody = map[string]interface{}(b)
		case Json:
			reqBody = map[string]interface{}(b)
		default:
			// Try to unmarshal as JSON
			var parsed interface{}
			if err := json.Unmarshal(resp.body, &parsed); err == nil {
				_ = parsed
			}
			reqBody = body
		}
	}

	// For multipart/form-data requests, the form fields and file uploads are
	// not represented by the `body` argument (they live on cfg). Reconstruct
	// a recordable body so the OpenAPI generator can render the multipart
	// request body and its file fields.
	var files map[string]string
	if ct := cfg.headers["Content-Type"]; strings.HasPrefix(ct, "multipart/form-data") {
		formFields := make(map[string]interface{})
		for k, vs := range cfg.formBody {
			if len(vs) > 0 {
				formFields[k] = vs[0]
			}
		}
		// Merge any body that was passed as a Form
		if form, ok := body.(Form); ok {
			for k, v := range form {
				formFields[k] = v
			}
		}
		if len(formFields) > 0 {
			reqBody = formFields
		}
		if len(cfg.files) > 0 {
			files = make(map[string]string, len(cfg.files))
			for _, f := range cfg.files {
				files[f.field] = filepath.Base(f.path)
			}
		}
	}

	// Parse response body
	var respBody interface{}
	if len(resp.body) > 0 {
		_ = json.Unmarshal(resp.body, &respBody)
	}

	// Build headers maps
	reqHeaders := make(map[string]string)
	for k, v := range cfg.headers {
		reqHeaders[k] = v
	}
	respHeaders := make(map[string]string)
	for k, vals := range resp.headers {
		if len(vals) > 0 {
			respHeaders[k] = vals[0]
		}
	}

	req := RecordedRequest{
		Method:  method,
		Path:    path,
		Headers: reqHeaders,
		Body:    reqBody,
		Query:   cfg.query,
		Files:   files,
	}
	rec := RecordedResponse{
		Status:  resp.code,
		Headers: respHeaders,
		Body:    respBody,
	}

	// Tag the recording with per-service server metadata (set via Server)
	// so the OpenAPI generator can emit per-operation servers.
	RecordExchange(RecordedExchange{
		Test:      c.t.Name(),
		Request:   req,
		Response:  rec,
		Server:    c.serverName,
		ServerURL: c.serverURL,
	})
}

// --- Shared Headers and Cookies ---

// Header sets a shared header that persists across all requests.
// Use nil to remove a shared header.
//
//	c.Header("Authorization", "Bearer token")
//	c.Header("Authorization", nil) // removes it
func (c *Client) Header(key string, value interface{}) {
	if value == nil {
		delete(c.sharedHeaders, key)
	} else {
		c.sharedHeaders[key] = fmt.Sprintf("%v", value)
	}
}

// Cookie sets a shared cookie that persists across all requests.
// Use nil to remove a shared cookie.
//
//	c.Cookie("session_id", "abc123")
//	c.Cookie("session_id", nil) // removes it
func (c *Client) Cookie(key string, value interface{}) {
	if value == nil {
		delete(c.sharedCookies, key)
	} else {
		c.sharedCookies[key] = fmt.Sprintf("%v", value)
	}
}
