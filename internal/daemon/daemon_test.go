package daemon

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Alex4386/komitake/internal/config"
	"github.com/Alex4386/komitake/internal/rcd"
	"github.com/Alex4386/komitake/internal/wireless"
)

func testRuntime(t *testing.T) config.Runtime {
	t.Helper()
	return config.Runtime{
		Address:     "127.0.0.1",
		PairingFile: filepath.Join(t.TempDir(), "pairing.json"),
		GroupInfo: wireless.GroupInfo{
			SSID:    "Gtest",
			PSK:     make([]byte, 32),
			Channel: 1,
		},
		HasGroupInfo: true,
		ServerInfo: rcd.ServerInfo{
			Name:      "Komitake",
			Ident:     make([]byte, 16),
			MasterKey: make([]byte, 64),
			Versions:  []uint8{2, 1},
		},
	}
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	m := NewManager(testRuntime(t))
	m.listen = func(network, address string) (net.Listener, error) {
		return noopListener{}, nil
	}
	m.listenPacket = func(network, address string) (net.PacketConn, error) {
		return net.ListenPacket("udp", "127.0.0.1:0")
	}
	m.newTranscoder = func(context.Context, string, *videoHub, *slog.Logger) (videoEncoder, error) {
		return nopVideoEncoder{}, nil
	}
	return m
}

type noopListener struct{}

func (noopListener) Accept() (net.Conn, error) { return nil, net.ErrClosed }
func (noopListener) Close() error              { return nil }
func (noopListener) Addr() net.Addr            { return &net.TCPAddr{IP: net.IPv4zero, Port: 0} }

func TestSetStatePairingPersistsPairingFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rt := testRuntime(t)
	rt.PairingFile = filepath.Join(dir, "pairing.json")
	manager := NewManager(rt)
	manager.listen = func(network string, address string) (net.Listener, error) {
		return noopListener{}, nil
	}
	defer manager.Close()

	if err := manager.SetState(context.Background(), StatePairing); err != nil {
		t.Fatalf("SetState(PAIRING) error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "pairing.json"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var record PairingRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if record.State != StatePairing {
		t.Fatalf("state = %q", record.State)
	}
	if record.SSID == "" || record.SeedHex == "" {
		t.Fatalf("expected pairing ssid and seed, got %+v", record)
	}
	if record.SSID[0] != 'P' {
		t.Fatalf("pairing ssid = %q, want prefix P", record.SSID)
	}
}

func TestWaitForDeviceReturnsExistingDevice(t *testing.T) {
	t.Parallel()

	manager := NewManager(config.Runtime{})
	manager.devices["abc"] = deviceRecord{
		kind:    "Fuji",
		ident:   "abc",
		address: "192.168.0.2",
		mac:     "00:11:22:33:44:55",
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	device, err := manager.WaitForDevice(ctx, "abc")
	if err != nil {
		t.Fatalf("WaitForDevice() error = %v", err)
	}
	if device.Ident != "abc" {
		t.Fatalf("ident = %q", device.Ident)
	}
}

func TestHandlePairingDeviceDoesNotLeavePairingOnFailure(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t)
	manager.connector = failingPairer{}
	defer manager.Close()

	if err := manager.SetState(context.Background(), StatePairing); err != nil {
		t.Fatalf("SetState(PAIRING) error = %v", err)
	}
	manager.handlePairingDevice(&rcd.Device{})
	if got := manager.CurrentState(); got != StatePairing {
		t.Fatalf("state = %s, want %s", got, StatePairing)
	}
}

type failingPairer struct{}

func (failingPairer) Pair(context.Context, *rcd.Device, wireless.GroupInfo) error {
	return errors.New("pair failed")
}

func TestDeviceConnectedNilDeviceDoesNotPanic(t *testing.T) {
	t.Parallel()

	m := newTestManager(t)
	defer m.Close()

	m.DeviceConnected(nil)
	m.DeviceDisconnected(nil)
}

func TestDeviceConnectedRejectsUnsupportedKind(t *testing.T) {
	t.Parallel()

	m := newTestManager(t)
	defer m.Close()

	m.DeviceConnected(&rcd.Device{Name: "Unknown"})

	if len(m.ListDevices()) != 0 {
		t.Fatal("an unsupported device was registered")
	}
}

// SetState(DOWN) must clear tracked devices, so `komitake stop` stops reporting
// connected karts.
func TestSetStateDownClearsDevices(t *testing.T) {
	t.Parallel()

	m := newTestManager(t)
	defer m.Close()

	if err := m.SetState(context.Background(), StateRunning); err != nil {
		t.Fatalf("SetState(RUNNING) error = %v", err)
	}

	m.mu.Lock()
	m.devices["abc"] = deviceRecord{kind: "Fuji", ident: "abc"}
	m.mu.Unlock()

	if err := m.SetState(context.Background(), StateDown); err != nil {
		t.Fatalf("SetState(DOWN) error = %v", err)
	}

	if devices := m.ListDevices(); len(devices) != 0 {
		t.Fatalf("ListDevices() = %v, want empty after stop", devices)
	}
}

func TestWaitForDeviceObservesConcurrentSignal(t *testing.T) {
	t.Parallel()

	m := newTestManager(t)
	defer m.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := make(chan DeviceSummary, 1)
	go func() {
		device, err := m.WaitForDevice(ctx, "abc")
		if err != nil {
			return
		}
		result <- device
	}()

	time.Sleep(100 * time.Millisecond)
	m.mu.Lock()
	m.devices["abc"] = deviceRecord{kind: "Fuji", ident: "abc"}
	m.signalLocked()
	m.mu.Unlock()

	select {
	case device := <-result:
		if device.Ident != "abc" {
			t.Fatalf("ident = %q", device.Ident)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("WaitForDevice missed the signal")
	}
}

func TestCloseUnblocksWaitForDevice(t *testing.T) {
	t.Parallel()

	m := newTestManager(t)

	done := make(chan error, 1)
	go func() {
		_, err := m.WaitForDevice(context.Background(), "abc")
		done <- err
	}()

	time.Sleep(100 * time.Millisecond)
	if err := m.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error after Close")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not unblock WaitForDevice")
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	t.Parallel()

	m := newTestManager(t)
	for i := 0; i < 3; i++ {
		if err := m.Close(); err != nil {
			t.Fatalf("Close() attempt %d error = %v", i, err)
		}
	}
}

// The pairing file holds the seed, and the pairing PSK is SHA-256(seed), so it
// must not be readable by other users.
func TestPairingFileIsNotWorldReadable(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rt := testRuntime(t)
	rt.PairingFile = filepath.Join(dir, "nested", "pairing.json")

	m := NewManager(rt)
	m.listen = func(network, address string) (net.Listener, error) {
		return noopListener{}, nil
	}
	defer m.Close()

	if err := m.SetState(context.Background(), StatePairing); err != nil {
		t.Fatalf("SetState(PAIRING) error = %v", err)
	}

	info, err := os.Stat(rt.PairingFile)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("pairing file mode = %#o, want 0600", perm)
	}

	dirInfo, err := os.Stat(filepath.Dir(rt.PairingFile))
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm&0o077 != 0 {
		t.Fatalf("pairing directory mode = %#o, want no group or other access", perm)
	}
}

func TestPersistPairingLeavesNoTempFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	rt := testRuntime(t)
	rt.PairingFile = filepath.Join(dir, "pairing.json")

	m := NewManager(rt)
	m.listen = func(network, address string) (net.Listener, error) {
		return noopListener{}, nil
	}
	defer m.Close()

	if err := m.SetState(context.Background(), StatePairing); err != nil {
		t.Fatalf("SetState(PAIRING) error = %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, entry := range entries {
		if entry.Name() != "pairing.json" {
			t.Fatalf("stray file left behind: %s", entry.Name())
		}
	}
}

func TestSetStateRejectedAfterClose(t *testing.T) {
	t.Parallel()

	m := newTestManager(t)
	if err := m.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := m.SetState(context.Background(), StateRunning); err == nil {
		t.Fatal("expected SetState to fail after Close")
	}
}

// The observer must see every transition and device event exactly once.
func TestObserverReceivesEvents(t *testing.T) {
	t.Parallel()

	m := newTestManager(t)
	obs := &recordingObserver{}
	m.SetObserver(obs)
	defer m.Close()

	if err := m.SetState(context.Background(), StateRunning); err != nil {
		t.Fatalf("SetState(RUNNING) error = %v", err)
	}
	m.registerDevice(&rcd.Device{Name: "Fuji", Ident: []byte{1, 2, 3, 4, 5, 6}, Address: "10.0.0.2"})

	obs.mu.Lock()
	defer obs.mu.Unlock()
	if obs.transitions == 0 {
		t.Fatal("observer saw no transitions")
	}
	if obs.connected != 1 {
		t.Fatalf("connected events = %d, want 1", obs.connected)
	}
	wantIdent := hex.EncodeToString([]byte{1, 2, 3, 4, 5, 6})
	if obs.lastIdent != wantIdent {
		t.Fatalf("connected ident = %q, want %q", obs.lastIdent, wantIdent)
	}
}

type recordingObserver struct {
	mu          sync.Mutex
	transitions int
	connected   int
	lastIdent   string
}

func (o *recordingObserver) StateChanged(_, _ State) {
	o.mu.Lock()
	o.transitions++
	o.mu.Unlock()
}

func (o *recordingObserver) DeviceConnected(d DeviceSummary) {
	o.mu.Lock()
	o.connected++
	o.lastIdent = d.Ident
	o.mu.Unlock()
}

func (o *recordingObserver) DeviceDisconnected(DeviceSummary) {}
func (o *recordingObserver) PairingCompleted(string, string)  {}

type nopVideoEncoder struct{}

func (nopVideoEncoder) writeFrame([]byte) error { return nil }
func (nopVideoEncoder) close()                  {}
