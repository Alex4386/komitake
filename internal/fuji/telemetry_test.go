package fuji

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestDecodeStatusTelemetryPhysicalPacket(t *testing.T) {
	t.Parallel()
	payload := make([]byte, 0x20)
	payload[0] = 0x05
	payload[3] = 3
	status, err := DecodeStatusTelemetry(payload)
	if err != nil {
		t.Fatal(err)
	}
	if status.BatteryBars != 3 || !status.CableConnected {
		t.Fatalf("status = %+v", status)
	}
}

func TestDecodeStatusTelemetryCableDisconnected(t *testing.T) {
	t.Parallel()
	payload := make([]byte, StatusPayloadSize)
	payload[0] = 0x06
	payload[3] = 4
	status, err := DecodeStatusTelemetry(payload)
	if err != nil {
		t.Fatal(err)
	}
	if status.CableConnected {
		t.Fatalf("status = %+v", status)
	}
}

func TestDecodeStatusTelemetryRejectsInvalidSizeAndBars(t *testing.T) {
	t.Parallel()
	if _, err := DecodeStatusTelemetry(make([]byte, 3)); err == nil {
		t.Fatal("expected size error")
	}
	payload := make([]byte, 0x20)
	payload[3] = 5
	if _, err := DecodeStatusTelemetry(payload); err == nil {
		t.Fatal("expected battery range error")
	}
}

func TestDecodeIMUTelemetryPhysicalPacket(t *testing.T) {
	t.Parallel()
	payload := make([]byte, 0x48+0xc)
	binary.LittleEndian.PutUint16(payload[0:2], 42)
	payload[2] = 0x60
	binary.LittleEndian.PutUint32(payload[4:8], 123456)
	binary.LittleEndian.PutUint32(payload[20:24], 1<<30)
	binary.LittleEndian.PutUint16(payload[0x48:0x4a], 4096)
	binary.LittleEndian.PutUint16(payload[0x4e:0x50], 100)
	imu, err := DecodeIMUTelemetry(payload)
	if err != nil {
		t.Fatal(err)
	}
	if imu.TimerUs != 123456 || imu.Orientation.R != 1 {
		t.Fatalf("imu = %+v", imu)
	}
	if math.Abs(imu.Accel.X-9.81) > 1e-9 || math.Abs(imu.Gyro.X-0.4375) > 1e-9 {
		t.Fatalf("imu scales = %+v", imu)
	}
}

func TestDecodeIMUTelemetryRejectsWrongSampleLength(t *testing.T) {
	t.Parallel()
	payload := make([]byte, 0x48+0xc)
	payload[2] = 1
	if _, err := DecodeIMUTelemetry(payload); err == nil {
		t.Fatal("expected sample length error")
	}
}
