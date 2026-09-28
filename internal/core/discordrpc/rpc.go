// Package discordrpc implements Discord Rich Presence over Discord's local
// IPC pipe only (AF_UNIX $XDG_RUNTIME_DIR/discord-ipc-{0..9} on Unix,
// \\.\pipe\discord-ipc-{0..9} on Windows). It never opens network sockets:
// everything goes through the locally running Discord client.
//
// Payload policy (directive D'5): details/state/timestamps only, zero account
// identifiers, opt-in, silent degradation when Discord is absent — failures
// are recorded for the status view but never surfaced as popups or spam.
package discordrpc

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Discord IPC opcodes.
const (
	opHandshake = 0
	opFrame     = 1
	opClose     = 2
	opPing      = 3
	opPong      = 4
)

// maxFrameLen guards against absurd frames from a misbehaving pipe peer.
const maxFrameLen = 1 << 20

// Activity is the presence payload Discord renders. Zero account fields by
// construction — the struct simply does not carry any.
type Activity struct {
	Details   string `json:"details"`
	State     string `json:"state"`
	StartUnix int64  `json:"start,omitempty"`
}

// ErrNotConnected is returned when no live Discord pipe is attached.
var ErrNotConnected = errors.New("discord ipc not connected")

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
			out = append(out, filepath.Join(base, fmt.Sprintf("discord-ipc-%d", i)))
		}
	}
	return out
}

func writeFrame(w io.Writer, op int, payload []byte) error {
	var hdr [8]byte
	binary.LittleEndian.PutUint32(hdr[0:4], uint32(op))
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	if len(payload) == 0 {
		return nil
	}
	_, err := w.Write(payload)
	return err
}

func readFrame(r io.Reader) (int, []byte, error) {
	var hdr [8]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	op := int(binary.LittleEndian.Uint32(hdr[0:4]))
	n := binary.LittleEndian.Uint32(hdr[4:8])
	if n > maxFrameLen {
		return 0, nil, fmt.Errorf("discord frame too large: %d", n)
	}
	if n == 0 {
		return op, nil, nil
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, nil, err
	}
	return op, buf, nil
}

func encodeJSON(v interface{}) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// Manager owns the pipe connection and keeps presence in sync. All public
// methods are safe for concurrent use. The connect loop uses capped
// exponential backoff — that is what gives "Discord restart -> reconnect".
type Manager struct {
	appID  string
	nowFn  func() time.Time
	dialer func(ctx context.Context, candidates []string) (net.Conn, error)

	mu        sync.Mutex
	conn      net.Conn
	enabled   bool
	connected bool
	inFlight  *Activity
	lastErr   string
	lastOK    time.Time
	stopCh    chan struct{}
	loopDone  chan struct{}

	dialTimeout time.Duration
	backoffMax  time.Duration
}

// Option tweaks manager behavior (tests inject fake dialers/clocks).
type Option func(*Manager)

func WithDialer(d func(ctx context.Context, candidates []string) (net.Conn, error)) Option {
	return func(m *Manager) { m.dialer = d }
}

func WithClock(now func() time.Time) Option {
	return func(m *Manager) { m.nowFn = now }
}

func NewManager(appID string, opts ...Option) *Manager {
	m := &Manager{
		appID:       appID,
		nowFn:       time.Now,
		dialTimeout: 750 * time.Millisecond,
		backoffMax:  30 * time.Second,
	}
	for _, o := range opts {
		o(m)
	}
	if m.dialer == nil {
		m.dialer = func(ctx context.Context, candidates []string) (net.Conn, error) {
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
	return m
}

// SetEnabled starts/stops the connect loop. Disabling sends the CLOSE frame
// (the RPC equivalent of the "Close" button), clears state and tears the
// pipe down — presence disappears well inside the 5s acceptance window.
func (m *Manager) SetEnabled(enabled bool) {
	m.mu.Lock()
	if m.enabled == enabled {
		m.mu.Unlock()
		return
	}
	m.enabled = enabled
	if enabled {
		m.stopCh = make(chan struct{})
		m.loopDone = make(chan struct{})
		go m.loop(m.stopCh, m.loopDone)
		m.mu.Unlock()
		return
	}
	stop := m.stopCh
	conn := m.conn
	m.stopCh = nil
	m.conn = nil
	m.connected = false
	m.inFlight = nil
	done := m.loopDone
	m.loopDone = nil
	m.mu.Unlock()

	if conn != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		raw, _ := encodeJSON(map[string]interface{}{"cmd": "CLOSE"}) // errcheck:ok static payload
		_ = writeFrameTimeout(ctx, conn, opFrame, raw)               // errcheck:ok best-effort close frame
		cancel()
		_ = conn.Close() // errcheck:ok teardown
	}
	if stop != nil {
		close(stop)
	}
	if done != nil {
		<-done
	}
}

func writeFrameTimeout(ctx context.Context, conn net.Conn, op int, payload []byte) error {
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl) // errcheck:ok deadline is advisory on pipes
	}
	done := make(chan error, 1)
	go func() { done <- writeFrame(conn, op, payload) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		_ = conn.SetDeadline(time.Time{}) // errcheck:ok resetting deadline
		return ctx.Err()
	}
}

func (m *Manager) loop(stop <-chan struct{}, done chan struct{}) {
	backoff := time.Second
	for {
		m.mu.Lock()
		enabled := m.enabled
		m.mu.Unlock()
		if !enabled {
			break
		}

		ctx, cancel := context.WithTimeout(context.Background(), m.dialTimeout)
		conn, err := m.dialer(ctx, socketCandidates())
		cancel()
		if err != nil {
			m.recordFailure("discord pipe unavailable")
			if !sleepOr(stop, backoff) {
				break
			}
			backoff *= 2
			if backoff > m.backoffMax {
				backoff = m.backoffMax
			}
			continue
		}
		if err := m.handshake(conn); err != nil {
			_ = conn.Close() // errcheck:ok closing after failed handshake
			m.recordFailure("handshake rejected: " + err.Error())
			if !sleepOr(stop, backoff) {
				break
			}
			continue
		}
		backoff = time.Second

		m.mu.Lock()
		old := m.conn
		m.conn = conn
		m.connected = true
		m.lastErr = ""
		m.lastOK = m.nowFn()
		pending := m.inFlight
		m.mu.Unlock()
		if old != nil && old != conn {
			_ = old.Close() // errcheck:ok replacing stale connection
		}
		if pending != nil {
			p := *pending
			_ = m.pushActivity(conn, &p) // errcheck:ok failure recorded by pushActivity
		}

		readerDone := make(chan struct{})
		go pumpReader(conn, readerDone)

		select {
		case <-readerDone:
		case <-stop:
		}

		m.mu.Lock()
		if m.conn == conn {
			m.conn = nil
			m.connected = false
		}
		m.mu.Unlock()
		_ = conn.Close() // errcheck:ok teardown on loop exit

		if stopped(stop) {
			break
		}
		// Discord restarted: retry fast once, then the normal ladder.
		if !sleepOr(stop, time.Second) {
			break
		}
	}
	m.mu.Lock()
	if m.loopDone == done {
		m.loopDone = nil
	}
	m.mu.Unlock()
	close(done)
}

func (m *Manager) handshake(conn net.Conn) error {
	m.mu.Lock()
	appID := m.appID
	m.mu.Unlock()
	if appID == "" {
		return errors.New("discord application id not configured")
	}
	raw, err := encodeJSON(map[string]interface{}{"v": 1, "client_id": appID})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := writeFrameTimeout(ctx, conn, opHandshake, raw); err != nil {
		return err
	}
	rd := make(chan error, 1)
	go func() {
		_, payload, err := readFrame(conn)
		if err != nil {
			rd <- err
			return
		}
		var msg struct {
			Evt string `json:"evt"`
		}
		_ = json.Unmarshal(payload, &msg) // errcheck:ok lenient parse; bare ack counts as READY
		if msg.Evt != "" && msg.Evt != "READY" {
			rd <- fmt.Errorf("unexpected event %q", msg.Evt)
			return
		}
		rd <- nil
	}()
	select {
	case err := <-rd:
		return err
	case <-time.After(3 * time.Second):
		return errors.New("handshake timeout")
	}
}

// pumpReader drains server frames (READY/ACTIVITY_JOIN requests, DISCONNECT,
// pings) so the pipe buffer never wedges; unsolicited commands are answered
// with an ack frame carrying the matching nonce when present.
func pumpReader(conn net.Conn, done chan struct{}) {
	defer close(done)
	for {
		op, payload, err := readFrame(conn)
		if err != nil {
			return
		}
		switch op {
		case opPing:
			_ = writeFrame(conn, opPong, payload) // errcheck:ok best-effort pong
		case opClose:
			return
		case opFrame:
			var msg struct {
				Nonce string `json:"nonce"`
				Evt   string `json:"evt"`
			}
			_ = json.Unmarshal(payload, &msg) // errcheck:ok drain loop must never fail on shape
			if msg.Evt == "DISCONNECT" {
				return
			}
			if msg.Nonce != "" {
				ack, _ := encodeJSON(map[string]interface{}{"cmd": "DISPATCH_ACK", "nonce": msg.Nonce}) // errcheck:ok static encode
				if ack != nil {
					_ = writeFrame(conn, opFrame, ack) // errcheck:ok best-effort ack
				}
			}
		}
	}
}

func (m *Manager) pushActivity(conn net.Conn, act *Activity) error {
	var activityJSON interface{}
	if act != nil && (act.Details != "" || act.State != "") {
		a := map[string]interface{}{}
		if act.Details != "" {
			a["details"] = act.Details
		}
		if act.State != "" {
			a["state"] = act.State
		}
		if act.StartUnix > 0 {
			a["timestamps"] = map[string]interface{}{"start": act.StartUnix}
		}
		activityJSON = a
	}
	raw, err := encodeJSON(map[string]interface{}{
		"cmd":  "SET_ACTIVITY",
		"args": map[string]interface{}{"pid": os.Getpid(), "activity": activityJSON},
	})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := writeFrameTimeout(ctx, conn, opFrame, raw); err != nil {
		m.recordFailure("send activity: " + err.Error())
		return err
	}
	return nil
}

// SetActivity pushes presence (nil or empty clears it). When a game session is
// active but Discord is not yet connected, the payload is remembered and
// re-sent right after the next successful handshake.
func (m *Manager) SetActivity(act *Activity) error {
	m.mu.Lock()
	m.inFlight = nil
	if act != nil && (act.Details != "" || act.State != "") {
		a := *act
		m.inFlight = &a
	}
	conn := m.conn
	enabled := m.enabled
	m.mu.Unlock()

	if !enabled {
		return nil // opt-in gate: nothing to do, nothing to report
	}
	if conn == nil {
		return ErrNotConnected
	}
	return m.pushActivity(conn, act)
}

// Status is what the settings UI renders.
type Status struct {
	Enabled     bool      `json:"enabled"`
	Connected   bool      `json:"connected"`
	AppIDSet    bool      `json:"app_id_set"`
	HasActivity bool      `json:"has_activity"`
	LastError   string    `json:"last_error,omitempty"`
	LastSuccess time.Time `json:"last_success,omitzero"`
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Status{
		Enabled:     m.enabled,
		Connected:   m.connected,
		AppIDSet:    m.appID != "",
		HasActivity: m.inFlight != nil,
		LastError:   m.lastErr,
		LastSuccess: m.lastOK,
	}
}

// SetAppID swaps the identity; the live connection is dropped so the loop
// re-handshakes with the new value.
func (m *Manager) SetAppID(id string) {
	m.mu.Lock()
	m.appID = id
	conn := m.conn
	m.mu.Unlock()
	if conn != nil {
		_ = conn.Close() // errcheck:ok forcing reconnect re-runs handshake
	}
}

func (m *Manager) recordFailure(msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastErr = msg
}

func stopped(stop <-chan struct{}) bool {
	select {
	case <-stop:
		return true
	default:
		return false
	}
}

func sleepOr(stop <-chan struct{}, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-stop:
		return false
	}
}
