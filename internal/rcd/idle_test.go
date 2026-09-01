package rcd

import (
	"context"
	"net"
	"testing"
	"time"
)

// switchbrew documents that the handshake channel is left open after a
// successful handshake, and that the kart resets its whole network connection
// if any RCD connection is lost. An idle timeout on a completed handshake would
// therefore knock a healthy kart offline, so the deadline must only bound the
// handshake itself.
//
// https://switchbrew.org/wiki/Mario_Kart_Live:_Home_Circuit
func TestServeKeepsCompletedHandshakeChannelOpen(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	svc := &completedService{id: 0x0001}
	srv := NewServer(server, svc)
	srv.IdleTimeout = 150 * time.Millisecond

	done := make(chan error, 1)
	go func() { done <- srv.Serve(context.Background()) }()

	// Drive one exchange so the service reports the handshake as done.
	conn := NewConn(client)
	if err := conn.WriteMessage(Message{Service: 0x0001, Command: 1}); err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}
	if _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}

	// Now go quiet for longer than the idle timeout. The channel must stay up.
	select {
	case err := <-done:
		t.Fatalf("Serve closed an established channel after idling: %v", err)
	case <-time.After(600 * time.Millisecond):
	}

	// And it must still be usable.
	if err := conn.WriteMessage(Message{Service: 0x0001, Command: 1}); err != nil {
		t.Fatalf("channel unusable after idling: %v", err)
	}
	if _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("ReadMessage() after idling error = %v", err)
	}
}

// An incomplete handshake must still be reaped, otherwise a silent peer pins a
// goroutine and a descriptor forever.
func TestServeTimesOutIncompleteHandshake(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	srv := NewServer(server, &completedService{id: 0x0001, established: false})
	srv.IdleTimeout = 150 * time.Millisecond

	done := make(chan error, 1)
	go func() { done <- srv.Serve(context.Background()) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected a timeout error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not reap an idle, incomplete handshake")
	}
}

// completedService reports an established session once it has handled a
// message, mirroring HandshakeService after command 4.
type completedService struct {
	id          uint16
	established bool
	handled     bool
}

func (c *completedService) ServiceID() uint16 { return c.id }

func (c *completedService) Handle(ctx context.Context, conn *Conn, msg Message) ([]byte, error) {
	c.handled = true
	return nil, nil
}

func (c *completedService) Close() error { return nil }

// Established implements the optional interface Serve uses to decide whether an
// idle deadline still applies.
func (c *completedService) Established() bool {
	return c.established || c.handled
}
