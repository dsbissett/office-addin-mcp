//go:build windows

package launch

import (
	"os/exec"
	"syscall"
)

// configurePlatformProcAttr makes the spawned process detached from the
// parent's console window so taskkill /T can reliably kill the whole tree
// without a stray cmd.exe holding stdin.
func configurePlatformProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
