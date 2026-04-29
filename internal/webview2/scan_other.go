//go:build !windows

package webview2

import "context"

// scanOSEndpoints is a no-op on non-Windows platforms. WebView2 itself is
// Windows-only, so this is purely a symmetric build target.
func scanOSEndpoints(_ context.Context) (Endpoint, error) {
	return Endpoint{}, ErrNotFound
}
