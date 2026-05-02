// Package session manages stateful CDP sessions across calls. One Session
// owns one CDP connection plus per-session state (sticky target/sessionId
// selection, snapshot, enabled-domain bookkeeping, event buffers, reconnect
// budget). Tool calls against the same session can run concurrently — the
// connection lock is read-shared during steady-state command dispatch, and
// per-resource state has its own narrower lock. The package is consumed by
// tools.Dispatcher in both daemon (persistent) and one-shot (ephemeral) modes.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/webview2"
)

// Sentinel errors returned by Acquire so the dispatcher can branch with
// errors.Is rather than substring-matching the wrapped Error() string.
var (
	// ErrReconnectBudgetExhausted means the session has hit the
	// ReconnectMax-per-ReconnectWindow ceiling. The user has to wait for the
	// window to slide before a fresh dial is permitted.
	ErrReconnectBudgetExhausted = errors.New("session: reconnect budget exhausted")
	// ErrDialFailed means the underlying webview2.Discover or cdp.Dial call
	// returned an error — typically the CDP endpoint is unreachable.
	ErrDialFailed = errors.New("session: dial failed")
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

// Session is one logical client session. Acquire takes a *read* lock on the
// connection, so multiple parallel tool calls can share one CDP connection
// without serializing — only dial / reconnect / close take the write lock.
// Per-resource state (selection, snapshot, enabled, eventBufs) has its own
// narrower lock so state mutations don't block steady-state CDP dispatch
// either.
type Session struct {
	id  string
	cfg Config

	// connMu protects conn + epConfig + reconnects. Read-locked during
	// steady-state command dispatch (the fast Acquire path); write-locked
	// only when dialing, reconnecting, or closing.
	connMu     sync.RWMutex
	conn       *cdp.Connection
	epConfig   webview2.Config
	reconnects []time.Time

	// lastUsedNano is the wall-clock ns of the most recent Acquire. Stored
	// atomically so the manager's gc loop doesn't have to take any session
	// lock when scanning candidates.
	lastUsedNano atomic.Int64

	// stateMu protects the per-call sticky state (selection, default, snapshot,
	// enabled). Held briefly by self-locking accessors; never held during a
	// CDP Send.
	stateMu      sync.Mutex
	hasSelection bool
	selection    Selection
	hasDefault   bool
	defaultSel   Selection
	snapshot     *Snapshot
	enabled      map[string]map[string]struct{}

	// eventMu protects the eventBufs map (the buffer values themselves carry
	// their own internal mutex, so this is just for the map shape).
	eventMu   sync.Mutex
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

// LastUsed returns the session's last-acquired timestamp. Lock-free read.
func (s *Session) LastUsed() time.Time {
	n := s.lastUsedNano.Load()
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n)
}

// Acquire ensures a live CDP connection for the requested endpoint and returns
// it under a connMu *read* lock so multiple goroutines can dispatch CDP
// commands concurrently against the same session. Callers MUST invoke the
// returned release function when finished — it drops the read lock. Release
// does NOT close the connection.
//
// On a missing/dead/endpoint-mismatched connection, Acquire briefly takes the
// write lock to dial. The reconnect budget gates this slow path.
func (s *Session) Acquire(ctx context.Context, ep webview2.Config) (*cdp.Connection, func(), error) {
	s.lastUsedNano.Store(time.Now().UnixNano())

	// Fast path: read-locked check. If the conn is healthy and the endpoint
	// matches, return immediately.
	s.connMu.RLock()
	if s.conn != nil && endpointEqual(s.epConfig, ep) && !connDone(s.conn) {
		conn := s.conn
		return conn, makeReleaseRLock(&s.connMu), nil
	}
	s.connMu.RUnlock()

	// Slow path: take the write lock, recheck (another goroutine may have
	// dialed in the gap), then dial if still needed.
	s.connMu.Lock()
	if s.conn != nil && endpointEqual(s.epConfig, ep) && !connDone(s.conn) {
		conn := s.conn
		s.connMu.Unlock()
		s.connMu.RLock()
		return conn, makeReleaseRLock(&s.connMu), nil
	}

	// Endpoint changed mid-session: drop the old conn so we re-dial.
	if !endpointEqual(s.epConfig, ep) && (s.epConfig.WSEndpoint != "" || s.epConfig.BrowserURL != "") {
		s.dropConnLocked()
	} else if s.conn != nil && connDone(s.conn) {
		s.dropConnLocked()
	}
	s.epConfig = ep

	if !s.canReconnectLocked() {
		s.connMu.Unlock()
		return nil, nil, fmt.Errorf("%w: session %q (%d in %s)",
			ErrReconnectBudgetExhausted, s.id, s.cfg.ReconnectMax, s.cfg.ReconnectWindow)
	}
	conn, err := dialEndpoint(ctx, ep)
	if err != nil {
		s.recordReconnectLocked() // count failed attempts too — they consume budget
		s.connMu.Unlock()
		// Wrap both ErrDialFailed (so the dispatcher can branch on it) and
		// the underlying err (so errors.Is(ctx.DeadlineExceeded) still fires).
		return nil, nil, fmt.Errorf("%w: %w", ErrDialFailed, err)
	}
	s.conn = conn
	s.recordReconnectLocked()

	// dropConnLocked above already cleared per-connection state; nothing more
	// to reset here. Downgrade to a read lock for the caller.
	s.connMu.Unlock()
	s.connMu.RLock()
	return conn, makeReleaseRLock(&s.connMu), nil
}

// makeReleaseRLock returns a once-only RUnlock closure. Defensive against a
// caller invoking release twice — that would panic on the underlying mutex.
func makeReleaseRLock(mu *sync.RWMutex) func() {
	released := false
	return func() {
		if released {
			return
		}
		released = true
		mu.RUnlock()
	}
}

// connDone reports whether the read pump has exited (connection died).
func connDone(c *cdp.Connection) bool {
	select {
	case <-c.Done():
		return true
	default:
		return false
	}
}

// Selected returns the cached selection for the given selector, if any.
func (s *Session) Selected(targetID, urlPattern string) (Selection, bool) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if !s.hasSelection {
		return Selection{}, false
	}
	if !s.selection.matches(targetID, urlPattern) {
		return Selection{}, false
	}
	return s.selection, true
}

// SetSelected caches the active selection for the given selector key.
func (s *Session) SetSelected(targetID, urlPattern string, target cdp.TargetInfo, sessionID string) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.hasSelection = true
	s.selection = Selection{
		SelectorTargetID:   targetID,
		SelectorURLPattern: urlPattern,
		Target:             target,
		SessionID:          sessionID,
	}
}

// InvalidateSelection clears the cached selection.
func (s *Session) InvalidateSelection() {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.hasSelection = false
	s.selection = Selection{}
}

// SetDefaultSelection records the sticky page chosen by pages.select. Empty
// selectors fall back to this.
func (s *Session) SetDefaultSelection(target cdp.TargetInfo, cdpSessionID string) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.hasDefault = true
	s.defaultSel = Selection{Target: target, SessionID: cdpSessionID}
}

// DefaultSelection returns the current sticky default, if any.
func (s *Session) DefaultSelection() (Selection, bool) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if !s.hasDefault {
		return Selection{}, false
	}
	return s.defaultSel, true
}

// ClearDefaultSelection drops the sticky default.
func (s *Session) ClearDefaultSelection() {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.hasDefault = false
	s.defaultSel = Selection{}
}

// SetSnapshot stores the latest page.snapshot UID table.
func (s *Session) SetSnapshot(snap *Snapshot) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.snapshot = snap
}

// Snapshot returns the cached snapshot, or nil.
func (s *Session) Snapshot() *Snapshot {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.snapshot
}

// Close terminates the session. Idempotent.
func (s *Session) Close() {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	s.dropConnLocked()
}

// dropConnLocked closes the connection and resets per-connection state. Must
// be called with connMu write-locked. Briefly takes stateMu and eventMu to
// reset their respective maps — order is connMu → stateMu → eventMu, never
// reversed elsewhere, so this can't deadlock.
func (s *Session) dropConnLocked() {
	if s.conn != nil {
		_ = s.conn.Close()
		s.conn = nil
	}
	s.stateMu.Lock()
	s.hasSelection = false
	s.selection = Selection{}
	s.hasDefault = false
	s.defaultSel = Selection{}
	s.snapshot = nil
	s.enabled = nil
	s.stateMu.Unlock()
	s.eventMu.Lock()
	s.eventBufs = nil
	s.eventMu.Unlock()
}

// EnsureEnabled issues "<domain>.enable" once per (cdpSessionID, domain) pair
// for this Session. Subsequent calls are no-ops until the connection is
// dropped (e.g. by reconnect). Concurrent first-callers may both issue Send
// — Chrome treats `<Domain>.enable` as idempotent, so this is harmless and
// avoids serializing the entire dispatch path on a one-shot enable.
func (s *Session) EnsureEnabled(ctx context.Context, sender Sender, cdpSessionID, domain string) error {
	if s.isEnabled(cdpSessionID, domain) {
		return nil
	}
	if _, err := sender.Send(ctx, cdpSessionID, domain+".enable", nil); err != nil {
		return err
	}
	s.markEnabled(cdpSessionID, domain)
	return nil
}

func (s *Session) isEnabled(cdpSessionID, domain string) bool {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.enabled == nil {
		return false
	}
	_, ok := s.enabled[cdpSessionID][domain]
	return ok
}

func (s *Session) markEnabled(cdpSessionID, domain string) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.enabled == nil {
		s.enabled = map[string]map[string]struct{}{}
	}
	if s.enabled[cdpSessionID] == nil {
		s.enabled[cdpSessionID] = map[string]struct{}{}
	}
	s.enabled[cdpSessionID][domain] = struct{}{}
}

// IsEnabled reports whether (cdpSessionID, domain) has been enabled. Only
// useful for tests and diagnostics — production paths go through EnsureEnabled.
func (s *Session) IsEnabled(cdpSessionID, domain string) bool {
	return s.isEnabled(cdpSessionID, domain)
}

// canReconnectLocked / recordReconnectLocked: must be called with connMu
// write-locked since they mutate s.reconnects.
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
