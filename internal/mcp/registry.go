package mcp

import (
	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/addintool"
	"github.com/dsbissett/office-addin-mcp/internal/tools/exceltool"
	"github.com/dsbissett/office-addin-mcp/internal/tools/inspecttool"
	"github.com/dsbissett/office-addin-mcp/internal/tools/interacttool"
	"github.com/dsbissett/office-addin-mcp/internal/tools/lifecycletool"
	"github.com/dsbissett/office-addin-mcp/internal/tools/onenotetool"
	"github.com/dsbissett/office-addin-mcp/internal/tools/outlooktool"
	"github.com/dsbissett/office-addin-mcp/internal/tools/pagetool"
	"github.com/dsbissett/office-addin-mcp/internal/tools/powerpointtool"
	"github.com/dsbissett/office-addin-mcp/internal/tools/wordtool"
)

// DefaultRegistry returns the registry exposed to MCP clients on
// initialize / tools/list. The high-level Office add-in surface
// (addin.*, pages.*, page.*, inspect.*, interact.*) plus each host's
// runScript escape hatch is always on.
func DefaultRegistry() *tools.Registry {
	r := tools.NewRegistry()
	lifecycletool.Register(r)
	addintool.Register(r)
	pagetool.Register(r)
	inspecttool.Register(r)
	interacttool.Register(r)
	exceltool.Register(r)
	wordtool.Register(r)
	outlooktool.Register(r)
	powerpointtool.Register(r)
	onenotetool.Register(r)
	return r
}
