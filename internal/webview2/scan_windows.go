//go:build windows

package webview2

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"time"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
)

// scanOSEndpoints implements WebView2 endpoint discovery on Windows.
//
// Excel inherits WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS as an env var, but the
// child msedgewebview2.exe processes parse it into actual command-line flags.
// `wmic` exposes those command lines, so the scan is:
//
//  1. Enumerate msedgewebview2.exe command lines via `wmic`.
//  2. Parse out every --remote-debugging-port=N occurrence.
//  3. Probe http://127.0.0.1:N/json/version for each.
//  4. Return the first responding endpoint.
//
// Returns ErrNotFound if wmic is missing, no msedgewebview2.exe is running,
// no port flag is present, or none of the probed ports answers.
func scanOSEndpoints(ctx context.Context) (Endpoint, error) {
	out, err := wmicMsedgeWebView2Output(ctx)
	if err != nil {
		return Endpoint{}, ErrNotFound
	}
	for _, port := range parseRemoteDebuggingPorts(out) {
		url := fmt.Sprintf("http://127.0.0.1:%d", port)
		probeCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
		ws, perr := cdp.ResolveBrowserWSURL(probeCtx, url)
		cancel()
		if perr == nil {
			return Endpoint{BrowserURL: url, WSURL: ws, Source: SourceScan}, nil
		}
	}
	return Endpoint{}, ErrNotFound
}

// wmicMsedgeWebView2Output runs `wmic process where ... get CommandLine` with
// a 5-second ceiling. Failures (wmic absent, access denied, slow box) bubble
// up so callers can degrade to ErrNotFound rather than retrying.
func wmicMsedgeWebView2Output(ctx context.Context) (string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	//nolint:gosec // fixed argv, no user-controlled input.
	cmd := exec.CommandContext(cmdCtx,
		"wmic", "process",
		"where", "name='msedgewebview2.exe'",
		"get", "CommandLine",
		"/format:list",
	)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// portRE matches `--remote-debugging-port=NNNN` anywhere in a string. Quoted
// forms (`--remote-debugging-port="9222"`) and the space-separated variant are
// not produced by Chromium's CommandLine serializer, so we keep this loose.
var portRE = regexp.MustCompile(`--remote-debugging-port=(\d+)`)

// parseRemoteDebuggingPorts pulls every distinct port number out of a blob
// of wmic CommandLine output. Order is preserved so probe order is
// deterministic for a given input.
func parseRemoteDebuggingPorts(blob string) []int {
	matches := portRE.FindAllStringSubmatch(blob, -1)
	seen := map[int]bool{}
	ports := make([]int, 0, len(matches))
	for _, m := range matches {
		p, err := strconv.Atoi(m[1])
		if err != nil || p <= 0 || p > 65535 {
			continue
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		ports = append(ports, p)
	}
	return ports
}
