package mcp

import (
	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/addintool"
	"github.com/dsbissett/office-addin-mcp/internal/tools/cdptool"
	"github.com/dsbissett/office-addin-mcp/internal/tools/exceltool"
	"github.com/dsbissett/office-addin-mcp/internal/tools/inspecttool"
	"github.com/dsbissett/office-addin-mcp/internal/tools/interacttool"
	"github.com/dsbissett/office-addin-mcp/internal/tools/lifecycletool"
	"github.com/dsbissett/office-addin-mcp/internal/tools/pagetool"
)

// CDPSelection chooses which slice of the raw cdp.* surface gets registered.
// Default zero value (Enabled=false) registers nothing — the high-level
// Office add-in surface is always on regardless. When Enabled is true and
// Domains is empty, every domain is registered (legacy --expose-raw-cdp
// behavior). When Enabled is true and Domains is non-empty, only the named
// domains' tools are registered (plus cdp.selectTarget, the cache primer).
type CDPSelection struct {
	Enabled bool
	Domains []string
}

// DefaultRegistry returns the registry exposed to MCP clients on
// initialize / tools/list. The high-level Office add-in surface
// (addin.*, pages.*, page.*, excel.*, interact via page.*) is always on.
// CDP exposure is controlled by sel — see CDPSelection.
func DefaultRegistry(sel CDPSelection) *tools.Registry {
	r := tools.NewRegistry()
	lifecycletool.Register(r)
	addintool.Register(r)
	pagetool.Register(r)
	inspecttool.Register(r)
	interacttool.Register(r)
	exceltool.Register(r)
	if sel.Enabled {
		if len(sel.Domains) == 0 {
			cdptool.Register(r)
		} else {
			cdptool.RegisterFiltered(r, sel.Domains)
		}
	}
	return r
}
