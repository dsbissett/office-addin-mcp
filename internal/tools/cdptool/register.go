package cdptool

import (
	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/cdptool/generated"
)

// Register adds raw Chrome DevTools Protocol tools to the registry: the
// hand-written cdp.selectTarget primer and every code-generated CDP tool from
// cdp/manifest.yaml. This is gated by --expose-raw-cdp at the MCP server
// level — the default registry omits cdp.* entirely in favor of the
// high-level addin.*, pages.*, page.*, excel.*, and interact tools.
//
// cdp.selectTarget stays hand-written: it primes the per-session selector
// cache and has no direct CDP equivalent.
func Register(r *tools.Registry) {
	r.MustRegister(SelectTarget())
	generated.RegisterGenerated(r)
}

// RegisterFiltered registers cdp.selectTarget plus only the code-generated
// tools whose domain appears in allowed. Names are matched case-sensitively
// against generated.Domains. Used by the --cdp-domains flag so an agent that
// only needs DOM + Page + Runtime doesn't see ~411 tools in tools/list.
//
// An empty allowed slice still registers cdp.selectTarget — the cache primer
// is useful with or without raw method tools, and skipping it would surprise
// callers that scripted around its presence.
func RegisterFiltered(r *tools.Registry, allowed []string) {
	r.MustRegister(SelectTarget())
	if len(allowed) == 0 {
		return
	}
	set := make(map[string]bool, len(allowed))
	for _, d := range allowed {
		set[d] = true
	}
	generated.RegisterGeneratedFiltered(r, set)
}

// Domains exposes the list of code-generated CDP domain names to callers
// outside this package (cmd/office-addin-mcp uses it for --list-cdp-domains
// and to validate user input). Returns a fresh slice — callers may sort /
// mutate it freely.
func Domains() []string {
	out := make([]string, len(generated.Domains))
	copy(out, generated.Domains)
	return out
}
