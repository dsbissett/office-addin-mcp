package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
)

// TargetSelector picks one CDP target. An empty selector falls back to
// FirstPageTarget; if no page exists, the caller is expected to create one.
type TargetSelector struct {
	TargetID   string
	URLPattern string
}

// ResolveTarget picks a target on the live connection. When the selector is
// empty and no page targets exist, it creates a fresh "about:blank" target —
// this preserves the Phase 1 headless-Chrome behavior for default evaluate.
func ResolveTarget(ctx context.Context, conn *cdp.Connection, sel TargetSelector) (cdp.TargetInfo, error) {
	targets, err := conn.GetTargets(ctx)
	if err != nil {
		return cdp.TargetInfo{}, err
	}
	if sel.TargetID != "" {
		for _, t := range targets {
			if t.TargetID == sel.TargetID {
				return t, nil
			}
		}
		return cdp.TargetInfo{}, fmt.Errorf("no target with targetId %q", sel.TargetID)
	}
	if sel.URLPattern != "" {
		for _, t := range targets {
			if strings.Contains(t.URL, sel.URLPattern) {
				return t, nil
			}
		}
		return cdp.TargetInfo{}, fmt.Errorf("no target with url containing %q", sel.URLPattern)
	}
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
