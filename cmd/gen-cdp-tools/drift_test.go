package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestLiveManifestDrift is the test-suite equivalent of `go generate ./... &&
// git diff --exit-code`: it runs the generator against the live manifest and
// protocol JSONs into a temp dir, then byte-compares each output file with
// the checked-in copy under internal/tools/cdptool/generated/. Any drift
// fails the test, prompting a regen + re-commit.
//
// CWD inside `go test ./cmd/gen-cdp-tools` is the package directory, so all
// paths are relative to it.
func TestLiveManifestDrift(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join("..", "..")
	if err := run(
		filepath.Join(root, "cdp", "manifest.yaml"),
		filepath.Join(root, "cdp", "protocol", "browser_protocol.json"),
		filepath.Join(root, "cdp", "protocol", "js_protocol.json"),
		tmp,
	); err != nil {
		t.Fatalf("generator: %v", err)
	}

	liveDir := filepath.Join(root, "internal", "tools", "cdptool", "generated")
	live, err := os.ReadDir(liveDir)
	if err != nil {
		t.Fatalf("read live: %v", err)
	}
	for _, e := range live {
		if e.IsDir() {
			continue
		}
		want, err := os.ReadFile(filepath.Join(liveDir, e.Name()))
		if err != nil {
			t.Fatalf("read live %s: %v", e.Name(), err)
		}
		got, err := os.ReadFile(filepath.Join(tmp, e.Name()))
		if err != nil {
			t.Fatalf("missing in regen: %s (run go generate ./...)", e.Name())
		}
		if !bytes.Equal(want, got) {
			t.Errorf("%s drifted from generator output. Run `go generate ./...` and commit.", e.Name())
		}
	}
	// And the reverse: nothing in the regen should be missing from live.
	regen, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatalf("read regen: %v", err)
	}
	for _, e := range regen {
		if e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(liveDir, e.Name())); err != nil {
			t.Errorf("regen produced %s but it's not checked in", e.Name())
		}
	}
}
