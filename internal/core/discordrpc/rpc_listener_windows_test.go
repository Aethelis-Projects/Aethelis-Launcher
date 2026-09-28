//go:build windows

package discordrpc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Microsoft/go-winio"
)

var pipeSeq atomic.Uint32

// listenFakeDiscord binds a real named pipe (the transport the Windows
// Discord client uses) so the pipe path and dialer are exercised exactly as
// production sees them.
func listenFakeDiscord(t *testing.T, runtimeDir, name string) (net.Listener, error) {
	t.Helper()
	_ = runtimeDir
	_ = name
	pipeName := fmt.Sprintf(`\\.\pipe\discord-ipc-e2e-%d`, pipeSeq.Add(1))
	ln, err := winio.ListenPipe(pipeName, &winio.PipeConfig{
		MessageMode: false, InputBufferSize: 64 << 10, OutputBufferSize: 64 << 10,
	})
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() { _ = ln.Close() }) // errcheck:ok test cleanup
	wln, ok := ln.(interface{ Accept() (net.Conn, error) })
	if !ok {
		return nil, errors.New("unexpected pipe listener type")
	}
	return &pipeListener{Listener: ln, accept: wln.Accept}, nil
}

type pipeListener struct {
	net.Listener
	accept func() (net.Conn, error)
}

func (l *pipeListener) Accept() (net.Conn, error) { return l.accept() }

// fakeDialer points the manager at the test pipe (the default dialer scans
// discord-ipc-0..9, which would collide with a real Discord on a dev box).
func fakeDialer(t *testing.T, runtimeDir, name string) Option {
	t.Helper()
	pipeName := fmt.Sprintf(`\\.\pipe\discord-ipc-e2e-%d`, pipeSeq.Load())
	return WithDialer(func(ctx context.Context, _ []string) (net.Conn, error) {
		c, err := winio.DialPipeContext(ctx, strings.TrimSpace(pipeName))
		if err != nil {
			return nil, err
		}
		return net.Conn(c), nil
	})
}
