package doccache

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doccache.json")
	s := Open(path, false)
	if s.Disabled() {
		t.Fatal("expected enabled store")
	}
	if _, ok := s.Get("excel", "Book1.xlsx"); ok {
		t.Fatal("expected cache miss on fresh store")
	}
	if err := s.Put(Entry{
		Host:        "excel",
		FilePath:    "Book1.xlsx",
		Fingerprint: "fp1",
		Data:        json.RawMessage(`{"k":"v"}`),
	}); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, ok := s.Get("excel", "Book1.xlsx")
	if !ok {
		t.Fatal("expected cache hit after put")
	}
	if got.Fingerprint != "fp1" {
		t.Errorf("fingerprint mismatch: %q", got.Fingerprint)
	}
	// Reopen: persistence round-trip.
	s2 := Open(path, false)
	got2, ok := s2.Get("excel", "Book1.xlsx")
	if !ok {
		t.Fatal("expected cache hit after reopen")
	}
	if got2.Fingerprint != "fp1" {
		t.Errorf("fingerprint after reopen: %q", got2.Fingerprint)
	}
}

func TestDisabledIsNoOp(t *testing.T) {
	s := Open("", true)
	if !s.Disabled() {
		t.Fatal("expected disabled")
	}
	if err := s.Put(Entry{Host: "excel", FilePath: "x", Fingerprint: "fp"}); err != nil {
		t.Fatalf("put on disabled returned error: %v", err)
	}
	if _, ok := s.Get("excel", "x"); ok {
		t.Fatal("disabled store should always miss")
	}
}

func TestEmptyAndTempPathsBypass(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doccache.json")
	s := Open(path, false)
	for _, fp := range []string{"", "C:\\Users\\bob\\AppData\\Local\\Temp\\foo.xlsx", "/tmp/bar.xlsx"} {
		if err := s.Put(Entry{Host: "excel", FilePath: fp, Fingerprint: "fp"}); err != nil {
			t.Fatalf("put %q: %v", fp, err)
		}
		if _, ok := s.Get("excel", fp); ok {
			t.Errorf("expected miss for non-cacheable path %q", fp)
		}
	}
}

func TestInvalidate(t *testing.T) {
	dir := t.TempDir()
	s := Open(filepath.Join(dir, "doccache.json"), false)
	_ = s.Put(Entry{Host: "excel", FilePath: "Book1.xlsx", Fingerprint: "fp"})
	if _, ok := s.Get("excel", "Book1.xlsx"); !ok {
		t.Fatal("setup: expected hit")
	}
	if err := s.Invalidate("excel", "Book1.xlsx"); err != nil {
		t.Fatalf("invalidate: %v", err)
	}
	if _, ok := s.Get("excel", "Book1.xlsx"); ok {
		t.Fatal("expected miss after invalidate")
	}
}
