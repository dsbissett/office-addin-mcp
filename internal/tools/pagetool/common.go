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

// requireSelector enforces that at least one of the three selector fields is
// set. When none are, it returns the missing_selector validation failure and
// ok=false; otherwise ok=true and the result is unused.
func requireSelector(targetID, urlPattern, surface string) (tools.Result, bool) {
	if targetID == "" && urlPattern == "" && surface == "" {
		return tools.Fail(tools.CategoryValidation, "missing_selector", "provide one of: targetId, urlPattern, surface", false), false
	}
	return tools.Result{}, true
}
