//go:build windows

package webview2

import (
	"context"
	"os"
	"os/exec"
	"reflect"
	"testing"
	"time"
)

// wmicAvailable reports whether wmic is resolvable on PATH. wmic is deprecated
// and absent on some trimmed Windows builds, so wmic-dependent assertions are
// skipped rather than failed when it is missing.
func wmicAvailable() bool {
	_, err := exec.LookPath("wmic")
	return err == nil
}

// TestWmicMsedgeWebView2Output_Success runs the real wmic query. The output is
// non-deterministic (depends on which msedgewebview2.exe processes happen to be
// running) so we assert only that the success branch returns without error and
// yields a string. This covers the wmic-success path of the helper.
func TestWmicMsedgeWebView2Output_Success(t *testing.T) {
	if !wmicAvailable() {
		t.Skip("wmic not available on PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	out, err := wmicMsedgeWebView2Output(ctx)
	if err != nil {
		t.Fatalf("wmicMsedgeWebView2Output: %v", err)
	}
	// Whatever wmic returns must be parseable without panicking.
	_ = parseRemoteDebuggingPorts(out)
}

// TestWmicMsedgeWebView2Output_ExecError forces cmd.Output() to fail by
// stripping wmic from PATH for the duration of the call, exercising the
// error-return branch of the helper. PATH is process-global, so this test must
// not run in parallel.
func TestWmicMsedgeWebView2Output_ExecError(t *testing.T) {
	old, had := os.LookupEnv("PATH")
	if err := os.Setenv("PATH", ""); err != nil {
		t.Fatalf("setenv PATH: %v", err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("PATH", old)
		} else {
			_ = os.Unsetenv("PATH")
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := wmicMsedgeWebView2Output(ctx)
	if err == nil {
		t.Fatalf("expected exec error with wmic absent from PATH, got output %q", out)
	}
	if out != "" {
		t.Errorf("expected empty output on error, got %q", out)
	}
}

// TestScanOSEndpoints_NoPortFlag drives the real scan. On a normal box the
// running msedgewebview2.exe processes (Windows Search/Widgets) carry no
// --remote-debugging-port flag, so the scan parses zero ports and returns
// ErrNotFound without probing. If the host happens to run a debugging-enabled
// WebView2 the scan may succeed; that branch is tolerated.
func TestScanOSEndpoints_NoPortFlag(t *testing.T) {
	if !wmicAvailable() {
		t.Skip("wmic not available on PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	ep, err := scanOSEndpoints(ctx)
	if err != nil {
		if err != ErrNotFound {
			t.Fatalf("got %v, want ErrNotFound", err)
		}
		if ep != (Endpoint{}) {
			t.Errorf("expected zero Endpoint on ErrNotFound, got %+v", ep)
		}
		return
	}
	// Tolerated: a debugging-enabled WebView2/Edge is actually running.
	if ep.Source != SourceScan {
		t.Errorf("scan success source = %q, want %q", ep.Source, SourceScan)
	}
	if ep.WSURL == "" {
		t.Error("scan success returned empty WSURL")
	}
}

// TestScanOSEndpoints_WmicError forces the wmic invocation to fail by stripping
// PATH, exercising the scanOSEndpoints branch that maps any wmic error to
// ErrNotFound. PATH is process-global, so this test must not run in parallel.
func TestScanOSEndpoints_WmicError(t *testing.T) {
	old, had := os.LookupEnv("PATH")
	if err := os.Setenv("PATH", ""); err != nil {
		t.Fatalf("setenv PATH: %v", err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("PATH", old)
		} else {
			_ = os.Unsetenv("PATH")
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ep, err := scanOSEndpoints(ctx)
	if err != ErrNotFound {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
	if ep != (Endpoint{}) {
		t.Errorf("expected zero Endpoint, got %+v", ep)
	}
}

// TestParseRemoteDebuggingPorts_QuotedFormNotMatched documents that the regex
// is intentionally loose and does NOT match the quoted serializer form, the
// space-separated variant, or a trailing non-digit, while still matching the
// bare =N form embedded in a larger token sequence.
func TestParseRemoteDebuggingPorts_EdgeForms(t *testing.T) {
	cases := []struct {
		name string
		blob string
		want []int
	}{
		{
			name: "quoted value not matched",
			blob: `--remote-debugging-port="9222"`,
			want: nil,
		},
		{
			name: "space separated not matched",
			blob: `--remote-debugging-port 9222`,
			want: nil,
		},
		{
			name: "embedded in surrounding flags",
			blob: `--type=renderer --remote-debugging-port=9222 --user-data-dir=x`,
			want: []int{9222},
		},
		{
			name: "trailing digits consumed greedily",
			blob: `--remote-debugging-port=9222abc`,
			want: []int{9222},
		},
		{
			name: "max valid port",
			blob: `--remote-debugging-port=65535`,
			want: []int{65535},
		},
		{
			name: "just over max dropped",
			blob: `--remote-debugging-port=65536`,
			want: nil,
		},
		{
			name: "zero dropped",
			blob: `--remote-debugging-port=0`,
			want: nil,
		},
		{
			name: "order preserved and deduped",
			blob: `--remote-debugging-port=9333 --remote-debugging-port=9222 --remote-debugging-port=9333`,
			want: []int{9333, 9222},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseRemoteDebuggingPorts(tc.blob)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ports = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestParseRemoteDebuggingPorts_HugeNumberOverflow ensures a port literal that
// overflows int parsing is dropped rather than panicking. strconv.Atoi returns
// an error for values past the int range, which the loop skips.
func TestParseRemoteDebuggingPorts_HugeNumberOverflow(t *testing.T) {
	blob := `--remote-debugging-port=99999999999999999999999999`
	if got := parseRemoteDebuggingPorts(blob); len(got) != 0 {
		t.Errorf("ports = %v, want empty (overflow dropped)", got)
	}
}
