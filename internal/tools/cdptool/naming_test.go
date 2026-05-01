package cdptool

import (
	"regexp"
	"strings"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// generatedToolNamePattern is the canonical shape every code-generated CDP
// tool name must match: cdp.<lowerDomain>.<lowerMethod>. The hand-written
// cdp.selectTarget primer pre-dates the convention and is allowed.
var generatedToolNamePattern = regexp.MustCompile(`^cdp\.[a-z][a-zA-Z]*\.[a-z][a-zA-Z]*$`)

var handWrittenExceptions = map[string]struct{}{
	"cdp.selectTarget": {},
}

// TestGeneratedToolNamesMatchPattern enforces the cdp.<domain>.<method>
// shape across every code-generated tool. Catches a manifest entry that
// inadvertently produces a malformed name (e.g. capitalized domain).
func TestGeneratedToolNamesMatchPattern(t *testing.T) {
	r := tools.NewRegistry()
	Register(r)

	var checked, exceptions int
	for _, tool := range r.List() {
		name := tool.Name
		if !strings.HasPrefix(name, "cdp.") {
			continue
		}
		if _, ok := handWrittenExceptions[name]; ok {
			exceptions++
			continue
		}
		if !generatedToolNamePattern.MatchString(name) {
			t.Errorf("tool %q does not match %s", name, generatedToolNamePattern)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no generated cdp.* tools registered — did register.go drop generated.RegisterGenerated?")
	}
	if exceptions < 1 {
		t.Errorf("expected cdp.selectTarget to remain registered as the hand-written cache primer; got %d exceptions", exceptions)
	}
}

// TestNoCDPDuplicates guards against name collisions inside the cdptool
// package. MustRegister panics on duplicates, so a successful Register call
// already proves uniqueness; this test makes the invariant explicit.
func TestNoCDPDuplicates(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("duplicate cdp.* tool registration: %v", r)
		}
	}()
	r := tools.NewRegistry()
	Register(r)
}
