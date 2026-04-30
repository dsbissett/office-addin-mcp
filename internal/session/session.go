// Package session manages stateful CDP sessions across calls. One Session
// owns one CDP connection, a sticky target/sessionId selection, a reconnect
// budget, and per-session serialization (Office.js context.sync flows do not
// parallelize). The package is consumed by tools.Dispatcher in both daemon
// (persistent) and one-shot (ephemeral) modes.
package session

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/webview2"
)

// Config tunes session lifetime and reconnect behavior.
type Config struct {
	// IdleTimeout: GC sessions that haven't been used in this long. 0 disables GC.
	IdleTimeout time.Duration
	// ReconnectMax: max reconnects allowed in any ReconnectWindow. Default 3.
	ReconnectMax int
	// ReconnectWindow: sliding window for the reconnect budget. Default 60s.
	ReconnectWindow time.Duration
}

func (c Config) withDefaults() Config {
	if c.ReconnectMax == 0 {
		c.ReconnectMax = 3
	}
	if c.ReconnectWindow == 0 {
		c.ReconnectWindow = 60 * time.Second
	}
	return c
}

// Selection records a sticky target choice tied to a (TargetID, URLPattern)
// selector key. The dispatcher's RunEnv.Attach uses this to skip
// Target.getTargets and Target.attachToTarget on cache hits.
type Selection struct {
	SelectorTargetID   string
	SelectorURLPattern string
	Target             cdp.TargetInfo
	SessionID          string
}

func (sel Selection) matches(targetID, urlPattern string) bool {
	return sel.SelectorTargetID == targetID && sel.SelectorURLPattern == urlPattern
}

// Session is one logical client session. It is safe for concurrent acquisition
// — Acquire blocks on the internal mutex; tool calls thus serialize per session.
type Session struct {
	id  string
	cfg Config

	mu       sync.Mutex // serializes tool calls and guards every field below
	conn     *cdp.Connection
	epConfig webview2.Config
	lastUsed time.Time

	reconnects []time.Time

	hasSelection bool
	selection    Selection
}

// ID returns the session identifier.
func (s *Session) ID() string { return s.id }

// LastUsed returns the session's last-acquired timestamp.
func (s *Session) LastUsed() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastUsed
}

// Acquire locks the session and ensures a live CDP connection. It returns the
// connection plus a release function that unlocks the session — callers MUST
// invoke release. The connection is NOT closed by release; that lives on
// Close (or via the manager's GC).
//
// If the previous connection was lost or the requested endpoint differs from
// the cached one, a fresh dial is attempted, subject to the reconnect budget.
func (s *Session) Acquire(ctx context.Context, ep webview2.Config) (*cdp.Connection, func(), error) {
	s.mu.Lock()
	s.lastUsed = time.Now()

	// If endpoint changed mid-session, drop the old conn so we re-dial.
	if !endpointEqual(s.epConfig, ep) && (s.epConfig.WSEndpoint != "" || s.epConfig.BrowserURL != "") {
		s.dropConnLocked()
	}
	s.epConfig = ep

	// Liveness check: if the read pump exited, the conn is dead.
	if s.conn != nil {
		select {
		case <-s.conn.Done():
			s.dropConnLocked()
		default:
		}
	}

	if s.conn == nil {
		if !s.canReconnectLocked() {
			s.mu.Unlock()
			return nil, nil, fmt.Errorf("session %q: reconnect budget exhausted (%d in %s)",
				s.id, s.cfg.ReconnectMax, s.cfg.ReconnectWindow)
		}
		conn, err := dialEndpoint(ctx, ep)
		if err != nil {
			s.recordReconnectLocked() // count failed attempts too — they consume budget
			s.mu.Unlock()
			return nil, nil, fmt.Errorf("dial: %w", err)
		}
		s.conn = conn
		s.recordReconnectLocked()
		s.hasSelection = false
		s.selection = Selection{}
	}

	conn := s.conn
	released := false
	release := func() {
		if released {
			return
		}
		released = true
		s.mu.Unlock()
	}
	return conn, release, nil
}

// Selected returns the cached selection for the given selector, if any.
// Must be called with the session lock held (i.e. between Acquire and release).
func (s *Session) Selected(targetID, urlPattern string) (Selection, bool) {
	if !s.hasSelection {
		return Selection{}, false
	}
	if !s.selection.matches(targetID, urlPattern) {
		return Selection{}, false
	}
	return s.selection, true
}

// SetSelected caches the active selection for the given selector key.
// Must be called with the session lock held.
func (s *Session) SetSelected(targetID, urlPattern string, target cdp.TargetInfo, sessionID string) {
	s.hasSelection = true
	s.selection = Selection{
		SelectorTargetID:   targetID,
		SelectorURLPattern: urlPattern,
		Target:             target,
		SessionID:          sessionID,
	}
}

// InvalidateSelection clears the cached selection. Must be called with the
// session lock held.
func (s *Session) InvalidateSelection() {
	s.hasSelection = false
	s.selection = Selection{}
}

// Close terminates the session. Idempotent.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dropConnLocked()
	s.hasSelection = false
}

func (s *Session) dropConnLocked() {
	if s.conn != nil {
		_ = s.conn.Close()
		s.conn = nil
	}
	s.hasSelection = false
	s.selection = Selection{}
}

func (s *Session) canReconnectLocked() bool {
	cutoff := time.Now().Add(-s.cfg.ReconnectWindow)
	pruned := s.reconnects[:0]
	for _, t := range s.reconnects {
		if t.After(cutoff) {
			pruned = append(pruned, t)
		}
	}
	s.reconnects = pruned
	return len(s.reconnects) < s.cfg.ReconnectMax
}

func (s *Session) recordReconnectLocked() {
	s.reconnects = append(s.reconnects, time.Now())
}

func endpointEqual(a, b webview2.Config) bool {
	return a.WSEndpoint == b.WSEndpoint && a.BrowserURL == b.BrowserURL
}

func dialEndpoint(ctx context.Context, ep webview2.Config) (*cdp.Connection, error) {
	endpoint, err := webview2.Discover(ctx, ep)
	if err != nil {
		return nil, err
	}
	return cdp.Dial(ctx, endpoint.WSURL)
}
