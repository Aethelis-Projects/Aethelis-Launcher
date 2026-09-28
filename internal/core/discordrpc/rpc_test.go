package discordrpc

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func waitFor(t *testing.T, cond func() bool, within time.Duration, what string) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

// fakeDiscord speaks just enough of the Discord IPC protocol over AF_UNIX to
// exercise the manager: it acks every handshake with READY, records frames
// from SET_ACTIVITY/CLOSE, and can drop the live connection on demand to
// simulate a Discord restart.
type fakeDiscord struct {
	t        *testing.T
	ln       net.Listener
	framesCh chan string

	mu         sync.Mutex
	activeConn net.Conn
}

func newFakeDiscord(t *testing.T, runtimeDir, name string) *fakeDiscord {
	t.Helper()
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(runtimeDir, name)
	_ = os.Remove(path) // errcheck:ok stale socket cleanup, if any
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeDiscord{t: t, ln: ln, framesCh: make(chan string, 64)}
	go f.acceptLoop()
	t.Cleanup(func() { _ = ln.Close() }) // errcheck:ok test cleanup
	return f
}

func (f *fakeDiscord) acceptLoop() {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		f.mu.Lock()
		f.activeConn = conn
		f.mu.Unlock()
		go f.serve(conn)
	}
}

func (f *fakeDiscord) serve(conn net.Conn) {
	for {
		op, payload, err := readFrame(conn)
		if err != nil {
			return
		}
		switch op {
		case opHandshake:
			if strings.Contains(string(payload), BuiltinAppID) {
				select {
				case f.framesCh <- string(payload):
				default: // errcheck:ok drop if not observed
				}
			}
			ack, _ := encodeJSON(map[string]interface{}{"evt": "READY", "data": map[string]interface{}{"client_id": "cid"}}) // errcheck:ok static test payload
			_ = writeFrame(conn, opFrame, ack)                                                                                   // errcheck:ok test pipe
		case opFrame:
			select {
			case f.framesCh <- string(payload):
			default: // drop when the test stopped reading (post-teardown CLOSE etc.)
			}
			if strings.Contains(string(payload), `"cmd":"CLOSE"`) {
				_ = conn.Close() // errcheck:ok mirror discord behavior
				f.mu.Lock()
				if f.activeConn == conn {
					f.activeConn = nil
				}
				f.mu.Unlock()
				return
			}
		}
	}
}

func (f *fakeDiscord) dropLive() {
	f.mu.Lock()
	c := f.activeConn
	f.activeConn = nil
	f.mu.Unlock()
	if c != nil {
		_ = c.Close() // errcheck:ok simulate discord process exit
	}
}

func TestRPC_FullLifecycle_WithFakeDiscord(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	fake := newFakeDiscord(t, dir, "discord-ipc-0")

	m := NewManager("1234567890")
	if m.Status().Enabled {
		t.Fatal("must start disabled (opt-in default off)")
	}
	m.SetEnabled(true)
	waitFor(t, func() bool { return m.Status().Connected }, 3*time.Second, "handshake")

	if err := m.SetActivity(&Activity{Details: "Minecraft 1.20.1", State: "Vault", StartUnix: 1700000000}); err != nil {
		t.Fatalf("SetActivity: %v", err)
	}
	select {
	case raw := <-fake.framesCh:
		var msg struct {
			Cmd  string `json:"cmd"`
			Args struct {
				Activity map[string]interface{} `json:"activity"`
			} `json:"args"`
		}
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			t.Fatalf("frame not json: %v / %s", err, raw)
		}
		if msg.Cmd != "SET_ACTIVITY" {
			t.Fatalf("cmd=%s", msg.Cmd)
		}
		if msg.Args.Activity["details"] != "Minecraft 1.20.1" || msg.Args.Activity["state"] != "Vault" {
			t.Fatalf("activity payload wrong: %v", msg.Args.Activity)
		}
		ts, ok := msg.Args.Activity["timestamps"].(map[string]interface{})
		if !ok || ts["start"].(float64) != 1700000000 {
			t.Fatalf("timestamps wrong: %v", msg.Args.Activity)
		}
		// Payload policy (D'5): zero account identifiers anywhere in the frame.
		for _, banned := range []string{"token", "access", "account", "email", "uuid"} {
			if strings.Contains(strings.ToLower(raw), banned) {
				t.Fatalf("payload contains identifier-like key %q: %s", banned, raw)
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no activity frame received")
	}

	// Clear sends activity:null.
	if err := m.SetActivity(nil); err != nil {
		t.Fatal(err)
	}
	select {
	case raw := <-fake.framesCh:
		if !strings.Contains(raw, `"activity":null`) {
			t.Fatalf("clear frame wrong: %s", raw)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no clear frame")
	}

	// Disable: graceful CLOSE within the 5s acceptance bound.
	start := time.Now()
	m.SetEnabled(false)
	if m.Status().Connected || m.Status().Enabled {
		t.Fatalf("still enabled/connected after disable: %+v", m.Status())
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("disable took %v (>5s)", d)
	}
	select {
	case raw := <-fake.framesCh:
		if !strings.Contains(raw, "CLOSE") {
			t.Fatalf("expected CLOSE frame on disable, got %s", raw)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no CLOSE frame observed")
	}
}

func TestRPC_ReconnectAfterDiscordRestart(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	fake := newFakeDiscord(t, dir, "discord-ipc-0")

	m := NewManager("app-id")
	m.SetEnabled(true)
	waitFor(t, func() bool { return m.Status().Connected }, 3*time.Second, "first handshake")

	// Discord process dies mid-session: no crash, no spam, just offline.
	fake.dropLive()
	waitFor(t, func() bool { return !m.Status().Connected }, 2*time.Second, "disconnect detected")
	if err := m.SetActivity(&Activity{Details: "queued", State: "resync"}); err != ErrNotConnected {
		t.Fatalf("offline SetActivity must return ErrNotConnected (recorded), got %v", err)
	}

	// Discord "restarts" on the same pipe; the manager reconnects quickly and
	// re-syncs the remembered activity.
	waitFor(t, func() bool { return m.Status().Connected }, 6*time.Second, "reconnect on same socket")
	waitFor(t, func() bool {
		select {
		case raw := <-fake.framesCh:
			return strings.Contains(raw, "queued")
		default:
			return false
		}
	}, 3*time.Second, "resync frame")

	m.SetEnabled(false)
}

func TestRPC_NoDiscord_SilentDegradation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir) // no listener anywhere

	m := NewManager("app-id")
	m.SetEnabled(true)
	time.Sleep(120 * time.Millisecond)
	st := m.Status()
	if st.Connected {
		t.Fatal("must not claim connection")
	}
	if err := m.SetActivity(&Activity{Details: "x", State: "y"}); err != ErrNotConnected {
		t.Fatalf("must surface ErrNotConnected (recorded, non-fatal), got %v", err)
	}
	waitFor(t, func() bool { return strings.Contains(m.Status().LastError, "pipe unavailable") }, 3*time.Second, "recorded error")
	m.SetEnabled(false)
}

func TestRPC_EmptyAppID_FallsBackToBuiltinIdentity(t *testing.T) {
	// v0.7.2 G8: the shipped constant is the identity; users can no longer
	// (and never had to) configure it. An empty constructor value must not
	// block the handshake.
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	fake := newFakeDiscord(t, dir, "discord-ipc-0")

	m := NewManager("")
	m.SetEnabled(true)
	waitFor(t, func() bool { return m.Status().Connected }, 3*time.Second, "handshake with builtin id")

	observed := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !observed {
		select {
		case raw := <-fake.framesCh:
			if strings.Contains(raw, BuiltinAppID) {
				observed = true
			}
		default:
			time.Sleep(20 * time.Millisecond)
		}
	}
	if !observed {
		t.Fatal("handshake must carry the builtin application id")
	}
	m.SetEnabled(false)
}

func TestRPC_PreservedAcrossReconnect_NoPendingWithoutActivity(t *testing.T) {
	// Unit-level: SetActivity while disconnected stores inFlight so the next
	// handshake can resync; clearing drops it.
	m := NewManager("id")
	m.SetEnabled(true)
	defer m.SetEnabled(false)
	if err := m.SetActivity(&Activity{Details: "d", State: "s"}); err != ErrNotConnected {
		t.Fatalf("expected ErrNotConnected offline, got %v", err)
	}
	if !m.Status().HasActivity {
		t.Fatal("inFlight activity must be remembered for resync")
	}
	_ = m.SetActivity(nil) // errcheck:ok clearing stored state, offline
	if m.Status().HasActivity {
		t.Fatal("clear must drop the remembered activity")
	}
}

func TestRPC_FrameCodecGuards(t *testing.T) {
	buf := make([]byte, 0, 64)
	w := &sliceWriter{b: &buf}
	payload := []byte(`{"a":1}`)
	if err := writeFrame(w, opFrame, payload); err != nil {
		t.Fatal(err)
	}
	op, got, err := readFrame(newBytesReader(buf))
	if err != nil {
		t.Fatal(err)
	}
	if op != opFrame || string(got) != string(payload) {
		t.Fatalf("roundtrip wrong: %d %s", op, got)
	}
	// oversized length header must be rejected without allocating
	hdr := []byte{1, 0, 0, 0, 0xff, 0xff, 0xff, 0xff}
	if _, _, err := readFrame(newBytesReader(hdr)); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("oversized frame must error, got %v", err)
	}
}

type sliceWriter struct{ b *[]byte }

func (w *sliceWriter) Write(p []byte) (int, error) {
	*w.b = append(*w.b, p...)
	return len(p), nil
}

func newBytesReader(b []byte) *byteReader { return &byteReader{b: b} }

type byteReader struct {
	b []byte
	i int
}

func (r *byteReader) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, os.ErrClosed
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}
