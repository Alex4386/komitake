package fuji

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestDialKartRetriesUntilPortOpens(t *testing.T) {
	t.Parallel()

	// Bind then close so we get a free port that refuses until we re-listen.
	probe, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("ListenTCP() error = %v", err)
	}
	addr := probe.Addr().(*net.TCPAddr)
	port := addr.Port
	_ = probe.Close()

	errCh := make(chan error, 1)
	go func() {
		time.Sleep(250 * time.Millisecond)
		ln, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
		if err != nil {
			errCh <- err
			return
		}
		defer ln.Close()
		conn, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		_ = conn.Close()
		errCh <- nil
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := dialKart(ctx, "127.0.0.1", port)
	if err != nil {
		t.Fatalf("dialKart() error = %v", err)
	}
	_ = client.Close()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("listener: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("listener did not accept")
	}
}

func TestDialKartFailsAfterAttempts(t *testing.T) {
	t.Parallel()

	probe, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("ListenTCP() error = %v", err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	_ = probe.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = dialKart(ctx, "127.0.0.1", port)
	if err == nil {
		t.Fatal("dialKart() error = nil, want failure")
	}
}
