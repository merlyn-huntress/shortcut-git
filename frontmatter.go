package main

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Frontmatter holds parsed frontmatter fields. Title is the editable title field.
// Informational fields are keyed with their full name (e.g., "informational_owner").
type Frontmatter struct {
	Title  string
	Fields map[string]any // all fields including title
}

// parseFrontmatter splits a markdown file into YAML frontmatter and body.
// Returns the parsed frontmatter and the markdown body (without leading newline).
func parseFrontmatter(content string) (*Frontmatter, string, error) {
	// B22: Normalize CRLF to LF
	content = strings.ReplaceAll(content, "\r\n", "\n")

	if !strings.HasPrefix(content, "---\n") {
		// No frontmatter, entire content is body
		return &Frontmatter{Fields: make(map[string]any)}, content, nil
	}

	end := strings.Index(content[4:], "\n---")
	if end == -1 {
		return nil, "", fmt.Errorf("unterminated frontmatter: missing closing ---")
	}

	fmRaw := content[4 : 4+end]
	// Body starts after closing "\n---\n"
	bodyStart := 4 + end + 4 // skip past "\n---"
	body := ""
	if bodyStart < len(content) {
		body = content[bodyStart:]
		// bodyStart points right after "\n---". Remaining starts with "\n\n<body>":
		// first \n closes the --- line, second \n is the blank separator.
		// renderFrontmatter outputs "---\n" + "\n" + body, so we strip exactly
		// two \n to maintain round-trip fidelity.
		body = strings.TrimPrefix(body, "\n") // strip \n after ---
		body = strings.TrimPrefix(body, "\n") // strip blank separator line
	}

	var fields map[string]any
	if err := yaml.Unmarshal([]byte(fmRaw), &fields); err != nil {
		return nil, "", fmt.Errorf("parsing frontmatter YAML: %w", err)
	}
	if fields == nil {
		fields = make(map[string]any)
	}

	fm := &Frontmatter{Fields: fields}
	if t, ok := fields["title"]; ok {
		fm.Title = fmt.Sprintf("%v", t)
	}

	return fm, body, nil
}

// informationalFieldOrder defines the output order for informational fields.
var informationalFieldOrder = []string{
	"informational_shortcut_id",
	"informational_shortcut_url",
	"informational_owner",
	"informational_status",
	"informational_type",
	"informational_archived",
	"informational_last_updated_at",
	"informational_github_pr_urls",
}

// renderFrontmatter produces a markdown file with YAML frontmatter and body.
// Field ordering: title first, then informational fields in defined order, then body.
func renderFrontmatter(fm *Frontmatter, body string) string {
	var b strings.Builder
	b.WriteString("---\n")

	// Title first
	if fm.Title != "" {
		b.WriteString("title: ")
		b.WriteString(yamlScalar(fm.Title))
		b.WriteString("\n")
	}

	// Informational fields in defined order
	for _, key := range informationalFieldOrder {
		val, ok := fm.Fields[key]
		if !ok {
			continue
		}
		writeField(&b, key, val)
	}

	b.WriteString("---\n")

	if body != "" {
		b.WriteString("\n")
		b.WriteString(body)
	}

	return b.String()
}

func writeField(b *strings.Builder, key string, val any) {
	switch v := val.(type) {
	case []any:
		b.WriteString(key + ":\n")
		for _, item := range v {
			// B13: Quote list items that contain YAML-special chars
			b.WriteString("  - " + yamlScalar(fmt.Sprintf("%v", item)) + "\n")
		}
	case []string:
		b.WriteString(key + ":\n")
		for _, item := range v {
			b.WriteString("  - " + yamlScalar(item) + "\n")
		}
	default:
		b.WriteString(key + ": " + yamlScalar(fmt.Sprintf("%v", v)) + "\n")
	}
}

// yamlScalar quotes a value if it contains characters that need quoting.
// B12: Also handles YAML 1.1 booleans, numeric strings, and other ambiguous values.
func yamlScalar(s string) string {
	if s == "" {
		return `""`
	}
	// Quote if it contains spaces, YAML-special characters, or could be ambiguous
	needsQuote := strings.ContainsAny(s, " :#{}[]|>&*!%@`\"'\n,") ||
		isYAMLSpecialValue(s) ||
		looksNumeric(s)
	if needsQuote {
		return fmt.Sprintf("%q", s)
	}
	return s
}

func isYAMLSpecialValue(s string) bool {
	lower := strings.ToLower(s)
	switch lower {
	case "true", "false", "null", "~",
		"yes", "no", "on", "off",
		".inf", "-.inf", ".nan":
		return true
	}
	return false
}

func looksNumeric(s string) bool {
	if len(s) == 0 {
		return false
	}
	// Check if it could be parsed as a number by YAML
	first := s[0]
	if first >= '0' && first <= '9' {
		return true
	}
	if (first == '-' || first == '+' || first == '.') && len(s) > 1 {
		return true
	}
	return false
}

// isInformationalField returns true if the field name starts with "informational_".
func isInformationalField(name string) bool {
	return strings.HasPrefix(name, "informational_")
}
