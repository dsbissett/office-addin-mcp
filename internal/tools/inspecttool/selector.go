package inspecttool

import (
	"github.com/dsbissett/office-addin-mcp/internal/addin"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// makeSelector builds a TargetSelector from the common targetId / urlPattern
// / surface params shared across page.* tools.
func makeSelector(targetID, urlPattern, surface string) tools.TargetSelector {
	return tools.TargetSelector{
		TargetID:   targetID,
		URLPattern: urlPattern,
		Surface:    addin.SurfaceType(surface),
	}
}
