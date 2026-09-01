package daemon

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Alex4386/komitake/internal/fuji"
	"github.com/Alex4386/komitake/internal/rcd"
)

// fakeControlConn is an in-memory control session for tests.
type fakeControlConn struct {
	mu             sync.Mutex
	value          []byte
	err            error
	setInfoErr     error
	setInfoCalled  bool
	telemetryPort  int
	lspControlPort int
	lspStreamPort  int
	setStateCalls  []byte
	setStateErr    error
	shutdownCalled bool
	shutdownErr    error
	closed         bool
}

func (f *fakeControlConn) GetParam(_ context.Context, _ string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]byte(nil), f.value...), f.err
}

func (f *fakeControlConn) SetConnectionInfo(
	_ context.Context,
	telemetryPort int,
	lspControlPort int,
	lspStreamPort int,
	_ uint16,
	_ int64,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setInfoCalled = true
	f.telemetryPort = telemetryPort
	f.lspControlPort = lspControlPort
	f.lspStreamPort = lspStreamPort
	return f.setInfoErr
}

func (f *fakeControlConn) SetState(_ context.Context, state byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setStateCalls = append(f.setStateCalls, state)
	return f.setStateErr
}

func (f *fakeControlConn) Shutdown(_ context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.shutdownCalled = true
	return f.shutdownErr
}

func (f *fakeControlConn) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakeControlConn) sawSetInfo() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.setInfoCalled
}

// fakeControlDialer hands out a preconfigured connection.
type fakeControlDialer struct {
	conn *fakeControlConn
	err  error
}

func (d *fakeControlDialer) Dial(context.Context, string) (controlConn, error) {
	if d.err != nil {
		return nil, d.err
	}
	return d.conn, nil
}

func productCodeBytes(serial string) []byte {
	raw := make([]byte, 5+len(serial)+1)
	binary.LittleEndian.PutUint16(raw[0:2], 1)
	binary.LittleEndian.PutUint16(raw[2:4], 7)
	raw[4] = 0
	copy(raw[5:], serial)
	return raw
}

func TestGetProductCodeUpdatesSerial(t *testing.T) {
	t.Parallel()

	m := newTestManager(t)
	m.devices["aabb"] = deviceRecord{
		kind: "Fuji", ident: "aabb", address: "10.0.0.2", mac: "aa:bb:cc:dd:ee:ff",
	}
	m.controls["aabb"] = &deviceControl{conn: &fakeControlConn{value: productCodeBytes("XKW999")}}

	device, pc, err := m.GetProductCode(context.Background(), "aa")
	if err != nil {
		t.Fatalf("GetProductCode: %v", err)
	}
	if pc.Serial != "XKW999" || device.Serial != "XKW999" {
		t.Fatalf("serial device=%q pc=%q", device.Serial, pc.Serial)
	}
	if m.devices["aabb"].serial != "XKW999" {
		t.Fatalf("cache = %q", m.devices["aabb"].serial)
	}
}

func TestGetDeviceParamRequiresSession(t *testing.T) {
	t.Parallel()

	m := newTestManager(t)
	m.devices["aabb"] = deviceRecord{kind: "Fuji", ident: "aabb", address: "10.0.0.2"}

	if _, _, err := m.GetProductCode(context.Background(), "aa"); err == nil {
		t.Fatal("expected error when no control session is open")
	}
}

// The control session must complete setup (connection_info) and cache the
// serial, and it must stay open for the device's lifetime.
func TestStartControlSessionKeepsAliveAndReadsSerial(t *testing.T) {
	t.Parallel()

	m := newTestManager(t)
	conn := &fakeControlConn{value: productCodeBytes("XKW777")}
	m.control = &fakeControlDialer{conn: conn}
	drive := &fakeDriveSender{}
	m.newDrive = func(string) (driveSender, error) { return drive, nil }
	m.devices["aabb"] = deviceRecord{kind: "Fuji", ident: "aabb", address: "10.0.0.2"}

	m.startControlSession("aabb", "10.0.0.2")

	deadline := time.After(2 * time.Second)
	for {
		m.mu.RLock()
		rec := m.devices["aabb"]
		dc := m.controls["aabb"]
		m.mu.RUnlock()
		if rec.serial == "XKW777" && dc != nil && dc.conn != nil {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("session not established: serial=%q", rec.serial)
		case <-time.After(5 * time.Millisecond):
		}
	}

	if !conn.sawSetInfo() {
		t.Fatal("SetConnectionInfo was not called; kart would reset its connection")
	}
	conn.mu.Lock()
	ports := [3]int{conn.telemetryPort, conn.lspControlPort, conn.lspStreamPort}
	conn.mu.Unlock()
	if want := [3]int{
		fujiTelemetryBasePort,
		fujiLSPControlBasePort,
		fujiLSPVideoBasePort,
	}; ports != want {
		t.Fatalf("connection_info ports = %v, want %v", ports, want)
	}
	conn.mu.Lock()
	states := append([]byte(nil), conn.setStateCalls...)
	conn.mu.Unlock()
	if len(states) == 0 || states[0] != 1 {
		t.Fatalf("SetState calls = %v, want drive state first", states)
	}
	if !drive.Armed() {
		t.Fatal("drive sender was not armed")
	}

	// Disconnecting must close the held session.
	ident, _ := hex.DecodeString("aabb")
	m.DeviceDisconnected(&rcd.Device{Name: "Fuji", Ident: ident})

	m.mu.RLock()
	_, stillOpen := m.controls["aabb"]
	m.mu.RUnlock()
	if stillOpen {
		t.Fatal("control session still tracked after disconnect")
	}
	if !conn.closed {
		t.Fatal("control connection not closed on disconnect")
	}
	drive.mu.Lock()
	driveClosed := drive.closed
	drive.mu.Unlock()
	if !driveClosed {
		t.Fatal("drive sender not closed on disconnect")
	}
	conn.mu.Lock()
	states = append([]byte(nil), conn.setStateCalls...)
	conn.mu.Unlock()
	if states[len(states)-1] != 0 {
		t.Fatalf("SetState calls = %v, want sleep state on disconnect", states)
	}
}

type fakeDriveSender struct {
	mu      sync.Mutex
	axes    fuji.DriveAxes
	armed   bool
	closed  bool
	healthy bool
	err     error
}

func (sender *fakeDriveSender) SetAxes(axes fuji.DriveAxes) {
	sender.mu.Lock()
	sender.axes = axes
	if sender.armed && sender.err == nil {
		sender.healthy = true
	}
	sender.mu.Unlock()
}
func (sender *fakeDriveSender) Arm(armed bool) {
	sender.mu.Lock()
	sender.armed = armed
	if armed && sender.err == nil {
		sender.healthy = true
	}
	sender.mu.Unlock()
}
func (sender *fakeDriveSender) Armed() bool {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	return sender.armed
}
func (sender *fakeDriveSender) Healthy() (bool, error) {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	return sender.healthy, sender.err
}
func (sender *fakeDriveSender) Close() error {
	sender.mu.Lock()
	sender.closed = true
	sender.mu.Unlock()
	return nil
}

func TestSetDriveUpdatesArmedSender(t *testing.T) {
	t.Parallel()
	manager := newTestManager(t)
	sender := &fakeDriveSender{armed: true, healthy: true}
	manager.devices["aabb"] = deviceRecord{kind: "Fuji", ident: "aabb"}
	manager.controls["aabb"] = &deviceControl{drive: sender, driveMode: true}
	state, err := manager.SetDrive("aa", 0.5, -0.25, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Applied {
		t.Fatal("drive command not marked applied")
	}
	sender.mu.Lock()
	axes := sender.axes
	sender.mu.Unlock()
	if axes.Steer != 0.5 || axes.Throttle != -0.25 || axes.Brake != 1 {
		t.Fatalf("axes = %+v", axes)
	}
}

func TestGetDriveDoesNotApplyBeforeSuccessfulWrite(t *testing.T) {
	t.Parallel()
	manager := newTestManager(t)
	sender := &fakeDriveSender{armed: true}
	manager.devices["aabb"] = deviceRecord{kind: "Fuji", ident: "aabb"}
	manager.controls["aabb"] = &deviceControl{drive: sender, driveMode: true}
	state, err := manager.GetDrive("aa")
	if err != nil {
		t.Fatal(err)
	}
	if state.Applied || state.Reason != "waiting for first drive UDP write" {
		t.Fatalf("state = %+v", state)
	}
}

func TestControlSetupSkipsDriveModeWhenConnectionInfoFails(t *testing.T) {
	t.Parallel()
	manager := newTestManager(t)
	connection := &fakeControlConn{
		value:      productCodeBytes("XKW777"),
		setInfoErr: fmt.Errorf("PARAM_STATE"),
	}
	manager.control = &fakeControlDialer{conn: connection}
	drive := &fakeDriveSender{}
	manager.newDrive = func(string) (driveSender, error) { return drive, nil }
	manager.devices["aabb"] = deviceRecord{kind: "Fuji", ident: "aabb", address: "10.0.0.2"}
	manager.startControlSession("aabb", "10.0.0.2")

	deadline := time.After(time.Second)
	for {
		connection.mu.Lock()
		called := connection.setInfoCalled
		states := append([]byte(nil), connection.setStateCalls...)
		connection.mu.Unlock()
		if called {
			if len(states) != 0 {
				t.Fatalf("SetState called without connection_info: %v", states)
			}
			return
		}
		select {
		case <-deadline:
			t.Fatal("connection_info not attempted")
		case <-time.After(time.Millisecond):
		}
	}
}

func TestSetDriveModeEnablesWithNeutralAxes(t *testing.T) {
	t.Parallel()
	manager := newTestManager(t)
	connection := &fakeControlConn{}
	sender := &fakeDriveSender{}
	manager.devices["aabb"] = deviceRecord{kind: "Fuji", ident: "aabb"}
	manager.controls["aabb"] = &deviceControl{
		conn: connection, drive: sender, controlSet: true,
		lastAxes: fuji.DriveAxes{Steer: 1, Throttle: 1, Brake: 1},
	}

	device, err := manager.SetDriveMode(context.Background(), "aa", true)
	if err != nil {
		t.Fatal(err)
	}
	if !device.DriveArmed || !sender.Armed() {
		t.Fatalf("device = %+v armed=%v", device, sender.Armed())
	}
	sender.mu.Lock()
	axes := sender.axes
	sender.mu.Unlock()
	if axes != (fuji.DriveAxes{}) {
		t.Fatalf("axes = %+v, want neutral", axes)
	}
	connection.mu.Lock()
	states := append([]byte(nil), connection.setStateCalls...)
	connection.mu.Unlock()
	if len(states) != 1 || states[0] != 1 {
		t.Fatalf("SetState calls = %v", states)
	}
}

func TestSetDriveModeDisablesAndCommandsDoNotRearm(t *testing.T) {
	t.Parallel()
	manager := newTestManager(t)
	connection := &fakeControlConn{}
	sender := &fakeDriveSender{armed: true, healthy: true}
	manager.devices["aabb"] = deviceRecord{kind: "Fuji", ident: "aabb", armed: true}
	manager.controls["aabb"] = &deviceControl{
		conn: connection, drive: sender, controlSet: true, driveMode: true,
		lastAxes: fuji.DriveAxes{Steer: 1, Throttle: 1, Brake: 1},
	}

	device, err := manager.SetDriveMode(context.Background(), "aa", false)
	if err != nil {
		t.Fatal(err)
	}
	if device.DriveArmed || sender.Armed() {
		t.Fatalf("device = %+v armed=%v", device, sender.Armed())
	}
	sender.mu.Lock()
	axes := sender.axes
	sender.mu.Unlock()
	if axes != (fuji.DriveAxes{}) {
		t.Fatalf("axes = %+v, want neutral", axes)
	}

	state, err := manager.SetDrive("aa", 1, 0.5, 0)
	if err != nil {
		t.Fatal(err)
	}
	if state.Applied || sender.Armed() {
		t.Fatalf("state = %+v armed=%v", state, sender.Armed())
	}
}

func TestSetDriveModeFailedDisableRemainsDisarmed(t *testing.T) {
	t.Parallel()
	manager := newTestManager(t)
	connection := &fakeControlConn{setStateErr: fmt.Errorf("PARAM_STATE")}
	sender := &fakeDriveSender{armed: true, healthy: true}
	manager.devices["aabb"] = deviceRecord{kind: "Fuji", ident: "aabb", armed: true}
	manager.controls["aabb"] = &deviceControl{
		conn: connection, drive: sender, controlSet: true, driveMode: true,
	}

	device, err := manager.SetDriveMode(context.Background(), "aa", false)
	if err == nil {
		t.Fatal("expected SetState failure")
	}
	if device.DriveArmed || sender.Armed() {
		t.Fatalf("device = %+v armed=%v", device, sender.Armed())
	}
}

func TestShutdownKartPowersOffAndRemovesDevice(t *testing.T) {
	t.Parallel()
	manager := newTestManager(t)
	connection := &fakeControlConn{}
	sender := &fakeDriveSender{armed: true, healthy: true}
	manager.devices["aabb"] = deviceRecord{kind: "Fuji", ident: "aabb", armed: true}
	manager.controls["aabb"] = &deviceControl{
		conn: connection, drive: sender, controlSet: true, driveMode: true,
	}

	device, err := manager.ShutdownKart(context.Background(), "aa")
	if err != nil {
		t.Fatal(err)
	}
	if device.Ident != "aabb" {
		t.Fatalf("device = %+v", device)
	}
	connection.mu.Lock()
	shutdownCalled := connection.shutdownCalled
	closed := connection.closed
	setStateCalls := append([]byte(nil), connection.setStateCalls...)
	connection.mu.Unlock()
	if !shutdownCalled || !closed {
		t.Fatalf("shutdownCalled=%v closed=%v", shutdownCalled, closed)
	}
	if len(setStateCalls) != 0 {
		t.Fatalf("SetState calls = %v, want none after Shutdown", setStateCalls)
	}
	if _, ok := manager.devices["aabb"]; ok {
		t.Fatal("device remained tracked after shutdown")
	}
	if _, ok := manager.controls["aabb"]; ok {
		t.Fatal("control session remained after shutdown")
	}
	if sender.Armed() {
		t.Fatal("drive sender remained armed")
	}
}
