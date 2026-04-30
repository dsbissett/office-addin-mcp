package tools

import (
	"context"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
)

// AttachedTarget bundles a freshly dialed connection attached to a resolved
// target. Callers MUST Close to release the connection. In Phase 5 this
// helper will be backed by a session pool; the API stays the same.
type AttachedTarget struct {
	Conn      *cdp.Connection
	Target    cdp.TargetInfo
	SessionID string
}

// Close releases the underlying connection.
func (a *AttachedTarget) Close() error {
	if a == nil || a.Conn == nil {
		return nil
	}
	return a.Conn.Close()
}

// AttachTarget opens a connection, resolves a target, and attaches via the
// flatten session model. Diagnostics are populated on success. On any error
// the connection is closed before returning.
func AttachTarget(ctx context.Context, env *RunEnv, sel TargetSelector) (*AttachedTarget, error) {
	conn, err := env.OpenConn(ctx)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	target, err := ResolveTarget(ctx, conn, sel)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if env.Diag != nil {
		env.Diag.TargetID = target.TargetID
	}
	sessionID, err := conn.AttachToTarget(ctx, target.TargetID)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if env.Diag != nil {
		env.Diag.SessionID = sessionID
	}
	return &AttachedTarget{Conn: conn, Target: target, SessionID: sessionID}, nil
}
