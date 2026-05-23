//go:build !windows

package launch

import (
	"os"
	"syscall"
)

// processAlive reports whether a process with the given PID is currently
// running. On Unix, os.FindProcess always succeeds, so liveness is probed with
// signal 0. LaunchExcel itself is Windows-only; this exists for cross-platform
// builds and tests.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}
