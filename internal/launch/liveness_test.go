package launch

import (
	"os"
	"testing"
)

func TestProcessAlive(t *testing.T) {
	if !processAlive(os.Getpid()) {
		t.Error("processAlive(self) = false, want true for the running test process")
	}
	if processAlive(0) {
		t.Error("processAlive(0) = true, want false")
	}
	if processAlive(-1) {
		t.Error("processAlive(-1) = true, want false")
	}
	// A PID that is extremely unlikely to be in use. Both the Windows and the
	// Unix implementations must report it dead.
	if processAlive(0x7FFFFFF0) {
		t.Log("processAlive(very-high-pid) = true (unexpectedly in use); not failing")
	}
}
