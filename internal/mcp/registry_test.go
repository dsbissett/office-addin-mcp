package mcp

import (
	"strings"
	"testing"
)

// TestDefaultRegistryHasNoRawCDP confirms the high-level surface ships
// without any cdp.* tools. The raw CDP surface was removed in Phase 0
// of the workflow-surface plan; this guard catches accidental
// reintroduction.
func TestDefaultRegistryHasNoRawCDP(t *testing.T) {
	r := DefaultRegistry()
	for _, tl := range r.List() {
		if strings.HasPrefix(tl.Name, "cdp.") {
			t.Errorf("registry leaked raw CDP tool %q; the cdp.* surface was removed in Phase 0", tl.Name)
		}
	}
	if len(r.List()) == 0 {
		t.Fatal("default registry is empty; high-level tools must always register")
	}
}

// TestDefaultRegistryIncludesMultiHostSurface confirms each host package
// (Excel, Word, Outlook, PowerPoint, OneNote) contributes at least one
// tool to the default registry. After Phase 0 the only host tools are
// the runScript escape hatches, so this asserts those still register.
func TestDefaultRegistryIncludesMultiHostSurface(t *testing.T) {
	r := DefaultRegistry()
	counts := map[string]int{}
	for _, tl := range r.List() {
		switch {
		case strings.HasPrefix(tl.Name, "excel."):
			counts["excel"]++
		case strings.HasPrefix(tl.Name, "word."):
			counts["word"]++
		case strings.HasPrefix(tl.Name, "outlook."):
			counts["outlook"]++
		case strings.HasPrefix(tl.Name, "powerpoint."):
			counts["powerpoint"]++
		case strings.HasPrefix(tl.Name, "onenote."):
			counts["onenote"]++
		}
	}
	for _, host := range []string{"excel", "word", "outlook", "powerpoint", "onenote"} {
		if counts[host] == 0 {
			t.Errorf("expected at least one %s.* tool registered by default; got 0", host)
		}
	}
}
