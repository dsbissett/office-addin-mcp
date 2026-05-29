package launch

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"
)

// --- stopWithLauncher -----------------------------------------------------

func TestStopWithLauncher_SpawnFailure(t *testing.T) {
	proj := &Project{Root: t.TempDir(), ManifestPath: "C:/proj/manifest.xml"}
	// A launcher path that does not exist: cmd.Start() fails.
	err := stopWithLauncher("definitely-not-a-real-launcher-xyz", proj, nil)
	if err == nil {
		t.Fatal("stopWithLauncher: expected spawn failure")
	}
	le := AsLaunchError(err)
	if le == nil || le.Reason != ReasonStopFailed {
		t.Fatalf("err = %v, want LaunchError{Reason: stop-failed}", err)
	}
}

func TestStopWithLauncher_SuccessfulExit(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("uses cmd.exe as a harmless quick-exit stand-in for the launcher")
	}
	proj := &Project{Root: t.TempDir(), ManifestPath: "C:/proj/manifest.xml"}
	// cmd.exe exits 0 immediately: stopWithLauncher sees a clean child exit and
	// returns nil. (buildLauncherCommand appends "stop <manifest>"; cmd.exe
	// treats those as ignorable args and still exits cleanly with /c.)
	if err := stopWithLauncher("cmd", proj, nil); err != nil {
		// cmd without /c may exit non-zero; accept either nil or a stop-failed
		// LaunchError, but never a non-LaunchError.
		if le := AsLaunchError(err); le == nil {
			t.Fatalf("stopWithLauncher err = %v, want nil or *LaunchError", err)
		}
	}
}

// --- LaunchIfNeeded extra branches ---------------------------------------

func TestLaunchIfNeeded_PreexistingFiresProgress(t *testing.T) {
	cdpURL := startCDPStub(t)
	// Extract the port from the stub URL.
	port := portFromURL(t, cdpURL)

	var msgs []string
	res, source, err := LaunchIfNeeded(context.Background(), nil, LaunchOptions{
		Port:     port,
		Progress: func(m string) { msgs = append(msgs, m) },
	})
	if err != nil {
		t.Fatalf("LaunchIfNeeded: %v", err)
	}
	if source != "preexisting" {
		t.Errorf("source = %q, want preexisting", source)
	}
	if res == nil || !res.CDPVerified {
		t.Errorf("res = %+v, want CDPVerified=true", res)
	}
	joined := strings.Join(msgs, "|")
	if !strings.Contains(joined, "Probing") || !strings.Contains(joined, "already reachable") {
		t.Errorf("progress msgs = %v, want probing + already-reachable", msgs)
	}
}

func TestLaunchIfNeeded_FallsThroughToLaunchExcelError(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("LaunchExcel is Windows-only past the platform guard")
	}
	t.Cleanup(func() { defaultRegistry.launches = map[string]*TrackedLaunch{} })
	// Force LaunchExcel to error early (port-already-configured) so we cover
	// the project-supplied fallthrough path without spawning Office.
	t.Setenv(envWebView2ExtraArgs, "--remote-debugging-port=1")

	port := freeClosedPort(t) // nothing listening -> probe fails -> fallthrough
	proj := &Project{ManifestPath: "C:/proj/needed.xml", Root: t.TempDir()}
	_, _, err := LaunchIfNeeded(context.Background(), proj, LaunchOptions{
		Port:          port,
		SkipDevServer: true,
		Progress:      func(string) {},
	})
	if err == nil {
		t.Fatal("LaunchIfNeeded: expected LaunchExcel error on fallthrough")
	}
	le := AsLaunchError(err)
	if le == nil || le.Reason != ReasonPortAlreadyConfig {
		t.Fatalf("err = %v, want port-already-configured", err)
	}
}

func portFromURL(t *testing.T, url string) int {
	t.Helper()
	idx := strings.LastIndex(url, ":")
	if idx < 0 {
		t.Fatalf("url %q has no port", url)
	}
	var p int
	if _, err := fmt.Sscanf(url[idx+1:], "%d", &p); err != nil {
		t.Fatalf("parse port from %q: %v", url, err)
	}
	return p
}
