package tapetest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// OpenAPI v3 document types for documentation generation.

type OpenAPIDocument struct {
	OpenAPI    string                                 `json:"openapi"`
	Info       OpenAPIInfo                            `json:"info"`
	Servers    []OpenAPIServer                        `json:"servers,omitempty"`
	Paths      map[string]map[string]OpenAPIOperation `json:"paths"`
	Components *OpenAPIComponents                     `json:"components,omitempty"`
}

type OpenAPIInfo struct {
	Title          string          `json:"title"`
	Description    string          `json:"description,omitempty"`
	TermsOfService string          `json:"termsOfService,omitempty"`
	Contact        *OpenAPIContact `json:"contact,omitempty"`
	License        *OpenAPILicense `json:"license,omitempty"`
	Version        string          `json:"version"`
}

// OpenAPIContact is the contact information object (info.contact).
type OpenAPIContact struct {
	Name  string `json:"name,omitempty"`
	URL   string `json:"url,omitempty"`
	Email string `json:"email,omitempty"`
}

// OpenAPILicense is the license information object (info.license).
type OpenAPILicense struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

type OpenAPIServer struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

type OpenAPIOperation struct {
	Summary     string                     `json:"summary,omitempty"`
	Description string                     `json:"description,omitempty"`
	OperationID string                     `json:"operationId,omitempty"`
	Tags        []string                   `json:"tags,omitempty"`
	Parameters  []OpenAPIParameter         `json:"parameters,omitempty"`
	RequestBody *OpenAPIRequestBody        `json:"requestBody,omitempty"`
	Responses   map[string]OpenAPIResponse `json:"responses"`
	Security    []map[string][]string      `json:"security,omitempty"`
	// Servers are per-operation server overrides. When present they take
	// precedence over the root-level servers for "Try it out", allowing each
	// operation to route to its own (relative) service URL.
	Servers []OpenAPIServer `json:"servers,omitempty"`
}

type OpenAPIParameter struct {
	Name        string         `json:"name"`
	In          string         `json:"in"`
	Description string         `json:"description,omitempty"`
	Required    bool           `json:"required"`
	Schema      *OpenAPISchema `json:"schema,omitempty"`
}

type OpenAPIRequestBody struct {
	Description string                      `json:"description,omitempty"`
	Content     map[string]OpenAPIMediaType `json:"content"`
	Required    bool                        `json:"required,omitempty"`
}

type OpenAPIMediaType struct {
	Schema   *OpenAPISchema   `json:"schema,omitempty"`
	Examples *OpenAPIExamples `json:"examples,omitempty"`
}

type OpenAPIExample struct {
	Summary string      `json:"summary,omitempty"`
	Value   interface{} `json:"value,omitempty"`
}

// OpenAPIExamples is an insertion-ordered collection of named OpenAPI examples.
// It marshals to a JSON object that preserves insertion order, so documentation
// tools that honor key order (such as swagger-ui) render the examples in the
// intended sequence.
type OpenAPIExamples struct {
	keys []string
	vals map[string]OpenAPIExample
}

// NewOpenAPIExamples creates an empty ordered example set.
func NewOpenAPIExamples() *OpenAPIExamples {
	return &OpenAPIExamples{vals: make(map[string]OpenAPIExample)}
}

// Set adds or replaces the named example, preserving the position of existing keys.
func (e *OpenAPIExamples) Set(key string, ex OpenAPIExample) {
	if e.vals == nil {
		e.vals = make(map[string]OpenAPIExample)
	}
	if _, ok := e.vals[key]; !ok {
		e.keys = append(e.keys, key)
	}
	e.vals[key] = ex
}

// Len returns the number of examples.
func (e *OpenAPIExamples) Len() int { return len(e.keys) }

// MarshalJSON renders the examples as an ordered JSON object.
func (e OpenAPIExamples) MarshalJSON() ([]byte, error) {
	var buf strings.Builder
	buf.WriteByte('{')
	for i, k := range e.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		vb, err := json.Marshal(e.vals[k])
		if err != nil {
			return nil, err
		}
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return []byte(buf.String()), nil
}

type OpenAPIResponse struct {
	Description string                      `json:"description"`
	Content     map[string]OpenAPIMediaType `json:"content,omitempty"`
}

type OpenAPISchema struct {
	Type        string                   `json:"type,omitempty"`
	Format      string                   `json:"format,omitempty"`
	Properties  map[string]OpenAPISchema `json:"properties,omitempty"`
	Items       *OpenAPISchema           `json:"items,omitempty"`
	Required    []string                 `json:"required,omitempty"`
	Description string                   `json:"description,omitempty"`
}

type OpenAPIComponents struct {
	Schemas         map[string]OpenAPISchema         `json:"schemas,omitempty"`
	SecuritySchemes map[string]OpenAPISecurityScheme `json:"securitySchemes,omitempty"`
}

// OpenAPISecurityScheme describes an authentication scheme in
// components.securitySchemes (OpenAPI v3).
type OpenAPISecurityScheme struct {
	Type             string                         `json:"type"`
	Scheme           string                         `json:"scheme,omitempty"`
	Description      string                         `json:"description,omitempty"`
	BearerFormat     string                         `json:"bearerFormat,omitempty"`
	In               string                         `json:"in,omitempty"`
	Name             string                         `json:"name,omitempty"`
	OpenIDConnectURL string                         `json:"openIdConnectUrl,omitempty"`
	Flows            map[string]OpenAPISecurityFlow `json:"flows,omitempty"`
}

// OpenAPISecurityFlow describes a single OAuth2 flow.
type OpenAPISecurityFlow struct {
	AuthorizationURL string            `json:"authorizationUrl,omitempty"`
	TokenURL         string            `json:"tokenUrl,omitempty"`
	Scopes           map[string]string `json:"scopes,omitempty"`
}

// NewOpenAPIDocument creates a new OpenAPI v3 document with sensible defaults.
func NewOpenAPIDocument(title, version string) *OpenAPIDocument {
	return &OpenAPIDocument{
		OpenAPI: "3.0.3",
		Info: OpenAPIInfo{
			Title:   title,
			Version: version,
		},
		Paths: make(map[string]map[string]OpenAPIOperation),
	}
}

// AddOperation adds an operation to the OpenAPI document.
func (doc *OpenAPIDocument) AddOperation(path, method string, op OpenAPIOperation) {
	if doc.Paths[path] == nil {
		doc.Paths[path] = make(map[string]OpenAPIOperation)
	}
	doc.Paths[path][method] = op
}

// AddServer adds a server URL to the OpenAPI document.
func (doc *OpenAPIDocument) AddServer(url, description string) {
	doc.Servers = append(doc.Servers, OpenAPIServer{
		URL:         url,
		Description: description,
	})
}

// ApplyGeneralAPIInfo applies go-swag "General API Info" metadata to the
// document's info object. Any non-empty field in `info` overrides the
// existing document value, mirroring go-swag where source annotations take
// precedence over CLI/flag defaults. Contact and License are only set when at
// least one of their sub-fields is present.
func (doc *OpenAPIDocument) ApplyGeneralAPIInfo(info GeneralAPIInfo) {
	if info.Title != "" {
		doc.Info.Title = info.Title
	}
	if info.Version != "" {
		doc.Info.Version = info.Version
	}
	if info.Description != "" {
		doc.Info.Description = info.Description
	}
	if info.TermsOfService != "" {
		doc.Info.TermsOfService = info.TermsOfService
	}
	if info.ContactName != "" || info.ContactURL != "" || info.ContactEmail != "" {
		doc.Info.Contact = &OpenAPIContact{
			Name:  info.ContactName,
			URL:   info.ContactURL,
			Email: info.ContactEmail,
		}
	}
	if info.LicenseName != "" {
		doc.Info.License = &OpenAPILicense{
			Name: info.LicenseName,
			URL:  info.LicenseURL,
		}
	}
}

// WriteJSON writes the OpenAPI document as JSON to the given file path.
func (doc *OpenAPIDocument) WriteJSON(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("tapetest: failed to create docs dir: %w", err)
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("tapetest: failed to marshal OpenAPI doc: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("tapetest: failed to write OpenAPI doc: %w", err)
	}

	return nil
}

// --- Generation ---

// endpointGroup collects all recordings that map to the same OpenAPI path+method.
type endpointGroup struct {
	openAPIPath      string
	method           string
	annotation       *HandlerAnnotation
	recordings       []RecordedExchange
	pathParams       map[string]string // from first matched recording
	readableExamples bool              // transform test names to readable format
}

// GenerateOpenAPIFromRecordings generates an OpenAPI document from recorded test exchanges
// and handler annotations. Test paths are matched to annotation templates and grouped.
// Each test case appears as an example with its actual request/response.
//
// basePath is the common URL prefix shared by all routes (e.g. "/api/v1",
// from the go-swag @BasePath directive). It is stripped from recorded request
// paths before matching against annotation templates so that recorded paths
// like "/api/v1/users/1" line up with templates like "/users/:id".
func GenerateOpenAPIFromRecordings(
	exchanges []RecordedExchange,
	annotations []HandlerAnnotation,
	securityDefs []SecurityDefinition,
	basePath, title, version string,
	readableExamples bool,
) *OpenAPIDocument {
	doc := NewOpenAPIDocument(title, version)

	// Build annotation lookup by method, sorted: static paths first, parameterized last
	annotationsByMethod := make(map[string][]HandlerAnnotation)
	for _, ann := range annotations {
		method := strings.ToLower(ann.Method)
		annotationsByMethod[method] = append(annotationsByMethod[method], ann)
	}
	for method, anns := range annotationsByMethod {
		sortAnnotations(anns)
		annotationsByMethod[method] = anns
	}

	// Group recordings by matched endpoint
	groups := make(map[string]*endpointGroup) // key: method+" "+openapiPath

	for _, ex := range exchanges {
		method := strings.ToLower(ex.Request.Method)
		testPath := ex.Request.Path

		// Strip the shared global base path prefix (e.g. "/api/v1", from the
		// go-swag @BasePath directive) so that recorded paths match the
		// relative annotation templates.
		if basePath != "" {
			testPath = strings.TrimPrefix(testPath, basePath)
			if testPath == "" {
				testPath = "/"
			}
		}

		// Strip a relative per-service server URL (the path prefix) so the
		// OpenAPI path is relative to the operation-level server. This makes
		// "/api/v1/users" + server "/api/v1" resolve to "/users" at the
		// operation, and lets recordings from different services that share a
		// relative path (e.g. Admin "/api/v1/users" and User "/api/v2/users")
		// merge into a single "/users" operation with multiple servers.
		//
		// Absolute server URLs (e.g. "https://user-api.example.com") are left
		// untouched: they carry no path prefix to remove and should be used
		// together with BaseUrl for the path prefix.
		if ex.ServerURL != "" && !IsAbsoluteURL(ex.ServerURL) {
			stripped := strings.TrimPrefix(testPath, ex.ServerURL)
			if stripped == "" {
				stripped = "/"
			}
			testPath = stripped
		}

		// Find matching annotation
		var matchedAnn *HandlerAnnotation
		var openAPIPath string
		var pathParams map[string]string

		for i, ann := range annotationsByMethod[method] {
			if matched, params := matchTestPathToTemplate(testPath, ann.Path); matched {
				matchedAnn = &annotationsByMethod[method][i]
				openAPIPath = templateToOpenAPIPath(ann.Path)
				pathParams = params
				break
			}
		}

		// Fallback: use test path as-is
		if openAPIPath == "" {
			openAPIPath = testPath
		}

		key := method + " " + openAPIPath
		if g, ok := groups[key]; ok {
			g.recordings = append(g.recordings, ex)
		} else {
			groups[key] = &endpointGroup{
				openAPIPath:      openAPIPath,
				method:           method,
				annotation:       matchedAnn,
				recordings:       []RecordedExchange{ex},
				pathParams:       pathParams,
				readableExamples: readableExamples,
			}
		}
	}

	// Build operations from groups
	for _, g := range groups {
		// Apply DocOrder filtering/sorting at the group level so that the
		// primary recording (path/query params, request body schema) reflects
		// the documented priority. Skip endpoints whose recordings are all
		// excluded via DocOrder(nil).
		ordered := OrderedRecordings(g.recordings)
		if len(ordered) == 0 {
			continue
		}
		g.recordings = ordered
		op := buildOperation(g)
		doc.AddOperation(g.openAPIPath, g.method, op)
	}

	// Add security schemes to components if any operations use them
	if doc.hasSecurityRequirements() {
		doc.Components = &OpenAPIComponents{
			SecuritySchemes: buildSecuritySchemes(securityDefs),
		}
	}

	return doc
}

// buildSecuritySchemes converts parsed go-swag SecurityDefinitions into
// OpenAPI v3 securitySchemes. When no definitions are declared, a sensible
// default JWT bearer scheme is provided so referenced schemes resolve.
func buildSecuritySchemes(defs []SecurityDefinition) map[string]OpenAPISecurityScheme {
	schemes := make(map[string]OpenAPISecurityScheme)

	if len(defs) > 0 {
		for _, sd := range defs {
			if sd.Name == "" {
				continue
			}
			schemes[sd.Name] = sd.ToOpenAPISecurityScheme()
		}
		return schemes
	}

	// Fallback default when no definitions are declared
	schemes["UserAuth"] = OpenAPISecurityScheme{
		Type:         "http",
		Scheme:       "bearer",
		BearerFormat: "JWT",
		Description:  "User authentication using Bearer token",
	}
	return schemes
}

// ToOpenAPISecurityScheme converts a parsed go-swag SecurityDefinition into an
// OpenAPI v3 security scheme object. go-swag flow names are mapped to their
// OpenAPI equivalents (application -> clientCredentials, accessCode -> authorizationCode).
func (sd SecurityDefinition) ToOpenAPISecurityScheme() OpenAPISecurityScheme {
	scheme := OpenAPISecurityScheme{
		Description: sd.Description,
	}

	switch sd.Type {
	case "basic":
		scheme.Type = "http"
		scheme.Scheme = "basic"
	case "bearer":
		scheme.Type = "http"
		scheme.Scheme = "bearer"
		if sd.BearerFormat != "" {
			scheme.BearerFormat = sd.BearerFormat
		} else {
			scheme.BearerFormat = "JWT"
		}
	case "openIdConnect":
		scheme.Type = "openIdConnect"
		scheme.OpenIDConnectURL = sd.OpenIDConnectURL
	case "apiKey":
		scheme.Type = "apiKey"
		scheme.In = sd.In
		scheme.Name = sd.HeaderName
	case "oauth2":
		scheme.Type = "oauth2"
		flow := OpenAPISecurityFlow{Scopes: sd.Scopes}

		// Map go-swag flow names to OpenAPI v3 flow keys
		switch sd.Flow {
		case "application":
			flow.TokenURL = sd.TokenURL
			scheme.Flows = map[string]OpenAPISecurityFlow{"clientCredentials": flow}
		case "implicit":
			flow.AuthorizationURL = sd.AuthorizationURL
			scheme.Flows = map[string]OpenAPISecurityFlow{"implicit": flow}
		case "password":
			flow.TokenURL = sd.TokenURL
			scheme.Flows = map[string]OpenAPISecurityFlow{"password": flow}
		case "accessCode":
			flow.AuthorizationURL = sd.AuthorizationURL
			flow.TokenURL = sd.TokenURL
			scheme.Flows = map[string]OpenAPISecurityFlow{"authorizationCode": flow}
		}
	}

	return scheme
}

// hasSecurityRequirements checks if any operation in the document has security requirements
func (doc *OpenAPIDocument) hasSecurityRequirements() bool {
	for _, pathMethods := range doc.Paths {
		for _, operation := range pathMethods {
			if len(operation.Security) > 0 {
				return true
			}
		}
	}
	return false
}

// groupServers collects the distinct per-service servers from a group's
// recordings, preserving first-seen order. An exchange only contributes a
// server when it carries a ServerURL. Recordings without server metadata are
// ignored, so a mixed suite still works.
func groupServers(recs []RecordedExchange) []OpenAPIServer {
	var servers []OpenAPIServer
	seen := make(map[string]bool)
	for _, rec := range recs {
		if rec.ServerURL == "" || seen[rec.ServerURL] {
			continue
		}
		seen[rec.ServerURL] = true
		servers = append(servers, OpenAPIServer{
			URL:         rec.ServerURL,
			Description: rec.Server,
		})
	}
	return servers
}

// buildOperation creates an OpenAPI operation from an endpoint group.
func buildOperation(g *endpointGroup) OpenAPIOperation {
	op := OpenAPIOperation{
		Responses: make(map[string]OpenAPIResponse),
	}

	// Apply annotation metadata
	if g.annotation != nil {
		op.Summary = g.annotation.Title
		op.Description = g.annotation.Description
		op.Tags = g.annotation.Tags

		// Add security requirements from annotations
		if len(g.annotation.Security) > 0 {
			for _, securityScheme := range g.annotation.Security {
				op.Security = append(op.Security, map[string][]string{securityScheme: {}})
			}
		}
	}

	// Generate operation ID
	op.OperationID = generateOperationID(g.method, g.openAPIPath)

	// Per-operation servers: when recordings carry service metadata
	// (Server/ServerURL), emit a servers array so "Try it out" can route
	// each endpoint to its own backend.
	if servers := groupServers(g.recordings); len(servers) > 0 {
		op.Servers = servers
	}

	// Add path parameters
	for paramName := range g.pathParams {
		op.Parameters = append(op.Parameters, OpenAPIParameter{
			Name:     paramName,
			In:       "path",
			Required: true,
			Schema: &OpenAPISchema{
				Type:        "string",
				Description: "string",
			},
		})
	}
	// Sort parameters for consistent output
	sort.Slice(op.Parameters, func(i, j int) bool {
		return op.Parameters[i].Name < op.Parameters[j].Name
	})

	// Add query parameters from all recordings, deduplicating by name.
	queryParams := make(map[string]bool)
	for _, rec := range g.recordings {
		for qKey := range rec.Request.Query {
			if queryParams[qKey] {
				continue
			}
			queryParams[qKey] = true
			op.Parameters = append(op.Parameters, OpenAPIParameter{
				Name:     qKey,
				In:       "query",
				Required: false,
				Schema: &OpenAPISchema{
					Type:        "string",
					Description: "string",
				},
			})
		}
	}

	// Build request body. A single endpoint may accept several content types
	// (e.g. application/json and multipart/form-data); each one becomes a
	// separate entry under requestBody.content so swagger-ui offers a media
	// type selector in the "Try it out" panel.
	op.RequestBody = buildRequestBody(g)

	// Build responses grouped by status code
	responsesByStatus := make(map[string][]RecordedExchange)
	for _, rec := range g.recordings {
		statusStr := fmt.Sprintf("%d", rec.Response.Status)
		responsesByStatus[statusStr] = append(responsesByStatus[statusStr], rec)
	}

	for statusStr, recs := range responsesByStatus {
		response := OpenAPIResponse{
			Description: httpStatusText(mustAtoi(statusStr)),
		}

		// Build response content with examples (ordered by DocOrder)
		content := OpenAPIMediaType{
			Schema: &OpenAPISchema{
				Type: "object",
			},
			Examples: NewOpenAPIExamples(),
		}

		for _, rec := range OrderedRecordings(recs) {
			if rec.Response.Body != nil {
				content.Examples.Set(rec.Test, OpenAPIExample{
					Summary: formatExampleSummary(rec.Test, g.readableExamples),
					Value:   rec.Response.Body,
				})
			}
		}

		if content.Examples.Len() > 0 {
			response.Content = map[string]OpenAPIMediaType{
				"application/json": content,
			}
		}

		op.Responses[statusStr] = response
	}

	return op
}

// buildRequestBody builds a request body that may declare several media types.
// Recordings are grouped by their (normalized) request content type, and each
// group produces one entry under requestBody.content. This lets swagger-ui show
// a media-type selector (e.g. application/json and multipart/form-data) in the
// "Try it out" panel. Returns nil if no recording carried a body or files.
func buildRequestBody(g *endpointGroup) *OpenAPIRequestBody {
	// Group recordings by normalized content type, preserving first-seen order.
	var order []string
	groupsByType := make(map[string][]RecordedExchange)
	for _, rec := range g.recordings {
		// Only consider recordings that actually carry a body or uploaded files.
		if rec.Request.Body == nil && len(rec.Request.Files) == 0 {
			continue
		}
		ct := normalizeContentType(rec.Request.Headers["Content-Type"])
		if _, ok := groupsByType[ct]; !ok {
			order = append(order, ct)
		}
		groupsByType[ct] = append(groupsByType[ct], rec)
	}
	if len(order) == 0 {
		return nil
	}

	content := make(map[string]OpenAPIMediaType, len(order))
	for _, ct := range order {
		recs := groupsByType[ct]
		switch ct {
		case "application/x-www-form-urlencoded":
			content[ct] = buildFormMediaType(recs, g.readableExamples)
		case "multipart/form-data":
			content[ct] = buildMultipartMediaType(recs, g.readableExamples)
		default:
			// Treat unknown content types (including application/json) as JSON.
			content[ct] = buildJSONMediaType(recs, g.readableExamples)
		}
	}

	return &OpenAPIRequestBody{Content: content}
}

// normalizeContentType collapses content-type header values (which may include
// parameters like "; charset=utf-8" or multipart boundaries) to a single
// canonical media type. Empty values default to application/json.
func normalizeContentType(ct string) string {
	ct = strings.TrimSpace(ct)
	switch {
	case ct == "":
		return "application/json"
	case strings.HasPrefix(ct, "multipart/form-data"):
		return "multipart/form-data"
	}
	if i := strings.Index(ct, ";"); i >= 0 {
		return strings.TrimSpace(ct[:i])
	}
	return ct
}

// buildJSONMediaType builds a JSON object media type from recorded bodies.
func buildJSONMediaType(recs []RecordedExchange, readableExamples bool) OpenAPIMediaType {
	properties := make(map[string]OpenAPISchema)
	examples := NewOpenAPIExamples()

	for _, rec := range OrderedRecordings(recs) {
		bodyMap, ok := rec.Request.Body.(map[string]interface{})
		if !ok {
			continue
		}
		for k, v := range bodyMap {
			if _, exists := properties[k]; !exists {
				properties[k] = OpenAPISchema{
					Type:        guessSchemaTypeFromValue(v),
					Description: guessSchemaTypeFromValue(v),
				}
			}
		}
		examples.Set(rec.Test, OpenAPIExample{
			Summary: formatExampleSummary(rec.Test, readableExamples),
			Value:   rec.Request.Body,
		})
	}

	return OpenAPIMediaType{
		Schema: &OpenAPISchema{
			Type:       "object",
			Properties: properties,
			Required:   sortedKeys(properties),
		},
		Examples: examples,
	}
}

// buildFormMediaType builds an application/x-www-form-urlencoded media type
// with editable string fields.
func buildFormMediaType(recs []RecordedExchange, readableExamples bool) OpenAPIMediaType {
	properties := make(map[string]OpenAPISchema)
	examples := NewOpenAPIExamples()

	for _, rec := range OrderedRecordings(recs) {
		bodyMap, ok := rec.Request.Body.(map[string]interface{})
		if !ok {
			continue
		}
		for k, v := range bodyMap {
			if _, exists := properties[k]; !exists {
				properties[k] = OpenAPISchema{
					Type:        guessSchemaTypeFromValue(v),
					Description: guessSchemaTypeFromValue(v),
				}
			}
		}
		examples.Set(rec.Test, OpenAPIExample{
			Summary: formatExampleSummary(rec.Test, readableExamples),
			Value:   rec.Request.Body,
		})
	}

	return OpenAPIMediaType{
		Schema: &OpenAPISchema{
			Type:       "object",
			Properties: properties,
			Required:   sortedKeys(properties),
		},
		Examples: examples,
	}
}

// buildMultipartMediaType builds a multipart/form-data media type. Regular form
// fields become string properties, while recorded file uploads become
// { type: string, format: binary } properties so swagger-ui renders file
// chooser inputs in the "Try it out" panel.
func buildMultipartMediaType(recs []RecordedExchange, readableExamples bool) OpenAPIMediaType {
	properties := make(map[string]OpenAPISchema)
	examples := NewOpenAPIExamples()

	for _, rec := range OrderedRecordings(recs) {
		if bodyMap, ok := rec.Request.Body.(map[string]interface{}); ok {
			for k, v := range bodyMap {
				if _, exists := properties[k]; !exists {
					properties[k] = OpenAPISchema{
						Type:        guessSchemaTypeFromValue(v),
						Description: guessSchemaTypeFromValue(v),
					}
				}
			}
		}
		// File upload fields -> binary string. These override any string field
		// with the same name so the UI shows a file chooser.
		for field := range rec.Request.Files {
			properties[field] = OpenAPISchema{
				Type:        "string",
				Format:      "binary",
				Description: "file",
			}
		}
		examples.Set(rec.Test, OpenAPIExample{
			Summary: formatExampleSummary(rec.Test, readableExamples),
			Value:   multipartExampleValue(rec),
		})
	}

	return OpenAPIMediaType{
		Schema: &OpenAPISchema{
			Type:       "object",
			Properties: properties,
			Required:   sortedKeys(properties),
		},
		Examples: examples,
	}
}

// multipartExampleValue builds a human-readable example for a multipart body:
// form field values plus "<filename>" placeholders for uploaded files.
func multipartExampleValue(rec RecordedExchange) map[string]interface{} {
	out := make(map[string]interface{})
	if bodyMap, ok := rec.Request.Body.(map[string]interface{}); ok {
		for k, v := range bodyMap {
			out[k] = v
		}
	}
	for field, filename := range rec.Request.Files {
		out[field] = "<" + filename + ">"
	}
	return out
}

// sortedKeys returns the keys of a map[string]OpenAPISchema in sorted order.
func sortedKeys(m map[string]OpenAPISchema) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// OrderedRecordings returns the recordings that should be documented,
// excluding any flagged via DocOrder(nil), and sorted by their DocOrder:
//
//   - 0 and positive values come first (ascending), so DocOrder(0) is first.
//   - recordings without an explicit DocOrder keep their natural order in the middle.
//   - negative values come last (ascending), so DocOrder(-1) is the final example.
//
// Recordings with the same priority preserve their original relative order.
func OrderedRecordings(recs []RecordedExchange) []RecordedExchange {
	if len(recs) == 0 {
		return recs
	}
	out := make([]RecordedExchange, 0, len(recs))
	for _, r := range recs {
		if r.ExcludeFromDocs {
			continue
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return docOrderLess(out[i], out[j])
	})
	return out
}

// docOrderLess compares two recordings by their DocOrder priority.
func docOrderLess(a, b RecordedExchange) bool {
	ta, va := docOrderTier(a)
	tb, vb := docOrderTier(b)
	if ta != tb {
		return ta < tb
	}
	return va < vb
}

// docOrderTier maps a recording's DocOrder into a (tier, value) sort key.
//
//	tier 0: explicit non-negative order (first examples), value = order
//	tier 1: unset order (natural middle), value = 0
//	tier 2: explicit negative order (last examples), value = order
func docOrderTier(rec RecordedExchange) (tier int, value int) {
	if rec.DocOrder == nil {
		return 1, 0
	}
	v := *rec.DocOrder
	if v >= 0 {
		return 0, v
	}
	return 2, v
}

// --- Path Matching Helpers ---

// matchTestPathToTemplate checks if a test path matches a template path.
// Template paths use :param or {param} for parameters.
func matchTestPathToTemplate(testPath, templatePath string) (bool, map[string]string) {
	testParts := splitPath(testPath)
	templateParts := splitPath(templatePath)

	if len(testParts) != len(templateParts) {
		return false, nil
	}

	params := make(map[string]string)
	for i := range testParts {
		tp := templateParts[i]
		if strings.HasPrefix(tp, ":") {
			params[tp[1:]] = testParts[i]
		} else if strings.HasPrefix(tp, "{") && strings.HasSuffix(tp, "}") {
			params[tp[1:len(tp)-1]] = testParts[i]
		} else if tp != testParts[i] {
			return false, nil
		}
	}

	return true, params
}

// templateToOpenAPIPath converts :param to {param} in a path template.
func templateToOpenAPIPath(templatePath string) string {
	parts := splitPath(templatePath)
	for i, part := range parts {
		if strings.HasPrefix(part, ":") {
			parts[i] = "{" + part[1:] + "}"
		}
	}
	if len(parts) == 0 {
		return "/"
	}
	return "/" + strings.Join(parts, "/")
}

// IsAbsoluteURL reports whether the given server URL is absolute (has a
// scheme such as "http://" or "https://"). Absolute server URLs are emitted
// verbatim as operation-level servers and are never used to strip a recorded
// request path; only relative server URLs (e.g. "/api/v1") are treated as a
// path prefix to strip.
func IsAbsoluteURL(serverURL string) bool {
	lower := strings.ToLower(serverURL)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// splitPath splits a path into segments, ignoring leading/trailing slashes.
func splitPath(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

// sortAnnotations sorts annotations so that static paths are matched before
// parameterized paths. This prevents /todos/search from matching /todos/:id.
func sortAnnotations(anns []HandlerAnnotation) {
	sort.SliceStable(anns, func(i, j int) bool {
		iParams := countPathParams(anns[i].Path)
		jParams := countPathParams(anns[j].Path)
		if iParams != jParams {
			return iParams < jParams // fewer params = more specific = first
		}
		// Same param count: sort alphabetically for determinism
		return anns[i].Path < anns[j].Path
	})
}

// countPathParams returns the number of parameterized segments in a path.
func countPathParams(path string) int {
	parts := splitPath(path)
	count := 0
	for _, p := range parts {
		if strings.HasPrefix(p, ":") || (strings.HasPrefix(p, "{") && strings.HasSuffix(p, "}")) {
			count++
		}
	}
	return count
}

// generateOperationID creates an operation ID from method and path.
func generateOperationID(method, path string) string {
	parts := splitPath(path)
	for i, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			parts[i] = "By" + strings.Title(part[1:len(part)-1])
		} else if strings.HasPrefix(part, ":") {
			parts[i] = "By" + strings.Title(part[1:])
		} else {
			parts[i] = strings.Title(part)
		}
	}
	return method + strings.Join(parts, "")
}

// guessSchemaTypeFromValue returns an OpenAPI type based on a Go value.
func guessSchemaTypeFromValue(v interface{}) string {
	switch v.(type) {
	case int, int64, int32, float64, float32:
		return "integer"
	case bool:
		return "boolean"
	default:
		return "string"
	}
}

// httpStatusText returns the status text for a code.
func httpStatusText(code int) string {
	statusTexts := map[int]string{
		200: "OK", 201: "Created", 204: "No Content",
		400: "Bad Request", 401: "Unauthorized", 403: "Forbidden",
		404: "Not Found", 405: "Method Not Allowed",
		500: "Internal Server Error", 502: "Bad Gateway", 503: "Service Unavailable",
	}
	if text, ok := statusTexts[code]; ok {
		return text
	}
	return fmt.Sprintf("Status %d", code)
}

// mustAtoi converts string to int, returns 0 on error.
func mustAtoi(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

// formatExampleSummary returns a readable summary for a test example.
// When readableExamples is true, transforms "TestCreateTodo" → "Create Todo".
// When false, returns the raw test name.
func formatExampleSummary(testName string, readableExamples bool) string {
	if readableExamples {
		return transformTestName(testName)
	}
	return testName
}

// transformTestName converts a Go test function name to a readable title.
// "TestCreateTodo" → "Create Todo", "TestGetTodoNotFound" → "Get Todo Not Found",
// "TestFullCRUDWorkflow" → "Full CRUD Workflow"
func transformTestName(name string) string {
	// Remove "Test" prefix
	name = strings.TrimPrefix(name, "Test")

	if name == "" {
		return "Test"
	}

	// Split camelCase into words, keeping acronyms like "CRUD" together
	var words []string
	var currentWord []byte
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c >= 'A' && c <= 'Z' {
			if len(currentWord) > 0 {
				prev := currentWord[len(currentWord)-1]
				if prev >= 'a' && prev <= 'z' {
					// lowercase → uppercase: always a word boundary
					words = append(words, string(currentWord))
					currentWord = []byte{c}
				} else if i+1 < len(name) && name[i+1] >= 'a' && name[i+1] <= 'z' {
					// uppercase → uppercase+lowercase: boundary (e.g., "CRUDW"|"orkflow")
					words = append(words, string(currentWord))
					currentWord = []byte{c}
				} else {
					// uppercase → uppercase+uppercase or end: same acronym
					currentWord = append(currentWord, c)
				}
			} else {
				currentWord = []byte{c}
			}
		} else {
			currentWord = append(currentWord, c)
		}
	}
	if len(currentWord) > 0 {
		words = append(words, string(currentWord))
	}

	return strings.Join(words, " ")
}
