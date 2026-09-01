package rcd

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

// echoService returns the request payload so round trips can be verified.
type echoService struct {
	id     uint16
	err    error
	closed bool
}

func (e *echoService) ServiceID() uint16 { return e.id }

func (e *echoService) Handle(ctx context.Context, conn *Conn, msg Message) ([]byte, error) {
	if e.err != nil {
		return nil, e.err
	}
	return msg.Data, nil
}

func (e *echoService) Close() error {
	e.closed = true
	return nil
}

func TestConnReadMessageRoundTrip(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	want := Message{Service: 0x100, Command: 7, Data: []byte("payload")}
	go func() {
		_ = NewConn(client).WriteMessage(want)
	}()

	got, err := NewConn(server).ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	if got.Service != want.Service || got.Command != want.Command || string(got.Data) != string(want.Data) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// The size guard must reject an oversized length before allocating.
func TestConnReadMessageRejectsOversizedLength(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		buf := make([]byte, headerSize)
		binary.BigEndian.PutUint32(buf[4:8], 0xFFFFFFF0)
		_, _ = client.Write(buf)
	}()

	if _, err := NewConn(server).ReadMessage(); !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("error = %v, want ErrPayloadTooLarge", err)
	}
}

func TestConnReadMessageTruncatedHeader(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()

	go func() {
		_, _ = client.Write(make([]byte, 4))
		_ = client.Close()
	}()

	if _, err := NewConn(server).ReadMessage(); err == nil {
		t.Fatal("expected an error for a truncated header")
	}
}

func TestServeDispatchesAndClosesServices(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	svc := &echoService{id: 0x100}
	srv := NewServer(server, svc)

	done := make(chan error, 1)
	go func() { done <- srv.Serve(context.Background()) }()

	conn := NewConn(client)
	if err := conn.WriteMessage(Message{Service: 0x100, Command: 1, Data: []byte("hi")}); err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}
	resp, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	if !resp.IsResponse || string(resp.Data) != "hi" || resp.Status != 0 {
		t.Fatalf("response = %+v", resp)
	}

	_ = client.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after the peer closed")
	}
	if !svc.closed {
		t.Fatal("service was not closed")
	}
}

func TestServeUnknownServiceReturnsBadRequest(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	srv := NewServer(server, &echoService{id: 0x100})
	go func() { _ = srv.Serve(context.Background()) }()

	conn := NewConn(client)
	if err := conn.WriteMessage(Message{Service: 0x999, Command: 1}); err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}
	resp, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	if resp.Status != ErrCodeBadRequest {
		t.Fatalf("status = %#x, want %#x", resp.Status, ErrCodeBadRequest)
	}
}

// An unexpected error type must still produce a reply. Previously it killed the
// connection silently, leaving the peer to wait for its own timeout.
func TestServeRepliesOnNonProtocolError(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	srv := NewServer(server, &echoService{id: 0x100, err: errors.New("boom")})
	go func() { _ = srv.Serve(context.Background()) }()

	conn := NewConn(client)
	if err := conn.WriteMessage(Message{Service: 0x100, Command: 1}); err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}
	resp, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	if resp.Status == 0 {
		t.Fatal("expected a non-zero status")
	}
}

// A silent peer must not pin the goroutine forever.
func TestServeIdleTimeoutClosesConnection(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	srv := NewServer(server, &echoService{id: 0x100})
	srv.IdleTimeout = 100 * time.Millisecond

	done := make(chan error, 1)
	go func() { done <- srv.Serve(context.Background()) }()

	select {
	case err := <-done:
		if err == nil || errors.Is(err, io.EOF) {
			t.Fatalf("error = %v, want a timeout", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not time out on an idle connection")
	}
}

// Cancelling the context must unblock a read that is already in progress.
func TestServeContextCancellationUnblocksRead(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	srv := NewServer(server, &echoService{id: 0x100})
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve ignored context cancellation while blocked in a read")
	}
}

// Device.Close must tolerate a nil receiver so cleanup paths can call it
// unconditionally.
func TestDeviceCloseNilReceiver(t *testing.T) {
	var device *Device
	if err := device.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

// Invoke must not block forever when the caller supplies no deadline. A silent
// peer previously parked the call indefinitely.
func TestInvokeAppliesDefaultDeadline(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()

	c := NewClientFromConn(client)
	c.ExchangeTimeout = 150 * time.Millisecond

	done := make(chan error, 1)
	go func() {
		// The peer never replies.
		_, err := c.Invoke(context.Background(), 0x100, 1, nil)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected a timeout error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Invoke blocked with no caller deadline")
	}
}

// Cancelling the context must unblock an in-flight exchange.
func TestInvokeHonorsContextCancellation(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()

	c := NewClientFromConn(client)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		_, err := c.Invoke(ctx, 0x100, 1, nil)
		done <- err
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error after cancellation")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Invoke ignored context cancellation")
	}
}
