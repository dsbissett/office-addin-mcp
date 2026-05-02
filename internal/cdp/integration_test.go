//go:build integration

// integration_test.go: opt-in headless-Chrome smoke. Build-tagged so CI's
// default `go test ./...` never runs it (Windows runners struggled with the
// DevToolsActivePort wait). Devs run it locally with
//
//	go test -tags integration ./internal/cdp/...
//
// The companion plan F9 also calls for a sample-Office-add-in smoke under
// internal/officejs; that lands separately when a fixture workbook is
// available.

package cdp_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
)

// findChrome returns a path to a Chrome/Chromium binary or "" if none is
// available. Honors CHROME_BIN, then probes typical install locations.
func findChrome() string {
	if v := os.Getenv("CHROME_BIN"); v != "" {
		if _, err := os.Stat(v); err == nil {
			return v
		}
	}
	candidates := []string{}
	switch runtime.GOOS {
	case "windows":
		candidates = []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		}
	case "linux":
		candidates = []string{
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
		}
	case "darwin":
		candidates = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	for _, name := range []string{"google-chrome", "chromium", "chromium-browser"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// readDevToolsPort polls the user-data-dir for the DevToolsActivePort file
// chrome writes after listening; returns the port + first-line.
func readDevToolsPort(dir string, deadline time.Time) (string, error) {
	path := filepath.Join(dir, "DevToolsActivePort")
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(path)
		if err == nil && len(b) > 0 {
			lines := strings.SplitN(string(b), "\n", 2)
			if port := strings.TrimSpace(lines[0]); port != "" {
				return port, nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return "", errors.New("DevToolsActivePort not written before deadline")
}

func TestEvaluate_AgainstHeadlessChrome(t *testing.T) {
	chrome := findChrome()
	if chrome == "" {
		t.Skip("no chrome/chromium binary available; set CHROME_BIN to run")
	}

	userDir := t.TempDir()
	cmd := exec.Command(chrome,
		"--headless=new",
		"--disable-gpu",
		"--no-first-run",
		"--no-default-browser-check",
		"--remote-debugging-port=0",
		"--user-data-dir="+userDir,
		"about:blank",
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start chrome: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	port, err := readDevToolsPort(userDir, time.Now().Add(15*time.Second))
	if err != nil {
		t.Fatalf("wait for chrome: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	wsURL, err := cdp.ResolveBrowserWSURL(ctx, "http://127.0.0.1:"+port)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	conn, err := cdp.Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	targets, err := conn.GetTargets(ctx)
	if err != nil {
		t.Fatalf("getTargets: %v", err)
	}
	target, ok := cdp.FirstPageTarget(targets)
	if !ok {
		tid, err := conn.CreateTarget(ctx, "about:blank")
		if err != nil {
			t.Fatalf("createTarget: %v", err)
		}
		target = cdp.TargetInfo{TargetID: tid, Type: "page"}
	}

	sessionID, err := conn.AttachToTarget(ctx, target.TargetID)
	if err != nil {
		t.Fatalf("attach: %v", err)
	}

	res, err := conn.Evaluate(ctx, sessionID, cdp.EvaluateParams{
		Expression:    "1+1",
		ReturnByValue: true,
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.ExceptionDetails != nil {
		t.Fatalf("evaluation threw: %s", res.ExceptionDetails.String())
	}
	if res.Result == nil || string(res.Result.Value) != "2" {
		var raw json.RawMessage
		if res.Result != nil {
			raw = res.Result.Value
		}
		t.Fatalf("expected value 2, got %s", string(raw))
	}
}
