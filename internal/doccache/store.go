// Package doccache persists per-document discovery snapshots (sheet lists,
// table catalogs, named ranges, etc.) keyed by (host, filePath, fingerprint).
//
// The store is read on first access and written through on every Put — atomic
// rename keeps the on-disk JSON consistent under concurrent writes from
// separate processes (one-shot CLI calls and the daemon both reach for the
// same file). Tools consult the cache from their Run methods; cache misses
// fall back to running the discovery payload and Put-ing the result.
//
// The file lives at %LOCALAPPDATA%\office-addin-mcp\doccache.json on Windows
// and $XDG_CACHE_HOME/office-addin-mcp/doccache.json (or ~/.cache/...)
// elsewhere, mode 0600 — same convention as daemon.json.
package doccache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Entry is one cached discovery snapshot. Fingerprint is the host-payload-
// supplied hash that detects drift; Data is the verbatim JSON returned by the
// payload (everything except filePath/fingerprint, which the cache layer owns).
type Entry struct {
	Host        string          `json:"host"`
	FilePath    string          `json:"filePath"`
	Fingerprint string          `json:"fingerprint"`
	Data        json.RawMessage `json:"data"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

// Store is the file-backed cache. The zero value is unusable — call Open.
//
// Method receivers are nil-safe so callers can pass a *Store unconditionally
// even when --no-doccache is set (Open returns a disabled store, not nil).
type Store struct {
	path     string
	disabled bool

	mu      sync.Mutex
	loaded  bool
	entries map[string]Entry
}

// Open returns a Store rooted at the given path. If path is empty, the
// platform default is used. If disabled is true, every Get returns a miss and
// every Put is a no-op — the on-disk file is never read or written.
func Open(path string, disabled bool) *Store {
	if disabled {
		return &Store{disabled: true}
	}
	if path == "" {
		path = DefaultPath()
	}
	return &Store{path: path, entries: map[string]Entry{}}
}

// DefaultPath returns the platform default cache file path. Honors
// LOCALAPPDATA on Windows and XDG_CACHE_HOME elsewhere; falls back to
// ~/.cache/office-addin-mcp on POSIX.
func DefaultPath() string {
	if runtime.GOOS == "windows" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
		}
		return filepath.Join(base, "office-addin-mcp", "doccache.json")
	}
	if base := os.Getenv("XDG_CACHE_HOME"); base != "" {
		return filepath.Join(base, "office-addin-mcp", "doccache.json")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".cache", "office-addin-mcp", "doccache.json")
	}
	return filepath.Join(os.TempDir(), "office-addin-mcp", "doccache.json")
}

// Disabled reports whether the cache is in no-op mode.
func (s *Store) Disabled() bool { return s == nil || s.disabled }

// Path returns the on-disk path. Empty when disabled.
func (s *Store) Path() string {
	if s == nil || s.disabled {
		return ""
	}
	return s.path
}

// Get returns the cached entry for (host, filePath) when one exists. The
// caller compares Entry.Fingerprint against the live fingerprint to decide
// whether the cache is fresh. Cache misses (and disabled stores) return
// (Entry{}, false) without error.
func (s *Store) Get(host, filePath string) (Entry, bool) {
	if s == nil || s.disabled {
		return Entry{}, false
	}
	if !cacheable(filePath) {
		return Entry{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return Entry{}, false
	}
	e, ok := s.entries[key(host, filePath)]
	return e, ok
}

// Put upserts an entry and persists the cache atomically. Returns nil on
// success or any I/O error encountered during persist. Disabled stores are a
// no-op and return nil.
func (s *Store) Put(e Entry) error {
	if s == nil || s.disabled {
		return nil
	}
	if !cacheable(e.FilePath) {
		return nil
	}
	if e.UpdatedAt.IsZero() {
		e.UpdatedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return err
	}
	s.entries[key(e.Host, e.FilePath)] = e
	return s.saveLocked()
}

// Invalidate drops the cached entry for (host, filePath). No error if absent.
func (s *Store) Invalidate(host, filePath string) error {
	if s == nil || s.disabled {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return err
	}
	delete(s.entries, key(host, filePath))
	return s.saveLocked()
}

func (s *Store) loadLocked() error {
	if s.loaded {
		return nil
	}
	s.loaded = true
	if s.entries == nil {
		s.entries = map[string]Entry{}
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("doccache: read %s: %w", s.path, err)
	}
	var on diskFile
	if err := json.Unmarshal(raw, &on); err != nil {
		return fmt.Errorf("doccache: decode %s: %w", s.path, err)
	}
	for _, e := range on.Entries {
		s.entries[key(e.Host, e.FilePath)] = e
	}
	return nil
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("doccache: mkdir %s: %w", filepath.Dir(s.path), err)
	}
	on := diskFile{Version: 1}
	for _, e := range s.entries {
		on.Entries = append(on.Entries, e)
	}
	raw, err := json.MarshalIndent(on, "", "  ")
	if err != nil {
		return fmt.Errorf("doccache: encode: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), "doccache-*.tmp")
	if err != nil {
		return fmt.Errorf("doccache: tmp: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("doccache: write tmp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("doccache: close tmp: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("doccache: chmod tmp: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("doccache: rename tmp: %w", err)
	}
	return nil
}

// cacheable filters out empty / temp file paths the plan flagged as bad cache
// keys. Untitled workbooks ("Book1") fall through — agents care about them
// equally — but truly empty filePaths and obvious temp paths bypass the cache.
func cacheable(p string) bool {
	if p == "" {
		return false
	}
	low := strings.ToLower(p)
	return !strings.Contains(low, "\\temp\\") &&
		!strings.Contains(low, "/tmp/") &&
		!strings.HasPrefix(low, "/private/var/folders/")
}

func key(host, filePath string) string { return host + "\x00" + filePath }

type diskFile struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}
