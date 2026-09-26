//go:build !windows

package wails

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
)

func openPathExecOS(cleanPath string, isDir bool) error {
	absPath, err := filepath.Abs(cleanPath)
	if err == nil {
		cleanPath = absPath
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		if isDir {
			cmd = exec.Command("open", cleanPath)
		} else {
			cmd = exec.Command("open", "-R", cleanPath)
		}
	default:
		if isDir {
			cmd = exec.Command("xdg-open", cleanPath)
		} else {
			cmd = exec.Command("xdg-open", filepath.Dir(cleanPath))
		}
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to open path: %w", err)
	}
	return nil
}
