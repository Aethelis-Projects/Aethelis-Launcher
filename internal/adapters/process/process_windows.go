//go:build windows

package process

import (
	"os/exec"
	"sync"
	"syscall"
	"unsafe"
)

var (
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObjectW         = kernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject  = kernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJobObject = kernel32.NewProc("AssignProcessToJobObject")

	globalJobOnce   sync.Once
	globalJobHandle syscall.Handle
)

const (
	JobObjectExtendedLimitInformation = 9
	JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE = 0x00002000
)

type jobobjectBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type ioCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type jobobjectExtendedLimitInformation struct {
	BasicLimitInformation jobobjectBasicLimitInformation
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryLimit uintptr
	PeakJobMemoryLimit    uintptr
}

func getOrCreateGlobalJob() syscall.Handle {
	globalJobOnce.Do(func() {
		hJob, _, _ := procCreateJobObjectW.Call(0, 0)
		if hJob == 0 {
			return
		}
		var info jobobjectExtendedLimitInformation
		info.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		ret, _, _ := procSetInformationJobObject.Call(
			hJob,
			JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)),
			uintptr(unsafe.Sizeof(info)),
		)
		if ret != 0 {
			globalJobHandle = syscall.Handle(hJob)
		}
	})
	return globalJobHandle
}

func applySysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}

func postStartHook(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	job := getOrCreateGlobalJob()
	if job == 0 {
		return
	}

	const PROCESS_SET_QUOTA = 0x0100
	const PROCESS_TERMINATE = 0x0001
	hProc, err := syscall.OpenProcess(PROCESS_SET_QUOTA|PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return
	}
	defer syscall.CloseHandle(hProc)

	_, _, _ = procAssignProcessToJobObject.Call(uintptr(job), uintptr(hProc))
}