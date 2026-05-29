package launch

import (
	"context"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// --- outputBuffer ---------------------------------------------------------

func TestOutputBuffer_AppendTailSnapshot(t *testing.T) {
	b := newOutputBuffer(3)
	// Multi-line chunk; blank lines and trailing \r/space are dropped.
	b.append([]byte("one\r\ntwo \nthree\nfour\n\n"))

	// max is 3, so the oldest ("one") is evicted.
	snap := b.snapshot()
	if len(snap) != 3 {
		t.Fatalf("snapshot len = %d, want 3 (ring capped): %v", len(snap), snap)
	}
	if snap[0] != "two" || snap[2] != "four" {
		t.Errorf("snapshot = %v, want [two three four]", snap)
	}
	// snapshot returns a copy: mutating it must not affect the buffer.
	snap[0] = "MUTATED"
	if again := b.snapshot(); again[0] == "MUTATED" {
		t.Error("snapshot did not return an independent copy")
	}

	if got := b.tail(); got != "two\nthree\nfour" {
		t.Errorf("tail = %q, want %q", got, "two\nthree\nfour")
	}
}

func TestOutputBuffer_TailLimitsToTwenty(t *testing.T) {
	b := newOutputBuffer(100)
	var sb strings.Builder
	for i := 0; i < 30; i++ {
		sb.WriteString("line")
		sb.WriteByte(byte('a' + i%26))
		sb.WriteByte('\n')
	}
	b.append([]byte(sb.String()))
	tail := strings.Split(b.tail(), "\n")
	if len(tail) != 20 {
		t.Errorf("tail line count = %d, want 20", len(tail))
	}
}

func TestOutputBuffer_DefaultMaxWhenNonPositive(t *testing.T) {
	b := newOutputBuffer(0)
	if b.max != 200 {
		t.Errorf("newOutputBuffer(0).max = %d, want 200", b.max)
	}
	b = newOutputBuffer(-5)
	if b.max != 200 {
		t.Errorf("newOutputBuffer(-5).max = %d, want 200", b.max)
	}
}

func TestOutputBuffer_NilReceiverSafe(t *testing.T) {
	var b *outputBuffer
	// All three must be nil-safe (drainPipe / tail may run against nil).
	b.append([]byte("x"))
	if got := b.tail(); got != "" {
		t.Errorf("nil.tail() = %q, want empty", got)
	}
	if got := b.snapshot(); got != nil {
		t.Errorf("nil.snapshot() = %v, want nil", got)
	}
}

func TestOutputBuffer_EmptyTailAndSnapshot(t *testing.T) {
	b := newOutputBuffer(5)
	if got := b.tail(); got != "" {
		t.Errorf("tail() on empty = %q, want empty", got)
	}
	if got := b.snapshot(); len(got) != 0 {
		t.Errorf("snapshot() on empty = %v, want empty", got)
	}
}

// --- localBinDir ----------------------------------------------------------

func TestLocalBinDir(t *testing.T) {
	got := localBinDir("C:/proj")
	if !strings.Contains(got, "node_modules") || !strings.Contains(got, ".bin") {
		t.Errorf("localBinDir = %q, want it to contain node_modules/.bin", got)
	}
}

// --- buildPackageScriptCommand -------------------------------------------

func TestBuildPackageScriptCommand(t *testing.T) {
	proj := &Project{
		Root:           "C:/proj",
		PackageManager: PackageManagerNpm,
		DevServer:      &DevServer{Script: "dev-server", Port: 3000},
	}
	env := []string{"FOO=bar"}
	cmd, err := buildPackageScriptCommand(proj, env)
	if err != nil {
		t.Fatalf("buildPackageScriptCommand: %v", err)
	}
	if cmd.Dir != "C:/proj" {
		t.Errorf("cmd.Dir = %q, want C:/proj", cmd.Dir)
	}
	if len(cmd.Env) != 1 || cmd.Env[0] != "FOO=bar" {
		t.Errorf("cmd.Env = %v, want [FOO=bar]", cmd.Env)
	}
	// The final two argv entries are always "run <script>" regardless of how
	// the runner path was resolved.
	args := cmd.Args
	if len(args) < 3 {
		t.Fatalf("cmd.Args = %v, want at least 3 entries", args)
	}
	if args[len(args)-2] != "run" || args[len(args)-1] != "dev-server" {
		t.Errorf("trailing args = %v, want [... run dev-server]", args)
	}
}

// --- ensureDevServer ------------------------------------------------------

func TestEnsureDevServer_NoDevServerReturnsNil(t *testing.T) {
	proj := &Project{Root: "C:/proj", PackageManager: PackageManagerNpm}
	h, err := ensureDevServer(context.Background(), proj, nil, time.Second)
	if err != nil {
		t.Fatalf("ensureDevServer: %v", err)
	}
	if h != nil {
		t.Errorf("handle = %+v, want nil when project declares no dev server", h)
	}
}

func TestEnsureDevServer_PreexistingPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port

	proj := &Project{
		Root:           t.TempDir(),
		PackageManager: PackageManagerNpm,
		DevServer:      &DevServer{Script: "dev-server", Port: port},
	}
	h, err := ensureDevServer(context.Background(), proj, nil, time.Second)
	if err != nil {
		t.Fatalf("ensureDevServer: %v", err)
	}
	if h == nil || !h.preexisting {
		t.Fatalf("handle = %+v, want preexisting=true", h)
	}
	if h.port != port {
		t.Errorf("handle.port = %d, want %d", h.port, port)
	}
	// stop() must be a no-op for a preexisting server (no panic, leaves it up).
	h.stop()
}

func TestEnsureDevServer_ScriptExitsBeforePortReady(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("uses cmd.exe as a harmless quick-exit child")
	}
	// Pick a definitely-closed port so IsPortListening always returns false,
	// forcing the spawn path. Using PackageManager "cmd" makes the runner
	// resolve to cmd.exe (via LookPath), and `cmd.exe run <script>` exits
	// quickly without ever opening the port — exercising the early-exit branch.
	closedPort := freeClosedPort(t)
	proj := &Project{
		Root:           t.TempDir(),
		PackageManager: PackageManager("cmd"),
		DevServer:      &DevServer{Script: "/c exit 0", Port: closedPort},
	}
	_, err := ensureDevServer(context.Background(), proj, nil, 5*time.Second)
	if err == nil {
		t.Fatal("ensureDevServer: expected error when script exits before port ready")
	}
	if !strings.Contains(err.Error(), "exited") && !strings.Contains(err.Error(), "timed out") {
		t.Errorf("err = %v, want exited/timed-out diagnostic", err)
	}
}

func TestEnsureDevServer_ContextCancelled(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("uses cmd.exe as a harmless long-running child")
	}
	closedPort := freeClosedPort(t)
	proj := &Project{
		Root:           t.TempDir(),
		PackageManager: PackageManager("cmd"),
		// Sleep long enough that the port never opens before we cancel.
		DevServer: &DevServer{Script: "/c timeout /t 30 /nobreak", Port: closedPort},
	}
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel almost immediately so the wait loop trips the ctx.Done() branch.
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	_, err := ensureDevServer(ctx, proj, nil, 30*time.Second)
	if err == nil {
		t.Fatal("ensureDevServer: expected error after context cancel")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("err = %v, want context-canceled", err)
	}
}

func TestEnsureDevServer_SpawnFailure(t *testing.T) {
	closedPort := freeClosedPort(t)
	// A package manager name that resolves to nothing and is not a real path:
	// exec.Command stores it verbatim and Start() fails to find the binary.
	proj := &Project{
		Root:           t.TempDir(),
		PackageManager: PackageManager("definitely-not-a-real-runner-xyz"),
		DevServer:      &DevServer{Script: "dev", Port: closedPort},
	}
	_, err := ensureDevServer(context.Background(), proj, nil, time.Second)
	if err == nil {
		t.Fatal("ensureDevServer: expected spawn failure for missing runner")
	}
	if !strings.Contains(err.Error(), "spawn dev server") {
		t.Errorf("err = %v, want 'spawn dev server' diagnostic", err)
	}
}

// --- process plumbing: attachOutput / drainPipe / waitChild / killProcess -

func TestAttachOutputAndWaitChild_CapturesEchoAndExit(t *testing.T) {
	cmd := harmlessEchoCommand(t, "hello-from-child")
	buf := newOutputBuffer(maxOutputLines)
	attachOutput(cmd, buf)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	exited := waitChild(cmd)
	select {
	case err := <-exited:
		if err != nil {
			t.Fatalf("child exited with error: %v", err)
		}
	case <-time.After(10 * time.Second):
		killProcess(cmd)
		t.Fatal("child did not exit in time")
	}
	// Give the drain goroutines a beat to flush after the process exits.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(strings.Join(buf.snapshot(), "\n"), "hello-from-child") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("output buffer = %v, want it to contain echoed text", buf.snapshot())
}

func TestKillProcess_TerminatesRunningChild(t *testing.T) {
	cmd := harmlessSleepCommand(t)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	exited := waitChild(cmd)
	killProcess(cmd)
	select {
	case <-exited:
		// Killed (or finished) — either way the wait returned.
	case <-time.After(15 * time.Second):
		t.Fatal("killProcess did not terminate the child")
	}
}

func TestKillProcess_NilSafe(t *testing.T) {
	killProcess(nil)
	killProcess(&exec.Cmd{}) // Process is nil
}

func TestDevServerHandle_StopNilAndNoProcess(t *testing.T) {
	var h *devServerHandle
	h.stop() // nil receiver
	(&devServerHandle{}).stop()
	(&devServerHandle{preexisting: true, cmd: &exec.Cmd{}}).stop()
}

// --- helpers --------------------------------------------------------------

// freeClosedPort binds an ephemeral port, captures it, then closes the
// listener so the port is (almost certainly) free again — useful when a test
// needs a port number that nothing is listening on.
func freeClosedPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// harmlessEchoCommand returns a cmd that prints text and exits 0, used to
// drive the spawn/drain/wait plumbing without touching Office or the network.
func harmlessEchoCommand(t *testing.T, text string) *exec.Cmd {
	t.Helper()
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/c", "echo "+text)
	}
	return exec.Command("echo", text)
}

// harmlessSleepCommand returns a cmd that blocks for a while, used to verify
// killProcess actually terminates a running child.
func harmlessSleepCommand(t *testing.T) *exec.Cmd {
	t.Helper()
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/c", "timeout", "/t", "30", "/nobreak")
	}
	return exec.Command("sleep", "30")
}
