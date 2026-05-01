// Package session manages stateful CDP sessions across calls. One Session
// owns one CDP connection, a sticky target/sessionId selection, a reconnect
// budget, and per-session serialization (Office.js context.sync flows do not
// parallelize). The package is consumed by tools.Dispatcher in both daemon
// (persistent) and one-shot (ephemeral) modes.
package session

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/webview2"
)

// Sender is the subset of *cdp.Connection that EnsureEnabled needs. Defined
// as an interface so tests can inject a recording stub without a live
// WebSocket. *cdp.Connection satisfies this naturally.
type Sender interface {
	Send(ctx context.Context, sessionID, method string, params any) (json.RawMessage, error)
}

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

	// hasDefault and defaultSel track the page selection installed by
	// pages.select. Empty-selector Attach calls return this in preference to
	// FirstPageTarget so the chosen page sticks across subsequent UID-based
	// tools (page.click, page.fill, …) without repeatedly threading targetId.
	hasDefault bool
	defaultSel Selection

	// snapshot stores the most recent page.snapshot output. UID-based
	// interaction tools resolve uids against this map. Cleared on reconnect
	// (dropConnLocked) since backendNodeIds are scoped to the live target.
	snapshot *Snapshot

	// enabled tracks per-CDP-session domains that have been issued
	// "<Domain>.enable". Cleared whenever the underlying conn is dropped
	// (dropConnLocked) — Chrome resets domain state on reconnect, so the
	// tracking has to follow.
	enabled map[string]map[string]struct{}

	// eventBufs holds the ring buffers fed by page.consoleLog /
	// page.networkLog pump goroutines, keyed by (kind, cdpSessionID).
	// Cleared on dropConnLocked since both the CDP sessions and the
	// goroutines reading them are invalidated when the socket reconnects.
	eventBufs map[bufKey]*EventBuf
}

// SnapshotNode is one entry in a page snapshot's UID → backendNodeId table.
type SnapshotNode struct {
	UID           string
	BackendNodeID int
	Role          string
	Name          string
}

// Snapshot is a frozen accessibility-tree projection: a stable UID per node
// pointing back at its CDP backendNodeId, scoped to a particular target +
// CDP flatten session. Interaction tools use it to resolve UIDs without
// re-walking the AX tree on every call.
type Snapshot struct {
	TargetID     string
	CDPSessionID string
	Nodes        map[string]SnapshotNode
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
		s.hasDefault = false
		s.defaultSel = Selection{}
		s.snapshot = nil
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

// SetDefaultSelection records the sticky page chosen by pages.select. Empty
// selectors fall back to this. Must be called with the session lock held.
func (s *Session) SetDefaultSelection(target cdp.TargetInfo, cdpSessionID string) {
	s.hasDefault = true
	s.defaultSel = Selection{Target: target, SessionID: cdpSessionID}
}

// DefaultSelection returns the current sticky default, if any. Must be called
// with the session lock held.
func (s *Session) DefaultSelection() (Selection, bool) {
	if !s.hasDefault {
		return Selection{}, false
	}
	return s.defaultSel, true
}

// ClearDefaultSelection drops the sticky default. Must be called with the
// session lock held.
func (s *Session) ClearDefaultSelection() {
	s.hasDefault = false
	s.defaultSel = Selection{}
}

// SetSnapshot stores the latest page.snapshot UID table. Must be called with
// the session lock held.
func (s *Session) SetSnapshot(snap *Snapshot) {
	s.snapshot = snap
}

// Snapshot returns the cached snapshot, or nil. Must be called with the
// session lock held.
func (s *Session) Snapshot() *Snapshot {
	return s.snapshot
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
	s.hasDefault = false
	s.defaultSel = Selection{}
	s.snapshot = nil
	s.enabled = nil
	s.dropEventBufsLocked()
}

// EnsureEnabled issues "<domain>.enable" exactly once per (cdpSessionID,
// domain) pair for this Session. Subsequent calls are no-ops until the
// connection is dropped (e.g. by reconnect). Must be called with the
// session lock held — i.e. between Acquire and its release.
func (s *Session) EnsureEnabled(ctx context.Context, sender Sender, cdpSessionID, domain string) error {
	if s.enabled == nil {
		s.enabled = map[string]map[string]struct{}{}
	}
	domains := s.enabled[cdpSessionID]
	if domains == nil {
		domains = map[string]struct{}{}
		s.enabled[cdpSessionID] = domains
	}
	if _, ok := domains[domain]; ok {
		return nil
	}
	if _, err := sender.Send(ctx, cdpSessionID, domain+".enable", nil); err != nil {
		return err
	}
	domains[domain] = struct{}{}
	return nil
}

// IsEnabled reports whether (cdpSessionID, domain) has been enabled. Only
// useful for tests and diagnostics — production paths go through EnsureEnabled.
// Must be called with the session lock held.
func (s *Session) IsEnabled(cdpSessionID, domain string) bool {
	if s.enabled == nil {
		return false
	}
	_, ok := s.enabled[cdpSessionID][domain]
	return ok
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
