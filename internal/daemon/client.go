package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// ErrNoDaemon is returned when no socket file is present (so the caller
// should run in-process).
var ErrNoDaemon = errors.New("daemon: not running")

// Probe checks whether a healthy daemon is reachable and returns its
// SocketInfo. ErrNoDaemon means the socket file is missing or stale.
func Probe(ctx context.Context, path string) (SocketInfo, error) {
	if path == "" {
		var err error
		path, err = SocketFilePath()
		if err != nil {
			return SocketInfo{}, err
		}
	}
	info, err := ReadSocketFile(path)
	if err != nil {
		return SocketInfo{}, ErrNoDaemon
	}
	healthy, err := pingHealth(ctx, info.Port)
	if err != nil {
		return SocketInfo{}, fmt.Errorf("%w: %v", ErrNoDaemon, err)
	}
	if !healthy {
		return SocketInfo{}, ErrNoDaemon
	}
	return info, nil
}

func pingHealth(ctx context.Context, port int) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	url := fmt.Sprintf("http://127.0.0.1:%d/v1/health", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("status %d", resp.StatusCode)
	}
	return true, nil
}

// CallDaemon POSTs req to the running daemon and returns the parsed envelope.
func CallDaemon(ctx context.Context, info SocketInfo, req CallRequest) (tools.Envelope, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return tools.Envelope{}, fmt.Errorf("marshal request: %w", err)
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/v1/call", info.Port)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return tools.Envelope{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+info.Token)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return tools.Envelope{}, fmt.Errorf("daemon call: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return tools.Envelope{}, fmt.Errorf("daemon read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return tools.Envelope{}, fmt.Errorf("daemon: HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	var env tools.Envelope
	if err := json.Unmarshal(respBody, &env); err != nil {
		return tools.Envelope{}, fmt.Errorf("daemon: decode envelope: %w", err)
	}
	return env, nil
}
