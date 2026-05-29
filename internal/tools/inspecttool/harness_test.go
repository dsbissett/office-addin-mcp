package inspecttool

import (
	"context"
	"errors"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/cdptest"
	"github.com/dsbissett/office-addin-mcp/internal/session"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// newSession returns a fresh, isolated session.Session whose EventBuf /
// MarkEventPumping / Snapshot / SetSnapshot methods back the corresponding
// RunEnv hooks. Each test gets its own so buffers and pump flags never bleed
// between cases.
func newSession() *session.Session {
	return session.NewManager(session.Config{}).Get("default")
}

// fakeEnvOpts configures fakeEnv.
type fakeEnvOpts struct {
	// resp drives the in-process CDP server's command replies.
	resp cdptest.Responder
	// target is the resolved target handed back by Attach.
	target cdp.TargetInfo
	// sessionID is the CDP flatten session id handed back by Attach.
	sessionID string
	// enableErr, when non-nil, makes every EnsureEnabled call fail with it.
	enableErr error
	// sess is the backing session for EventBuf/Snapshot hooks. When nil a
	// fresh one is created.
	sess *session.Session
}

// fakeEnv builds a RunEnv whose Attach hands the tool a real *cdp.Connection
// backed by an in-process CDP server, and whose event-buffer / snapshot hooks
// delegate to a real session.Session. This is the reusable seam for happy and
// error coverage of every inspecttool run* function.
func fakeEnv(t *testing.T, opts fakeEnvOpts) (*tools.RunEnv, *session.Session) {
	t.Helper()
	srv := cdptest.NewServer(t, opts.resp)
	sess := opts.sess
	if sess == nil {
		sess = newSession()
	}
	sid := opts.sessionID
	if sid == "" {
		sid = "cdp-1"
	}
	conn := srv.Dial(t)
	env := &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Attach: func(context.Context, tools.TargetSelector) (*tools.AttachedTarget, error) {
			return &tools.AttachedTarget{Conn: conn, Target: opts.target, SessionID: sid}, nil
		},
		EnsureEnabled: func(context.Context, string, string) error {
			return opts.enableErr
		},
		EventBuf: func(kind session.EventBufKind, cdpSessionID string, max int) *session.EventBuf {
			return sess.EventBuf(kind, cdpSessionID, max)
		},
		MarkEventPumping: func(kind session.EventBufKind, cdpSessionID string, max int) bool {
			return sess.MarkEventPumping(kind, cdpSessionID, max)
		},
		Snapshot:    sess.Snapshot,
		SetSnapshot: sess.SetSnapshot,
	}
	return env, sess
}

// cdptestServer starts an in-process CDP server with the given responder and
// returns a dialed *cdp.Connection. Used when a test needs to build its own
// RunEnv by hand rather than via fakeEnv.
func cdptestServer(t *testing.T, resp cdptest.Responder) *cdp.Connection {
	t.Helper()
	return cdptest.NewServer(t, resp).Dial(t)
}

// cdpRemote returns a *cdp.RemoteError carrying msg, usable wherever an error
// is needed (it implements error).
func cdpRemote(msg string) *cdp.RemoteError {
	return &cdp.RemoteError{Code: -32000, Message: msg}
}

// errEnv returns a RunEnv whose Attach always fails — exercises the attach
// failure branch without a server.
func errEnv() *tools.RunEnv {
	return &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Attach: func(context.Context, tools.TargetSelector) (*tools.AttachedTarget, error) {
			return nil, errors.New("no target")
		},
	}
}
