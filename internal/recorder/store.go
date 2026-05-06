// Package recorder handles recording and storage of macro sequences for playback.
package recorder

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
)

// Entry represents a single recorded tool call.
type Entry struct {
	Tool   string `json:"tool"`
	Params any    `json:"params"`
}

// Macro is the on-disk format for a recorded macro.
type Macro struct {
	Name    string  `json:"name"`
	Entries []Entry `json:"entries"`
}

// Store manages macro persistence. Methods are thread-safe.
type Store struct {
	mu     sync.Mutex
	dir    string            // Macro directory
	active string            // Currently active macro name (when recording)
	buf    []Entry           // Recording buffer
	cache  map[string]*Macro // Loaded macros
}

// DefaultDir returns the platform default macros directory path. Honors
// LOCALAPPDATA on Windows and XDG_CACHE_HOME elsewhere; falls back to
// ~/.cache/office-addin-mcp elsewhere.
func DefaultDir() string {
	if runtime.GOOS == "windows" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
		}
		return filepath.Join(base, "office-addin-mcp", "macros")
	}
	if base := os.Getenv("XDG_CACHE_HOME"); base != "" {
		return filepath.Join(base, "office-addin-mcp", "macros")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".cache", "office-addin-mcp", "macros")
	}
	return filepath.Join(os.TempDir(), "office-addin-mcp", "macros")
}

// New creates or opens a Store at the given directory.
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("recorder.New: mkdir: %w", err)
	}
	return &Store{
		dir:   dir,
		cache: make(map[string]*Macro),
	}, nil
}

// StartRecording begins a new recording session. Returns error if already recording.
func (s *Store) StartRecording(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != "" {
		return fmt.Errorf("recorder: already recording macro %q", s.active)
	}
	s.active = name
	s.buf = nil
	return nil
}

// Append adds an entry to the active recording buffer.
func (s *Store) Append(tool string, params json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == "" {
		return fmt.Errorf("recorder: not currently recording")
	}
	var p any
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return fmt.Errorf("recorder: unmarshal params: %w", err)
		}
	}
	s.buf = append(s.buf, Entry{Tool: tool, Params: p})
	return nil
}

// StopRecording flushes the active recording to disk and returns the macro.
// Returns error if not currently recording or if write fails.
func (s *Store) StopRecording() (*Macro, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == "" {
		return nil, fmt.Errorf("recorder: not currently recording")
	}
	name := s.active
	m := &Macro{Name: name, Entries: s.buf}

	// Write to disk atomically via a temp file.
	path := filepath.Join(s.dir, name+".json")
	tmpPath := path + ".tmp"
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("recorder.StopRecording: marshal: %w", err)
	}
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return nil, fmt.Errorf("recorder.StopRecording: write: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("recorder.StopRecording: rename: %w", err)
	}

	// Update cache.
	s.cache[name] = m
	s.active = ""
	s.buf = nil

	return m, nil
}

// LoadAll reads all macros from disk into the cache.
func (s *Store) LoadAll() (map[string]*Macro, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s.cache, nil
		}
		return nil, fmt.Errorf("recorder.LoadAll: readdir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !json.Valid([]byte(entry.Name()+"=true")) {
			continue
		}
		name := entry.Name()
		if !isJSON(name) {
			continue
		}
		macroName := name[:len(name)-5] // Strip .json

		if _, ok := s.cache[macroName]; ok {
			continue
		}

		path := filepath.Join(s.dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("recorder.LoadAll: read %q: %w", name, err)
		}
		var m Macro
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("recorder.LoadAll: unmarshal %q: %w", name, err)
		}
		m.Name = macroName
		s.cache[macroName] = &m
	}

	return s.cache, nil
}

// Get returns a macro by name, or (nil, false).
func (s *Store) Get(name string) (*Macro, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.cache[name]
	return m, ok
}

// List returns all macro names sorted.
func (s *Store) List() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	names := make([]string, 0, len(s.cache))
	for name := range s.cache {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Delete removes a macro from disk and cache.
func (s *Store) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.dir, name+".json")
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("recorder.Delete: %w", err)
	}
	delete(s.cache, name)
	return nil
}

// isJSON returns true if name ends with .json.
func isJSON(name string) bool {
	return len(name) > 5 && name[len(name)-5:] == ".json"
}
