package resources

import (
	"strings"
	"testing"
)

// TestParseURI_ParseError exercises the url.Parse failure branch with a control
// character that net/url rejects.
func TestParseURI_ParseError(t *testing.T) {
	_, err := ParseURI("office://excel/\x7f\x01")
	if err == nil {
		t.Fatal("expected parse error for control characters")
	}
	if !strings.Contains(err.Error(), "invalid URI") {
		t.Errorf("error = %v, want it to mention invalid URI", err)
	}
}

// TestString_HostOnly covers the len(Parts)==0 branch of (*ParsedURI).String,
// which the round-trip table never hits (all its URIs have parts).
func TestString_HostOnly(t *testing.T) {
	parsed, err := ParseURI("office://outlook")
	if err != nil {
		t.Fatalf("ParseURI: %v", err)
	}
	if got := parsed.String(); got != "office://outlook" {
		t.Errorf("String() = %q, want office://outlook", got)
	}
}
