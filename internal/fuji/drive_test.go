package fuji

import (
	"encoding/binary"
	"errors"
	"testing"
)

func TestPackDrivePacketPhysicalWireLayout(t *testing.T) {
	t.Parallel()
	packet := PackDrivePacket(DriveAxes{Throttle: -1, Steer: 1, Brake: 1}, 0x78563412)
	if len(packet) != 0x20 {
		t.Fatalf("packet length = %#x", len(packet))
	}
	if int8(packet[0]) != -128 || int8(packet[1]) != 127 || packet[2] != reverseLightFlag|brakeLightFlag || packet[3] != 0 {
		t.Fatalf("controls = % x", packet[:4])
	}
	if counter := binary.LittleEndian.Uint32(packet[4:8]); counter != 0x78563412 {
		t.Fatalf("counter = %#x", counter)
	}
	for offset, value := range packet[8:] {
		if value != 0 {
			t.Fatalf("padding byte %#x = %#x", offset+8, value)
		}
	}
}

func TestBrakeLightDoesNotSuppressThrottle(t *testing.T) {
	t.Parallel()
	packet := PackDrivePacket(DriveAxes{Throttle: 1, Brake: 1}, 0)
	if int8(packet[0]) != 127 || packet[2] != brakeLightFlag {
		t.Fatalf("packet controls = % x", packet[:4])
	}
}

func TestReverseThrottleSetsReverseFlag(t *testing.T) {
	t.Parallel()
	packet := PackDrivePacket(DriveAxes{Throttle: -0.5}, 0)
	if packet[2] != reverseLightFlag {
		t.Fatalf("status flags = %#x", packet[2])
	}
}

func TestFloatToS8UsesFullSignedRange(t *testing.T) {
	t.Parallel()
	if floatToS8(1) != 127 || floatToS8(-1) != -128 || floatToS8(0) != 0 {
		t.Fatal("signed range conversion failed")
	}
}

type fakeDriveConnection struct {
	packets [][]byte
	written int
	err     error
	closed  bool
}

func (connection *fakeDriveConnection) Write(packet []byte) (int, error) {
	connection.packets = append(connection.packets, append([]byte(nil), packet...))
	if connection.err != nil {
		return 0, connection.err
	}
	if connection.written != 0 {
		return connection.written, nil
	}
	return len(packet), nil
}
func (connection *fakeDriveConnection) Close() error { connection.closed = true; return nil }

func TestDriveSenderArmWritesCounterZeroImmediately(t *testing.T) {
	t.Parallel()
	connection := &fakeDriveConnection{}
	sender := &DriveSender{connection: connection, stop: make(chan struct{})}
	sender.Arm(true)
	if len(connection.packets) != 1 {
		t.Fatalf("writes = %d", len(connection.packets))
	}
	if counter := binary.LittleEndian.Uint32(connection.packets[0][4:8]); counter != 0 {
		t.Fatalf("counter = %d", counter)
	}
	healthy, err := sender.Healthy()
	if !healthy || err != nil {
		t.Fatalf("healthy=%v error=%v", healthy, err)
	}
	close(sender.stop)
}

func TestDriveSenderReportsWriteFailure(t *testing.T) {
	t.Parallel()
	connection := &fakeDriveConnection{err: errors.New("write failed")}
	sender := &DriveSender{connection: connection, stop: make(chan struct{})}
	sender.Arm(true)
	healthy, err := sender.Healthy()
	if healthy || err == nil {
		t.Fatalf("healthy=%v error=%v", healthy, err)
	}
	close(sender.stop)
}
