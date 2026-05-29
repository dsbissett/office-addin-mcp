package addin

import (
	"os"
	"path/filepath"
	"testing"
)

// surfaceByURL indexes a manifest's surfaces by their URL for assertions.
func surfaceByURL(m *Manifest) map[string]Surface {
	out := make(map[string]Surface, len(m.Surfaces))
	for _, s := range m.Surfaces {
		out[s.URL] = s
	}
	return out
}

// requirementKeys flattens a requirement-set slice to "Name@MinVersion" keys.
func requirementKeys(rs []RequirementSet) map[string]bool {
	out := make(map[string]bool, len(rs))
	for _, r := range rs {
		out[r.Name+"@"+r.MinVersion] = true
	}
	return out
}

// TestParseManifest_XMLFullOverrides drives the testdata XML fixture so the
// VersionOverrides extension-point and runtime branches in parseXMLManifest
// (and surfaceForExtensionPoint) are exercised.
func TestParseManifest_XMLFullOverrides(t *testing.T) {
	p := filepath.Join("testdata", "full-overrides.xml")
	m, err := ParseManifest(p)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if m.Kind != "xml" {
		t.Errorf("Kind = %q, want xml", m.Kind)
	}
	if m.Path != p {
		t.Errorf("Path = %q, want %q", m.Path, p)
	}
	if m.ID == "" || m.DisplayName != "Full Overrides Addin" {
		t.Errorf("identity wrong: id=%q name=%q", m.ID, m.DisplayName)
	}
	// Two hosts both retained.
	if len(m.Hosts) != 2 || m.Hosts[0] != "Document" || m.Hosts[1] != "Workbook" {
		t.Errorf("hosts = %v", m.Hosts)
	}

	// Requirements: base + VersionOverrides set, deduped, empty Names skipped.
	keys := requirementKeys(m.Requirements)
	if !keys["ExcelApi@1.7"] {
		t.Errorf("missing ExcelApi@1.7 in %v", m.Requirements)
	}
	if !keys["SharedRuntime@1.1"] {
		t.Errorf("missing SharedRuntime@1.1 (VersionOverrides) in %v", m.Requirements)
	}
	if keys["@9.9"] || keys["@0.0"] {
		t.Errorf("empty-name requirement should be skipped: %v", m.Requirements)
	}
	// ExcelApi@1.7 appears in both base and overrides — must be deduped to one.
	var excelCount int
	for _, r := range m.Requirements {
		if r.Name == "ExcelApi" && r.MinVersion == "1.7" {
			excelCount++
		}
	}
	if excelCount != 1 {
		t.Errorf("ExcelApi@1.7 not deduped: count=%d", excelCount)
	}

	byURL := surfaceByURL(m)

	// Default taskpane from DefaultSettings/SourceLocation.
	if s, ok := byURL["https://localhost:3000/taskpane.html"]; !ok || s.Type != SurfaceTaskpane {
		t.Errorf("default taskpane surface wrong: %+v ok=%v", s, ok)
	}
	// FunctionFile: direct SourceLocation, XSIType empty -> Commands default.
	if s, ok := byURL["https://localhost:3000/commands.html"]; !ok || s.Type != SurfaceCommands {
		t.Errorf("FunctionFile commands surface wrong: %+v ok=%v", s, ok)
	}
	// ExtensionPoint Page -> taskpane.
	if s, ok := byURL["https://localhost:3000/cmdpage.html"]; !ok || s.Type != SurfaceTaskpane {
		t.Errorf("cmd Page surface wrong: %+v ok=%v", s, ok)
	}
	// ContentArea -> content surface.
	if s, ok := byURL["https://localhost:3000/content.html"]; !ok || s.Type != SurfaceContent {
		t.Errorf("ContentArea surface wrong: %+v ok=%v", s, ok)
	}
	// CustomFunctions Script -> cf-runtime.
	if s, ok := byURL["https://localhost:3000/functions.js"]; !ok || s.Type != SurfaceCFRuntime {
		t.Errorf("cf script surface wrong: %+v ok=%v", s, ok)
	}
	// CustomFunctions Page -> taskpane (Page path is always taskpane).
	if s, ok := byURL["https://localhost:3000/functions.html"]; !ok || s.Type != SurfaceTaskpane {
		t.Errorf("cf page surface wrong: %+v ok=%v", s, ok)
	}
	// TaskPaneApp direct SourceLocation -> taskpane.
	if s, ok := byURL["https://localhost:3000/tp2.html"]; !ok || s.Type != SurfaceTaskpane {
		t.Errorf("taskpane-direct surface wrong: %+v ok=%v", s, ok)
	}
	// Runtime long lifetime -> shared (treated as taskpane).
	if s, ok := byURL["https://localhost:3000/shared.html"]; !ok || s.Type != SurfaceTaskpane {
		t.Errorf("long runtime surface wrong: %+v ok=%v", s, ok)
	}
	// Runtime short lifetime -> taskpane.
	if s, ok := byURL["https://localhost:3000/short.html"]; !ok || s.Type != SurfaceTaskpane {
		t.Errorf("short runtime surface wrong: %+v ok=%v", s, ok)
	}
	// Unresolvable resIDs and empty URLs produce no surface.
	if _, ok := byURL[""]; ok {
		t.Errorf("empty-url surface should be dropped")
	}
}

// TestParseManifest_JSONUnified drives the JSON fixture covering displayName
// precedence, scope-to-host mapping for every scope, capability dedup, and the
// custom-functions action detection.
func TestParseManifest_JSONUnified(t *testing.T) {
	p := filepath.Join("testdata", "unified.json")
	m, err := ParseManifest(p)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if m.Kind != "json" {
		t.Errorf("Kind = %q, want json", m.Kind)
	}
	if m.ID != "json-unified-id" {
		t.Errorf("ID = %q", m.ID)
	}
	// displayName takes precedence over name.full / name.short.
	if m.DisplayName != "Explicit Display Name" {
		t.Errorf("DisplayName = %q, want explicit", m.DisplayName)
	}

	// Every scope maps to its host; duplicate workbook deduped; unknown scope passes through.
	wantHosts := map[string]bool{
		"Workbook": true, "Document": true, "Presentation": true,
		"Mailbox": true, "weirdscope": true,
	}
	got := map[string]int{}
	for _, h := range m.Hosts {
		got[h]++
	}
	for h := range wantHosts {
		if got[h] != 1 {
			t.Errorf("host %q count = %d, want 1 (hosts=%v)", h, got[h], m.Hosts)
		}
	}

	// Capability dedup + empty-name skip.
	keys := requirementKeys(m.Requirements)
	if !keys["ExcelApi@1.7"] {
		t.Errorf("missing ExcelApi@1.7 in %v", m.Requirements)
	}
	if len(m.Requirements) != 1 {
		t.Errorf("requirements should dedup to 1, got %v", m.Requirements)
	}

	byURL := surfaceByURL(m)
	if s, ok := byURL["https://example.com/tp.html"]; !ok || s.Type != SurfaceTaskpane {
		t.Errorf("taskpane surface wrong: %+v ok=%v", s, ok)
	}
	// executeFunction action marks the page runtime as cf-runtime.
	if s, ok := byURL["https://example.com/fn.html"]; !ok || s.Type != SurfaceCFRuntime {
		t.Errorf("cf page surface wrong: %+v ok=%v", s, ok)
	}
	if s, ok := byURL["https://example.com/fn.js"]; !ok || s.Type != SurfaceCFRuntime {
		t.Errorf("cf script surface wrong: %+v ok=%v", s, ok)
	}
}

// TestParseManifest_JSONHostFallback exercises the name.short fallback for
// DisplayName and the top-level host fallback when no scopes are declared.
func TestParseManifest_JSONHostFallback(t *testing.T) {
	p := filepath.Join("testdata", "hostfallback.json")
	m, err := ParseManifest(p)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if m.DisplayName != "OnlyShort" {
		t.Errorf("DisplayName = %q, want OnlyShort (name.short fallback)", m.DisplayName)
	}
	if len(m.Hosts) != 1 || m.Hosts[0] != "Mailbox" {
		t.Errorf("hosts = %v, want [Mailbox] (top-level host fallback)", m.Hosts)
	}
	if len(m.Surfaces) != 0 {
		t.Errorf("expected no surfaces, got %v", m.Surfaces)
	}
}

// TestParseManifest_ReadError covers the os.ReadFile error path.
func TestParseManifest_ReadError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "does-not-exist.xml")
	if _, err := ParseManifest(p); err == nil {
		t.Fatalf("expected read error for missing file")
	}
}

// TestParseManifest_XMLParseError covers a file that begins with "<" but is
// not valid XML.
func TestParseManifest_XMLParseError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.xml")
	if err := os.WriteFile(p, []byte("<OfficeApp><unclosed>"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := ParseManifest(p); err == nil {
		t.Fatalf("expected XML parse error")
	}
}

// TestParseManifest_JSONParseError covers a file that begins with "{" but is
// not valid JSON.
func TestParseManifest_JSONParseError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(p, []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := ParseManifest(p); err == nil {
		t.Fatalf("expected JSON parse error")
	}
}

// TestParseManifest_BOMAndWhitespace ensures the BOM strip + leading-whitespace
// trim in the format detector still routes an XML manifest to the XML parser.
// (xml.Unmarshal tolerates a leading BOM; the detector's TrimPrefix branch is
// what is under test here.)
func TestParseManifest_BOMAndWhitespace(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bom.xml")
	body := "\ufeff   \r\n  " + `<OfficeApp><Id>bom-id</Id><DisplayName DefaultValue="B"/></OfficeApp>`
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	m, err := ParseManifest(p)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if m.Kind != "xml" || m.ID != "bom-id" || m.DisplayName != "B" {
		t.Errorf("BOM/whitespace XML not parsed: %+v", m)
	}
}
