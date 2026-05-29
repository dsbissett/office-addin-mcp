package addin

import (
	"sort"
	"strings"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
)

// ClassifiedTarget pairs a CDP target with a surface label derived from the
// loaded manifest (or URL heuristics when no manifest is available).
type ClassifiedTarget struct {
	cdp.TargetInfo
	Surface     SurfaceType `json:"surface"`
	MatchedURL  string      `json:"matchedUrl,omitempty"`
	AddinID     string      `json:"addinId,omitempty"`
	DisplayName string      `json:"displayName,omitempty"`
}

// ClassifyTargets labels each CDP target according to the manifest's declared
// surfaces. Targets with no manifest match are labeled by URL heuristics
// (devtools://, about:blank → unrelated; http(s) page → unknown). When
// manifest is nil, only URL heuristics are used.
//
// The longest matching surface pattern wins, so a more specific match (e.g.
// "host/path/dialog.html") beats the bare host of a less specific surface.
func ClassifyTargets(targets []cdp.TargetInfo, manifest *Manifest) []ClassifiedTarget {
	var ordered []Surface
	if manifest != nil {
		ordered = append(ordered, manifest.Surfaces...)
		sort.SliceStable(ordered, func(i, j int) bool {
			return len(ordered[i].Pattern) > len(ordered[j].Pattern)
		})
	}

	out := make([]ClassifiedTarget, 0, len(targets))
	for _, t := range targets {
		ct := ClassifiedTarget{TargetInfo: t}
		if manifest != nil {
			ct.AddinID = manifest.ID
			ct.DisplayName = manifest.DisplayName
		}
		ct.Surface = classifyOne(t.URL, ordered, &ct)
		out = append(out, ct)
	}
	return out
}

// classifyOne returns the surface label for a single CDP URL. It walks the
// pre-sorted (longest pattern first) surface list and falls back to URL
// heuristics when no manifest pattern matches.
func classifyOne(rawURL string, ordered []Surface, ct *ClassifiedTarget) SurfaceType {
	for _, s := range ordered {
		if s.Pattern == "" {
			continue
		}
		if strings.Contains(rawURL, s.Pattern) {
			ct.MatchedURL = s.URL
			return s.Type
		}
	}
	return heuristicSurface(rawURL)
}

// heuristicSurface picks a label without manifest knowledge. Used as a
// fallback when the manifest is missing or no surface pattern matches.
func heuristicSurface(rawURL string) SurfaceType {
	if hasAnyPrefix(rawURL, "about:", "devtools://", "chrome://", "edge://") {
		return ""
	}
	lower := strings.ToLower(rawURL)
	switch {
	case strings.Contains(lower, "dialog"):
		return SurfaceDialog
	case isCustomFunctionsURL(lower):
		return SurfaceCFRuntime
	case hasAnyPrefix(rawURL, "http://", "https://"):
		return SurfaceTaskpane
	default:
		return ""
	}
}

// isCustomFunctionsURL reports whether a lowercased URL points at a
// custom-functions runtime page.
func isCustomFunctionsURL(lower string) bool {
	return strings.Contains(lower, "functions.html") || strings.Contains(lower, "/functions/")
}

// hasAnyPrefix reports whether s begins with any of the given prefixes.
func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// FindSurface returns the first classified target matching surface. ok=false
// if none. Callers commonly want exactly one taskpane / dialog at a time.
func FindSurface(classified []ClassifiedTarget, surface SurfaceType) (ClassifiedTarget, bool) {
	for _, c := range classified {
		if c.Surface == surface {
			return c, true
		}
	}
	return ClassifiedTarget{}, false
}
