// Package webview2 owns endpoint discovery — finding the WebSocket URL of a
// running CDP-enabled browser (Chrome, Edge, Excel-hosted WebView2). The
// package is stratified above internal/cdp: it uses cdp.ResolveBrowserWSURL
// to probe but does not import any WebSocket protocol code.
package webview2

import (
	"context"
	"errors"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
)

// Source describes how an Endpoint was discovered.
type Source string

const (
	SourceWSEndpoint Source = "ws-endpoint"
	SourceBrowserURL Source = "browser-url"
	SourceDefault    Source = "default-9222"
	SourceScan       Source = "os-scan"
)

// DefaultBrowserURL is the conventional Chrome/WebView2 DevTools HTTP endpoint.
const DefaultBrowserURL = "http://127.0.0.1:9222"

// Endpoint is a resolved CDP endpoint.
type Endpoint struct {
	BrowserURL string // HTTP REST endpoint (may be empty if WSEndpoint given directly)
	WSURL      string // WebSocket URL to dial
	Source     Source
}

// Config controls Discover priority.
type Config struct {
	WSEndpoint string // explicit WS URL; highest priority, no probe
	BrowserURL string // explicit HTTP REST endpoint to probe via /json/version
}

// ErrNotFound is returned when no endpoint can be discovered.
var ErrNotFound = errors.New("webview2: no debugger endpoint found")

// Discover resolves an Endpoint per the priority ladder in PLAN.md §6:
//
//  1. cfg.WSEndpoint               — used directly without probing.
//  2. cfg.BrowserURL               — probed via /json/version; failure is
//     hard, since the user explicitly named it.
//  3. DefaultBrowserURL (:9222)    — probed; soft failure falls through.
//  4. OS-specific scan             — Windows: WebView2 user-data-dir scan
//     (v1 stub returns ErrNotFound). Other: ErrNotFound.
func Discover(ctx context.Context, cfg Config) (Endpoint, error) {
	if cfg.WSEndpoint != "" {
		return Endpoint{WSURL: cfg.WSEndpoint, Source: SourceWSEndpoint}, nil
	}
	if cfg.BrowserURL != "" {
		ws, err := cdp.ResolveBrowserWSURL(ctx, cfg.BrowserURL)
		if err != nil {
			return Endpoint{}, fmt.Errorf("webview2: probe %s: %w", cfg.BrowserURL, err)
		}
		return Endpoint{BrowserURL: cfg.BrowserURL, WSURL: ws, Source: SourceBrowserURL}, nil
	}
	if ws, err := cdp.ResolveBrowserWSURL(ctx, DefaultBrowserURL); err == nil {
		return Endpoint{BrowserURL: DefaultBrowserURL, WSURL: ws, Source: SourceDefault}, nil
	}
	if ep, err := scanOSEndpoints(ctx); err == nil {
		return ep, nil
	}
	return Endpoint{}, ErrNotFound
}
