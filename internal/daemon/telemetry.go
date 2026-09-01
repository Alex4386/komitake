package daemon

import "github.com/Alex4386/komitake/internal/fuji"

func (manager *Manager) applyTelemetryPacket(deviceIdent string, datagram []byte) {
	if len(datagram) < 2 {
		return
	}
	payload := datagram[1:]
	switch datagram[0] {
	case fuji.TelemetryStatus:
		status, err := fuji.DecodeStatusTelemetry(payload)
		if err != nil {
			return
		}
		manager.mu.Lock()
		if record, exists := manager.devices[deviceIdent]; exists {
			batteryBars := status.BatteryBars
			cableConnected := status.CableConnected
			record.battery = &batteryBars
			record.cableConnected = &cableConnected
			if control := manager.controls[deviceIdent]; control != nil {
				control.mu.Lock()
				record.armed = control.driveMode && control.drive != nil && control.drive.Armed()
				control.mu.Unlock()
			}
			manager.devices[deviceIdent] = record
			manager.signalLocked()
		}
		manager.mu.Unlock()
	case fuji.TelemetryIMU:
		imu, err := fuji.DecodeIMUTelemetry(payload)
		if err != nil {
			return
		}
		manager.mu.Lock()
		if record, exists := manager.devices[deviceIdent]; exists {
			record.imu = &imu
			manager.devices[deviceIdent] = record
		}
		manager.mu.Unlock()
	}
}
