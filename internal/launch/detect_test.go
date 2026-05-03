package launch

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestDetectAddin_XMLManifest(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{
  "name": "addin",
  "scripts": {"dev-server": "webpack serve"},
  "config": {"dev_server_port": 3000}
}`)
	writeFile(t, filepath.Join(dir, "manifest.xml"), `<?xml version="1.0"?>
<OfficeApp xmlns="urn:office">
  <Hosts>
    <Host Name="Workbook" />
  </Hosts>
</OfficeApp>`)

	project, err := DetectAddin(dir)
	if err != nil {
		t.Fatalf("DetectAddin: %v", err)
	}
	if project.ManifestKind != ManifestKindXML {
		t.Errorf("ManifestKind = %s, want xml", project.ManifestKind)
	}
	if project.PackageManager != PackageManagerNpm {
		t.Errorf("PackageManager = %s, want npm", project.PackageManager)
	}
	if project.DevServer == nil || project.DevServer.Port != 3000 || project.DevServer.Script != "dev-server" {
		t.Errorf("DevServer = %+v, want {dev-server, 3000}", project.DevServer)
	}
}

func TestDetectAddin_JSONManifest(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"name":"addin"}`)
	writeFile(t, filepath.Join(dir, "manifest.json"), `{
  "extensions": [
    {"requirements": {"scopes": ["Workbook"]}}
  ]
}`)
	writeFile(t, filepath.Join(dir, "pnpm-lock.yaml"), "lockfileVersion: 6.0\n")

	project, err := DetectAddin(dir)
	if err != nil {
		t.Fatalf("DetectAddin: %v", err)
	}
	if project.ManifestKind != ManifestKindJSON {
		t.Errorf("ManifestKind = %s, want json", project.ManifestKind)
	}
	if project.PackageManager != PackageManagerPnpm {
		t.Errorf("PackageManager = %s, want pnpm", project.PackageManager)
	}
	if project.DevServer != nil {
		t.Errorf("DevServer = %+v, want nil (no scripts/config)", project.DevServer)
	}
}

func TestDetectAddin_NoProject(t *testing.T) {
	dir := t.TempDir()
	if _, err := DetectAddin(dir); err == nil {
		t.Fatal("DetectAddin: expected error for empty dir")
	}
}

func TestDetectAddin_PackageWithoutManifest(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"name":"x"}`)
	if _, err := DetectAddin(dir); err == nil {
		t.Fatal("DetectAddin: expected error when package.json has no neighboring manifest")
	}
}

func TestDetectAddin_WalksUpward(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"name":"x"}`)
	writeFile(t, filepath.Join(root, "manifest.xml"), `<OfficeApp><Hosts><Host Name="Workbook"/></Hosts></OfficeApp>`)
	deep := filepath.Join(root, "src", "subdir")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	project, err := DetectAddin(deep)
	if err != nil {
		t.Fatalf("DetectAddin: %v", err)
	}
	if project.Root != root {
		t.Errorf("Root = %s, want %s", project.Root, root)
	}
}

func TestDetectAddin_DevServerPortFromString(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{
  "scripts": {"dev": "webpack serve"},
  "config": {"dev_server_port": "8080"}
}`)
	writeFile(t, filepath.Join(dir, "manifest.xml"), `<OfficeApp><Hosts><Host Name="Workbook"/></Hosts></OfficeApp>`)

	project, err := DetectAddin(dir)
	if err != nil {
		t.Fatalf("DetectAddin: %v", err)
	}
	if project.DevServer == nil || project.DevServer.Port != 8080 {
		t.Errorf("DevServer = %+v, want port 8080", project.DevServer)
	}
}

func TestDetectAddin_NonWorkbookXMLAccepted(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{}`)
	writeFile(t, filepath.Join(dir, "manifest.xml"), `<OfficeApp><Hosts><Host Name="Document"/></Hosts></OfficeApp>`)
	project, err := DetectAddin(dir)
	if err != nil {
		t.Fatalf("DetectAddin: %v", err)
	}
	if project.ManifestKind != ManifestKindXML {
		t.Errorf("ManifestKind = %s, want xml", project.ManifestKind)
	}
}
