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

// DefaultRegistry returns the registry exposed to MCP clients on
// initialize / tools/list. The high-level Office add-in surface
// (addin.*, pages.*, page.*, excel.*, interact via page.*) is always on.
// When exposeRawCDP is true, the ~411 code-generated cdp.* tools and the
// cdp.selectTarget cache primer are also registered so power users can
// drive Chrome DevTools Protocol directly.
func DefaultRegistry(exposeRawCDP bool) *tools.Registry {
	r := tools.NewRegistry()
	lifecycletool.Register(r)
	addintool.Register(r)
	pagetool.Register(r)
	inspecttool.Register(r)
	interacttool.Register(r)
	exceltool.Register(r)
	if exposeRawCDP {
		cdptool.Register(r)
	}
	return r
}
