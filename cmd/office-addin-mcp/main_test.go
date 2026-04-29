package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestVersionSubcommand builds the binary and runs `version`, verifying the
// Phase 0 acceptance criterion: `go build ./...` produces a binary that
// prints version.
func TestVersionSubcommand(t *testing.T) {
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

	cases := []string{"version", "--version", "-v"}
	for _, arg := range cases {
		t.Run(arg, func(t *testing.T) {
			var out bytes.Buffer
			cmd := exec.Command(bin, arg)
			cmd.Stdout = &out
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("run %q: %v", arg, err)
			}
			got := strings.TrimSpace(out.String())
			if got == "" {
				t.Fatalf("expected non-empty version, got %q", got)
			}
		})
	}
}

func TestUnknownSubcommandExits2(t *testing.T) {
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

	cmd := exec.Command(bin, "nope")
	if err := cmd.Run(); err == nil {
		t.Fatalf("expected non-zero exit for unknown subcommand")
	} else if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 2 {
		t.Fatalf("expected exit code 2, got %v", err)
	}
}

func TestNoArgsExits2(t *testing.T) {
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

	cmd := exec.Command(bin)
	if err := cmd.Run(); err == nil {
		t.Fatalf("expected non-zero exit when no args provided")
	} else if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 2 {
		t.Fatalf("expected exit code 2, got %v", err)
	}
}
