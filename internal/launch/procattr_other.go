//go:build !windows

package launch

import "os/exec"

// configurePlatformProcAttr is a no-op on non-Windows; LaunchExcel guards the
// call against running on those platforms anyway.
func configurePlatformProcAttr(cmd *exec.Cmd) {
	_ = cmd
}
