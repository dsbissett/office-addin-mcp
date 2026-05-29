// Package addin owns Office add-in domain knowledge: parsing manifests,
// classifying CDP targets against the surfaces a manifest declares, and the
// embedded helpers that probe Office.context (requirement sets, custom
// functions runtime, dialog API). The package has no CDP dependency itself —
// it is consumed by tools/addintool, which wires the parser and classifier
// into agent-facing tools.
package addin

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// SurfaceType labels the kind of webview/runtime a manifest entry expects.
type SurfaceType string

const (
	SurfaceTaskpane  SurfaceType = "taskpane"
	SurfaceContent   SurfaceType = "content"
	SurfaceDialog    SurfaceType = "dialog"
	SurfaceCFRuntime SurfaceType = "cf-runtime"
	SurfaceCommands  SurfaceType = "commands"
)

// Surface is one entry point declared by a manifest: the URL the host loads
// for that surface plus a substring pattern useful for matching CDP target URLs.
type Surface struct {
	Type    SurfaceType `json:"type"`
	URL     string      `json:"url,omitempty"`
	Pattern string      `json:"pattern,omitempty"`
}

// RequirementSet is one Set entry from a manifest's <Requirements> block (or
// the JSON manifest's `requirements.formFactors[].requirements.sets[]`).
type RequirementSet struct {
	Name       string `json:"name"`
	MinVersion string `json:"minVersion,omitempty"`
}

// Manifest is the structured projection of an Office add-in manifest. Only
// fields useful to target classification, requirement probing, and identity
// reporting are extracted. Unknown elements are ignored.
type Manifest struct {
	Path         string           `json:"path"`
	Kind         string           `json:"kind"`
	ID           string           `json:"id,omitempty"`
	DisplayName  string           `json:"displayName,omitempty"`
	Hosts        []string         `json:"hosts,omitempty"`
	Requirements []RequirementSet `json:"requirements,omitempty"`
	Surfaces     []Surface        `json:"surfaces,omitempty"`
}

// ErrUnknownManifest is returned when a path is neither a recognizable XML
// nor JSON Office add-in manifest.
var ErrUnknownManifest = errors.New("addin: file is not an Office add-in manifest")

// ParseManifest reads the file at path and returns the structured Manifest.
// XML manifests are detected by leading `<` after stripping whitespace/BOM;
// everything else is parsed as JSON.
func ParseManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("addin: read %s: %w", path, err)
	}
	trimmed := strings.TrimLeft(strings.TrimPrefix(string(data), "\ufeff"), " \t\r\n")
	switch {
	case strings.HasPrefix(trimmed, "<"):
		return parseManifestKind(path, "xml", data, parseXMLManifest)
	case strings.HasPrefix(trimmed, "{"):
		return parseManifestKind(path, "json", data, parseJSONManifest)
	default:
		return nil, ErrUnknownManifest
	}
}

// parseManifestKind runs the given parser, wraps any error with the path, and
// stamps the resulting manifest with its source path and kind.
func parseManifestKind(path, kind string, data []byte, parse func([]byte) (*Manifest, error)) (*Manifest, error) {
	m, err := parse(data)
	if err != nil {
		return nil, fmt.Errorf("addin: parse %s: %w", path, err)
	}
	m.Path = path
	m.Kind = kind
	return m, nil
}

type xmlOfficeApp struct {
	XMLName     xml.Name `xml:"OfficeApp"`
	ID          string   `xml:"Id"`
	DisplayName struct {
		DefaultValue string `xml:"DefaultValue,attr"`
	} `xml:"DisplayName"`
	Hosts struct {
		Hosts []struct {
			Name string `xml:"Name,attr"`
		} `xml:"Host"`
	} `xml:"Hosts"`
	Requirements struct {
		Sets struct {
			Sets []struct {
				Name       string `xml:"Name,attr"`
				MinVersion string `xml:"MinVersion,attr"`
			} `xml:"Set"`
		} `xml:"Sets"`
	} `xml:"Requirements"`
	DefaultSettings struct {
		SourceLocation struct {
			DefaultValue string `xml:"DefaultValue,attr"`
		} `xml:"SourceLocation"`
	} `xml:"DefaultSettings"`
	VersionOverrides *xmlVersionOverrides `xml:"VersionOverrides"`
}

type xmlVersionOverrides struct {
	Hosts struct {
		Hosts []xmlVOHost `xml:"Host"`
	} `xml:"Hosts"`
	Resources struct {
		URLs struct {
			Items []struct {
				ID         string `xml:"id,attr"`
				DefaultVal string `xml:"DefaultValue,attr"`
			} `xml:"Url"`
		} `xml:"Urls"`
	} `xml:"Resources"`
	Requirements struct {
		Sets struct {
			Sets []struct {
				Name       string `xml:"Name,attr"`
				MinVersion string `xml:"MinVersion,attr"`
			} `xml:"Set"`
		} `xml:"Sets"`
	} `xml:"Requirements"`
}

type xmlVOHost struct {
	XSIType         string `xml:"type,attr"`
	DesktopFormFact struct {
		ExtensionPoints []xmlExtensionPoint `xml:",any"`
	} `xml:"DesktopFormFactor"`
	Runtimes struct {
		Runtimes []struct {
			ID         string `xml:"id,attr"`
			LifeTime   string `xml:"lifetime,attr"`
			ResourceID string `xml:"resid,attr"`
		} `xml:"Runtime"`
	} `xml:"Runtimes"`
}

type xmlExtensionPoint struct {
	XMLName xml.Name
	XSIType string `xml:"type,attr"`
	// SourceLocation appears at this level for FunctionFile / CustomFunctions.
	SourceLocation struct {
		ResID string `xml:"resid,attr"`
	} `xml:"SourceLocation"`
	// Script is used by CustomFunctions extension points.
	Script struct {
		SourceLocation struct {
			ResID string `xml:"resid,attr"`
		} `xml:"SourceLocation"`
	} `xml:"Script"`
	Page struct {
		SourceLocation struct {
			ResID string `xml:"resid,attr"`
		} `xml:"SourceLocation"`
	} `xml:"Page"`
}

func parseXMLManifest(data []byte) (*Manifest, error) {
	var doc xmlOfficeApp
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	m := &Manifest{
		ID:          strings.TrimSpace(doc.ID),
		DisplayName: doc.DisplayName.DefaultValue,
	}
	applyXMLHostsAndRequirements(m, &doc)
	applyXMLDefaultTaskpane(m, &doc)
	if doc.VersionOverrides != nil {
		applyXMLVersionOverrides(m, doc.VersionOverrides)
	}
	return m, nil
}

// applyXMLHostsAndRequirements copies the top-level <Hosts> and <Requirements>
// blocks onto the manifest.
func applyXMLHostsAndRequirements(m *Manifest, doc *xmlOfficeApp) {
	for _, h := range doc.Hosts.Hosts {
		if h.Name != "" {
			m.Hosts = append(m.Hosts, h.Name)
		}
	}
	for _, s := range doc.Requirements.Sets.Sets {
		if s.Name == "" {
			continue
		}
		m.Requirements = append(m.Requirements, RequirementSet{Name: s.Name, MinVersion: s.MinVersion})
	}
}

// applyXMLDefaultTaskpane records the taskpane surface declared by
// <DefaultSettings><SourceLocation/>.
func applyXMLDefaultTaskpane(m *Manifest, doc *xmlOfficeApp) {
	if u := strings.TrimSpace(doc.DefaultSettings.SourceLocation.DefaultValue); u != "" {
		m.Surfaces = append(m.Surfaces, Surface{
			Type: SurfaceTaskpane, URL: u, Pattern: urlPattern(u),
		})
	}
}

// applyXMLVersionOverrides folds the <VersionOverrides> requirements and
// per-host surfaces (extension points + runtimes) into the manifest.
func applyXMLVersionOverrides(m *Manifest, vo *xmlVersionOverrides) {
	urls := map[string]string{}
	for _, u := range vo.Resources.URLs.Items {
		urls[u.ID] = u.DefaultVal
	}
	for _, s := range vo.Requirements.Sets.Sets {
		if s.Name == "" {
			continue
		}
		m.Requirements = appendRequirementUnique(m.Requirements, RequirementSet{Name: s.Name, MinVersion: s.MinVersion})
	}
	for _, h := range vo.Hosts.Hosts {
		applyXMLVOHost(m, urls, h)
	}
}

// applyXMLVOHost records the extension-point and runtime surfaces declared by
// one <VersionOverrides> host.
func applyXMLVOHost(m *Manifest, urls map[string]string, h xmlVOHost) {
	for _, ep := range h.DesktopFormFact.ExtensionPoints {
		applyXMLExtensionPoint(m, urls, ep)
	}
	for _, rt := range h.Runtimes.Runtimes {
		applyXMLRuntime(m, urls, rt.ResourceID)
	}
}

// applyXMLExtensionPoint resolves the URLs an extension point references and
// records a surface for each one that resolves.
func applyXMLExtensionPoint(m *Manifest, urls map[string]string, ep xmlExtensionPoint) {
	if u, ok := resolveResID(urls, ep.SourceLocation.ResID); ok {
		m.Surfaces = appendSurfaceUnique(m.Surfaces, surfaceForExtensionPoint(ep.XSIType, u))
	}
	if u, ok := resolveResID(urls, ep.Page.SourceLocation.ResID); ok {
		m.Surfaces = appendSurfaceUnique(m.Surfaces, Surface{Type: SurfaceTaskpane, URL: u, Pattern: urlPattern(u)})
	}
	if u, ok := resolveResID(urls, ep.Script.SourceLocation.ResID); ok {
		m.Surfaces = appendSurfaceUnique(m.Surfaces, Surface{Type: SurfaceCFRuntime, URL: u, Pattern: urlPattern(u)})
	}
}

// applyXMLRuntime records a taskpane surface for a runtime resource. Both
// short- and long-lifetime (shared) runtimes are treated as taskpanes.
func applyXMLRuntime(m *Manifest, urls map[string]string, resID string) {
	u, ok := resolveResID(urls, resID)
	if !ok {
		return
	}
	m.Surfaces = appendSurfaceUnique(m.Surfaces, Surface{Type: SurfaceTaskpane, URL: u, Pattern: urlPattern(u)})
}

func surfaceForExtensionPoint(xsiType, u string) Surface {
	t := SurfaceCommands
	switch {
	case containsFold(xsiType, "CustomFunctions"):
		t = SurfaceCFRuntime
	case containsFold(xsiType, "TaskPane"):
		t = SurfaceTaskpane
	case containsFold(xsiType, "ContentArea"):
		t = SurfaceContent
	}
	return Surface{Type: t, URL: u, Pattern: urlPattern(u)}
}

type jsonManifest struct {
	ID   string `json:"id"`
	Name struct {
		Short string `json:"short"`
		Full  string `json:"full"`
	} `json:"name"`
	DisplayName   string          `json:"displayName"`
	Authorization struct{}        `json:"authorization"`
	Extensions    []jsonExtension `json:"extensions"`
	Host          string          `json:"host"`
}

type jsonExtension struct {
	Requirements struct {
		Scopes       []string `json:"scopes"`
		FormFactors  []string `json:"formFactors"`
		Capabilities []struct {
			Name       string `json:"name"`
			MinVersion string `json:"minVersion"`
		} `json:"capabilities"`
	} `json:"requirements"`
	Runtimes []jsonRuntime `json:"runtimes"`
}

type jsonRuntime struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Code struct {
		Page   string `json:"page"`
		Script string `json:"script"`
	} `json:"code"`
	Lifetime string `json:"lifetime"`
	Actions  []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"actions"`
}

func parseJSONManifest(data []byte) (*Manifest, error) {
	var doc jsonManifest
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	m := &Manifest{ID: doc.ID, DisplayName: jsonDisplayName(&doc)}
	for _, ext := range doc.Extensions {
		applyJSONExtension(m, ext)
	}
	if len(m.Hosts) == 0 && doc.Host != "" {
		m.Hosts = []string{doc.Host}
	}
	return m, nil
}

// jsonDisplayName picks the friendly name, preferring the explicit
// displayName, then name.full, then name.short.
func jsonDisplayName(doc *jsonManifest) string {
	switch {
	case doc.DisplayName != "":
		return doc.DisplayName
	case doc.Name.Full != "":
		return doc.Name.Full
	default:
		return doc.Name.Short
	}
}

// applyJSONExtension folds one extension's scopes, capabilities, and runtimes
// into the manifest.
func applyJSONExtension(m *Manifest, ext jsonExtension) {
	for _, scope := range ext.Requirements.Scopes {
		m.Hosts = appendStringUnique(m.Hosts, jsonScopeToHost(scope))
	}
	for _, c := range ext.Requirements.Capabilities {
		if c.Name == "" {
			continue
		}
		m.Requirements = appendRequirementUnique(m.Requirements, RequirementSet{Name: c.Name, MinVersion: c.MinVersion})
	}
	for _, rt := range ext.Runtimes {
		applyJSONRuntime(m, rt)
	}
}

// applyJSONRuntime records the page and script surfaces for one runtime. A
// page that drives a custom-function action is classified as a CF runtime.
func applyJSONRuntime(m *Manifest, rt jsonRuntime) {
	if rt.Code.Page != "" {
		t := SurfaceTaskpane
		if hasCustomFunctionsAction(rt.Actions) {
			t = SurfaceCFRuntime
		}
		m.Surfaces = appendSurfaceUnique(m.Surfaces, Surface{Type: t, URL: rt.Code.Page, Pattern: urlPattern(rt.Code.Page)})
	}
	if rt.Code.Script != "" {
		m.Surfaces = appendSurfaceUnique(m.Surfaces, Surface{Type: SurfaceCFRuntime, URL: rt.Code.Script, Pattern: urlPattern(rt.Code.Script)})
	}
}

func hasCustomFunctionsAction(actions []struct {
	ID   string `json:"id"`
	Type string `json:"type"`
},
) bool {
	for _, a := range actions {
		if strings.EqualFold(a.Type, "customFunction") || strings.EqualFold(a.Type, "executeFunction") {
			return true
		}
	}
	return false
}

func jsonScopeToHost(scope string) string {
	switch strings.ToLower(scope) {
	case "workbook":
		return "Workbook"
	case "document":
		return "Document"
	case "presentation":
		return "Presentation"
	case "mailbox":
		return "Mailbox"
	default:
		return scope
	}
}

func resolveResID(urls map[string]string, resID string) (string, bool) {
	if resID == "" {
		return "", false
	}
	v, ok := urls[resID]
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

// urlPattern returns a substring useful for matching this URL against a CDP
// target URL. For http(s) URLs we use host+path; for file:// or webview://
// schemes we use the path. The pattern is intentionally lossy — CDP target
// URLs frequently differ from manifest URLs in their query/hash and may be
// opened from a CDN-style origin.
func urlPattern(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		// Fall back to the basename — useful for file:// and resource paths.
		return filepath.Base(raw)
	}
	return hostPathPattern(parsed.Host, parsed.Path)
}

// hostPathPattern joins host and path, dropping an empty or root-only path so
// the pattern stays a useful substring for matching CDP target URLs.
func hostPathPattern(host, path string) string {
	if path == "/" || path == "" {
		return host
	}
	return host + path
}

func appendStringUnique(out []string, v string) []string {
	if v == "" {
		return out
	}
	for _, e := range out {
		if e == v {
			return out
		}
	}
	return append(out, v)
}

func appendRequirementUnique(out []RequirementSet, r RequirementSet) []RequirementSet {
	for _, e := range out {
		if e.Name == r.Name {
			return out
		}
	}
	return append(out, r)
}

func appendSurfaceUnique(out []Surface, s Surface) []Surface {
	if s.URL == "" {
		return out
	}
	for _, e := range out {
		if e.URL == s.URL && e.Type == s.Type {
			return out
		}
	}
	return append(out, s)
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
