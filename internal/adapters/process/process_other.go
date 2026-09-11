//go:build !windows

package process

import "os/exec"

func applySysProcAttr(cmd *exec.Cmd) {
	// No-op for non-Windows platforms
}
