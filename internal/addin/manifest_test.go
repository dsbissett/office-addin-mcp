package addin

import (
	"os"
	"path/filepath"
	"testing"
)

const xmlSample = `<?xml version="1.0" encoding="UTF-8"?>
<OfficeApp xmlns="http://schemas.microsoft.com/office/appforoffice/1.1"
           xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
           xsi:type="TaskPaneApp">
  <Id>11111111-2222-3333-4444-555555555555</Id>
  <Version>1.0.0.0</Version>
  <ProviderName>Test</ProviderName>
  <DefaultLocale>en-US</DefaultLocale>
  <DisplayName DefaultValue="Sample Addin"/>
  <Description DefaultValue="desc"/>
  <Hosts>
    <Host Name="Workbook"/>
  </Hosts>
  <Requirements>
    <Sets DefaultMinVersion="1.1">
      <Set Name="ExcelApi" MinVersion="1.7"/>
    </Sets>
  </Requirements>
  <DefaultSettings>
    <SourceLocation DefaultValue="https://localhost:3000/taskpane.html"/>
  </DefaultSettings>
  <VersionOverrides xmlns="http://schemas.microsoft.com/office/taskpaneappversionoverrides" xsi:type="VersionOverridesV1_0">
    <Hosts>
      <Host xsi:type="Workbook">
        <DesktopFormFactor>
          <ExtensionPoint xsi:type="CustomFunctions">
            <Script><SourceLocation resid="cf-script"/></Script>
            <Page><SourceLocation resid="cf-page"/></Page>
          </ExtensionPoint>
        </DesktopFormFactor>
      </Host>
    </Hosts>
    <Resources>
      <Urls>
        <Url id="cf-script" DefaultValue="https://localhost:3000/functions.js"/>
        <Url id="cf-page" DefaultValue="https://localhost:3000/functions.html"/>
      </Urls>
    </Resources>
  </VersionOverrides>
</OfficeApp>`

func TestParseManifest_XML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "manifest.xml")
	if err := os.WriteFile(p, []byte(xmlSample), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	m, err := ParseManifest(p)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if m.ID == "" || m.DisplayName != "Sample Addin" {
		t.Errorf("identity wrong: id=%q name=%q", m.ID, m.DisplayName)
	}
	if len(m.Hosts) != 1 || m.Hosts[0] != "Workbook" {
		t.Errorf("hosts = %v", m.Hosts)
	}
	if len(m.Requirements) == 0 {
		t.Errorf("expected requirements, got none")
	}
	var foundTaskpane, foundCF bool
	for _, s := range m.Surfaces {
		if s.Type == SurfaceTaskpane && s.URL == "https://localhost:3000/taskpane.html" {
			foundTaskpane = true
		}
		if s.Type == SurfaceCFRuntime {
			foundCF = true
		}
	}
	if !foundTaskpane {
		t.Errorf("missing taskpane surface; surfaces = %+v", m.Surfaces)
	}
	if !foundCF {
		t.Errorf("missing cf-runtime surface; surfaces = %+v", m.Surfaces)
	}
}

func TestParseManifest_JSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "manifest.json")
	contents := `{
  "id": "abc",
  "name": {"short": "X", "full": "Sample"},
  "extensions": [{
    "requirements": {
      "scopes": ["workbook"],
      "capabilities": [{"name": "ExcelApi", "minVersion": "1.7"}]
    },
    "runtimes": [
      {"id":"taskpane","type":"general","code":{"page":"https://example.com/tp.html"},"actions":[{"id":"x","type":"openPage"}]},
      {"id":"cf","type":"general","code":{"page":"https://example.com/fn.html","script":"https://example.com/fn.js"},"actions":[{"id":"y","type":"customFunction"}]}
    ]
  }]
}`
	if err := os.WriteFile(p, []byte(contents), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	m, err := ParseManifest(p)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if m.ID != "abc" || m.DisplayName != "Sample" {
		t.Errorf("identity wrong: %+v", m)
	}
	if len(m.Hosts) == 0 || m.Hosts[0] != "Workbook" {
		t.Errorf("hosts = %v", m.Hosts)
	}
	var taskpane, cfPage, cfScript bool
	for _, s := range m.Surfaces {
		if s.Type == SurfaceTaskpane && s.URL == "https://example.com/tp.html" {
			taskpane = true
		}
		if s.Type == SurfaceCFRuntime && s.URL == "https://example.com/fn.html" {
			cfPage = true
		}
		if s.Type == SurfaceCFRuntime && s.URL == "https://example.com/fn.js" {
			cfScript = true
		}
	}
	if !taskpane || !cfPage || !cfScript {
		t.Errorf("missing surfaces taskpane=%v cfPage=%v cfScript=%v in %+v", taskpane, cfPage, cfScript, m.Surfaces)
	}
}

func TestUrlPattern(t *testing.T) {
	cases := map[string]string{
		"https://localhost:3000/taskpane.html": "localhost:3000/taskpane.html",
		"https://example.com/":                 "example.com",
		"file:///c/foo/bar.html":               "bar.html",
	}
	for in, want := range cases {
		if got := urlPattern(in); got != want {
			t.Errorf("urlPattern(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseManifest_UnknownFormat(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "weird.txt")
	if err := os.WriteFile(p, []byte("just text"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := ParseManifest(p); err == nil {
		t.Fatalf("expected error for non-manifest file")
	}
}
