package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/dsbissett/office-addin-mcp/internal/addin"
	"github.com/dsbissett/office-addin-mcp/internal/cdp"
)

// TargetSelector picks one CDP target. An empty selector falls back to
// FirstPageTarget; if no page exists, the caller is expected to create one.
//
// Selectors are evaluated in priority order: TargetID, URLPattern, Surface,
// then default. Surface resolution requires a parsed manifest — when no
// manifest is loaded, Surface falls back to URL heuristics from
// internal/addin.ClassifyTargets.
type TargetSelector struct {
	TargetID   string
	URLPattern string
	// Surface selects a target by its manifest-declared role
	// ("taskpane", "content", "dialog", "cf-runtime"). Empty disables
	// surface matching.
	Surface addin.SurfaceType
	// AddinID restricts Surface matching to a specific manifest ID. Useful
	// only when several manifests are loaded simultaneously.
	AddinID string
}

// ResolveTarget picks a target on the live connection. When the selector is
// empty and no page targets exist, it creates a fresh "about:blank" target —
// this preserves the Phase 1 headless-Chrome behavior for default evaluate.
//
// manifest may be nil; in that case Surface selection falls back to the URL
// heuristics in internal/addin.heuristicSurface.
func ResolveTarget(ctx context.Context, conn *cdp.Connection, sel TargetSelector, manifest *addin.Manifest) (cdp.TargetInfo, error) {
	targets, err := conn.GetTargets(ctx)
	if err != nil {
		return cdp.TargetInfo{}, err
	}
	switch {
	case sel.TargetID != "":
		return targetByID(targets, sel.TargetID)
	case sel.URLPattern != "":
		return targetByURLPattern(targets, sel.URLPattern)
	case sel.Surface != "":
		return targetBySurface(targets, sel, manifest)
	}
	return defaultTarget(ctx, conn, targets)
}

func targetByID(targets []cdp.TargetInfo, targetID string) (cdp.TargetInfo, error) {
	for _, t := range targets {
		if t.TargetID == targetID {
			return t, nil
		}
	}
	return cdp.TargetInfo{}, fmt.Errorf("no target with targetId %q", targetID)
}

func targetByURLPattern(targets []cdp.TargetInfo, pattern string) (cdp.TargetInfo, error) {
	for _, t := range targets {
		if strings.Contains(t.URL, pattern) {
			return t, nil
		}
	}
	return cdp.TargetInfo{}, fmt.Errorf("no target with url containing %q", pattern)
}

func targetBySurface(targets []cdp.TargetInfo, sel TargetSelector, manifest *addin.Manifest) (cdp.TargetInfo, error) {
	for _, ct := range addin.ClassifyTargets(targets, manifest) {
		if surfaceMatches(ct, sel, manifest) {
			return ct.TargetInfo, nil
		}
	}
	return cdp.TargetInfo{}, fmt.Errorf("no target classified as surface %q", sel.Surface)
}

// surfaceMatches reports whether a classified target satisfies the surface
// selector, honoring the optional add-in id restriction.
func surfaceMatches(ct addin.ClassifiedTarget, sel TargetSelector, manifest *addin.Manifest) bool {
	if ct.Surface != sel.Surface {
		return false
	}
	if sel.AddinID != "" && manifest != nil && !strings.EqualFold(manifest.ID, sel.AddinID) {
		return false
	}
	return true
}

// defaultTarget returns the first page target, creating an "about:blank" page
// when none exists.
func defaultTarget(ctx context.Context, conn *cdp.Connection, targets []cdp.TargetInfo) (cdp.TargetInfo, error) {
	if t, ok := cdp.FirstPageTarget(targets); ok {
		return t, nil
	}
	tid, err := conn.CreateTarget(ctx, "about:blank")
	if err != nil {
		return cdp.TargetInfo{}, fmt.Errorf("no page target available and createTarget failed: %w", err)
	}
	return cdp.TargetInfo{TargetID: tid, Type: "page", URL: "about:blank"}, nil
}

// IsInternalURL reports whether a URL is a browser-internal scheme that should
// be hidden from default tool listings.
func IsInternalURL(u string) bool {
	switch {
	case strings.HasPrefix(u, "devtools://"),
		strings.HasPrefix(u, "chrome://"),
		strings.HasPrefix(u, "edge://"):
		return true
	}
	return false
}
