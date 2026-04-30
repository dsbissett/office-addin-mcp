// Package daemon implements the office-addin-mcp background server. The HTTP
// API is the contract between the long-lived daemon process and short-lived
// `call` invocations: the call subcommand probes a well-known socket file,
// and if the recorded daemon is healthy it routes the request there instead
// of running in-process.
package daemon

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// SocketInfo is the contents of the well-known daemon socket file. Encoded
// as JSON at SocketFilePath. Mode 0600.
type SocketInfo struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

// SocketFilePath returns the platform-appropriate well-known location for
// the daemon socket file. On Windows this is under %LOCALAPPDATA%; on other
// platforms it follows XDG_CACHE_HOME via os.UserCacheDir.
func SocketFilePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("daemon: locate user cache dir: %w", err)
	}
	return filepath.Join(dir, "office-addin-mcp", "daemon.json"), nil
}

// WriteSocketFile atomically writes the socket file with mode 0600. The
// parent directory is created with mode 0700 when missing.
func WriteSocketFile(path string, info SocketInfo) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("daemon: mkdir %s: %w", filepath.Dir(path), err)
	}
	body, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return fmt.Errorf("daemon: write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("daemon: rename: %w", err)
	}
	return nil
}

// ReadSocketFile loads the daemon's recorded port + token. Returns
// fs.ErrNotExist when the file is absent — call subcommand treats that as
// "no daemon running, route in-process".
func ReadSocketFile(path string) (SocketInfo, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return SocketInfo{}, err
	}
	var info SocketInfo
	if err := json.Unmarshal(b, &info); err != nil {
		return SocketInfo{}, fmt.Errorf("daemon: parse %s: %w", path, err)
	}
	if info.Port == 0 || info.Token == "" {
		return SocketInfo{}, errors.New("daemon: socket file missing port or token")
	}
	return info, nil
}

// RemoveSocketFile deletes the socket file. Idempotent — missing file is OK.
func RemoveSocketFile(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// GenerateToken returns a 32-byte random token, base64-URL encoded for use
// in HTTP Authorization headers.
func GenerateToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("daemon: rng: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

// platformNote is informational — surfaced in `daemon` startup logs.
func platformNote() string {
	return fmt.Sprintf("os=%s arch=%s pid=%d", runtime.GOOS, runtime.GOARCH, os.Getpid())
}
