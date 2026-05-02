package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	mcpserver "github.com/dsbissett/office-addin-mcp/internal/mcp"
)

func TestBuildCDPSelection_EmptyCSVPreservesEnabled(t *testing.T) {
	got, err := buildCDPSelection(true, "")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	want := mcpserver.CDPSelection{Enabled: true}
	if got.Enabled != want.Enabled || len(got.Domains) != 0 {
		t.Errorf("got = %+v, want %+v", got, want)
	}
}

func TestBuildCDPSelection_NamedDomainsImplyEnabled(t *testing.T) {
	got, err := buildCDPSelection(false, "DOM, Page,Runtime")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !got.Enabled {
		t.Error("Enabled = false, want true (non-empty domains imply enabled)")
	}
	if len(got.Domains) != 3 || got.Domains[0] != "DOM" || got.Domains[1] != "Page" || got.Domains[2] != "Runtime" {
		t.Errorf("Domains = %v, want [DOM Page Runtime]", got.Domains)
	}
}

func TestBuildCDPSelection_RejectsUnknownDomain(t *testing.T) {
	_, err := buildCDPSelection(false, "DOM,NotARealDomain,Page")
	if err == nil {
		t.Fatal("err = nil, want unknown-domain failure")
	}
	if !strings.Contains(err.Error(), "NotARealDomain") {
		t.Errorf("err = %v, want it to mention the bad name", err)
	}
}

func buildBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "office-addin-mcp")
	if os.PathSeparator == '\\' {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("go build: %v", err)
	}
	return bin
}

// TestVersionFlag verifies --version prints a non-empty version and exits 0.
func TestVersionFlag(t *testing.T) {
	bin := buildBinary(t)
	var out bytes.Buffer
	cmd := exec.Command(bin, "--version")
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run --version: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got == "" {
		t.Fatalf("expected non-empty version, got %q", got)
	}
}

// TestUnknownFlagExits2 verifies that bad flags fail with exit 2.
func TestUnknownFlagExits2(t *testing.T) {
	bin := buildBinary(t)
	cmd := exec.Command(bin, "--definitely-not-a-flag")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit for unknown flag")
	}
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 2 {
		t.Fatalf("expected exit code 2, got %v", err)
	}
}

// TestPositionalArgRejected verifies that legacy subcommand-style invocations
// (e.g. `office-addin-mcp call`) get a clear error rather than silently
// starting an MCP stdio server.
func TestPositionalArgRejected(t *testing.T) {
	bin := buildBinary(t)
	var stderr bytes.Buffer
	cmd := exec.Command(bin, "call")
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit for positional arg")
	}
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 2 {
		t.Fatalf("expected exit code 2, got %v", err)
	}
	if !strings.Contains(stderr.String(), "unexpected argument") {
		t.Errorf("missing helpful error in stderr: %q", stderr.String())
	}
}
