package powerpointtool

import (
	"strings"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/officejs"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

func TestRegisterCompilesSchemas(t *testing.T) {
	r := tools.NewRegistry()
	Register(r)
	if got := len(r.List()); got != 1 {
		t.Fatalf("expected 1 powerpoint.* tool registered (runScript only), got %d", got)
	}
}

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
		if !strings.HasPrefix(tool.Name, "powerpoint.") {
			continue
		}
		if !available[tool.Name] {
			t.Errorf("tool %q registered but no JS payload found", tool.Name)
		}
	}
}
