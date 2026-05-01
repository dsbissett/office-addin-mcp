package lifecycletool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
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

func TestDetectTool_HappyPath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"name":"x"}`)
	writeFile(t, filepath.Join(dir, "manifest.xml"),
		`<OfficeApp><Hosts><Host Name="Workbook"/></Hosts></OfficeApp>`)

	raw, _ := json.Marshal(map[string]string{"cwd": dir})
	res := Detect().Run(context.Background(), raw, &tools.RunEnv{})
	if res.Err != nil {
		t.Fatalf("Detect failed: %+v", res.Err)
	}
	body, _ := json.Marshal(res.Data)
	if !contains(string(body), `"manifestKind":"xml"`) {
		t.Errorf("missing manifestKind: %s", body)
	}
}

func TestDetectTool_NoProject(t *testing.T) {
	dir := t.TempDir()
	raw, _ := json.Marshal(map[string]string{"cwd": dir})
	res := Detect().Run(context.Background(), raw, &tools.RunEnv{})
	if res.Err == nil {
		t.Fatal("Detect: expected error for empty dir")
	}
	if res.Err.Code != "addin_not_found" {
		t.Errorf("Code = %s, want addin_not_found", res.Err.Code)
	}
}

func TestStopTool_NoActiveLaunch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"name":"x"}`)
	writeFile(t, filepath.Join(dir, "manifest.xml"),
		`<OfficeApp><Hosts><Host Name="Workbook"/></Hosts></OfficeApp>`)

	raw, _ := json.Marshal(map[string]string{"cwd": dir})
	res := Stop().Run(context.Background(), raw, &tools.RunEnv{})
	if res.Err != nil {
		t.Fatalf("Stop returned error for no-op: %+v", res.Err)
	}
	body, _ := json.Marshal(res.Data)
	if !contains(string(body), `"stopped":0`) {
		t.Errorf("expected stopped=0, got %s", body)
	}
}

func TestStopTool_All(t *testing.T) {
	raw, _ := json.Marshal(map[string]bool{"all": true})
	res := Stop().Run(context.Background(), raw, &tools.RunEnv{})
	if res.Err != nil {
		t.Fatalf("Stop all failed: %+v", res.Err)
	}
}

func TestRegister_AllTools(t *testing.T) {
	r := tools.NewRegistry()
	Register(r)
	for _, name := range []string{"addin.detect", "addin.launch", "addin.stop"} {
		tool, ok := r.Get(name)
		if !ok {
			t.Errorf("tool %s not registered", name)
			continue
		}
		if !tool.NoSession {
			t.Errorf("tool %s should be NoSession", name)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(sub) > 0 && indexOf(s, sub) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
