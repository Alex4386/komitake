package fuji

import (
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"sync"
	"time"
)

const (
	DrivePort        = 5102
	drivePacketSize  = 0x20
	driveHz          = 30
	reverseLightFlag = 0x01
	brakeLightFlag   = 0x04
)

type DriveAxes struct {
	Steer    float64
	Throttle float64
	// Brake controls the independent brake-light byte.
	Brake float64
}

// PackDrivePacket builds the physical Fuji UDP teleoperation frame.
func PackDrivePacket(axes DriveAxes, counter uint32) []byte {
	packet := make([]byte, drivePacketSize)
	packet[0] = byte(floatToS8(axes.Throttle))
	packet[1] = byte(floatToS8(axes.Steer))
	if axes.Throttle < 0 {
		packet[2] |= reverseLightFlag
	}
	if axes.Brake > 0 {
		packet[2] |= brakeLightFlag
	}
	binary.LittleEndian.PutUint32(packet[4:8], counter)
	return packet
}

func floatToS8(value float64) int8 {
	value = math.Max(-1, math.Min(1, value))
	if value < 0 {
		return int8(math.Round(value * 128))
	}
	return int8(math.Round(value * 127))
}

type driveConnection interface {
	Write([]byte) (int, error)
	Close() error
}

type DriveSender struct {
	connection   driveConnection
	writeMu      sync.Mutex
	mu           sync.Mutex
	axes         DriveAxes
	armed        bool
	counter      uint32
	lastWriteAt  time.Time
	lastWriteErr error
	stop         chan struct{}
	done         sync.WaitGroup
}

func NewDriveSender(host string) (*DriveSender, error) {
	destination, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, fmt.Sprintf("%d", DrivePort)))
	if err != nil {
		return nil, err
	}
	connection, err := net.DialUDP("udp", nil, destination)
	if err != nil {
		return nil, err
	}
	sender := &DriveSender{connection: connection, stop: make(chan struct{})}
	sender.done.Add(1)
	go sender.loop()
	return sender, nil
}

func (sender *DriveSender) loop() {
	defer sender.done.Done()
	ticker := time.NewTicker(time.Second / driveHz)
	defer ticker.Stop()
	for {
		select {
		case <-sender.stop:
			return
		case <-ticker.C:
			sender.writeLatest()
		}
	}
}

func (sender *DriveSender) writeLatest() {
	sender.writeMu.Lock()
	defer sender.writeMu.Unlock()
	sender.mu.Lock()
	if !sender.armed {
		sender.mu.Unlock()
		return
	}
	packet := PackDrivePacket(sender.axes, sender.counter)
	sender.counter++
	sender.mu.Unlock()
	writtenBytes, err := sender.connection.Write(packet)
	if err == nil && writtenBytes != len(packet) {
		err = fmt.Errorf("short drive write: wrote %d of %d bytes", writtenBytes, len(packet))
	}
	sender.mu.Lock()
	sender.lastWriteAt = time.Now()
	sender.lastWriteErr = err
	sender.mu.Unlock()
}

func (sender *DriveSender) SetAxes(axes DriveAxes) {
	sender.mu.Lock()
	sender.axes = axes
	armed := sender.armed
	sender.mu.Unlock()
	if armed {
		sender.writeLatest()
	}
}
func (sender *DriveSender) Arm(armed bool) {
	sender.mu.Lock()
	sender.armed = armed
	sender.mu.Unlock()
	if armed {
		sender.writeLatest()
	}
}
func (sender *DriveSender) Armed() bool {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	return sender.armed
}
func (sender *DriveSender) Healthy() (bool, error) {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	return sender.armed && !sender.lastWriteAt.IsZero() && sender.lastWriteErr == nil, sender.lastWriteErr
}
func (sender *DriveSender) Close() error {
	select {
	case <-sender.stop:
	default:
		close(sender.stop)
	}
	sender.done.Wait()
	return sender.connection.Close()
}
