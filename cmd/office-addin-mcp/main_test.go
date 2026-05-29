package main

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func buildBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "office-addin-mcp")
	if os.PathSeparator == '\\' {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("go build: %v", err)
	}
	return bin
}

// TestVersionFlag verifies --version prints a non-empty version and exits 0.
func TestVersionFlag(t *testing.T) {
	bin := buildBinary(t)
	var out bytes.Buffer
	cmd := exec.Command(bin, "--version")
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run --version: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got == "" {
		t.Fatalf("expected non-empty version, got %q", got)
	}
}

// TestUnknownFlagExits2 verifies that bad flags fail with exit 2.
func TestUnknownFlagExits2(t *testing.T) {
	bin := buildBinary(t)
	cmd := exec.Command(bin, "--definitely-not-a-flag")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit for unknown flag")
	}
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 2 {
		t.Fatalf("expected exit code 2, got %v", err)
	}
}

// TestPositionalArgRejected verifies that legacy subcommand-style invocations
// (e.g. `office-addin-mcp call`) get a clear error rather than silently
// starting an MCP stdio server.
func TestPositionalArgRejected(t *testing.T) {
	bin := buildBinary(t)
	var stderr bytes.Buffer
	cmd := exec.Command(bin, "call")
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit for positional arg")
	}
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 2 {
		t.Fatalf("expected exit code 2, got %v", err)
	}
	if !strings.Contains(stderr.String(), "unexpected argument") {
		t.Errorf("missing helpful error in stderr: %q", stderr.String())
	}
}

// --- In-process tests for run() and pure helpers ----------------------------
//
// The subprocess tests above prove the released binary's exit codes, but they
// don't contribute to this package's statement coverage (coverage is measured
// on the test binary, not the spawned process). The tests below call run() and
// the helpers directly so the flag-parsing, logging-setup, auto-launch, and
// happy-path wiring are all covered in-process.

// redirectCacheDirs points the recorder and doccache default paths at a unique
// temp dir so run()'s happy path never writes to the user's real
// %LOCALAPPDATA%\office-addin-mcp cache. It restores the previous values on
// cleanup. Honoring LOCALAPPDATA (Windows) and XDG_CACHE_HOME (POSIX) covers
// both DefaultDir/DefaultPath branches.
func redirectCacheDirs(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
}

// runInProc invokes run() with the given args, capturing stdout/stderr. It is
// for the early-return paths (no MCP serve loop reached).
func runInProc(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	redirectCacheDirs(t)
	var outBuf, errBuf bytes.Buffer
	code = run(args, &outBuf, &errBuf)
	return code, outBuf.String(), errBuf.String()
}

func TestRun_Version(t *testing.T) {
	code, stdout, _ := runInProc(t, "--version")
	if code != 0 {
		t.Fatalf("--version exit = %d, want 0", code)
	}
	if strings.TrimSpace(stdout) != version {
		t.Fatalf("--version stdout = %q, want %q", strings.TrimSpace(stdout), version)
	}
}

func TestRun_Help(t *testing.T) {
	// flag's built-in -h/--help triggers flag.ErrHelp → run returns 0.
	code, _, stderr := runInProc(t, "--help")
	if code != 0 {
		t.Fatalf("--help exit = %d, want 0", code)
	}
	if !strings.Contains(stderr, "usage: office-addin-mcp") {
		t.Fatalf("--help did not print usage; stderr=%q", stderr)
	}
}

func TestRun_UnknownFlag(t *testing.T) {
	code, _, stderr := runInProc(t, "--definitely-not-a-flag")
	if code != 2 {
		t.Fatalf("unknown flag exit = %d, want 2", code)
	}
	if stderr == "" {
		t.Fatalf("expected flag error on stderr")
	}
}

func TestRun_PositionalArgRejected(t *testing.T) {
	code, _, stderr := runInProc(t, "call")
	if code != 2 {
		t.Fatalf("positional arg exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "unexpected argument") {
		t.Fatalf("missing helpful error; stderr=%q", stderr)
	}
}

func TestRun_BadLogLevel(t *testing.T) {
	code, _, stderr := runInProc(t, "--log-level", "verbose")
	if code != 2 {
		t.Fatalf("bad log level exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "invalid --log-level") {
		t.Fatalf("missing log-level error; stderr=%q", stderr)
	}
}

func TestRun_LogFileOpenError(t *testing.T) {
	// Point --log-file at a path whose parent is a regular file, so OpenFile
	// fails and run returns 1 before reaching the serve loop.
	dir := t.TempDir()
	notADir := filepath.Join(dir, "file")
	if err := os.WriteFile(notADir, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	badPath := filepath.Join(notADir, "log.txt") // parent is a file → open fails
	code, _, stderr := runInProc(t, "--log-file", badPath)
	if code != 1 {
		t.Fatalf("log-file open error exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "open log file") {
		t.Fatalf("missing open-log-file error; stderr=%q", stderr)
	}
}

// withStdinEOF replaces os.Stdin with an *os.File pipe whose write end is
// closed immediately, so any reader sees EOF. It also swaps os.Stdout to a
// throwaway pipe so the MCP server's writes don't pollute the test's real
// stdout. Restores both on cleanup. Returns a drain channel for stdout so the
// write end never blocks the server.
func withStdinEOF(t *testing.T) {
	t.Helper()

	origStdin := os.Stdin
	origStdout := os.Stdout

	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	// Close the write end now: readers of inR observe EOF immediately.
	if err := inW.Close(); err != nil {
		t.Fatalf("close stdin writer: %v", err)
	}

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	// Drain anything the server writes so it can't block on a full pipe.
	drainDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, outR)
		close(drainDone)
	}()

	os.Stdin = inR
	os.Stdout = outW

	t.Cleanup(func() {
		os.Stdin = origStdin
		os.Stdout = origStdout
		_ = outW.Close() // unblock the drain goroutine
		<-drainDone
		_ = outR.Close()
		_ = inR.Close()
		// Restore the default slog handler other packages may rely on.
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	})
}

// runHappyPath drives run() all the way to srv.Run with a closed (EOF) stdin so
// the MCP serve loop returns promptly instead of blocking. It guards with a
// timeout so a regression can't hang the suite.
func runHappyPath(t *testing.T, args ...string) int {
	t.Helper()
	redirectCacheDirs(t)
	withStdinEOF(t)

	var outBuf, errBuf bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- run(args, &outBuf, &errBuf)
	}()

	select {
	case code := <-done:
		return code
	case <-time.After(20 * time.Second):
		t.Fatalf("run() did not return within timeout; serve loop likely blocked. stderr=%q", errBuf.String())
		return -1
	}
}

// TestRun_ServeLoopReturnsOnStdinEOF covers the full happy-path wiring: log
// setup, session manager, recorder, doccache, server construction, and
// srv.Run returning when stdin hits EOF before any JSON-RPC handshake.
func TestRun_ServeLoopReturnsOnStdinEOF(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "diag.log")
	// Valid --log-file exercises the file-sink branch; --no-doccache exercises
	// the disabled-cache path. EOF stdin makes srv.Run return.
	code := runHappyPath(t, "--log-file", logFile, "--log-level", "debug", "--no-doccache")
	// A clean EOF before initialize yields either a nil error (return 0) or a
	// transport error (return 1); both are valid terminations of the loop.
	if code != 0 && code != 1 {
		t.Fatalf("serve loop exit = %d, want 0 or 1", code)
	}
}

// TestRun_LaunchAddinNoProject covers the --launch-addin branch where cwd has
// no Office add-in project: autoLaunchAddin fails, run logs a warning and still
// proceeds into the serve loop (which returns on EOF stdin).
func TestRun_LaunchAddinNoProject(t *testing.T) {
	// Run from a temp dir with no package.json so DetectAddin fails fast.
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	code := runHappyPath(t, "--launch-addin")
	if code != 0 && code != 1 {
		t.Fatalf("launch-addin serve exit = %d, want 0 or 1", code)
	}
}

// TestRun_RecorderUnavailable covers the branch where recorder.New fails
// (its os.MkdirAll can't create the macros dir) so run logs a warning and
// proceeds with rec == nil. Pointing LOCALAPPDATA/XDG_CACHE_HOME at a regular
// file makes a path component non-creatable.
func TestRun_RecorderUnavailable(t *testing.T) {
	withStdinEOF(t)

	dir := t.TempDir()
	bad := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(bad, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	// recorder.DefaultDir builds <base>/office-addin-mcp/macros; with base being
	// a regular file, MkdirAll fails on the file component.
	t.Setenv("LOCALAPPDATA", bad)
	t.Setenv("XDG_CACHE_HOME", bad)

	var outBuf, errBuf bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- run(nil, &outBuf, &errBuf)
	}()
	select {
	case code := <-done:
		if code != 0 && code != 1 {
			t.Fatalf("recorder-unavailable serve exit = %d, want 0 or 1", code)
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("run() blocked; stderr=%q", errBuf.String())
	}
}

// TestRun_DangerousEnvFlag covers the env-var path for dangerous CDP gating.
func TestRun_DangerousEnvFlag(t *testing.T) {
	t.Setenv(dangerousEnvVar, "1")
	code := runHappyPath(t) // no flags; dangerous comes from env
	if code != 0 && code != 1 {
		t.Fatalf("dangerous-env serve exit = %d, want 0 or 1", code)
	}
}

func TestAutoLaunchAddin_NoProject(t *testing.T) {
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	url, err := autoLaunchAddin(t.Context())
	if err == nil {
		t.Fatalf("expected detect error in a non-project dir, got url=%q", url)
	}
	if url != "" {
		t.Fatalf("expected empty url on failure, got %q", url)
	}
	if !strings.Contains(err.Error(), "detect under") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseLogLevel(t *testing.T) {
	cases := []struct {
		in      string
		want    slog.Level
		wantErr bool
	}{
		{"debug", slog.LevelDebug, false},
		{"DEBUG", slog.LevelDebug, false},
		{"info", slog.LevelInfo, false},
		{"", slog.LevelInfo, false},
		{"  Info  ", slog.LevelInfo, false},
		{"warn", slog.LevelWarn, false},
		{"warning", slog.LevelWarn, false},
		{"error", slog.LevelError, false},
		{"verbose", 0, true},
		{"trace", 0, true},
	}
	for _, c := range cases {
		got, err := parseLogLevel(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseLogLevel(%q) expected error, got level %v", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseLogLevel(%q) unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseLogLevel(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestEnvFlagSet(t *testing.T) {
	const name = "OAMCP_TEST_FLAG"
	truthy := []string{"1", "true", "TRUE", "yes"}
	for _, v := range truthy {
		t.Setenv(name, v)
		if !envFlagSet(name) {
			t.Errorf("envFlagSet(%q=%q) = false, want true", name, v)
		}
	}
	falsy := []string{"", "0", "false", "no", "True", "YES", "on"}
	for _, v := range falsy {
		t.Setenv(name, v)
		if envFlagSet(name) {
			t.Errorf("envFlagSet(%q=%q) = true, want false", name, v)
		}
	}
}

func TestWriteUsage(t *testing.T) {
	var buf bytes.Buffer
	writeUsage(&buf)
	out := buf.String()
	for _, want := range []string{
		"usage: office-addin-mcp",
		"--browser-url",
		"--ws-endpoint",
		"--launch-addin",
		"--launch-excel",
		"--allow-dangerous-cdp",
		"--no-doccache",
		"--version",
		dangerousEnvVar,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("usage missing %q\nfull:\n%s", want, out)
		}
	}
}
