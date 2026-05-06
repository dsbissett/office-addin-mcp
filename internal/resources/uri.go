// Package resources implements MCP resource protocol support. Resources allow
// LLM clients to reference Office documents by URI (office://excel/..., etc.)
// and receive push notifications on changes.
package resources

import (
	"fmt"
	"net/url"
	"strings"
)

// ParsedURI represents a parsed office:// URI.
type ParsedURI struct {
	// Host is the Office application: "excel", "word", "outlook", "pp", "onenote"
	Host string
	// Parts are the slash-delimited path segments after the host.
	// For office://excel/Book1/Sheet1!A1:D20, Parts = ["Book1", "Sheet1!A1:D20"]
	Parts []string
	// Raw is the original URI string.
	Raw string
}

// ParseURI parses an office:// URI into its components.
// Returns error if the scheme is invalid or host is missing.
//
// Recognized hosts: excel, word, outlook, pp, onenote
// The host is case-insensitive; Parts preserve their original case.
func ParseURI(uri string) (*ParsedURI, error) {
	parsed := &ParsedURI{Raw: uri}

	// Parse as URL to extract scheme and path.
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("invalid URI: %w", err)
	}

	if u.Scheme != "office" {
		return nil, fmt.Errorf("invalid scheme: expected 'office', got %q", u.Scheme)
	}

	// u.Host is the domain part; u.Path is everything after the domain.
	// For office://excel/Book1/Sheet1, u.Host = "excel", u.Path = "/Book1/Sheet1"
	if u.Host == "" {
		return nil, fmt.Errorf("missing host in URI")
	}

	// Normalize host to lowercase for validation; store lowercase.
	host := strings.ToLower(u.Host)
	switch host {
	case "excel", "word", "outlook", "pp", "onenote":
		parsed.Host = host
	default:
		return nil, fmt.Errorf("unknown host: %q", u.Host)
	}

	// Split the path into segments (skip leading empty segment from leading /).
	if u.Path != "" {
		parts := strings.Split(u.Path, "/")
		// parts[0] is empty due to leading /; skip it.
		for i := 1; i < len(parts); i++ {
			if parts[i] != "" {
				parsed.Parts = append(parsed.Parts, parts[i])
			}
		}
	}

	return parsed, nil
}

// String returns the canonical office:// URI form.
func (p *ParsedURI) String() string {
	if len(p.Parts) == 0 {
		return fmt.Sprintf("office://%s", p.Host)
	}
	return fmt.Sprintf("office://%s/%s", p.Host, strings.Join(p.Parts, "/"))
}
