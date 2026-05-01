// Package pagetool registers the pages.* tools (list, select, close,
// handleDialog) and page.navigate. The list/select pair lets agents enumerate
// CDP page targets, classify them by manifest surface, and pick a sticky
// default that subsequent UID-based interaction tools operate on.
package pagetool

import (
	"github.com/dsbissett/office-addin-mcp/internal/addin"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

func makeSelector(targetID, urlPattern, surface string) tools.TargetSelector {
	return tools.TargetSelector{
		TargetID:   targetID,
		URLPattern: urlPattern,
		Surface:    addin.SurfaceType(surface),
	}
}
