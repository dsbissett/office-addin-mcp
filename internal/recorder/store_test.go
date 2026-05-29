package recorder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

// newStore is a small helper that creates a Store in a fresh temp dir.
func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestNewCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "macros")
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil store")
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat created dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected New to create a directory")
	}
	if s.cache == nil {
		t.Fatal("expected initialized cache map")
	}
}

func TestNewMkdirFails(t *testing.T) {
	// Create a regular file, then ask New to MkdirAll a path *under* that file.
	// On every OS, mkdir of a child of a non-directory fails.
	base := t.TempDir()
	file := filepath.Join(base, "afile")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("setup write: %v", err)
	}
	_, err := New(filepath.Join(file, "macros"))
	if err == nil {
		t.Fatal("expected error when MkdirAll target is under a file")
	}
}

func TestStartRecordingAndAlreadyRecording(t *testing.T) {
	s := newStore(t)
	if err := s.StartRecording("m1"); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}
	if s.active != "m1" {
		t.Fatalf("active = %q, want m1", s.active)
	}
	// Second StartRecording must fail while one is active.
	if err := s.StartRecording("m2"); err == nil {
		t.Fatal("expected error when already recording")
	}
}

func TestStartRecordingClearsStaleBuffer(t *testing.T) {
	s := newStore(t)
	if err := s.StartRecording("m1"); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}
	if err := s.Append("excel.runScript", json.RawMessage(`{"a":1}`)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if _, err := s.StopRecording(); err != nil {
		t.Fatalf("StopRecording: %v", err)
	}
	// A new recording must start with an empty buffer.
	if err := s.StartRecording("m2"); err != nil {
		t.Fatalf("second StartRecording: %v", err)
	}
	if s.buf != nil {
		t.Fatalf("expected nil buffer at start of new recording, got %v", s.buf)
	}
}

func TestAppendNotRecording(t *testing.T) {
	s := newStore(t)
	if err := s.Append("excel.runScript", json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected error appending while not recording")
	}
}

func TestAppendMalformedParams(t *testing.T) {
	s := newStore(t)
	if err := s.StartRecording("m"); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}
	if err := s.Append("excel.runScript", json.RawMessage(`{not json`)); err == nil {
		t.Fatal("expected unmarshal error for malformed params")
	}
}

func TestAppendEmptyParams(t *testing.T) {
	s := newStore(t)
	if err := s.StartRecording("m"); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}
	// Zero-length params should be accepted and stored with nil Params.
	if err := s.Append("noop", nil); err != nil {
		t.Fatalf("Append nil params: %v", err)
	}
	if err := s.Append("noop2", json.RawMessage(``)); err != nil {
		t.Fatalf("Append empty params: %v", err)
	}
	m, err := s.StopRecording()
	if err != nil {
		t.Fatalf("StopRecording: %v", err)
	}
	if len(m.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m.Entries))
	}
	for i, e := range m.Entries {
		if e.Params != nil {
			t.Errorf("entry %d: expected nil Params, got %v", i, e.Params)
		}
	}
}

func TestRecordRoundTrip(t *testing.T) {
	s := newStore(t)
	if err := s.StartRecording("macroA"); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}
	if err := s.Append("excel.runScript", json.RawMessage(`{"script":"x"}`)); err != nil {
		t.Fatalf("Append 1: %v", err)
	}
	if err := s.Append("word.applyEdits", json.RawMessage(`{"edits":[1,2,3]}`)); err != nil {
		t.Fatalf("Append 2: %v", err)
	}
	m, err := s.StopRecording()
	if err != nil {
		t.Fatalf("StopRecording: %v", err)
	}
	if m.Name != "macroA" {
		t.Errorf("Name = %q, want macroA", m.Name)
	}
	if len(m.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m.Entries))
	}
	if m.Entries[0].Tool != "excel.runScript" {
		t.Errorf("entry 0 tool = %q", m.Entries[0].Tool)
	}

	// Active recording must be cleared after stop.
	if s.active != "" {
		t.Errorf("active = %q, want empty after stop", s.active)
	}

	// Cache should now hold the macro.
	got, ok := s.Get("macroA")
	if !ok {
		t.Fatal("expected cache hit after StopRecording")
	}
	if got.Name != "macroA" || len(got.Entries) != 2 {
		t.Errorf("cached macro mismatch: %+v", got)
	}

	// File should exist on disk at the expected path.
	path := filepath.Join(s.dir, "macroA.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read macro file: %v", err)
	}
	var disk Macro
	if err := json.Unmarshal(data, &disk); err != nil {
		t.Fatalf("unmarshal disk macro: %v", err)
	}
	if disk.Name != "macroA" || len(disk.Entries) != 2 {
		t.Errorf("disk macro mismatch: %+v", disk)
	}

	// The temp file must have been renamed away.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("expected temp file to be gone, stat err = %v", err)
	}
}

func TestStopRecordingNotRecording(t *testing.T) {
	s := newStore(t)
	if _, err := s.StopRecording(); err == nil {
		t.Fatal("expected error stopping when not recording")
	}
}

func TestStopRecordingWriteFails(t *testing.T) {
	s := newStore(t)
	if err := s.StartRecording("blocked"); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}
	// Place a directory exactly where the temp file would be written so
	// os.WriteFile fails.
	tmpPath := filepath.Join(s.dir, "blocked.json.tmp")
	if err := os.MkdirAll(tmpPath, 0o755); err != nil {
		t.Fatalf("setup mkdir tmp: %v", err)
	}
	if _, err := s.StopRecording(); err == nil {
		t.Fatal("expected write error when temp path is a directory")
	}
}

func TestStopRecordingRenameFails(t *testing.T) {
	s := newStore(t)
	if err := s.StartRecording("collide"); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}
	// Place a non-empty directory exactly where the final file should be
	// renamed to. Rename onto a non-empty directory fails on all platforms.
	finalPath := filepath.Join(s.dir, "collide.json")
	if err := os.MkdirAll(finalPath, 0o755); err != nil {
		t.Fatalf("setup mkdir final: %v", err)
	}
	if err := os.WriteFile(filepath.Join(finalPath, "child"), []byte("x"), 0o600); err != nil {
		t.Fatalf("setup write child: %v", err)
	}
	if _, err := s.StopRecording(); err == nil {
		t.Fatal("expected rename error when final path is a non-empty directory")
	}
	// The temp file should have been cleaned up after rename failure.
	if _, err := os.Stat(finalPath + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("expected temp file removed after rename failure, stat err = %v", err)
	}
}

func TestLoadAllEmptyDirReturnsCache(t *testing.T) {
	s := newStore(t)
	got, err := s.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty cache, got %d entries", len(got))
	}
}

func TestLoadAllMissingDirReturnsCache(t *testing.T) {
	s := newStore(t)
	// Point the store at a directory that does not exist.
	s.dir = filepath.Join(t.TempDir(), "does-not-exist")
	got, err := s.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll on missing dir: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty cache, got %d entries", len(got))
	}
}

func TestLoadAllRegularFileAsDir(t *testing.T) {
	s := newStore(t)
	// Point s.dir at a regular file. On Windows, os.ReadDir of a regular file
	// returns an error for which os.IsNotExist (and errors.Is(err, ErrNotExist))
	// is TRUE, so LoadAll treats it as a missing directory and returns the
	// (empty) cache with no error. We assert the observable behavior rather than
	// a specific error to stay portable.
	f := filepath.Join(t.TempDir(), "regular")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatalf("setup write: %v", err)
	}
	s.dir = f
	got, err := s.LoadAll()
	if runtime.GOOS == "windows" {
		if err != nil {
			t.Fatalf("LoadAll on file-as-dir (windows): unexpected error %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("expected empty cache, got %d", len(got))
		}
		return
	}
	// On non-windows, ReadDir of a regular file yields a non-NotExist error and
	// LoadAll propagates it.
	if err == nil {
		t.Fatal("expected readdir error on non-windows")
	}
}

func TestLoadAllReaddirNonNotExistError(t *testing.T) {
	s := newStore(t)
	// A path containing a NUL byte cannot be opened; os.ReadDir returns
	// "invalid argument" (EINVAL), which is NOT classified as ErrNotExist, so
	// LoadAll must propagate it as a wrapped error.
	s.dir = "bad\x00path"
	_, err := s.LoadAll()
	if err == nil {
		t.Fatal("expected a non-NotExist readdir error to propagate")
	}
}

// TestLoadAllLoadsMacrosFromDisk verifies LoadAll reads and unmarshals every
// .json macro file on disk into the cache, while skipping non-.json files and
// subdirectories. The macro name is derived from the filename (with .json
// stripped), overriding whatever Name was serialized on disk.
func TestLoadAllLoadsMacrosFromDisk(t *testing.T) {
	s := newStore(t)

	good := Macro{Name: "ignored-on-disk", Entries: []Entry{{Tool: "t", Params: map[string]any{"k": "v"}}}}
	goodData, err := json.MarshalIndent(good, "", "  ")
	if err != nil {
		t.Fatalf("marshal good: %v", err)
	}
	if err := os.WriteFile(filepath.Join(s.dir, "good.json"), goodData, 0o600); err != nil {
		t.Fatalf("write good: %v", err)
	}
	// A non-json file and a subdirectory are also present to exercise the loop;
	// neither should be loaded.
	if err := os.WriteFile(filepath.Join(s.dir, "notes.txt"), []byte("hi"), 0o600); err != nil {
		t.Fatalf("write notes: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(s.dir, "subdir"), 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	out, err := s.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected exactly one macro loaded; got %d: %v", len(out), keys(out))
	}
	m, ok := out["good"]
	if !ok {
		t.Fatalf("expected macro keyed by filename %q; got keys %v", "good", keys(out))
	}
	// Name is derived from the filename, not the on-disk Name field.
	if m.Name != "good" {
		t.Errorf("Name = %q, want %q (derived from filename)", m.Name, "good")
	}
	if len(m.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(m.Entries))
	}
	if m.Entries[0].Tool != "t" {
		t.Errorf("entry tool = %q, want %q", m.Entries[0].Tool, "t")
	}
	params, ok := m.Entries[0].Params.(map[string]any)
	if !ok {
		t.Fatalf("expected map params, got %T", m.Entries[0].Params)
	}
	if params["k"] != "v" {
		t.Errorf("params[k] = %v, want %q", params["k"], "v")
	}

	// LoadAll must be idempotent: a second pass returns the same loaded macro.
	out2, err := s.LoadAll()
	if err != nil {
		t.Fatalf("second LoadAll: %v", err)
	}
	if out2["good"] != m {
		t.Error("expected the cached macro pointer to be reused on a second LoadAll")
	}
}

func TestLoadAllPreservesCache(t *testing.T) {
	s := newStore(t)
	// A macro already in the cache must survive a LoadAll pass: the cache-hit
	// short-circuit skips re-reading the on-disk file, so the cached entries win.
	cached := &Macro{Name: "dup", Entries: []Entry{{Tool: "from-cache"}}}
	s.cache["dup"] = cached

	disk := Macro{Name: "dup", Entries: []Entry{{Tool: "from-disk"}}}
	diskData, err := json.MarshalIndent(disk, "", "  ")
	if err != nil {
		t.Fatalf("marshal disk: %v", err)
	}
	if err := os.WriteFile(filepath.Join(s.dir, "dup.json"), diskData, 0o600); err != nil {
		t.Fatalf("write dup: %v", err)
	}

	out, err := s.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if out["dup"] != cached {
		t.Fatal("expected cached macro pointer to be preserved")
	}
	if len(out["dup"].Entries) != 1 || out["dup"].Entries[0].Tool != "from-cache" {
		t.Errorf("expected cached entries to win, got %+v", out["dup"].Entries)
	}
}

func TestGetMissAndHit(t *testing.T) {
	s := newStore(t)
	if _, ok := s.Get("absent"); ok {
		t.Fatal("expected miss for absent macro")
	}
	m := &Macro{Name: "present"}
	s.cache["present"] = m
	got, ok := s.Get("present")
	if !ok {
		t.Fatal("expected hit for present macro")
	}
	if got != m {
		t.Fatal("expected the same cached pointer")
	}
}

func TestListSorted(t *testing.T) {
	s := newStore(t)
	if got := s.List(); len(got) != 0 {
		t.Fatalf("expected empty list, got %v", got)
	}
	s.cache["charlie"] = &Macro{Name: "charlie"}
	s.cache["alpha"] = &Macro{Name: "alpha"}
	s.cache["bravo"] = &Macro{Name: "bravo"}
	got := s.List()
	want := []string{"alpha", "bravo", "charlie"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("List() = %v, want %v", got, want)
	}
}

func TestDeleteRemovesFileAndCache(t *testing.T) {
	s := newStore(t)
	if err := s.StartRecording("gone"); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}
	if err := s.Append("t", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if _, err := s.StopRecording(); err != nil {
		t.Fatalf("StopRecording: %v", err)
	}
	path := filepath.Join(s.dir, "gone.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("setup: macro file should exist: %v", err)
	}
	if _, ok := s.Get("gone"); !ok {
		t.Fatal("setup: macro should be cached")
	}

	if err := s.Delete("gone"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected file removed, stat err = %v", err)
	}
	if _, ok := s.Get("gone"); ok {
		t.Error("expected macro removed from cache")
	}
}

func TestDeleteMissingIsNoError(t *testing.T) {
	s := newStore(t)
	// Deleting a macro that was never written must not error (ErrNotExist swallowed).
	if err := s.Delete("never-existed"); err != nil {
		t.Fatalf("Delete missing: %v", err)
	}
}

func TestDeleteOtherError(t *testing.T) {
	s := newStore(t)
	// Make the would-be file path a non-empty directory so os.Remove fails with
	// a non-NotExist error.
	dirPath := filepath.Join(s.dir, "stubborn.json")
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		t.Fatalf("setup mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirPath, "child"), []byte("x"), 0o600); err != nil {
		t.Fatalf("setup write child: %v", err)
	}
	if err := s.Delete("stubborn"); err == nil {
		t.Fatal("expected error removing a non-empty directory")
	}
}

func TestIsJSON(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"macro.json", true},
		{"a.json", true},
		{".json", false},   // len == 5, not > 5
		{"x.JSON", false},  // case-sensitive
		{"foo.txt", false}, // wrong suffix
		{"", false},
		{"json", false},
	}
	for _, c := range cases {
		if got := isJSON(c.name); got != c.want {
			t.Errorf("isJSON(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestDefaultDirWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-specific branch")
	}
	t.Setenv("LOCALAPPDATA", `C:\Custom\AppData\Local`)
	got := DefaultDir()
	want := filepath.Join(`C:\Custom\AppData\Local`, "office-addin-mcp", "macros")
	if got != want {
		t.Errorf("DefaultDir() = %q, want %q", got, want)
	}
}

func TestDefaultDirWindowsFallback(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-specific branch")
	}
	t.Setenv("LOCALAPPDATA", "")
	t.Setenv("USERPROFILE", `C:\Users\tester`)
	got := DefaultDir()
	want := filepath.Join(`C:\Users\tester`, "AppData", "Local", "office-addin-mcp", "macros")
	if got != want {
		t.Errorf("DefaultDir() = %q, want %q", got, want)
	}
}

func TestDefaultDirXDG(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-windows branch")
	}
	t.Setenv("XDG_CACHE_HOME", "/custom/cache")
	got := DefaultDir()
	want := filepath.Join("/custom/cache", "office-addin-mcp", "macros")
	if got != want {
		t.Errorf("DefaultDir() = %q, want %q", got, want)
	}
}

func TestDefaultDirHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-windows branch")
	}
	t.Setenv("XDG_CACHE_HOME", "")
	got := DefaultDir()
	// Just assert the suffix; the home dir is environment-dependent.
	suffix := filepath.Join("office-addin-mcp", "macros")
	if len(got) < len(suffix) || got[len(got)-len(suffix):] != suffix {
		t.Errorf("DefaultDir() = %q, want suffix %q", got, suffix)
	}
}

// keys returns the sorted-ish key list of a macro map for diagnostics.
func keys(m map[string]*Macro) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
