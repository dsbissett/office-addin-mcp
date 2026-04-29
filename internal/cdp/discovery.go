package cdp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// BrowserVersion mirrors /json/version.
type BrowserVersion struct {
	Browser              string `json:"Browser"`
	ProtocolVersion      string `json:"Protocol-Version"`
	UserAgent            string `json:"User-Agent"`
	V8Version            string `json:"V8-Version"`
	WebKitVersion        string `json:"WebKit-Version"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// ResolveBrowserWSURL probes <browserURL>/json/version and returns the
// browser-level webSocketDebuggerUrl. browserURL is typically
// http://127.0.0.1:9222.
func ResolveBrowserWSURL(ctx context.Context, browserURL string) (string, error) {
	u, err := url.Parse(browserURL)
	if err != nil {
		return "", fmt.Errorf("parse %q: %w", browserURL, err)
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/json/version"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("probe %s: %w", u.String(), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("probe %s: status %d", u.String(), resp.StatusCode)
	}
	var v BrowserVersion
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return "", fmt.Errorf("decode %s: %w", u.String(), err)
	}
	if v.WebSocketDebuggerURL == "" {
		return "", fmt.Errorf("probe %s: missing webSocketDebuggerUrl", u.String())
	}
	return v.WebSocketDebuggerURL, nil
}
