//go:build windows

package launch

import (
	"fmt"
	"syscall"
	"unsafe"
)

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// PhysicalMemoryMB reports total physical memory in MB via GlobalMemoryStatusEx.
func PhysicalMemoryMB() (int, error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GlobalMemoryStatusEx")
	var msex memoryStatusEx
	msex.Length = uint32(unsafe.Sizeof(msex))
	ret, _, err := proc.Call(uintptr(unsafe.Pointer(&msex)))
	if ret == 0 {
		return 0, fmt.Errorf("GlobalMemoryStatusEx failed: %w", err)
	}
	return int(msex.TotalPhys / (1024 * 1024)), nil
}
