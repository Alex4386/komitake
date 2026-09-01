package daemon

import (
	"context"
	"encoding/hex"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/Alex4386/komitake/internal/fuji"
	"github.com/Alex4386/komitake/internal/logging"
	"github.com/Alex4386/komitake/internal/rcd"
)

// maxHandshakeConns caps concurrent in-flight handshakes. The listener is
// reachable by every station on the AP, so an unbounded accept loop lets a peer
// exhaust goroutines and file descriptors by connecting and staying silent.
const maxHandshakeConns = 16

func (m *Manager) ListDevices() []DeviceSummary {
	m.mu.RLock()
	out := make([]DeviceSummary, 0, len(m.devices))
	for _, d := range m.devices {
		out = append(out, d.summary())
	}
	ap := m.ap
	m.mu.RUnlock()

	// Signal is read from the AP (hostapd), which does socket I/O, so query it
	// outside the lock and only when an AP is present.
	if ap != nil {
		for i := range out {
			if dbm, ok := ap.StationSignalDBM(out[i].MACAddress); ok {
				out[i].SignalDBM = &dbm
			}
		}
	}
	return out
}

// WaitForDevice blocks until a kart is connected. An empty ident matches any
// device. Matching is by RCD ident; serial is filled asynchronously after
// connect and is not used as the wait key.
func (m *Manager) WaitForDevice(ctx context.Context, ident string) (DeviceSummary, error) {
	for {
		m.mu.RLock()
		device, found := m.matchDeviceLocked(ident)
		ch := m.notify
		closed := m.closed
		m.mu.RUnlock()

		if found {
			return device, nil
		}
		if closed {
			return DeviceSummary{}, errors.New("daemon is shutting down")
		}

		select {
		case <-ctx.Done():
			return DeviceSummary{}, ctx.Err()
		case <-ch:
		}
	}
}

func (m *Manager) matchDeviceLocked(ident string) (DeviceSummary, bool) {
	for _, d := range m.devices {
		if ident == "" || d.ident == ident {
			return d.summary(), true
		}
	}
	return DeviceSummary{}, false
}

func (m *Manager) DeviceConnected(device *rcd.Device) {
	if device == nil {
		return
	}
	if device.Name != "Fuji" {
		m.logger.Debug("rejecting unsupported device", "name", device.Name, "address", device.Address)
		_ = device.Close()
		return
	}

	ident := hex.EncodeToString(device.Ident)
	m.mu.RLock()
	state := m.state
	m.mu.RUnlock()
	m.logger.Info("device connected", "state", state, "address", device.Address, "ident", ident)

	switch state {
	case StatePairing:
		go m.handlePairingDevice(device)
	case StateRunning:
		m.registerDevice(device)
	default:
		_ = device.Close()
	}
}

func (m *Manager) DeviceDisconnected(device *rcd.Device) {
	if device == nil {
		return
	}

	ident := hex.EncodeToString(device.Ident)
	m.logger.Info("device disconnected", "address", device.Address, "ident", ident)

	m.mu.Lock()
	_, existed := m.devices[ident]
	delete(m.devices, ident)
	m.closeControlLocked(ident)
	m.mu.Unlock()

	if existed {
		m.events.DeviceDisconnected(DeviceSummary{
			Kind:       "Fuji",
			Ident:      ident,
			Address:    device.Address,
			MACAddress: fuji.FormatMAC(device.Ident),
		})
	}

	m.mu.Lock()
	m.signalLocked()
	m.mu.Unlock()
}

// registerDevice records a kart whose handshake completed in normal mode. The
// handshake connection is held open by the RCD server goroutine in acceptLoop;
// here we track the connectivity record and open a persistent control session
// that completes setup and reads the product_code serial.
func (m *Manager) registerDevice(device *rcd.Device) {
	ident := hex.EncodeToString(device.Ident)
	record := deviceRecord{
		kind:    "Fuji",
		ident:   ident,
		address: device.Address,
		mac:     fuji.FormatMAC(device.Ident),
	}

	m.mu.Lock()
	m.devices[ident] = record
	m.logger.Info("device ready", "ident", ident, "address", device.Address)
	m.signalLocked()
	m.mu.Unlock()

	m.events.DeviceConnected(record.summary())
	m.startControlSession(ident, device.Address)
}

// startControlSession opens a Fuji session (control + event channels) to a
// freshly-registered kart and keeps it open. The kart resets its network
// connection unless both channels stay open and setup is completed; without this
// the kart cycles connect/disconnect. Reading product_code fills the serial.
func (m *Manager) startControlSession(ident, address string) {
	if address == "" {
		return
	}
	ctx, cancel := context.WithCancel(m.baseCtx)
	dc := &deviceControl{cancel: cancel}

	m.mu.Lock()
	if _, ok := m.devices[ident]; !ok {
		m.mu.Unlock()
		cancel()
		return
	}
	m.closeControlLocked(ident)
	m.controls[ident] = dc
	dialer := m.control
	m.mu.Unlock()

	m.backgroundWG.Add(1)
	go func() {
		defer m.backgroundWG.Done()

		conn, err := dialer.Dial(ctx, address)
		if err != nil {
			m.logger.Debug("control session dial failed", "ident", ident, "address", address, "error", err)
			m.mu.Lock()
			if m.controls[ident] == dc {
				delete(m.controls, ident)
			}
			m.mu.Unlock()
			cancel()
			return
		}

		// Read the serial first, then complete setup, mirroring the control
		// sequence openkartd uses on hardware.
		var serial string
		if raw, err := conn.GetParam(ctx, "product_code"); err != nil {
			m.logger.Debug("product_code unavailable", "ident", ident, "error", err)
		} else if pc, err := fuji.DecodeProductCode(raw); err != nil {
			m.logger.Debug("product_code decode failed", "ident", ident, "error", err)
		} else {
			serial = pc.Serial
		}

		m.mu.Lock()
		if m.controls[ident] != dc || ctx.Err() != nil {
			m.mu.Unlock()
			_ = conn.Close()
			return
		}
		dc.conn = conn
		m.mu.Unlock()

		connectionInfoSet := false
		driveMode := false
		media, mediaErr := m.openDeviceMedia(ctx, ident, address)
		if mediaErr != nil {
			m.logger.Warn("Fuji media setup failed", "ident", ident, "error", mediaErr)
		} else if ctx.Err() != nil {
			media.close()
			media = nil
		} else {
			dc.mu.Lock()
			if ctx.Err() == nil {
				dc.media = media
			} else {
				media.close()
				media = nil
			}
			dc.mu.Unlock()
		}
		if media == nil {
			m.logger.Warn("control setup skipped without media ports", "ident", ident)
		} else if err := conn.SetConnectionInfo(
			ctx,
			media.telemetryPort,
			media.lspControlPort,
			media.lspVideoPort,
			1,
			0,
		); err != nil {
			m.logger.Warn("control setup (connection_info) failed", "ident", ident, "error", err)
			if detachedMedia := dc.detachMedia(); detachedMedia != nil {
				detachedMedia.close()
			}
			media = nil
		} else {
			connectionInfoSet = true
			dc.mu.Lock()
			dc.controlSet = true
			dc.mu.Unlock()
			if err := conn.SetState(ctx, 1); err != nil {
				m.logger.Warn("SetState(drive) failed", "ident", ident, "error", err)
			} else {
				driveMode = true
				dc.mu.Lock()
				dc.driveMode = true
				dc.mu.Unlock()
			}
		}

		drive, driveErr := m.newDrive(address)
		if driveErr != nil {
			m.logger.Warn("drive sender failed", "ident", ident, "error", driveErr)
		} else if driveMode {
			drive.Arm(true)
		}

		m.mu.Lock()
		if m.controls[ident] != dc || ctx.Err() != nil {
			m.mu.Unlock()
			if drive != nil {
				_ = drive.Close()
			}
			if detachedMedia := dc.detachMedia(); detachedMedia != nil {
				detachedMedia.close()
			}
			_ = conn.Close()
			return
		}
		dc.mu.Lock()
		dc.drive = drive
		if drive != nil {
			drive.SetAxes(dc.lastAxes)
		}
		dc.mu.Unlock()
		var summary DeviceSummary
		if rec, ok := m.devices[ident]; ok {
			if serial != "" {
				rec.serial = serial
			}
			rec.armed = driveMode && drive != nil && drive.Armed()
			m.devices[ident] = rec
			summary = rec.summary()
		}
		m.signalLocked()
		m.mu.Unlock()

		m.logger.Info("control session established", "ident", ident, "serial", serial,
			"connection_info", connectionInfoSet, "drive_mode", driveMode, "drive_sender", drive != nil)
		m.events.DeviceConnected(summary)
	}()
}

// closeControlLocked tears down the control session for one kart. The caller
// must hold m.mu.
func (m *Manager) closeControlLocked(ident string) {
	m.teardownControlLocked(ident, true)
}

// teardownControlLocked closes drive, media, and the Fuji session. When
// sleepFirst is true, the kart is put to sleep before channels close (normal
// disconnect). When false, channels close immediately. That path is used after
// Shutdown so the kart powers off instead of resetting its network.
func (m *Manager) teardownControlLocked(ident string, sleepFirst bool) {
	dc, ok := m.controls[ident]
	if !ok {
		return
	}
	if dc.cancel != nil {
		dc.cancel()
	}
	dc.mu.Lock()
	if dc.drive != nil {
		dc.drive.Arm(false)
		_ = dc.drive.Close()
		dc.drive = nil
	}
	media := dc.media
	dc.media = nil
	dc.mu.Unlock()
	if media != nil {
		media.close()
	}
	m.video.remove(ident)
	if dc.conn != nil {
		if sleepFirst {
			_ = dc.conn.SetState(context.Background(), 0)
		}
		_ = dc.conn.Close()
	}
	delete(m.controls, ident)
}

// closeAllControlsLocked tears down every control session. The caller must hold
// m.mu.
func (m *Manager) closeAllControlsLocked() {
	for ident := range m.controls {
		m.closeControlLocked(ident)
	}
}

func (m *Manager) handlePairingDevice(device *rcd.Device) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m.logger.Info("starting pairing flow", "address", device.Address, "ident", hex.EncodeToString(device.Ident))
	if err := m.connector.Pair(ctx, device, m.cfg.GroupInfo); err != nil {
		m.logger.Error("pairing failed", "address", device.Address, "ident", hex.EncodeToString(device.Ident), "error", err)
		_ = device.Close()
		return
	}
	m.logger.Info("pairing completed", "address", device.Address, "ident", hex.EncodeToString(device.Ident),
		"game_ssid", m.cfg.GroupInfo.SSID, "channel", m.cfg.GroupInfo.Channel,
		logging.Secret("psk", m.cfg.GroupInfo.PSK))
	m.events.PairingCompleted(device.Address, hex.EncodeToString(device.Ident))
	// Bring the game AP up BEFORE closing the pairing handshake. SetGroupInfo
	// already committed NVRAM and started a network reset; closing RCD forces
	// another reset. If that happens while the pairing BSS is still the only
	// AP on air, the kart's first join attempts miss the game SSID.
	if err := m.SetState(context.Background(), StateRunning); err != nil {
		m.logger.Error("failed returning to normal mode after pairing", "error", err,
			"ssid", m.cfg.GroupInfo.SSID, "channel", m.cfg.GroupInfo.Channel)
	}
	_ = device.Close()
}

func (m *Manager) acceptLoop(listener net.Listener, pairing bool) {
	defer m.backgroundWG.Done()
	mode := "normal"
	if pairing {
		mode = "pairing"
	}
	m.logger.Info("accept loop started", "mode", mode, "address", listener.Addr().String())

	slots := make(chan struct{}, maxHandshakeConns)
	var wg sync.WaitGroup
	defer wg.Wait()

	for {
		conn, err := listener.Accept()
		if err != nil {
			// Only a closed listener is terminal; transient errors (EMFILE) retry.
			if errors.Is(err, net.ErrClosed) || m.baseCtx.Err() != nil {
				m.logger.Info("accept loop stopped", "mode", mode, "address", listener.Addr().String())
				return
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				continue
			}
			m.logger.Warn("accept failed, retrying", "mode", mode, "error", err)
			select {
			case <-m.baseCtx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
			continue
		}
		m.logger.Debug("accepted connection", "mode", mode, "remote", conn.RemoteAddr().String())

		select {
		case slots <- struct{}{}:
		default:
			m.logger.Warn("handshake connection limit reached, dropping",
				"mode", mode, "remote", conn.RemoteAddr().String(), "limit", maxHandshakeConns)
			_ = conn.Close()
			continue
		}

		wg.Add(1)
		go func(conn net.Conn) {
			defer wg.Done()
			defer func() { <-slots }()

			handshake := rcd.NewHandshakeService(m.cfg.ServerInfo, m, pairing)
			server := rcd.NewServer(conn, handshake)
			// Propagate the component logger so protocol tracing inherits the
			// daemon's level and carries the connection mode.
			server.SetLogger(logging.New(m.logger.With("mode", mode)))

			// baseCtx, not Background: this must unwind on daemon shutdown.
			if err := server.Serve(m.baseCtx); err != nil {
				m.logger.Debug("handshake session ended", "mode", mode, "error", err)
			}
		}(conn)
	}
}

func (m *Manager) clearDevicesLocked() {
	m.closeAllControlsLocked()
	for key := range m.devices {
		delete(m.devices, key)
	}
}

func (m *Manager) signalLocked() {
	close(m.notify)
	m.notify = make(chan struct{})
}
