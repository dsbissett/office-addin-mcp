//go:build windows

package launch

import "golang.org/x/sys/windows"

// processAlive reports whether a process with the given PID is currently
// running. Used to detect a manually-closed Excel before reusing a tracked
// launch record. Uses OpenProcess + GetExitCodeProcess: a still-running process
// reports exit code STILL_ACTIVE (259).
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	const stillActive = 259 // STILL_ACTIVE
	return code == stillActive
}
