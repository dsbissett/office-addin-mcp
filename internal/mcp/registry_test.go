package mcp

import (
	"strings"
	"testing"
)

// TestDefaultRegistryHidesRawCDP confirms the high-level surface ships
// without any cdp.* tools when --expose-raw-cdp is off. Phase 6 contract:
// agents see addin/pages/page/excel/interact only by default.
func TestDefaultRegistryHidesRawCDP(t *testing.T) {
	r := DefaultRegistry(CDPSelection{})
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
	r := DefaultRegistry(CDPSelection{Enabled: true})
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

// TestDefaultRegistryIncludesMultiHostSurface confirms that the host
// packages added by the multi-host plan (Word, Outlook, PowerPoint,
// OneNote) are registered alongside the existing Excel surface on the
// default high-level registry — no flag required.
func TestDefaultRegistryIncludesMultiHostSurface(t *testing.T) {
	r := DefaultRegistry(CDPSelection{})
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

// TestCDPDomainsFilterRegistersOnlyNamedDomains confirms F7 behavior: a
// non-empty Domains list registers only those domains' cdp.* tools (plus
// cdp.selectTarget) and skips everything else.
func TestCDPDomainsFilterRegistersOnlyNamedDomains(t *testing.T) {
	r := DefaultRegistry(CDPSelection{Enabled: true, Domains: []string{"DOM", "Page"}})
	var domNames, pageNames, animationNames, runtimeNames int
	hasSelectTarget := false
	for _, tl := range r.List() {
		switch {
		case tl.Name == "cdp.selectTarget":
			hasSelectTarget = true
		case strings.HasPrefix(tl.Name, "cdp.dOM."):
			domNames++
		case strings.HasPrefix(tl.Name, "cdp.page."):
			pageNames++
		case strings.HasPrefix(tl.Name, "cdp.animation."):
			animationNames++
		case strings.HasPrefix(tl.Name, "cdp.runtime."):
			runtimeNames++
		}
	}
	if !hasSelectTarget {
		t.Error("cdp.selectTarget missing; cache primer should always register when Enabled=true")
	}
	if domNames == 0 {
		t.Error("DOM domain produced no cdp.dOM.* tools")
	}
	if pageNames == 0 {
		t.Error("Page domain produced no cdp.page.* tools")
	}
	if animationNames != 0 {
		t.Errorf("Animation should be filtered out; got %d tools", animationNames)
	}
	if runtimeNames != 0 {
		t.Errorf("Runtime should be filtered out; got %d tools", runtimeNames)
	}
}
