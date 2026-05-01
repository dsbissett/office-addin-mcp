package launch

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	defaultDevServerTimeout = 90 * time.Second
	devServerProbeTimeout   = 1500 * time.Millisecond
	probeInterval           = 500 * time.Millisecond
	maxOutputLines          = 200
)

// devServerHandle owns a spawned dev-server process. preexisting=true means
// the port was already listening when EnsureDevServer ran, so the caller did
// not start the server and must not kill it on shutdown.
type devServerHandle struct {
	cmd         *exec.Cmd
	port        int
	preexisting bool
	output      *outputBuffer
}

func (h *devServerHandle) stop() {
	if h == nil || h.preexisting || h.cmd == nil || h.cmd.Process == nil {
		return
	}
	killProcess(h.cmd)
}

// EnsureDevServer probes the project's dev-server port. If nothing is
// listening and the project declares a dev script, it spawns the package
// manager's `run <script>` and waits up to timeout for the port to open.
//
// A preexisting server is detected as preexisting=true on the returned
// handle so addin.stop will leave it running.
func ensureDevServer(ctx context.Context, project *Project, env []string, timeout time.Duration) (*devServerHandle, error) {
	if project.DevServer == nil {
		return nil, nil
	}
	port := project.DevServer.Port
	if IsPortListening(port, devServerProbeTimeout) {
		return &devServerHandle{port: port, preexisting: true}, nil
	}
	if timeout <= 0 {
		timeout = defaultDevServerTimeout
	}

	cmd, err := buildPackageScriptCommand(project, env)
	if err != nil {
		return nil, err
	}
	output := newOutputBuffer(maxOutputLines)
	attachOutput(cmd, output)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("launch: spawn dev server (%s run %s): %w", project.PackageManager, project.DevServer.Script, err)
	}

	deadline := time.Now().Add(timeout)
	exited := waitChild(cmd)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			killProcess(cmd)
			return nil, ctx.Err()
		case st := <-exited:
			return nil, fmt.Errorf("launch: dev server script %q exited (%v) before port %d became ready: %s",
				project.DevServer.Script, st, port, output.tail())
		default:
		}
		if IsPortListening(port, devServerProbeTimeout) {
			return &devServerHandle{cmd: cmd, port: port, output: output}, nil
		}
		select {
		case <-time.After(probeInterval):
		case <-ctx.Done():
			killProcess(cmd)
			return nil, ctx.Err()
		}
	}

	killProcess(cmd)
	return nil, fmt.Errorf("launch: timed out waiting for dev server at http://localhost:%d (script %q): %s",
		port, project.DevServer.Script, output.tail())
}

// buildPackageScriptCommand assembles `<runner> run <script>` for the
// project's package manager, resolved to an absolute path on Windows so that
// cmd.exe does not pick up a stray shim from the cwd or
// project/node_modules/.bin (the parent shim's `%~dp0` lookup breaks if it
// runs from the wrong directory).
func buildPackageScriptCommand(project *Project, env []string) (*exec.Cmd, error) {
	runner := string(project.PackageManager)
	if runtime.GOOS == "windows" {
		// Try .cmd shim explicitly so we don't accidentally invoke the
		// JS source directly via PowerShell rules.
		if abs, err := exec.LookPath(runner + ".cmd"); err == nil {
			runner = abs
		} else if abs, err := exec.LookPath(runner); err == nil {
			runner = abs
		}
	}
	cmd := exec.Command(runner, "run", project.DevServer.Script) //nolint:gosec // runner derived from package manager allow-list
	cmd.Dir = project.Root
	cmd.Env = env
	configurePlatformProcAttr(cmd)
	return cmd, nil
}

// outputBuffer keeps the last N lines of stdout+stderr from a child process.
// It is used to build helpful error messages when launch / dev-server steps
// fail, mirroring the reference's 200-line ring buffer.
type outputBuffer struct {
	mu    sync.Mutex
	lines []string
	max   int
}

func newOutputBuffer(max int) *outputBuffer {
	if max <= 0 {
		max = 200
	}
	return &outputBuffer{max: max}
}

func (b *outputBuffer) append(chunk []byte) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, line := range strings.Split(string(chunk), "\n") {
		line = strings.TrimRight(line, "\r ")
		if line == "" {
			continue
		}
		b.lines = append(b.lines, line)
		if len(b.lines) > b.max {
			b.lines = b.lines[len(b.lines)-b.max:]
		}
	}
}

func (b *outputBuffer) tail() string {
	if b == nil {
		return ""
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.lines) == 0 {
		return ""
	}
	tail := b.lines
	if len(tail) > 20 {
		tail = tail[len(tail)-20:]
	}
	return strings.Join(tail, "\n")
}

func (b *outputBuffer) snapshot() []string {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, len(b.lines))
	copy(out, b.lines)
	return out
}

// attachOutput pipes a child's stdout+stderr into the output buffer. Errors
// piping the streams are non-fatal — we lose visibility but the launch can
// still succeed.
func attachOutput(cmd *exec.Cmd, buf *outputBuffer) {
	stdout, err := cmd.StdoutPipe()
	if err == nil {
		go drainPipe(stdout, buf)
	}
	stderr, err := cmd.StderrPipe()
	if err == nil {
		go drainPipe(stderr, buf)
	}
}

func drainPipe(r interface{ Read(p []byte) (int, error) }, buf *outputBuffer) {
	chunk := make([]byte, 4096)
	for {
		n, err := r.Read(chunk)
		if n > 0 {
			buf.append(chunk[:n])
		}
		if err != nil {
			return
		}
	}
}

// waitChild returns a channel that emits the child's exit status as soon as
// the process exits. Multiple readers are not supported — call once.
func waitChild(cmd *exec.Cmd) <-chan error {
	ch := make(chan error, 1)
	go func() {
		ch <- cmd.Wait()
	}()
	return ch
}

// killProcess terminates a child process tree on the host platform.
// Best-effort: failures are swallowed because the only recourse is to log,
// and the caller is already in a cleanup path.
func killProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if runtime.GOOS == "windows" {
		// Use taskkill /T /F to terminate the whole tree; npm.cmd spawns
		// node which spawns webpack-dev-server, so plain Kill leaks the
		// grandchildren.
		_ = exec.Command("taskkill", "/pid", fmt.Sprintf("%d", cmd.Process.Pid), "/T", "/F").Run() //nolint:gosec // fixed argv with formatted pid
		return
	}
	_ = cmd.Process.Kill()
}

// localBinDir returns the project's node_modules/.bin path. Used both for
// resolving office-addin-debugging and for prepending to PATH.
func localBinDir(root string) string {
	return filepath.Join(root, "node_modules", ".bin")
}

// errLauncherMissing is returned when neither a local node_modules/.bin shim
// nor an npx on PATH can be found.
var errLauncherMissing = errors.New("launch: office-addin-debugging not found")
