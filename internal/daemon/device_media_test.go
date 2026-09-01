package daemon

import (
	"context"
	"io"
	"net"
	"testing"
	"time"
)

func TestReserveDeviceMediaClosesEarlierSocketsWhenVideoBindFails(t *testing.T) {
	manager := NewManager(testRuntime(t))
	telemetry := &trackingPacketConn{}
	control := &trackingListener{}
	manager.listenPacket = func(_ string, _ string) (net.PacketConn, error) {
		if telemetry.used {
			return nil, net.ErrClosed
		}
		telemetry.used = true
		return telemetry, nil
	}
	manager.listen = func(_ string, _ string) (net.Listener, error) { return control, nil }
	if _, err := manager.reserveDeviceMedia(context.Background(), "kart", "10.0.0.2", 0); err == nil {
		t.Fatal("expected video bind error")
	}
	if !telemetry.closed || !control.closed {
		t.Fatalf("telemetry closed=%v control closed=%v", telemetry.closed, control.closed)
	}
}

func TestServeLSPControlSendsExactInitialRecord(t *testing.T) {
	manager := NewManager(testRuntime(t))
	server, client := net.Pipe()
	listener := &singleConnectionListener{connection: server}
	media := &deviceMedia{ident: "kart", deviceAddress: "pipe", controlListener: listener}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { manager.serveLSPControl(ctx, media); close(done) }()
	record := make([]byte, initialLVNIRecordSize)
	if _, err := io.ReadFull(client, record); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	if string(record[:4]) != "LVNI" {
		t.Fatalf("record = %x", record)
	}
	for _, value := range record[4:] {
		if value != 0 {
			t.Fatalf("record = %x", record)
		}
	}
	cancel()
	client.Close()
	listener.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("control server did not stop")
	}
}

type trackingPacketConn struct{ used, closed bool }

func (c *trackingPacketConn) ReadFrom([]byte) (int, net.Addr, error) { return 0, nil, net.ErrClosed }
func (c *trackingPacketConn) WriteTo([]byte, net.Addr) (int, error)  { return 0, nil }
func (c *trackingPacketConn) Close() error                           { c.closed = true; return nil }
func (c *trackingPacketConn) LocalAddr() net.Addr                    { return &net.UDPAddr{} }
func (c *trackingPacketConn) SetDeadline(time.Time) error            { return nil }
func (c *trackingPacketConn) SetReadDeadline(time.Time) error        { return nil }
func (c *trackingPacketConn) SetWriteDeadline(time.Time) error       { return nil }

type trackingListener struct{ closed bool }

func (l *trackingListener) Accept() (net.Conn, error) { return nil, net.ErrClosed }
func (l *trackingListener) Close() error              { l.closed = true; return nil }
func (l *trackingListener) Addr() net.Addr            { return &net.TCPAddr{} }

type singleConnectionListener struct {
	connection net.Conn
	closed     bool
}

func (l *singleConnectionListener) Accept() (net.Conn, error) {
	if l.connection != nil {
		connection := l.connection
		l.connection = nil
		return connection, nil
	}
	return nil, net.ErrClosed
}
func (l *singleConnectionListener) Close() error {
	l.closed = true
	if l.connection != nil {
		return l.connection.Close()
	}
	return nil
}
func (l *singleConnectionListener) Addr() net.Addr { return &net.TCPAddr{} }
