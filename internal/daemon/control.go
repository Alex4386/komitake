package daemon

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/Alex4386/komitake/internal/deviceselect"
	"github.com/Alex4386/komitake/internal/fuji"
)

// controlConn is a live Fuji session held open for the lifetime of a connected
// kart. It wraps both post-handshake channels (control and event); the kart
// resets its network connection if either is dropped.
type controlConn interface {
	GetParam(ctx context.Context, name string) ([]byte, error)
	SetConnectionInfo(ctx context.Context, telemetryPort, lspControlPort, lspStreamPort int, unknown uint16, timestamp int64) error
	SetState(ctx context.Context, state byte) error
	Shutdown(ctx context.Context) error
	Close() error
}

// controlDialer opens a session to a kart. It is an interface so tests can
// inject a fake without a real kart.
type controlDialer interface {
	Dial(ctx context.Context, host string) (controlConn, error)
}

type fujiControlDialer struct{}

func (fujiControlDialer) Dial(ctx context.Context, host string) (controlConn, error) {
	s, err := fuji.Connect(ctx, host)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// deviceControl tracks a kart's control session and the cancel func that bounds
// its setup goroutine.
type driveSender interface {
	SetAxes(fuji.DriveAxes)
	Arm(bool)
	Armed() bool
	Healthy() (bool, error)
	Close() error
}

type deviceControl struct {
	conn   controlConn
	cancel context.CancelFunc
	drive  driveSender
	media  *deviceMedia

	mu         sync.Mutex
	lastAxes   fuji.DriveAxes
	controlSet bool
	driveMode  bool
}

// ResolveDevice picks a connected kart by ident or serial (adb-style).
func (m *Manager) ResolveDevice(selector string) (DeviceSummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.resolveDeviceLocked(selector)
}

func (m *Manager) resolveDeviceLocked(selector string) (DeviceSummary, error) {
	devs := make([]deviceselect.Device, 0, len(m.devices))
	for _, d := range m.devices {
		devs = append(devs, deviceselect.Device{
			Ident:      d.ident,
			Serial:     d.serial,
			Kind:       d.kind,
			Address:    d.address,
			MACAddress: d.mac,
		})
	}
	match, err := deviceselect.Resolve(selector, devs)
	if err != nil {
		return DeviceSummary{}, err
	}
	rec, ok := m.devices[match.Ident]
	if !ok {
		return DeviceSummary{}, fmt.Errorf("no device matches %q", selector)
	}
	return rec.summary(), nil
}

// PublishVideoFrame publishes a complete frame to current subscribers.
// It is primarily useful for protocol integrations and deterministic tests.
func (manager *Manager) PublishVideoFrame(frame VideoFrame) {
	manager.video.publish(frame)
}

// StreamVideo subscribes to complete Annex-B H.264 frames for one connected kart.
// A selector follows the same ident/serial resolution rules as drive commands.
type VideoStreamOptions struct {
	FreshKeyFrame bool
}

func (manager *Manager) StreamVideo(ctx context.Context, selector string) (<-chan VideoFrame, DeviceSummary, error) {
	return manager.StreamVideoWithOptions(ctx, selector, VideoStreamOptions{})
}

func (manager *Manager) StreamVideoWithOptions(ctx context.Context, selector string, options VideoStreamOptions) (<-chan VideoFrame, DeviceSummary, error) {
	manager.mu.RLock()
	device, err := manager.resolveDeviceLocked(selector)
	manager.mu.RUnlock()
	if err != nil {
		return nil, DeviceSummary{}, err
	}
	return manager.video.subscribe(ctx, device.Ident, options.FreshKeyFrame), device, nil
}

// GetDeviceParam reads a Fuji control parameter over the kart's live control
// session. It never opens a second control connection: the kart resets its
// network if a control channel is dropped, so reads reuse the held session.
func (m *Manager) GetDeviceParam(ctx context.Context, selector, name string) (DeviceSummary, []byte, error) {
	m.mu.RLock()
	device, err := m.resolveDeviceLocked(selector)
	if err != nil {
		m.mu.RUnlock()
		return DeviceSummary{}, nil, err
	}
	dc := m.controls[device.Ident]
	m.mu.RUnlock()

	if dc == nil || dc.conn == nil {
		return device, nil, fmt.Errorf("device %s has no control session", device.Ident)
	}
	value, err := dc.conn.GetParam(ctx, name)
	if err != nil {
		return device, nil, err
	}
	return device, value, nil
}

// GetProductCode reads product_code over the live control session and updates
// the cached serial on the device record.
func (m *Manager) GetProductCode(ctx context.Context, selector string) (DeviceSummary, fuji.ProductCode, error) {
	device, raw, err := m.GetDeviceParam(ctx, selector, "product_code")
	if err != nil {
		return device, fuji.ProductCode{}, err
	}
	pc, err := fuji.DecodeProductCode(raw)
	if err != nil {
		return device, fuji.ProductCode{}, err
	}

	m.mu.Lock()
	rec, ok := m.devices[device.Ident]
	if ok {
		rec.serial = pc.Serial
		m.devices[device.Ident] = rec
		device = rec.summary()
		m.signalLocked()
	}
	m.mu.Unlock()

	return device, pc, nil
}

// DriveState is the last command and whether its verified UDP packet was written.
type DriveState struct {
	DeviceID  string
	Steer     float64
	Throttle  float64
	Brake     float64
	Applied   bool
	Reason    string
	UpdatedAt time.Time
}

func (m *Manager) SetDriveMode(ctx context.Context, selector string, enabled bool) (DeviceSummary, error) {
	m.mu.RLock()
	device, err := m.resolveDeviceLocked(selector)
	if err != nil {
		m.mu.RUnlock()
		return DeviceSummary{}, err
	}
	control := m.controls[device.Ident]
	m.mu.RUnlock()
	if control == nil {
		return DeviceSummary{}, fmt.Errorf("device %s has no control session", device.Ident)
	}

	control.mu.Lock()
	if control.conn == nil || !control.controlSet {
		control.mu.Unlock()
		return DeviceSummary{}, fmt.Errorf("device %s control session is not ready", device.Ident)
	}
	if enabled && control.drive == nil {
		control.mu.Unlock()
		return DeviceSummary{}, fmt.Errorf("device %s has no drive sender", device.Ident)
	}
	if control.driveMode == enabled {
		control.mu.Unlock()
		return device, nil
	}

	neutralAxes := fuji.DriveAxes{}
	control.lastAxes = neutralAxes
	if control.drive != nil {
		control.drive.SetAxes(neutralAxes)
	}
	if !enabled {
		control.driveMode = false
		if control.drive != nil {
			control.drive.Arm(false)
		}
	}
	stateErr := control.conn.SetState(ctx, map[bool]byte{false: 0, true: 1}[enabled])
	if stateErr == nil && enabled {
		control.driveMode = true
		control.drive.Arm(true)
	}
	armed := control.driveMode && control.drive != nil && control.drive.Armed()
	control.mu.Unlock()

	m.mu.Lock()
	if record, ok := m.devices[device.Ident]; ok {
		record.armed = armed
		m.devices[device.Ident] = record
		device = record.summary()
		m.signalLocked()
	}
	m.mu.Unlock()
	if stateErr != nil {
		return device, fmt.Errorf("set device %s drive mode: %w", device.Ident, stateErr)
	}
	return device, nil
}

// ShutdownKart sends Fuji Control Shutdown (0x09) and closes the kart's RCD
// channels so it powers off instead of network-resetting, per switchbrew.
func (m *Manager) ShutdownKart(ctx context.Context, selector string) (DeviceSummary, error) {
	m.mu.RLock()
	device, err := m.resolveDeviceLocked(selector)
	if err != nil {
		m.mu.RUnlock()
		return DeviceSummary{}, err
	}
	control := m.controls[device.Ident]
	m.mu.RUnlock()
	if control == nil {
		return DeviceSummary{}, fmt.Errorf("device %s has no control session", device.Ident)
	}

	control.mu.Lock()
	conn := control.conn
	ready := conn != nil && control.controlSet
	control.mu.Unlock()
	if !ready {
		return DeviceSummary{}, fmt.Errorf("device %s control session is not ready", device.Ident)
	}

	if shutdownErr := conn.Shutdown(ctx); shutdownErr != nil {
		return device, fmt.Errorf("shutdown device %s: %w", device.Ident, shutdownErr)
	}

	m.mu.Lock()
	summary := device
	if record, ok := m.devices[device.Ident]; ok {
		summary = record.summary()
		delete(m.devices, device.Ident)
	}
	m.teardownControlLocked(device.Ident, false)
	m.signalLocked()
	m.mu.Unlock()
	m.events.DeviceDisconnected(summary)
	return summary, nil
}

func (m *Manager) SetDrive(selector string, steer, throttle, brake float64) (DriveState, error) {
	m.mu.RLock()
	device, err := m.resolveDeviceLocked(selector)
	if err != nil {
		m.mu.RUnlock()
		return DriveState{}, err
	}
	control := m.controls[device.Ident]
	m.mu.RUnlock()
	if control == nil {
		return DriveState{}, fmt.Errorf("device %s has no control session", device.Ident)
	}

	steer = math.Max(-1, math.Min(1, steer))
	throttle = math.Max(-1, math.Min(1, throttle))
	brake = math.Max(0, math.Min(1, brake))
	axes := fuji.DriveAxes{Steer: steer, Throttle: throttle, Brake: brake}
	control.mu.Lock()
	control.lastAxes = axes
	sender := control.drive
	driveMode := control.driveMode
	if sender != nil {
		sender.SetAxes(axes)
	}
	control.mu.Unlock()
	applied, reason := driveDeliveryState(driveMode, sender)
	return DriveState{
		DeviceID:  device.Ident,
		Steer:     steer,
		Throttle:  throttle,
		Brake:     brake,
		Applied:   applied,
		Reason:    reason,
		UpdatedAt: time.Now().UTC(),
	}, nil
}

func (m *Manager) GetDrive(selector string) (DriveState, error) {
	m.mu.RLock()
	device, err := m.resolveDeviceLocked(selector)
	if err != nil {
		m.mu.RUnlock()
		return DriveState{}, err
	}
	control := m.controls[device.Ident]
	m.mu.RUnlock()
	if control == nil {
		return DriveState{DeviceID: device.Ident}, nil
	}
	control.mu.Lock()
	axes := control.lastAxes
	driveMode := control.driveMode
	sender := control.drive
	control.mu.Unlock()
	applied, reason := driveDeliveryState(driveMode, sender)
	return DriveState{
		DeviceID: device.Ident,
		Steer:    axes.Steer,
		Throttle: axes.Throttle,
		Brake:    axes.Brake,
		Applied:  applied,
		Reason:   reason,
	}, nil
}

func driveDeliveryState(driveMode bool, sender driveSender) (bool, string) {
	if !driveMode {
		return false, "Fuji drive state is not active"
	}
	if sender == nil || !sender.Armed() {
		return false, "drive sender is not armed"
	}
	healthy, err := sender.Healthy()
	if err != nil {
		return false, fmt.Sprintf("drive UDP write failed: %v", err)
	}
	if !healthy {
		return false, "waiting for first drive UDP write"
	}
	return true, "UDP packet sent; kart acknowledgement unavailable"
}

func (control *deviceControl) detachMedia() *deviceMedia {
	control.mu.Lock()
	defer control.mu.Unlock()
	media := control.media
	control.media = nil
	return media
}
