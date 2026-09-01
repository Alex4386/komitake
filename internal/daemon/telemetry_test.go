package daemon

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/Alex4386/komitake/internal/fuji"
)

func TestServeTelemetryUpdatesBatteryAndIMU(t *testing.T) {
	t.Parallel()
	manager := newTestManager(t)
	manager.devices["aabb"] = deviceRecord{kind: "Fuji", ident: "aabb", address: "10.0.0.2"}
	sender := &fakeDriveSender{armed: true}
	manager.controls["aabb"] = &deviceControl{drive: sender, driveMode: true}
	status := make([]byte, 1+0x20)
	status[0] = fuji.TelemetryStatus
	status[1] = 1
	status[1+3] = 3
	manager.applyTelemetryPacket("aabb", status)
	imu := make([]byte, 1+0x48+0xc)
	imu[0] = fuji.TelemetryIMU
	imu[1+2] = 0x60
	binary.LittleEndian.PutUint32(imu[1+4:1+8], 456789)
	binary.LittleEndian.PutUint32(imu[1+0x14:1+0x18], 1<<30)
	binary.LittleEndian.PutUint16(imu[1+0x48:1+0x4a], 4096)
	binary.LittleEndian.PutUint16(imu[1+0x4e:1+0x50], 100)
	manager.applyTelemetryPacket("aabb", imu)

	deadline := time.Now().Add(time.Second)
	for {
		devices := manager.ListDevices()
		if len(devices) == 1 && devices[0].Battery != nil && devices[0].CableConnected != nil && devices[0].IMU != nil {
			if *devices[0].Battery != 3 || !*devices[0].CableConnected || !devices[0].DriveArmed {
				t.Fatalf("device = %+v", devices[0])
			}
			if devices[0].IMU.TimerUs != 456789 || devices[0].IMU.Orientation.R != 1 {
				t.Fatalf("IMU = %+v", devices[0].IMU)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("telemetry not applied")
		}
		time.Sleep(time.Millisecond)
	}
}
