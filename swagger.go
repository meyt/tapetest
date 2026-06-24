package tapetest

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// SwaggerUIOption defines a single swagger-ui configuration option.
type SwaggerUIOption struct {
	Name    string // camelCase swagger-ui property name
	Type    string // "bool", "string", "int"
	Default string // default value as string
	Help    string // description for CLI help
}

// SwaggerUIOptions is the complete list of supported swagger-ui configuration
// options. Used to build a SwaggerUIConfig via BuildSwaggerConfig.
var SwaggerUIOptions = []SwaggerUIOption{
	// Boolean options
	{"syntaxHighlight", "bool", "true", "Enable syntax highlighting in responses"},
	{"deepLinking", "bool", "true", "Enable deep linking for tags and operations"},
	{"persistAuthorization", "bool", "true", "Persist authorization data across reloads"},
	{"showExtensions", "bool", "false", "Show vendor extensions (x- fields)"},
	{"showCommonExtensions", "bool", "false", "Show common extensions (pattern, maxLength, etc.)"},
	{"tryItOutEnabled", "bool", "false", "Enable 'Try it out' section by default"},
	{"displayRequestDuration", "bool", "false", "Display request duration in responses"},
	{"withCredentials", "bool", "false", "Send credentials with cross-origin requests"},

	// String options
	{"docExpansion", "string", "none", "Default expansion depth: list, full, or none"},
	{"defaultModelRendering", "string", "example", "Default model rendering: example or model"},
	{"filter", "string", "", "Filter string (or 'true' to show filter box)"},
	{"validatorUrl", "string", "", "Schema validator URL (empty to disable)"},
	{"operationsSorter", "string", "", "Sort operations: alpha or method"},
	{"tagsSorter", "string", "", "Sort tags: alpha"},

	// Integer options
	{"defaultModelsExpandDepth", "int", "1", "Models expand depth (-1 to completely hide)"},
	{"defaultModelExpandDepth", "int", "1", "Model expand depth (-1 to completely hide)"},
	{"maxDisplayedTags", "int", "-1", "Max displayed tags (-1 for all)"},
}

// OptionToFlag converts a camelCase option name to kebab-case CLI flag name.
// e.g. "syntaxHighlight" → "syntax-highlight"
func OptionToFlag(name string) string {
	var result []byte
	for i, c := range name {
		if c >= 'A' && c <= 'Z' {
			if i > 0 {
				result = append(result, '-')
			}
			result = append(result, byte(c+32))
		} else {
			result = append(result, byte(c))
		}
	}
	return string(result)
}

// OptionToEnv converts a camelCase option name to SCREAMING_SNAKE_CASE env var.
// e.g. "syntaxHighlight" → "TAPETEST_SYNTAX_HIGHLIGHT"
func OptionToEnv(name string) string {
	var result []byte
	result = append(result, "TAPETEST_"...)
	for i, c := range name {
		if c >= 'A' && c <= 'Z' {
			if i > 0 {
				result = append(result, '_')
			}
			result = append(result, byte(c))
		} else if c >= 'a' && c <= 'z' {
			result = append(result, byte(c-32))
		} else {
			result = append(result, byte(c))
		}
	}
	return string(result)
}

// SwaggerUIConfig holds all swagger-ui configuration values.
type SwaggerUIConfig struct {
	Values map[string]interface{}
}

// DefaultSwaggerUIConfig returns a config with default values from SwaggerUIOptions.
func DefaultSwaggerUIConfig() SwaggerUIConfig {
	cfg := SwaggerUIConfig{Values: make(map[string]interface{})}
	for _, opt := range SwaggerUIOptions {
		cfg.Values[opt.Name] = parseOptionValue(opt.Type, opt.Default)
	}
	return cfg
}

// BuildSwaggerConfig builds a SwaggerUIConfig from a string map (from flags/env vars).
func BuildSwaggerConfig(values map[string]string) SwaggerUIConfig {
	cfg := DefaultSwaggerUIConfig()
	for _, opt := range SwaggerUIOptions {
		if val, ok := values[opt.Name]; ok {
			cfg.Values[opt.Name] = parseOptionValue(opt.Type, val)
		}
	}
	return cfg
}

func parseOptionValue(typ, val string) interface{} {
	switch typ {
	case "bool":
		if b, err := parseBool(val); err == nil {
			return b
		}
		return false
	case "int":
		var n int
		fmt.Sscanf(val, "%d", &n)
		return n
	default:
		return val
	}
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no":
		return false, nil
	}
	return false, fmt.Errorf("invalid bool: %s", s)
}

// ToJS generates JavaScript property assignments for the SwaggerUIBundle config.
func (c SwaggerUIConfig) ToJS() string {
	var lines []string
	for _, opt := range SwaggerUIOptions {
		val, ok := c.Values[opt.Name]
		if !ok {
			continue
		}
		switch opt.Type {
		case "bool":
			lines = append(lines, fmt.Sprintf("            %s: %t,", opt.Name, val.(bool)))
		case "int":
			lines = append(lines, fmt.Sprintf("            %s: %d,", opt.Name, val.(int)))
		case "string":
			s := val.(string)
			if s == "" {
				continue // skip empty string options
			}
			lines = append(lines, fmt.Sprintf("            %s: %q,", opt.Name, s))
		}
	}
	return strings.Join(lines, "\n")
}

// GenerateSwaggerUI generates a complete Swagger UI static site.
func GenerateSwaggerUI(outputDir string, specFile string, config SwaggerUIConfig, cssFiles, jsFiles []string, swaggerUICSS, swaggerUIJS string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("tapetest: failed to create output dir: %w", err)
	}

	// Resolve swagger-ui core CSS/JS (copy local files or use URLs)
	resolvedSwaggerUICSS, err := resolveAssetPath(swaggerUICSS, outputDir)
	if err != nil {
		return fmt.Errorf("tapetest: failed to resolve swagger-ui CSS: %w", err)
	}
	resolvedSwaggerUIJS, err := resolveAssetPath(swaggerUIJS, outputDir)
	if err != nil {
		return fmt.Errorf("tapetest: failed to resolve swagger-ui JS: %w", err)
	}

	// Copy local custom CSS/JS files to output directory
	resolvedCSS, err := copyAssetFiles(cssFiles, outputDir)
	if err != nil {
		return fmt.Errorf("tapetest: failed to copy CSS files: %w", err)
	}
	resolvedJS, err := copyAssetFiles(jsFiles, outputDir)
	if err != nil {
		return fmt.Errorf("tapetest: failed to copy JS files: %w", err)
	}

	// Generate index.html
	indexHTML := generateSwaggerIndexHTML(specFile, config, resolvedSwaggerUICSS, resolvedSwaggerUIJS, resolvedCSS, resolvedJS)
	indexPath := filepath.Join(outputDir, "index.html")
	if err := os.WriteFile(indexPath, []byte(indexHTML), 0644); err != nil {
		return fmt.Errorf("tapetest: failed to write index.html: %w", err)
	}

	return nil
}

// resolveAssetPath handles a single asset path: copies local files to output dir,
// or returns URLs as-is. Returns the reference path for HTML.
func resolveAssetPath(path, outputDir string) (string, error) {
	if path == "" {
		return "", nil
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path, nil
	}
	// Local file — copy to output dir
	resolved, err := copyAssetFiles([]string{path}, outputDir)
	if err != nil {
		return "", err
	}
	if len(resolved) > 0 {
		return resolved[0], nil
	}
	return path, nil
}

// copyAssetFiles copies local files to the output directory and returns the
// filenames (or original URLs) to reference in HTML.
func copyAssetFiles(files []string, outputDir string) ([]string, error) {
	var resolved []string
	absOutputDir, _ := filepath.Abs(outputDir)

	for _, f := range files {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		// URL — reference directly
		if strings.HasPrefix(f, "http://") || strings.HasPrefix(f, "https://") {
			resolved = append(resolved, f)
			continue
		}

		// Local file
		absSrc, _ := filepath.Abs(f)
		base := filepath.Base(f)
		absDst := filepath.Join(absOutputDir, base)

		// Same file — just reference it, don't copy (would truncate)
		if absSrc == absDst {
			resolved = append(resolved, base)
			continue
		}

		// Copy to output dir
		src, err := os.Open(f)
		if err != nil {
			return nil, fmt.Errorf("failed to open %s: %w", f, err)
		}
		defer src.Close()

		dst, err := os.Create(absDst)
		if err != nil {
			src.Close()
			return nil, fmt.Errorf("failed to create %s: %w", base, err)
		}
		defer dst.Close()

		if _, err := io.Copy(dst, src); err != nil {
			src.Close()
			dst.Close()
			return nil, fmt.Errorf("failed to copy %s: %w", f, err)
		}
		src.Close()
		dst.Close()

		resolved = append(resolved, base)
	}
	return resolved, nil
}

// generateSwaggerIndexHTML returns an HTML page with embedded Swagger UI.
func generateSwaggerIndexHTML(specFile string, config SwaggerUIConfig, swaggerUICSS, swaggerUIJS string, cssFiles, jsFiles []string) string {
	// Swagger-ui core CSS
	swaggerCSSLink := fmt.Sprintf("    <link rel=\"stylesheet\" type=\"text/css\" href=\"%s\">\n", swaggerUICSS)

	// Swagger-ui core JS
	swaggerJSTag := fmt.Sprintf("    <script src=\"%s\"></script>\n", swaggerUIJS)

	// Build custom CSS link tags
	var cssLinks strings.Builder
	for _, f := range cssFiles {
		cssLinks.WriteString(fmt.Sprintf("    <link rel=\"stylesheet\" type=\"text/css\" href=\"%s\">\n", f))
	}

	// Build custom JS script tags
	var jsTags strings.Builder
	for _, f := range jsFiles {
		jsTags.WriteString(fmt.Sprintf("    <script src=\"%s\"></script>\n", f))
	}

	configJS := config.ToJS()

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
	   <meta charset="UTF-8">
	   <meta name="viewport" content="width=device-width, initial-scale=1.0">
	   <title>API Documentation</title>
%s%s</head>
<body>
	   <div id="swagger-ui"></div>
%s%s    <script>
	       SwaggerUIBundle({
	           url: "%s",
	           dom_id: '#swagger-ui',
	           presets: [
	               SwaggerUIBundle.presets.apis,
	               SwaggerUIBundle.SwaggerUIStandalonePreset
	           ],
	           layout: "BaseLayout",
%s
	       });
	   </script>
</body>
</html>`, swaggerCSSLink, cssLinks.String(), swaggerJSTag, jsTags.String(), specFile, configJS)
}

// DefaultSwaggerUICSS is the default CDN URL for swagger-ui CSS.
const DefaultSwaggerUICSS = "https://unpkg.com/swagger-ui-dist@5/swagger-ui.css"

// DefaultSwaggerUIJS is the default CDN URL for swagger-ui JS bundle.
const DefaultSwaggerUIJS = "https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"

// GenerateDocsOptions holds all configuration options for GenerateDocs.
//
// Use it to generate OpenAPI documentation and a Swagger UI site directly from
// your recorded test exchanges and (optionally) source annotations, without a
// separate CLI binary.
type GenerateDocsOptions struct {
	// RecordingDir is the path to the .tapetest recording directory.
	RecordingDir string
	// OutputDir is the output directory for the generated docs.
	OutputDir string
	// Title is the API title shown in the OpenAPI info section.
	Title string
	// Version is the API version shown in the OpenAPI info section.
	Version string
	// SourceDir is the source directory parsed for go-swag handler annotations,
	// security definitions and general API info. When empty, no annotations are parsed.
	SourceDir string
	// ServerURL is the API server base URL added to the OpenAPI servers field.
	// When empty, falls back to the @host/@BasePath directives from SourceDir.
	ServerURL string
	// ReadableExamples transforms test names into a human-readable format in examples.
	ReadableExamples bool
	// SwaggerUICSS is the Swagger UI CSS file path or URL.
	// Defaults to DefaultSwaggerUICSS when empty.
	SwaggerUICSS string
	// SwaggerUIJS is the Swagger UI JS bundle file path or URL.
	// Defaults to DefaultSwaggerUIJS when empty.
	SwaggerUIJS string
	// CSSFiles are custom CSS files (local paths or URLs) injected into the Swagger UI.
	CSSFiles []string
	// JSFiles are custom JS files (local paths or URLs) injected into the Swagger UI.
	JSFiles []string
	// Config is the Swagger UI runtime configuration.
	// When its Values map is nil, DefaultSwaggerUIConfig is used.
	Config SwaggerUIConfig
}

// GenerateDocs generates an OpenAPI v3 spec and a Swagger UI site from recorded
// test exchanges and (optionally) source annotations.
//
// All options are provided via the GenerateDocsOptions struct. For example:
//
//	err := tapetest.GenerateDocs(tapetest.GenerateDocsOptions{
//	    RecordingDir: ".tapetest",
//	    OutputDir:    "docs",
//	    Title:        "My API",
//	    Version:      "1.0.0",
//	    SourceDir:    ".",
//	})
func GenerateDocs(opts GenerateDocsOptions) error {
	if opts.RecordingDir == "" {
		return fmt.Errorf("tapetest: recording directory is required")
	}
	if opts.OutputDir == "" {
		return fmt.Errorf("tapetest: output directory is required")
	}

	// Fall back to defaults for the swagger-ui core assets.
	swaggerUICSS := opts.SwaggerUICSS
	if swaggerUICSS == "" {
		swaggerUICSS = DefaultSwaggerUICSS
	}
	swaggerUIJS := opts.SwaggerUIJS
	if swaggerUIJS == "" {
		swaggerUIJS = DefaultSwaggerUIJS
	}

	// Fall back to the default swagger-ui runtime config when none is supplied.
	config := opts.Config
	if config.Values == nil {
		config = DefaultSwaggerUIConfig()
	}

	exchanges, err := LoadRecordings(opts.RecordingDir)
	if err != nil {
		return fmt.Errorf("tapetest: failed to load recordings from %s: %w", opts.RecordingDir, err)
	}

	var annotations []HandlerAnnotation
	var securityDefs []SecurityDefinition
	var generalInfo GeneralAPIInfo
	if opts.SourceDir != "" {
		annotations, err = ParseAnnotationsFromDir(opts.SourceDir)
		if err != nil {
			return fmt.Errorf("tapetest: failed to parse annotations from %s: %w", opts.SourceDir, err)
		}
		securityDefs, err = ParseSecurityDefinitionsFromDir(opts.SourceDir)
		if err != nil {
			return fmt.Errorf("tapetest: failed to parse security definitions from %s: %w", opts.SourceDir, err)
		}
		generalInfo, err = ParseGeneralAPIInfoFromDir(opts.SourceDir)
		if err != nil {
			return fmt.Errorf("tapetest: failed to parse general API info from %s: %w", opts.SourceDir, err)
		}
	}

	doc := GenerateOpenAPIFromRecordings(exchanges, annotations, securityDefs, generalInfo.BasePath, opts.Title, opts.Version, opts.ReadableExamples)

	// Apply go-swag General API Info (overrides flag defaults when present)
	doc.ApplyGeneralAPIInfo(generalInfo)

	// Add server URL: explicit flag takes precedence, otherwise fall back to
	// the @host/@BasePath directives.
	if opts.ServerURL != "" {
		doc.AddServer(opts.ServerURL, "")
	} else if generalInfo.HasServerInfo() {
		doc.AddServer(generalInfo.ServerURL(), "")
	}

	specPath := filepath.Join(opts.OutputDir, "openapi.json")
	if err := doc.WriteJSON(specPath); err != nil {
		return fmt.Errorf("tapetest: failed to write OpenAPI spec: %w", err)
	}

	if err := GenerateSwaggerUI(opts.OutputDir, "openapi.json", config, opts.CSSFiles, opts.JSFiles, swaggerUICSS, swaggerUIJS); err != nil {
		return fmt.Errorf("tapetest: failed to generate Swagger UI: %w", err)
	}

	specURL := strings.ReplaceAll(specPath, "\\", "/")
	fmt.Printf("Documentation generated successfully!\n")
	fmt.Printf("  OpenAPI spec: %s\n", specURL)
	fmt.Printf("  Swagger UI:   %s/index.html\n", opts.OutputDir)
	fmt.Printf("  %d endpoints documented from %d recordings\n",
		countOperations(doc), len(exchanges))

	return nil
}

// countOperations counts the total number of operations in an OpenAPI document.
func countOperations(doc *OpenAPIDocument) int {
	count := 0
	for _, methods := range doc.Paths {
		count += len(methods)
	}
	return count
}
