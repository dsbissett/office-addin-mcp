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
