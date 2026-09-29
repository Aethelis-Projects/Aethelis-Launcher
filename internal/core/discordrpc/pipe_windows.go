//go:build windows

package discordrpc

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/Microsoft/go-winio"
)

// dialUnavailableText is asserted on by tests; keep in sync with pipe_unix.go.
const dialUnavailableText = "discord pipe unavailable"

// Discord's Windows client exposes the same RPC protocol over named pipes
// (\\.\pipe\discord-ipc-{0..9}) rather than AF_UNIX sockets.
func socketCandidates() []string {
	out := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		out = append(out, fmt.Sprintf(`\\.\pipe\%s%d`, pipeNamePrefix, i))
	}
	return out
}

// platformDialer dials named pipes with the manager's dial timeout so a
// missing Discord client degrades silently instead of hanging the loop.
func platformDialer() func(ctx context.Context, candidates []string) (net.Conn, error) {
	return func(ctx context.Context, candidates []string) (net.Conn, error) {
		var lastErr error
		for _, p := range candidates {
			conn, err := winio.DialPipeContext(ctx, p)
			if err == nil {
				return net.Conn(conn), nil
			}
			lastErr = err
			if ctx.Err() != nil {
				break // timeout/cancel: remaining candidates would all block
			}
		}
		if lastErr == nil {
			lastErr = errors.New("no discord ipc candidates")
		}
		return nil, lastErr
	}
}
