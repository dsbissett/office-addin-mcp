//go:build windows

package webview2

import (
	"reflect"
	"testing"
)

func TestParseRemoteDebuggingPorts(t *testing.T) {
	// Representative wmic /format:list output. Two msedgewebview2 processes
	// share the same port; a third is missing the flag entirely. We expect
	// dedup, no spurious matches from the bare process command line.
	const blob = `

CommandLine="C:\Program Files (x86)\Microsoft\EdgeWebView\Application\msedgewebview2.exe" --type=renderer --remote-debugging-port=9222 --user-data-dir="C:\Users\u\AppData\Local\Microsoft\Office\WebView" --foo=bar

CommandLine="C:\Program Files (x86)\Microsoft\EdgeWebView\Application\msedgewebview2.exe" --type=gpu-process --remote-debugging-port=9222

CommandLine="C:\Program Files (x86)\Microsoft\EdgeWebView\Application\msedgewebview2.exe" --type=utility

CommandLine="C:\Program Files (x86)\Microsoft\EdgeWebView\Application\msedgewebview2.exe" --type=renderer --remote-debugging-port=9333
`
	got := parseRemoteDebuggingPorts(blob)
	want := []int{9222, 9333}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ports = %v, want %v", got, want)
	}
}

func TestParseRemoteDebuggingPorts_OutOfRange(t *testing.T) {
	const blob = `--remote-debugging-port=0 --remote-debugging-port=70000 --remote-debugging-port=8080`
	got := parseRemoteDebuggingPorts(blob)
	want := []int{8080}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ports = %v, want %v", got, want)
	}
}

func TestParseRemoteDebuggingPorts_None(t *testing.T) {
	if got := parseRemoteDebuggingPorts("CommandLine=...\n"); len(got) != 0 {
		t.Errorf("ports = %v, want empty", got)
	}
}
