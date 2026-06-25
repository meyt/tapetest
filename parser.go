package tapetest

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// HandlerAnnotation represents parsed documentation from a Go handler function.
// These are extracted from comment annotations above handler functions and use
// go-swag-compatible directives:
//
//	// @Summary Get a todo by ID
//	// @Description Returns a single todo item
//	// @Tags todos
//	// @Router /todos/{id} [get]
//	// @Security UserAuth
//	func (a *App) getTodo(c echo.Context) error { ... }
//
// For backwards compatibility the legacy directives @Title, @Tag, @Method and
// @Path are still recognised as aliases (see annotationAliases).
type HandlerAnnotation struct {
	Title       string   // @Summary (alias @Title)
	Description string   // @Description (multiple lines are concatenated)
	Tags        []string // @Tags (comma-separated; alias @Tag)
	Method      string   // @Router [method] (alias @Method)
	Path        string   // @Router path (alias @Path)
	Security    []string // @Security (can be multiple)
	FuncName    string   // Go function name
	File        string   // Source file path
}

// annotationRegexps defines the supported go-swag-compatible annotation keys,
// matched case-insensitively (matching go-swag's behaviour).
var annotationRegexps = struct {
	summary     *regexp.Regexp
	description *regexp.Regexp
	tags        *regexp.Regexp
	router      *regexp.Regexp
	security    *regexp.Regexp
}{
	summary:     regexp.MustCompile(`(?i)^@Summary\s+(.+)$`),
	description: regexp.MustCompile(`(?i)^@Description\s+(.+)$`),
	tags:        regexp.MustCompile(`(?i)^@Tags?\s+(.+)$`),
	router:      regexp.MustCompile(`(?i)^@Router\s+(\S+)\s+\[?([A-Za-z]+)\]?\s*$`),
	security:    regexp.MustCompile(`(?i)^@Security\s+(.+)$`),
}

// annotationAliases are legacy tapetest directives that map onto the
// go-swag-compatible fields. They are kept so existing code bases keep working.
var (
	aliasTitle  = regexp.MustCompile(`(?i)^@Title\s+(.+)$`)
	aliasTag    = regexp.MustCompile(`(?i)^@Tag\s+(.+)$`)
	aliasMethod = regexp.MustCompile(`(?i)^@Method\s+(.+)$`)
	aliasPath   = regexp.MustCompile(`(?i)^@Path\s+(.+)$`)
)

// securityDefinitionStartPatterns matches a securityDefinitions marker and
// returns the scheme type plus the name. The directive is matched
// case-insensitively to match go-swag's behaviour.
//
// go-swag directives:
//
//	// @securityDefinitions.basic BasicAuth
//	// @securityDefinitions.apikey ApiKeyAuth
//	// @securitydefinitions.oauth2.application OAuth2Application
//	// @securitydefinitions.oauth2.implicit OAuth2Implicit
//	// @securitydefinitions.oauth2.password OAuth2Password
//	// @securitydefinitions.oauth2.accessCode OAuth2AccessCode
//
// tapetest extensions (covering the remaining OpenAPI v3 scheme types that
// go-swag has no directive for):
//
//	// @securityDefinitions.bearer BearerAuth   (http + bearer)
//	// @securitydefinitions.openIdConnect OpenID (openIdConnect)
var securityDefinitionStartPatterns = []securityDefinitionMarker{
	{regexp.MustCompile(`(?i)^@securityDefinitions\.basic\s+(\w+)\s*$`), "basic", ""},
	{regexp.MustCompile(`(?i)^@securityDefinitions\.apikey\s+(\w+)\s*$`), "apiKey", ""},
	{regexp.MustCompile(`(?i)^@securityDefinitions\.bearer\s+(\w+)\s*$`), "bearer", ""},
	{regexp.MustCompile(`(?i)^@securitydefinitions\.oauth2\.application\s+(\w+)\s*$`), "oauth2", "application"},
	{regexp.MustCompile(`(?i)^@securitydefinitions\.oauth2\.implicit\s+(\w+)\s*$`), "oauth2", "implicit"},
	{regexp.MustCompile(`(?i)^@securitydefinitions\.oauth2\.password\s+(\w+)\s*$`), "oauth2", "password"},
	{regexp.MustCompile(`(?i)^@securitydefinitions\.oauth2\.accessCode\s+(\w+)\s*$`), "oauth2", "accessCode"},
	{regexp.MustCompile(`(?i)^@securitydefinitions\.openIdConnect\s+(\w+)\s*$`), "openIdConnect", ""},
}

type securityDefinitionMarker struct {
	re   *regexp.Regexp
	typ  string // "basic", "apiKey", "bearer", "oauth2", "openIdConnect"
	flow string // oauth2 flow name ("" for non-oauth2)
}

// securityPropertyPatterns match the sub-properties that follow a
// securityDefinitions marker (@in, @name, @description, @tokenUrl,
// @authorizationUrl, @bearerFormat, @openIdConnectUrl, @scope.<name>).
var securityPropertyPatterns = []securityPropertyPattern{
	{regexp.MustCompile(`(?i)^@in\s+(.+)$`), "in"},
	{regexp.MustCompile(`(?i)^@name\s+(.+)$`), "name"},
	{regexp.MustCompile(`(?i)^@description\s+(.+)$`), "description"},
	{regexp.MustCompile(`(?i)^@tokenUrl\s+(.+)$`), "tokenUrl"},
	{regexp.MustCompile(`(?i)^@authorizationUrl\s+(.+)$`), "authorizationUrl"},
	{regexp.MustCompile(`(?i)^@bearerFormat\s+(.+)$`), "bearerFormat"},
	{regexp.MustCompile(`(?i)^@openIdConnectUrl\s+(.+)$`), "openIdConnectUrl"},
}

var securityScopePattern = regexp.MustCompile(`(?i)^@scope\.(\S+)\s+(.+)$`)

// SecurityDefinition represents a parsed go-swag security scheme definition.
// These are declared once per project (typically in main.go) and are mapped
// to OpenAPI v3 components.securitySchemes entries.
//
//	// @securityDefinitions.apikey UserAuth
//	// @in header
//	// @name Authorization
//	// @description Admin panel user JWT token.
type SecurityDefinition struct {
	Name             string            // scheme name (e.g. "UserAuth")
	Type             string            // "basic", "apiKey", "bearer", "oauth2", or "openIdConnect"
	Flow             string            // oauth2 flow: application, implicit, password, accessCode
	In               string            // apiKey location: header, query, cookie
	HeaderName       string            // apiKey header/parameter name
	Description      string            // human-readable description
	TokenURL         string            // oauth2 tokenUrl
	AuthorizationURL string            // oauth2 authorizationUrl
	BearerFormat     string            // bearer token format (e.g. "JWT"); defaults to "JWT"
	OpenIDConnectURL string            // openIdConnect discovery URL
	Scopes           map[string]string // oauth2 scope -> description
}

// ParseAnnotations parses Go source files in the given directory
// and extracts handler annotations from function comments.
//
//	annotations, err := ParseAnnotations("./...")
func ParseAnnotations(dirPattern string) ([]HandlerAnnotation, error) {
	var annotations []HandlerAnnotation

	fset := token.NewFileSet()

	// Determine packages to parse
	pkgs, err := parser.ParseDir(fset, ".", nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("tapetest: failed to parse directory: %w", err)
	}

	for _, pkg := range pkgs {
		for filename, file := range pkg.Files {
			if !strings.HasSuffix(filename, ".go") {
				continue
			}
			if strings.HasSuffix(filename, "_test.go") {
				continue
			}

			fileAnnotations := parseFileAnnotations(file, filename)
			annotations = append(annotations, fileAnnotations...)
		}
	}

	return annotations, nil
}

// collectGoFiles walks dir recursively and returns the paths of all
// non-test Go files found within it.
func collectGoFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("tapetest: failed to read directory: %w", err)
	}
	return files, nil
}

// ParseAnnotationsFromDir parses all non-test Go files in the specified
// directory (recursively) for go-swag handler annotations.
func ParseAnnotationsFromDir(dir string) ([]HandlerAnnotation, error) {
	files, err := collectGoFiles(dir)
	if err != nil {
		return nil, err
	}

	var annotations []HandlerAnnotation
	for _, path := range files {
		fileAnnotations, err := parseFileAnnotation(path)
		if err != nil {
			continue // skip files that can't be parsed
		}
		annotations = append(annotations, fileAnnotations...)
	}

	return annotations, nil
}

// parseFileAnnotations extracts annotations from a parsed Go file's AST.
func parseFileAnnotations(file *ast.File, filename string) []HandlerAnnotation {
	var annotations []HandlerAnnotation

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		if fn.Doc == nil {
			continue
		}

		ann := HandlerAnnotation{
			FuncName: fn.Name.Name,
			File:     filename,
		}

		hasAnnotation := false
		for _, comment := range fn.Doc.List {
			for _, line := range strings.Split(comment.Text, "\n") {
				text := strings.TrimSpace(strings.TrimPrefix(line, "//"))
				if text == "" {
					continue
				}
				if applyAnnotationLine(&ann, text) {
					hasAnnotation = true
				}
			}
		}

		if hasAnnotation && ann.Method != "" && ann.Path != "" {
			annotations = append(annotations, ann)
		}
	}

	return annotations
}

// applyAnnotationLine applies a single (already trimmed) annotation line to the
// given annotation, recognising the go-swag-compatible directives and the
// legacy tapetest aliases. It reports whether the line matched a known
// directive.
func applyAnnotationLine(ann *HandlerAnnotation, text string) bool {
	if m := annotationRegexps.summary.FindStringSubmatch(text); len(m) == 2 {
		ann.Title = strings.TrimSpace(m[1])
		return true
	}
	if m := annotationRegexps.description.FindStringSubmatch(text); len(m) == 2 {
		val := strings.TrimSpace(m[1])
		// Multiple @Description lines are concatenated, matching go-swag.
		if ann.Description == "" {
			ann.Description = val
		} else {
			ann.Description = ann.Description + "\n" + val
		}
		return true
	}
	if m := annotationRegexps.tags.FindStringSubmatch(text); len(m) == 2 {
		// @Tags accepts a comma-separated list (go-swag). Each value is
		// trimmed; empty entries are ignored.
		for _, t := range strings.Split(m[1], ",") {
			if t = strings.TrimSpace(t); t != "" {
				ann.Tags = append(ann.Tags, t)
			}
		}
		return true
	}
	if m := annotationRegexps.router.FindStringSubmatch(text); len(m) == 3 {
		ann.Path = strings.TrimSpace(m[1])
		ann.Method = strings.ToUpper(strings.TrimSpace(m[2]))
		return true
	}
	if m := annotationRegexps.security.FindStringSubmatch(text); len(m) == 2 {
		ann.Security = append(ann.Security, strings.TrimSpace(m[1]))
		return true
	}

	// Legacy tapetest aliases (kept for backwards compatibility).
	if m := aliasTitle.FindStringSubmatch(text); len(m) == 2 {
		ann.Title = strings.TrimSpace(m[1])
		return true
	}
	if m := aliasTag.FindStringSubmatch(text); len(m) == 2 {
		ann.Tags = append(ann.Tags, strings.TrimSpace(m[1]))
		return true
	}
	if m := aliasMethod.FindStringSubmatch(text); len(m) == 2 {
		ann.Method = strings.ToUpper(strings.TrimSpace(m[1]))
		return true
	}
	if m := aliasPath.FindStringSubmatch(text); len(m) == 2 {
		ann.Path = strings.TrimSpace(m[1])
		return true
	}
	return false
}

// parseFileAnnotation extracts annotations from a single Go source file.
func parseFileAnnotation(path string) ([]HandlerAnnotation, error) {
	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	return parseFileAnnotations(file, path), nil
}

// securityPropertyPattern pairs a sub-property regex with its key.
type securityPropertyPattern struct {
	re  *regexp.Regexp
	key string
}

// ParseSecurityDefinitionsFromDir parses go-swag securityDefinitions
// annotations from all (non-test) Go files in the given directory.
//
//	defs, err := ParseSecurityDefinitionsFromDir(".")
func ParseSecurityDefinitionsFromDir(dir string) ([]SecurityDefinition, error) {
	files, err := collectGoFiles(dir)
	if err != nil {
		return nil, err
	}

	var defs []SecurityDefinition
	for _, path := range files {
		fileDefs, err := parseFileSecurityDefinitions(path)
		if err != nil {
			continue
		}
		defs = append(defs, fileDefs...)
	}

	return defs, nil
}

// ParseSecurityDefinitionsFromFiles parses go-swag securityDefinitions
// annotations from the provided Go source files.
func ParseSecurityDefinitionsFromFiles(files []string) ([]SecurityDefinition, error) {
	var defs []SecurityDefinition
	for _, f := range files {
		fileDefs, err := parseFileSecurityDefinitions(f)
		if err != nil {
			continue
		}
		defs = append(defs, fileDefs...)
	}
	return defs, nil
}

// parseFileSecurityDefinitions scans all comment groups (including
// free-floating/file-level comments) in a Go source file for go-swag
// securityDefinitions markers and their sub-properties.
func parseFileSecurityDefinitions(path string) ([]SecurityDefinition, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	return parseSecurityDefinitionsFromAST(file), nil
}

// parseSecurityDefinitionsFromAST walks every comment group in the AST and
// extracts security definitions. A marker line starts a new definition; any
// subsequent @in/@name/@description/@tokenUrl/@authorizationUrl/@scope.* lines
// are attached to the most recently started definition within that comment group.
func parseSecurityDefinitionsFromAST(file *ast.File) []SecurityDefinition {
	var defs []SecurityDefinition

	for _, group := range file.Comments {
		var current *SecurityDefinition

		for _, comment := range group.List {
			text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))

			lines := strings.Split(text, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}

				// Check for a security definition marker
				if matched, name, typ, flow := matchSecurityDefinition(line); matched {
					def := SecurityDefinition{
						Name:   name,
						Type:   typ,
						Flow:   flow,
						Scopes: map[string]string{},
					}
					defs = append(defs, def)
					current = &defs[len(defs)-1]
					continue
				}

				if current == nil {
					continue
				}

				// Sub-property lines
				if key, val, ok := matchSecurityProperty(line); ok {
					applySecurityProperty(current, key, val)
					continue
				}

				if scopes := securityScopePattern.FindStringSubmatch(line); len(scopes) == 3 {
					if current.Scopes == nil {
						current.Scopes = map[string]string{}
					}
					current.Scopes[scopes[1]] = strings.TrimSpace(scopes[2])
				}
			}
		}
	}

	// Clean up empty scopes maps
	for i := range defs {
		if len(defs[i].Scopes) == 0 {
			defs[i].Scopes = nil
		}
	}

	return defs
}

// matchSecurityDefinition checks whether a line is a securityDefinitions
// marker. Returns (matched, name, type, flow).
func matchSecurityDefinition(line string) (bool, string, string, string) {
	for _, m := range securityDefinitionStartPatterns {
		if matches := m.re.FindStringSubmatch(line); len(matches) == 2 {
			return true, matches[1], m.typ, m.flow
		}
	}
	return false, "", "", ""
}

// matchSecurityProperty checks whether a line is a known sub-property.
func matchSecurityProperty(line string) (string, string, bool) {
	for _, p := range securityPropertyPatterns {
		if matches := p.re.FindStringSubmatch(line); len(matches) == 2 {
			return p.key, strings.TrimSpace(matches[1]), true
		}
	}
	return "", "", false
}

// applySecurityProperty sets a sub-property on a security definition.
func applySecurityProperty(sd *SecurityDefinition, key, val string) {
	switch key {
	case "in":
		sd.In = val
	case "name":
		sd.HeaderName = val
	case "description":
		sd.Description = val
	case "tokenUrl":
		sd.TokenURL = val
	case "authorizationUrl":
		sd.AuthorizationURL = val
	case "bearerFormat":
		sd.BearerFormat = val
	case "openIdConnectUrl":
		sd.OpenIDConnectURL = val
	}
}
