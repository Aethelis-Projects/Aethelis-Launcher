//go:build windows

package wails

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"
)

func openPathExecOS(cleanPath string, isDir bool) error {
	absPath, err := filepath.Abs(cleanPath)
	if err == nil {
		cleanPath = absPath
	}
	var cmdLine string
	if isDir {
		cmdLine = fmt.Sprintf(`explorer.exe "%s"`, cleanPath)
	} else {
		cmdLine = fmt.Sprintf(`explorer.exe /select,"%s"`, cleanPath)
	}
	cmd := exec.Command("explorer")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: cmdLine}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to open path: %w", err)
	}
	return nil
}
