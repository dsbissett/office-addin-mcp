package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestGolden runs the generator against the small synthetic protocol +
// manifest in testdata/ and byte-compares the output to the checked-in
// golden files. Refresh after intentional template changes by running
// `go run ./cmd/gen-cdp-tools -manifest cmd/gen-cdp-tools/testdata/fixture_manifest.yaml ...`
// against the testdata/golden directory.
func TestGolden(t *testing.T) {
	tmp := t.TempDir()
	if err := run(
		filepath.Join("testdata", "fixture_manifest.yaml"),
		filepath.Join("testdata", "fixture_protocol.json"),
		filepath.Join("testdata", "fixture_jsproto.json"),
		tmp,
	); err != nil {
		t.Fatalf("generator: %v", err)
	}

	goldenDir := filepath.Join("testdata", "golden")
	entries, err := os.ReadDir(goldenDir)
	if err != nil {
		t.Fatalf("read golden dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no golden files — was the generator output captured?")
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		want, err := os.ReadFile(filepath.Join(goldenDir, e.Name()))
		if err != nil {
			t.Fatalf("read golden %s: %v", e.Name(), err)
		}
		got, err := os.ReadFile(filepath.Join(tmp, e.Name()))
		if err != nil {
			t.Fatalf("read generated %s: %v", e.Name(), err)
		}
		if !bytes.Equal(want, got) {
			t.Errorf("%s differs from golden\n--- want ---\n%s\n--- got ---\n%s",
				e.Name(), want, got)
		}
	}
}

// TestDeterministic re-runs the generator twice into separate temp dirs and
// asserts byte equality. Catches map-iteration-order bugs that aren't visible
// from a single golden run.
func TestDeterministic(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	for _, dir := range []string{a, b} {
		if err := run(
			filepath.Join("testdata", "fixture_manifest.yaml"),
			filepath.Join("testdata", "fixture_protocol.json"),
			filepath.Join("testdata", "fixture_jsproto.json"),
			dir,
		); err != nil {
			t.Fatalf("generator: %v", err)
		}
	}

	entries, err := os.ReadDir(a)
	if err != nil {
		t.Fatalf("read a: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fa, err := os.ReadFile(filepath.Join(a, e.Name()))
		if err != nil {
			t.Fatalf("read %s in a: %v", e.Name(), err)
		}
		fb, err := os.ReadFile(filepath.Join(b, e.Name()))
		if err != nil {
			t.Fatalf("read %s in b: %v", e.Name(), err)
		}
		if !bytes.Equal(fa, fb) {
			t.Errorf("%s: non-deterministic across runs", e.Name())
		}
	}
}
