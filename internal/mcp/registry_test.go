package mcp

import (
	"strings"
	"testing"
)

// TestDefaultRegistryHidesRawCDP confirms the high-level surface ships
// without any cdp.* tools when --expose-raw-cdp is off. Phase 6 contract:
// agents see addin/pages/page/excel/interact only by default.
func TestDefaultRegistryHidesRawCDP(t *testing.T) {
	r := DefaultRegistry(false)
	for _, tl := range r.List() {
		if strings.HasPrefix(tl.Name, "cdp.") {
			t.Errorf("default registry leaked raw CDP tool %q; expected --expose-raw-cdp gate", tl.Name)
		}
	}
	if len(r.List()) == 0 {
		t.Fatal("default registry is empty; high-level tools must always register")
	}
}

// TestExposeRawCDPRegistersGenerated confirms the generated cdp.* surface
// shows up when ExposeRawCDP is true. We sample a couple of well-known names
// rather than counting — counts will drift as the manifest evolves.
func TestExposeRawCDPRegistersGenerated(t *testing.T) {
	r := DefaultRegistry(true)
	names := map[string]bool{}
	for _, tl := range r.List() {
		names[tl.Name] = true
	}
	for _, want := range []string{
		"cdp.selectTarget",
		"cdp.runtime.evaluate",
		"cdp.target.getTargets",
		"cdp.page.navigate",
	} {
		if !names[want] {
			t.Errorf("expected %q registered when ExposeRawCDP=true", want)
		}
	}
}
