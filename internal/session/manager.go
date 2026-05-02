package session

import (
	"sync"
	"time"

	internallog "github.com/dsbissett/office-addin-mcp/internal/log"
)

// Manager owns a pool of named Sessions, with optional idle GC.
type Manager struct {
	cfg Config

	mu       sync.Mutex
	sessions map[string]*Session

	closed     chan struct{}
	closedOnce sync.Once
}

// NewManager creates a manager with the given config and starts the GC loop
// when IdleTimeout > 0.
func NewManager(cfg Config) *Manager {
	cfg = cfg.withDefaults()
	m := &Manager{
		cfg:      cfg,
		sessions: map[string]*Session{},
		closed:   make(chan struct{}),
	}
	if cfg.IdleTimeout > 0 {
		go m.gcLoop()
	}
	return m
}

// Get returns the named session, creating it if missing. The empty string
// resolves to "default".
func (m *Manager) Get(id string) *Session {
	if id == "" {
		id = "default"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		s = &Session{id: id, cfg: m.cfg}
		m.sessions[id] = s
	}
	return s
}

// Drop removes and closes a session by id. No-op if absent.
func (m *Manager) Drop(id string) {
	if id == "" {
		id = "default"
	}
	m.mu.Lock()
	s, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	if ok {
		s.Close()
	}
}

// Snapshot returns a list of session ids currently in the pool. Order is
// undefined.
func (m *Manager) Snapshot() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	return ids
}

// Close stops the GC loop and closes every session.
func (m *Manager) Close() {
	m.closedOnce.Do(func() {
		close(m.closed)
		m.mu.Lock()
		victims := m.sessions
		m.sessions = map[string]*Session{}
		m.mu.Unlock()
		for _, s := range victims {
			s.Close()
		}
	})
}

func (m *Manager) gcLoop() {
	defer internallog.RecoverGoroutine("session.gcLoop")
	interval := m.cfg.IdleTimeout / 4
	if interval < time.Second {
		interval = time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-m.closed:
			return
		case <-t.C:
			m.gcOnce()
		}
	}
}

func (m *Manager) gcOnce() {
	cutoff := time.Now().Add(-m.cfg.IdleTimeout)
	var stale []*Session
	m.mu.Lock()
	for id, s := range m.sessions {
		if s.LastUsed().Before(cutoff) && !s.LastUsed().IsZero() {
			stale = append(stale, s)
			delete(m.sessions, id)
		}
	}
	m.mu.Unlock()
	for _, s := range stale {
		s.Close()
	}
}
