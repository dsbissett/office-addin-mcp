package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
