//go:build windows

package webview2

import "context"

// scanOSEndpoints (Windows) is a v1 stub. PLAN.md §10 calls out future work
// here: enumerate WebView2 user-data dirs (DevToolsActivePort files) and
// scan process command lines for --remote-debugging-port arguments.
func scanOSEndpoints(_ context.Context) (Endpoint, error) {
	return Endpoint{}, ErrNotFound
}
