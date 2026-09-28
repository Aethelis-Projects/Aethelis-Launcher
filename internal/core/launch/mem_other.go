//go:build !linux && !windows

package launch

import "errors"

// PhysicalMemoryMB is unsupported on this platform (no release binaries are
// built for it); callers fall back to the conservative heuristic default.
func PhysicalMemoryMB() (int, error) {
	return 0, errors.New("physical memory detection unsupported on this platform")
}
