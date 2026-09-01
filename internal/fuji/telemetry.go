package fuji

import (
	"encoding/binary"
	"fmt"
)

const (
	TelemetryStatus          = 1
	TelemetryIMU             = 2
	StatusPayloadSize        = 0x20
	statusCableConnectedMask = 0x01
)

type StatusTelemetry struct {
	BatteryBars    int
	CableConnected bool
}

// DecodeStatusTelemetry parses the type-1 payload after the leading type byte.
// Switchbrew documents payload byte 0 as a cable-status bitmask where bit 0
// means cable connected; byte 3 is the HUD battery level from 0 to 4 bars.
func DecodeStatusTelemetry(payload []byte) (StatusTelemetry, error) {
	if len(payload) < 4 {
		return StatusTelemetry{}, fmt.Errorf("status telemetry: need at least 4 bytes, got %#x", len(payload))
	}
	if payload[3] > 4 {
		return StatusTelemetry{}, fmt.Errorf("status telemetry: battery bars out of range: %d", payload[3])
	}
	return StatusTelemetry{
		BatteryBars:    int(payload[3]),
		CableConnected: payload[0]&statusCableConnectedMask != 0,
	}, nil
}

type Vec3 struct{ X, Y, Z float64 }
type Quat struct{ I, J, K, R float64 }

type IMUTelemetry struct {
	TimerUs     uint32
	Orientation Quat
	Accel       Vec3
	Gyro        Vec3
}

// DecodeIMUTelemetry parses the type-2 payload after the leading type byte.
func DecodeIMUTelemetry(payload []byte) (IMUTelemetry, error) {
	if len(payload) < 0x48 {
		return IMUTelemetry{}, fmt.Errorf("imu telemetry: need at least 0x48 bytes, got %#x", len(payload))
	}
	flags := payload[2]
	sampleCount := int(flags&7) + 1
	requiredSize := 0x48 + 0xc*sampleCount
	if len(payload) < requiredSize {
		return IMUTelemetry{}, fmt.Errorf("imu telemetry: need at least %#x bytes for %d samples, got %#x", requiredSize, sampleCount, len(payload))
	}
	out := IMUTelemetry{
		TimerUs: binary.LittleEndian.Uint32(payload[4:8]),
		Orientation: Quat{
			I: float64(int32(binary.LittleEndian.Uint32(payload[8:12]))) / (1 << 30),
			J: float64(int32(binary.LittleEndian.Uint32(payload[12:16]))) / (1 << 30),
			K: float64(int32(binary.LittleEndian.Uint32(payload[16:20]))) / (1 << 30),
			R: float64(int32(binary.LittleEndian.Uint32(payload[20:24]))) / (1 << 30),
		},
	}
	accelScale := 9.81 / 4096.0
	if flags&0x20 == 0 {
		accelScale = 15.99 * 9.81 / 4096.0
	}
	gyroScale := 0.004375
	if flags&0x40 == 0 {
		gyroScale = 0.1
	}
	offset := 0x48 + 0xc*(sampleCount-1)
	out.Accel = Vec3{
		X: float64(int16(binary.LittleEndian.Uint16(payload[offset:offset+2]))) * accelScale,
		Y: float64(int16(binary.LittleEndian.Uint16(payload[offset+2:offset+4]))) * accelScale,
		Z: float64(int16(binary.LittleEndian.Uint16(payload[offset+4:offset+6]))) * accelScale,
	}
	out.Gyro = Vec3{
		X: float64(int16(binary.LittleEndian.Uint16(payload[offset+6:offset+8]))) * gyroScale,
		Y: float64(int16(binary.LittleEndian.Uint16(payload[offset+8:offset+10]))) * gyroScale,
		Z: float64(int16(binary.LittleEndian.Uint16(payload[offset+10:offset+12]))) * gyroScale,
	}
	return out, nil
}
