package discordrpc

// v0.7.2 review (owner): winio.DialPipeContext busy-loops on ERROR_PIPE_BUSY
// until its context ends, so the manager MUST pass a deadline-bound context -
// otherwise a busy pipe (Discord open, all slots taken) would block the RPC
// loop indefinitely instead of degrading into backoff. The production context
// is context.WithTimeout(ctx, m.dialTimeout); this pins that contract on every
// platform (the dialer is reached through the same injection point).

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestManager_DialerReceivesBoundedContext(t *testing.T) {
	m := NewManager("appid")
	deadlineHit := make(chan struct{}, 1)
	m.SetDialerForTest(func(ctx context.Context, _ []string) (net.Conn, error) {
		dl, ok := ctx.Deadline()
		if !ok {
			return nil, errors.New("dialer ctx has no deadline - winio would busy-loop on a pipe that exists but is full")
		}
		if d := time.Until(dl); d <= 0 || d > 3*time.Second {
			return nil, errors.New("dialer ctx deadline is not the bounded dial timeout")
		}
		<-ctx.Done() // emulate winio's ERROR_PIPE_BUSY retry loop
		select {
		case deadlineHit <- struct{}{}:
		default: // errcheck:ok single observer
		}
		return nil, ctx.Err()
	})
	if m.dialer == nil {
		t.Fatal("dialer not installed")
	}
	m.dialTimeout = 50 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), m.dialTimeout)
	defer cancel()
	start := time.Now()
	_, err := m.dialer(ctx, socketCandidates())
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline-exceeded propagation, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("dialer ignored the ctx deadline: took %v", elapsed)
	}
	select {
	case <-deadlineHit:
	default:
		t.Fatal("dialer never saw ctx cancellation")
	}
}
