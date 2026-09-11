package process

import (
	"context"
	"io"
	"os/exec"

	"github.com/nord-launcher/launcher/internal/core/ports"
)

type DefaultProcessManager struct{}

func NewProcessManager() ports.ProcessManager {
	return &DefaultProcessManager{}
}

func (m *DefaultProcessManager) StartProcess(
	ctx context.Context,
	executable string,
	args []string,
	dir string,
	env []string,
	stdout, stderr io.Writer,
) (ports.ProcessHandle, error) {
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	applySysProcAttr(cmd)

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	postStartHook(cmd)

	return &osProcessHandle{cmd: cmd}, nil
}

type osProcessHandle struct {
	cmd *exec.Cmd
}

func (h *osProcessHandle) PID() int {
	if h.cmd.Process != nil {
		return h.cmd.Process.Pid
	}
	return 0
}

func (h *osProcessHandle) Wait() (int, error) {
	err := h.cmd.Wait()
	if h.cmd.ProcessState != nil {
		return h.cmd.ProcessState.ExitCode(), err
	}
	return -1, err
}

func (h *osProcessHandle) Kill() error {
	if h.cmd.Process != nil {
		return h.cmd.Process.Kill()
	}
	return nil
}
