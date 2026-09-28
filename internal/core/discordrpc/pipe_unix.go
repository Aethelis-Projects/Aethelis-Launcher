//go:build !windows

package discordrpc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
)

// dialUnavailableText is asserted on by tests; keep in sync with pipe_windows.go.
const dialUnavailableText = "discord pipe unavailable"

// socketCandidates lists the AF_UNIX endpoints the Discord client listens on
// ($XDG_RUNTIME_DIR/discord-ipc-{0..9}).
func socketCandidates() []string {
	var out []string
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		if home, err := os.UserHomeDir(); err == nil {
			base = filepath.Join(home, ".cache")
		}
	}
	if base != "" {
		for i := 0; i < 10; i++ {
			out = append(out, filepath.Join(base, fmt.Sprintf("%s%d", pipeNamePrefix, i)))
		}
	}
	return out
}

// platformDialer dials AF_UNIX sockets over the candidate paths.
func platformDialer() func(ctx context.Context, candidates []string) (net.Conn, error) {
	return func(ctx context.Context, candidates []string) (net.Conn, error) {
		var lastErr error
		for _, p := range candidates {
			var d net.Dialer
			conn, err := d.DialContext(ctx, "unix", p)
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		if lastErr == nil {
			lastErr = errors.New("no discord ipc candidates")
		}
		return nil, lastErr
	}
}
