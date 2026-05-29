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
	host, err := validateHost(u.Host)
	if err != nil {
		return nil, err
	}
	parsed.Host = host
	parsed.Parts = splitPathParts(u.Path)

	return parsed, nil
}

// knownHosts is the set of recognized Office application hosts.
var knownHosts = map[string]bool{
	"excel":   true,
	"word":    true,
	"outlook": true,
	"pp":      true,
	"onenote": true,
}

// validateHost normalizes rawHost to lowercase and verifies it is a recognized
// Office application. It returns the normalized host or an error.
func validateHost(rawHost string) (string, error) {
	if rawHost == "" {
		return "", fmt.Errorf("missing host in URI")
	}

	host := strings.ToLower(rawHost)
	if !knownHosts[host] {
		return "", fmt.Errorf("unknown host: %q", rawHost)
	}

	return host, nil
}

// splitPathParts splits a URL path into non-empty slash-delimited segments,
// skipping the leading empty segment produced by the leading slash.
func splitPathParts(path string) []string {
	if path == "" {
		return nil
	}

	var parts []string
	for _, seg := range strings.Split(path, "/") {
		if seg != "" {
			parts = append(parts, seg)
		}
	}
	return parts
}

// String returns the canonical office:// URI form.
func (p *ParsedURI) String() string {
	if len(p.Parts) == 0 {
		return fmt.Sprintf("office://%s", p.Host)
	}
	return fmt.Sprintf("office://%s/%s", p.Host, strings.Join(p.Parts, "/"))
}
