//go:build linux

package launch

import "golang.org/x/sys/unix"

// PhysicalMemoryMB reports total physical memory in MB (rounded down).
func PhysicalMemoryMB() (int, error) {
	var si unix.Sysinfo_t
	if err := unix.Sysinfo(&si); err != nil {
		return 0, err
	}
	// si.Totalram is measured in page-size units.
	return int(si.Totalram) * int(si.Unit) / (1024 * 1024), nil
}
