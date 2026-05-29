package doccache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestOpenDisabledAndDefaultPath covers the disabled branch of Open and the
// empty-path branch that falls back to DefaultPath.
func TestOpenDisabledAndDefaultPath(t *testing.T) {
	d := Open("", true)
	if !d.Disabled() {
		t.Fatal("expected disabled store")
	}
	if d.path != "" {
		t.Errorf("disabled store should not resolve a path, got %q", d.path)
	}

	// Empty path on an enabled store falls back to DefaultPath.
	s := Open("", false)
	if s.Disabled() {
		t.Fatal("expected enabled store")
	}
	if s.path == "" {
		t.Fatal("expected Open to resolve a default path when given empty path")
	}
	if s.path != DefaultPath() {
		t.Errorf("Open empty path = %q, want DefaultPath %q", s.path, DefaultPath())
	}
}

// TestPath covers Path for nil, disabled, and enabled stores.
func TestPath(t *testing.T) {
	var nilStore *Store
	if nilStore.Path() != "" {
		t.Errorf("nil store Path = %q, want empty", nilStore.Path())
	}
	if got := Open("", true).Path(); got != "" {
		t.Errorf("disabled store Path = %q, want empty", got)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "doccache.json")
	if got := Open(p, false).Path(); got != p {
		t.Errorf("enabled store Path = %q, want %q", got, p)
	}
}

// TestDefaultPath exercises DefaultPath. The exact result is platform-specific,
// so assert it ends with the expected filename and is non-empty on every OS.
func TestDefaultPath(t *testing.T) {
	got := DefaultPath()
	if got == "" {
		t.Fatal("DefaultPath returned empty")
	}
	if filepath.Base(got) != "doccache.json" {
		t.Errorf("DefaultPath base = %q, want doccache.json", filepath.Base(got))
	}
	if filepath.Base(filepath.Dir(got)) != "office-addin-mcp" {
		t.Errorf("DefaultPath parent dir = %q, want office-addin-mcp", filepath.Base(filepath.Dir(got)))
	}
}

// TestDefaultPathWindowsLocalAppData drives the Windows LOCALAPPDATA branch and
// its USERPROFILE fallback. Only meaningful on Windows; skipped elsewhere
// because DefaultPath keys off runtime.GOOS.
func TestDefaultPathWindowsLocalAppData(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only path resolution")
	}
	t.Setenv("LOCALAPPDATA", `C:\fake\local`)
	want := filepath.Join(`C:\fake\local`, "office-addin-mcp", "doccache.json")
	if got := DefaultPath(); got != want {
		t.Errorf("DefaultPath with LOCALAPPDATA = %q, want %q", got, want)
	}

	// Empty LOCALAPPDATA falls back to USERPROFILE\AppData\Local.
	t.Setenv("LOCALAPPDATA", "")
	t.Setenv("USERPROFILE", `C:\fake\user`)
	want = filepath.Join(`C:\fake\user`, "AppData", "Local", "office-addin-mcp", "doccache.json")
	if got := DefaultPath(); got != want {
		t.Errorf("DefaultPath with empty LOCALAPPDATA = %q, want %q", got, want)
	}
}

// TestDefaultPathXDG drives the non-Windows XDG_CACHE_HOME branch. Skipped on
// Windows where that branch is unreachable.
func TestDefaultPathXDG(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-windows-only path resolution")
	}
	t.Setenv("XDG_CACHE_HOME", "/fake/xdg")
	want := filepath.Join("/fake/xdg", "office-addin-mcp", "doccache.json")
	if got := DefaultPath(); got != want {
		t.Errorf("DefaultPath with XDG_CACHE_HOME = %q, want %q", got, want)
	}
}

// TestDefaultPathHomeFallback drives the non-Windows ~/.cache fallback when
// XDG_CACHE_HOME is unset.
func TestDefaultPathHomeFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-windows-only path resolution")
	}
	t.Setenv("XDG_CACHE_HOME", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home dir available: %v", err)
	}
	want := filepath.Join(home, ".cache", "office-addin-mcp", "doccache.json")
	if got := DefaultPath(); got != want {
		t.Errorf("DefaultPath home fallback = %q, want %q", got, want)
	}
}

// TestListSortsByUpdatedAtDesc verifies List filters by host and returns
// most-recently-updated first.
func TestListSortsByUpdatedAtDesc(t *testing.T) {
	dir := t.TempDir()
	s := Open(filepath.Join(dir, "doccache.json"), false)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	puts := []Entry{
		{Host: "excel", FilePath: "a.xlsx", Fingerprint: "a", UpdatedAt: base.Add(1 * time.Hour)},
		{Host: "excel", FilePath: "b.xlsx", Fingerprint: "b", UpdatedAt: base.Add(3 * time.Hour)},
		{Host: "excel", FilePath: "c.xlsx", Fingerprint: "c", UpdatedAt: base.Add(2 * time.Hour)},
		{Host: "word", FilePath: "d.docx", Fingerprint: "d", UpdatedAt: base.Add(5 * time.Hour)},
	}
	for _, e := range puts {
		if err := s.Put(e); err != nil {
			t.Fatalf("put %s: %v", e.FilePath, err)
		}
	}

	got := s.List("excel")
	if len(got) != 3 {
		t.Fatalf("List(excel) returned %d entries, want 3", len(got))
	}
	// Expect newest-first ordering: b (3h), c (2h), a (1h).
	wantOrder := []string{"b.xlsx", "c.xlsx", "a.xlsx"}
	for i, w := range wantOrder {
		if got[i].FilePath != w {
			t.Errorf("List[%d] = %q, want %q", i, got[i].FilePath, w)
		}
	}
	// Host filtering: word entry must not leak into excel results.
	for _, e := range got {
		if e.Host != "excel" {
			t.Errorf("List(excel) leaked host %q", e.Host)
		}
	}

	if w := s.List("word"); len(w) != 1 || w[0].FilePath != "d.docx" {
		t.Errorf("List(word) = %+v, want single d.docx", w)
	}

	if u := s.List("powerpoint"); u != nil {
		t.Errorf("List(unknown host) = %+v, want nil", u)
	}
}

// TestListDisabledAndNil covers the nil/disabled early-return in List.
func TestListDisabledAndNil(t *testing.T) {
	var nilStore *Store
	if got := nilStore.List("excel"); got != nil {
		t.Errorf("nil store List = %+v, want nil", got)
	}
	if got := Open("", true).List("excel"); got != nil {
		t.Errorf("disabled store List = %+v, want nil", got)
	}
}

// TestListLoadError verifies List returns nil when the backing file is
// corrupt (loadLocked error path).
func TestListLoadError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doccache.json")
	if err := os.WriteFile(path, []byte("not json {"), 0o600); err != nil {
		t.Fatalf("seed corrupt file: %v", err)
	}
	s := Open(path, false)
	if got := s.List("excel"); got != nil {
		t.Errorf("List over corrupt file = %+v, want nil", got)
	}
}

// TestPutSetsUpdatedAt verifies Put stamps UpdatedAt when zero and preserves a
// caller-supplied value otherwise.
func TestPutSetsUpdatedAt(t *testing.T) {
	dir := t.TempDir()
	s := Open(filepath.Join(dir, "doccache.json"), false)

	before := time.Now().UTC()
	if err := s.Put(Entry{Host: "excel", FilePath: "z.xlsx", Fingerprint: "fp"}); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, ok := s.Get("excel", "z.xlsx")
	if !ok {
		t.Fatal("expected hit")
	}
	if got.UpdatedAt.IsZero() {
		t.Fatal("expected UpdatedAt to be stamped on zero-value Put")
	}
	if got.UpdatedAt.Before(before) {
		t.Errorf("stamped UpdatedAt %v is before test start %v", got.UpdatedAt, before)
	}

	explicit := time.Date(2020, 6, 1, 12, 0, 0, 0, time.UTC)
	if err := s.Put(Entry{Host: "excel", FilePath: "z2.xlsx", Fingerprint: "fp", UpdatedAt: explicit}); err != nil {
		t.Fatalf("put explicit: %v", err)
	}
	got2, _ := s.Get("excel", "z2.xlsx")
	if !got2.UpdatedAt.Equal(explicit) {
		t.Errorf("explicit UpdatedAt overwritten: got %v want %v", got2.UpdatedAt, explicit)
	}
}

// TestGetCorruptFileMisses verifies Get swallows a load error from a corrupt
// file and reports a miss rather than panicking.
func TestGetCorruptFileMisses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doccache.json")
	if err := os.WriteFile(path, []byte("{ this is : not valid json"), 0o600); err != nil {
		t.Fatalf("seed corrupt file: %v", err)
	}
	s := Open(path, false)
	if _, ok := s.Get("excel", "Book1.xlsx"); ok {
		t.Error("expected miss over corrupt file")
	}
}

// TestPutCorruptFileReturnsError verifies Put surfaces the decode error from a
// corrupt backing file (loadLocked error propagation through Put).
func TestPutCorruptFileReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doccache.json")
	if err := os.WriteFile(path, []byte("garbage"), 0o600); err != nil {
		t.Fatalf("seed corrupt file: %v", err)
	}
	s := Open(path, false)
	if err := s.Put(Entry{Host: "excel", FilePath: "Book1.xlsx", Fingerprint: "fp"}); err == nil {
		t.Error("expected Put to return decode error over corrupt file")
	}
}

// TestInvalidateCorruptFileReturnsError verifies Invalidate propagates the
// load error from a corrupt file.
func TestInvalidateCorruptFileReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doccache.json")
	if err := os.WriteFile(path, []byte("{nope"), 0o600); err != nil {
		t.Fatalf("seed corrupt file: %v", err)
	}
	s := Open(path, false)
	if err := s.Invalidate("excel", "Book1.xlsx"); err == nil {
		t.Error("expected Invalidate to return decode error over corrupt file")
	}
}

// TestInvalidateDisabledAndNil covers the nil/disabled early-return in
// Invalidate.
func TestInvalidateDisabledAndNil(t *testing.T) {
	var nilStore *Store
	if err := nilStore.Invalidate("excel", "x"); err != nil {
		t.Errorf("nil store Invalidate = %v, want nil", err)
	}
	if err := Open("", true).Invalidate("excel", "x"); err != nil {
		t.Errorf("disabled store Invalidate = %v, want nil", err)
	}
}

// TestInvalidateAbsentIsNoError verifies deleting an unknown key succeeds and
// persists cleanly.
func TestInvalidateAbsentIsNoError(t *testing.T) {
	dir := t.TempDir()
	s := Open(filepath.Join(dir, "doccache.json"), false)
	if err := s.Invalidate("excel", "never-stored.xlsx"); err != nil {
		t.Errorf("invalidate absent key = %v, want nil", err)
	}
}

// TestLoadReadErrorNonNotExist drives the loadLocked read-error branch that is
// not ErrNotExist: pointing the store path at a directory makes os.ReadFile
// fail with a non-NotExist error, which Get swallows as a miss.
func TestLoadReadErrorNonNotExist(t *testing.T) {
	dir := t.TempDir()
	// Use the temp dir itself as the "file" path. Reading a directory as a
	// file returns an error that is not os.ErrNotExist on both Windows and
	// POSIX.
	s := Open(dir, false)
	if _, ok := s.Get("excel", "Book1.xlsx"); ok {
		t.Error("expected miss when backing path is a directory")
	}
	// Put should surface the read error.
	if err := s.Put(Entry{Host: "excel", FilePath: "Book1.xlsx", Fingerprint: "fp"}); err == nil {
		t.Error("expected Put error when backing path is a directory")
	}
}

// TestSaveMkdirError drives the saveLocked MkdirAll failure path: when an
// ancestor of the cache path is a regular file, MkdirAll(dir) fails.
func TestSaveMkdirError(t *testing.T) {
	dir := t.TempDir()
	// Create a regular file, then nest the cache "directory" underneath it.
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed blocker file: %v", err)
	}
	// path = <dir>/blocker/sub/doccache.json — MkdirAll must traverse through
	// "blocker", which is a file, and fail.
	path := filepath.Join(blocker, "sub", "doccache.json")
	s := Open(path, false)
	err := s.Put(Entry{Host: "excel", FilePath: "Book1.xlsx", Fingerprint: "fp"})
	if err == nil {
		t.Fatal("expected Put to fail when an ancestor path component is a file")
	}
}

// TestSaveAtomicRenameRoundTrip verifies the atomic-rename persist path: after
// a Put, the file exists, decodes to the diskFile schema with version 1, and
// reopening recovers the entry. Also confirms the temp file is cleaned up.
func TestSaveAtomicRenameRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "doccache.json")
	s := Open(path, false)
	if err := s.Put(Entry{Host: "excel", FilePath: "Book1.xlsx", Fingerprint: "fp1", Data: json.RawMessage(`{"a":1}`)}); err != nil {
		t.Fatalf("put: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persisted file: %v", err)
	}
	var on diskFile
	if err := json.Unmarshal(raw, &on); err != nil {
		t.Fatalf("decode persisted file: %v", err)
	}
	if on.Version != 1 {
		t.Errorf("diskFile.Version = %d, want 1", on.Version)
	}
	if len(on.Entries) != 1 || on.Entries[0].FilePath != "Book1.xlsx" {
		t.Errorf("persisted entries = %+v, want single Book1.xlsx", on.Entries)
	}

	// No leftover temp files in the directory.
	matches, err := filepath.Glob(filepath.Join(dir, "nested", "doccache-*.tmp"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("found leftover temp files: %v", matches)
	}

	// Reopen recovers the entry through loadLocked.
	s2 := Open(path, false)
	got, ok := s2.Get("excel", "Book1.xlsx")
	if !ok {
		t.Fatal("expected hit after reopen")
	}
	if got.Fingerprint != "fp1" {
		t.Errorf("fingerprint after reopen = %q, want fp1", got.Fingerprint)
	}
}

// TestSaveFileMode asserts the persisted file ends up at mode 0600 (chmod step
// in saveLocked). File-mode bits are not enforced on Windows, so skip there.
func TestSaveFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix file modes not enforced on windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "doccache.json")
	s := Open(path, false)
	if err := s.Put(Entry{Host: "excel", FilePath: "Book1.xlsx", Fingerprint: "fp"}); err != nil {
		t.Fatalf("put: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file mode = %o, want 600", perm)
	}
}

// TestGetNilAndUncacheable covers Get's nil-receiver and non-cacheable
// early-returns explicitly (not just through other tests).
func TestGetNilAndUncacheable(t *testing.T) {
	var nilStore *Store
	if _, ok := nilStore.Get("excel", "Book1.xlsx"); ok {
		t.Error("nil store Get should miss")
	}
	dir := t.TempDir()
	s := Open(filepath.Join(dir, "doccache.json"), false)
	if _, ok := s.Get("excel", ""); ok {
		t.Error("empty filePath Get should miss (non-cacheable)")
	}
}

// TestPutNilReceiver covers Put's nil-receiver early-return.
func TestPutNilReceiver(t *testing.T) {
	var nilStore *Store
	if err := nilStore.Put(Entry{Host: "excel", FilePath: "Book1.xlsx", Fingerprint: "fp"}); err != nil {
		t.Errorf("nil store Put = %v, want nil", err)
	}
}

// TestKeyAndCacheable exercise the unexported helpers directly to nail the few
// remaining branches and document the expected contract.
func TestKeyAndCacheable(t *testing.T) {
	if key("excel", "Book1.xlsx") == key("word", "Book1.xlsx") {
		t.Error("key should differ by host")
	}
	if key("excel", "a") == key("excel", "b") {
		t.Error("key should differ by filePath")
	}

	cases := map[string]bool{
		"":                                      false,
		"Book1.xlsx":                            true,
		`C:\Users\me\Documents\Book.xlsx`:       true,
		`C:\Users\me\AppData\Local\Temp\x.xlsx`: false,
		"/tmp/x.xlsx":                           false,
		"/private/var/folders/ab/cd/x.xlsx":     false,
		"/Users/me/Documents/x.xlsx":            true,
	}
	for p, want := range cases {
		if got := cacheable(p); got != want {
			t.Errorf("cacheable(%q) = %v, want %v", p, got, want)
		}
	}
}
