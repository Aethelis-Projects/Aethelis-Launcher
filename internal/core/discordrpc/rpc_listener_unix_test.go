//go:build !windows

package discordrpc

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

// listenFakeDiscord binds an AF_UNIX socket; XDG_RUNTIME_DIR (set by the
// tests) is what the default platform dialer scans, so no dialer injection
// is needed on Unix.
func listenFakeDiscord(t *testing.T, runtimeDir, name string) (net.Listener, string, error) {
	t.Helper()
	path := filepath.Join(runtimeDir, name)
	_ = os.Remove(path) // errcheck:ok stale socket cleanup, if any
	ln, err := net.Listen("unix", path)
	return ln, path, err
}

// fakeDialer returns nil on Unix (the default dialer already finds the fake
// through XDG_RUNTIME_DIR).
func fakeDialer(t *testing.T, addr string) Option {
	t.Helper()
	return nil
}
