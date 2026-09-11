//go:build !windows

package process

import "os/exec"

func applySysProcAttr(cmd *exec.Cmd) {
	// No-op for non-Windows platforms
}

func postStartHook(cmd *exec.Cmd) {
	// No-op for non-Windows platforms
}