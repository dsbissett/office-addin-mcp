package wordtool

import (
	"strings"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/officejs"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// TestRegisterCompilesSchemas guards every word.* tool's JSON Schema against
// regressions. Registry.MustRegister panics on a malformed schema, so a clean
// run here is the test.
func TestRegisterCompilesSchemas(t *testing.T) {
	r := tools.NewRegistry()
	Register(r)
	if got := len(r.List()); got != 8 {
		t.Fatalf("expected 8 word.* tools registered, got %d", got)
	}
}

// TestEveryRunPayloadToolHasJSPayload catches Go-side tools whose runPayload
// call would fail at runtime because the matching JS file was forgotten.
func TestEveryRunPayloadToolHasJSPayload(t *testing.T) {
	if err := officejs.Preload(); err != nil {
		t.Fatalf("preload payloads: %v", err)
	}
	available := map[string]bool{}
	for _, n := range officejs.Names() {
		available[n] = true
	}
	r := tools.NewRegistry()
	Register(r)
	for _, tool := range r.List() {
		if !strings.HasPrefix(tool.Name, "word.") {
			continue
		}
		if !available[tool.Name] {
			t.Errorf("tool %q registered but no JS payload found", tool.Name)
		}
	}
}
