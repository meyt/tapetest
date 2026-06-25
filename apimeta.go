package tapetest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
)

// GeneralAPIInfo holds the go-swag "General API Info" metadata declared once
// per project (typically at the top of main.go). These are the top-level
// fields of the OpenAPI document's `info` object plus the host/basePath that
// map to an OpenAPI v3 server entry.
//
//	//	@title			Swagger Example API
//	//	@version		1.0
//	//	@description	This is a sample server celler server.
//	//	@termsOfService	http://swagger.io/terms/
//
//	//	@contact.name	API Support
//	//	@contact.url	http://www.swagger.io/support
//	//	@contact.email	support@swagger.io
//
//	//	@license.name	Apache 2.0
//	//	@license.url	http://www.apache.org/licenses/LICENSE-2.0.html
//
//	//	@host		localhost:8080
//	//	@BasePath	/api/v1
//	//	@schemes	http,https
type GeneralAPIInfo struct {
	Title          string   // @title
	Version        string   // @version
	Description    string   // @description
	TermsOfService string   // @termsOfService
	ContactName    string   // @contact.name
	ContactURL     string   // @contact.url
	ContactEmail   string   // @contact.email
	LicenseName    string   // @license.name
	LicenseURL     string   // @license.url
	Host           string   // @host
	BasePath       string   // @BasePath
	Schemes        []string // @schemes (e.g. "http", "https")
}

// generalAPIInfoPatterns maps each go-swag General API Info directive to its
// parser regex. Keys are matched case-insensitively (matching go-swag), with
// the exception that @BasePath is commonly written in mixed case.
var generalAPIInfoPatterns = []generalInfoPattern{
	{regexp.MustCompile(`(?i)^@title\s+(.+)$`), "title"},
	{regexp.MustCompile(`(?i)^@version\s+(.+)$`), "version"},
	{regexp.MustCompile(`(?i)^@description\s+(.+)$`), "description"},
	{regexp.MustCompile(`(?i)^@termsOfService\s+(.+)$`), "termsOfService"},
	{regexp.MustCompile(`(?i)^@contact\.name\s+(.+)$`), "contact.name"},
	{regexp.MustCompile(`(?i)^@contact\.url\s+(.+)$`), "contact.url"},
	{regexp.MustCompile(`(?i)^@contact\.email\s+(.+)$`), "contact.email"},
	{regexp.MustCompile(`(?i)^@license\.name\s+(.+)$`), "license.name"},
	{regexp.MustCompile(`(?i)^@license\.url\s+(.+)$`), "license.url"},
	{regexp.MustCompile(`(?i)^@host\s+(.+)$`), "host"},
	{regexp.MustCompile(`(?i)^@basePath\s+(.+)$`), "basePath"},
	{regexp.MustCompile(`(?i)^@schemes\s+(.+)$`), "schemes"},
}

type generalInfoPattern struct {
	re  *regexp.Regexp
	key string
}

// primaryGeneralKeys identifies directives that only ever appear in a General
// API Info block. A comment group is treated as General API Info only when it
// contains at least one of these keys, which prevents accidentally capturing a
// @description line that belongs to a @securityDefinitions block.
var primaryGeneralKeys = map[string]bool{
	"title":          true,
	"version":        true,
	"termsOfService": true,
	"contact.name":   true,
	"contact.url":    true,
	"contact.email":  true,
	"license.name":   true,
	"license.url":    true,
	"host":           true,
	"basePath":       true,
	"schemes":        true,
}

// ParseGeneralAPIInfoFromDir scans all (non-test) Go files in the given
// directory for go-swag General API Info directives. Fields are merged across
// files: the first non-empty value for each directive wins.
//
//	info, err := ParseGeneralAPIInfoFromDir(".")
func ParseGeneralAPIInfoFromDir(dir string) (GeneralAPIInfo, error) {
	files, err := collectGoFiles(dir)
	if err != nil {
		return GeneralAPIInfo{}, err
	}

	var info GeneralAPIInfo
	for _, path := range files {
		fileInfo, err := parseFileGeneralAPIInfo(path)
		if err != nil {
			continue
		}
		info = mergeGeneralAPIInfo(info, fileInfo)
	}
	return info, nil
}

// ParseGeneralAPIInfoFromFiles scans the provided Go source files.
func ParseGeneralAPIInfoFromFiles(files []string) (GeneralAPIInfo, error) {
	var info GeneralAPIInfo
	for _, f := range files {
		fileInfo, err := parseFileGeneralAPIInfo(f)
		if err != nil {
			continue
		}
		info = mergeGeneralAPIInfo(info, fileInfo)
	}
	return info, nil
}

// parseFileGeneralAPIInfo parses a single Go source file for General API Info.
func parseFileGeneralAPIInfo(path string) (GeneralAPIInfo, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return GeneralAPIInfo{}, err
	}
	return parseGeneralAPIInfoFromAST(file), nil
}

// parseGeneralAPIInfoFromAST walks every comment group in the AST. A group is
// only parsed as General API Info if it contains at least one "primary"
// directive (title, version, host, basePath, contact.*, license.*, ...). This
// avoids capturing @description lines that belong to security definition
// blocks.
func parseGeneralAPIInfoFromAST(file *ast.File) GeneralAPIInfo {
	var info GeneralAPIInfo
	var hasInfo bool

	for _, group := range file.Comments {
		// Collect raw directive lines for this group first, so we can decide
		// whether the group is a General API Info block before applying values.
		type match struct {
			key   string
			value string
		}
		var matches []match
		isGeneral := false

		for _, comment := range group.List {
			text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
			for _, line := range strings.Split(text, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				for _, p := range generalAPIInfoPatterns {
					if m := p.re.FindStringSubmatch(line); len(m) == 2 {
						matches = append(matches, match{p.key, strings.TrimSpace(m[1])})
						if primaryGeneralKeys[p.key] {
							isGeneral = true
						}
						break
					}
				}
			}
		}

		if !isGeneral {
			continue
		}

		hasInfo = true
		for _, m := range matches {
			applyGeneralAPIInfoField(&info, m.key, m.value)
		}
	}

	_ = hasInfo
	return info
}

// applyGeneralAPIInfoField sets a single General API Info directive value,
// preserving any previously set value (first write wins).
func applyGeneralAPIInfoField(info *GeneralAPIInfo, key, val string) {
	switch key {
	case "title":
		if info.Title == "" {
			info.Title = val
		}
	case "version":
		if info.Version == "" {
			info.Version = val
		}
	case "description":
		if info.Description == "" {
			info.Description = val
		}
	case "termsOfService":
		if info.TermsOfService == "" {
			info.TermsOfService = val
		}
	case "contact.name":
		if info.ContactName == "" {
			info.ContactName = val
		}
	case "contact.url":
		if info.ContactURL == "" {
			info.ContactURL = val
		}
	case "contact.email":
		if info.ContactEmail == "" {
			info.ContactEmail = val
		}
	case "license.name":
		if info.LicenseName == "" {
			info.LicenseName = val
		}
	case "license.url":
		if info.LicenseURL == "" {
			info.LicenseURL = val
		}
	case "host":
		if info.Host == "" {
			info.Host = val
		}
	case "basePath":
		if info.BasePath == "" {
			info.BasePath = val
		}
	case "schemes":
		if len(info.Schemes) == 0 {
			for _, s := range strings.Split(val, ",") {
				s = strings.TrimSpace(s)
				if s != "" {
					info.Schemes = append(info.Schemes, s)
				}
			}
		}
	}
}

// mergeGeneralAPIInfo merges two General API Info structs; values from `src`
// fill empty fields in `dst` (first non-empty value wins).
func mergeGeneralAPIInfo(dst, src GeneralAPIInfo) GeneralAPIInfo {
	applyGeneralAPIInfoField(&dst, "title", src.Title)
	applyGeneralAPIInfoField(&dst, "version", src.Version)
	applyGeneralAPIInfoField(&dst, "description", src.Description)
	applyGeneralAPIInfoField(&dst, "termsOfService", src.TermsOfService)
	applyGeneralAPIInfoField(&dst, "contact.name", src.ContactName)
	applyGeneralAPIInfoField(&dst, "contact.url", src.ContactURL)
	applyGeneralAPIInfoField(&dst, "contact.email", src.ContactEmail)
	applyGeneralAPIInfoField(&dst, "license.name", src.LicenseName)
	applyGeneralAPIInfoField(&dst, "license.url", src.LicenseURL)
	applyGeneralAPIInfoField(&dst, "host", src.Host)
	applyGeneralAPIInfoField(&dst, "basePath", src.BasePath)
	if len(dst.Schemes) == 0 {
		dst.Schemes = src.Schemes
	}
	return dst
}

// ServerURL combines the @host, @BasePath and @schemes directives into an
// OpenAPI v3 server URL. Returns an empty string when neither host nor
// basePath is set. The first declared scheme is used (defaulting to "http").
func (g GeneralAPIInfo) ServerURL() string {
	if g.Host == "" && g.BasePath == "" {
		return ""
	}
	scheme := "http"
	if len(g.Schemes) > 0 {
		scheme = g.Schemes[0]
	}
	base := g.BasePath
	if base != "" && !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	return scheme + "://" + g.Host + base
}

// HasServerInfo reports whether host and/or basePath were declared.
func (g GeneralAPIInfo) HasServerInfo() bool {
	return g.Host != "" || g.BasePath != ""
}

// HasAny reports whether any General API Info directive was parsed.
func (g GeneralAPIInfo) HasAny() bool {
	return g.Title != "" || g.Version != "" || g.Description != "" ||
		g.TermsOfService != "" || g.ContactName != "" || g.ContactURL != "" ||
		g.ContactEmail != "" || g.LicenseName != "" || g.LicenseURL != "" ||
		g.Host != "" || g.BasePath != "" || len(g.Schemes) > 0
}
