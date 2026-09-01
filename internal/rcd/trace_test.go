package rcd

import (
	"bytes"
	"context"
	"encoding/hex"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Alex4386/komitake/internal/logging"
)

// traceServerInfo uses distinctive key material so a leak is unambiguous in the
// captured output.
func traceServerInfo() ServerInfo {
	return ServerInfo{
		Name:      "Komitake",
		Ident:     bytes.Repeat([]byte{0x5a}, 16),
		MasterKey: bytes.Repeat([]byte{0xc3}, 64),
		Versions:  []uint8{2, 1},
	}
}

// Verbose logging is the usual way keys escape into log files. At trace level,
// which dumps every payload, the derived secret key must still never appear.
func TestTraceLoggingNeverLeaksSecretKey(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewLogger(&buf, logging.Options{Level: logging.LevelTrace})

	info := traceServerInfo()
	svc := NewHandshakeService(info, nil, true)
	svc.SetLogger(logger)

	// Walk the pairing flow far enough to release the secret key.
	begin := make([]byte, 0x50)
	begin[0] = 1
	copy(begin[0x10:0x20], []byte("Fuji"))
	copy(begin[0x20:0x30], bytes.Repeat([]byte{0x11}, 16))

	if _, err := svc.Handle(context.Background(), nil, Message{Command: 1, Data: begin}); err != nil {
		t.Fatalf("command 1 error = %v", err)
	}

	// Command 2 with a non-matching pairing ID routes to command 3 in pairing mode.
	negotiate := make([]byte, 0x22)
	negotiate[0x20] = 1
	negotiate[0x21] = 2
	if _, err := svc.Handle(context.Background(), nil, Message{Command: 2, Data: negotiate}); err != nil {
		t.Fatalf("command 2 error = %v", err)
	}

	secret, err := svc.Handle(context.Background(), nil, Message{Command: 3, Data: make([]byte, 0x20)})
	if err != nil {
		t.Fatalf("command 3 error = %v", err)
	}
	if len(secret) != 64 {
		t.Fatalf("secret key is %d bytes, want 64", len(secret))
	}

	out := buf.String()
	if out == "" {
		t.Fatal("no trace output was captured")
	}

	// The secret key, the master key it derives from, and the pairing ID must
	// all be absent in hex form.
	_, expectedPairingID := info.PairingKeys(bytes.Repeat([]byte{0x11}, 16), "Fuji")
	for name, value := range map[string][]byte{
		"secret key": secret,
		"master key": info.MasterKey,
		"pairing id": expectedPairingID,
	} {
		if strings.Contains(out, hex.EncodeToString(value)) {
			t.Fatalf("%s leaked into trace output:\n%s", name, out)
		}
	}

	// Redaction markers should be present instead.
	if !strings.Contains(out, "redacted") {
		t.Fatalf("expected redaction markers in trace output:\n%s", out)
	}
}

// The handshake command-3 response is the derived secret key, so the server's
// response trace must redact it rather than dumping the payload.
func TestServeRedactsSecretKeyResponse(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewLogger(&buf, logging.Options{Level: logging.LevelTrace})

	client, server := net.Pipe()
	defer client.Close()

	secret := bytes.Repeat([]byte{0xd5}, 64)
	srv := NewServer(server, &fixedService{id: handshakeServiceID, response: secret})
	srv.SetLogger(logger)
	go func() { _ = srv.Serve(context.Background()) }()

	conn := NewConn(client)
	if err := conn.WriteMessage(Message{Service: handshakeServiceID, Command: 3}); err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}
	if _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	// Let the trace record flush.
	time.Sleep(50 * time.Millisecond)

	if out := buf.String(); strings.Contains(out, hex.EncodeToString(secret)) {
		t.Fatalf("secret key response was dumped:\n%s", out)
	}
}

// A payload for a service marked sensitive must be redacted. The Fuji pairing
// request carries a raw PSK.
func TestMarkSensitiveServiceRedactsPayload(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewLogger(&buf, logging.Options{Level: logging.LevelTrace})

	client, server := net.Pipe()
	defer server.Close()

	psk := bytes.Repeat([]byte{0x7e}, 32)

	c := NewClientFromConn(client)
	c.SetLogger(logger)
	c.MarkSensitiveService(0x102)
	c.ExchangeTimeout = 200 * time.Millisecond

	// net.Pipe is unbuffered, so the peer must be drained or WriteMessage blocks
	// and the request is never traced. The reply is deliberately omitted.
	go func() { _, _ = io.Copy(io.Discard, server) }()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = c.Invoke(context.Background(), 0x102, 1, psk)
	}()
	<-done

	out := buf.String()
	if strings.Contains(out, hex.EncodeToString(psk)) {
		t.Fatalf("sensitive payload was dumped:\n%s", out)
	}
	if !strings.Contains(out, "redacted") {
		t.Fatalf("expected a redaction marker:\n%s", out)
	}
}

// Non-sensitive payloads should still be dumped: that is the point of trace.
func TestTraceDumpsOrdinaryPayloads(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewLogger(&buf, logging.Options{Level: logging.LevelTrace})

	client, server := net.Pipe()
	defer server.Close()

	payload := []byte{0xde, 0xad, 0xbe, 0xef}

	c := NewClientFromConn(client)
	c.SetLogger(logger)
	c.ExchangeTimeout = 200 * time.Millisecond

	go func() { _, _ = io.Copy(io.Discard, server) }()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = c.Invoke(context.Background(), 0x100, 1, payload)
	}()
	<-done

	if out := buf.String(); !strings.Contains(out, "deadbeef") {
		t.Fatalf("ordinary payload was not traced:\n%s", out)
	}
}

// Error codes must render symbolically so operators do not have to decode hex.
func TestErrorNameCoversKnownCodes(t *testing.T) {
	t.Parallel()

	for code, want := range map[uint32]string{
		ErrCodeMissizedPayload:   "MISSIZED_PAYLOAD",
		ErrCodeHandshakeHash:     "HANDSHAKE_HASH",
		ErrCodeParamUnrecognized: "PARAM_UNRECOGNIZED",
	} {
		if got := ErrorName(code); got != want {
			t.Fatalf("ErrorName(%#x) = %q, want %q", code, got, want)
		}
	}
	if got := ErrorName(0xdead); got != "" {
		t.Fatalf("ErrorName(unknown) = %q, want empty", got)
	}

	err := &Error{Code: ErrCodeHandshakeHash}
	if !strings.Contains(err.Error(), "HANDSHAKE_HASH") {
		t.Fatalf("Error() = %q, want a symbolic name", err.Error())
	}
}

// fixedService returns a canned response, for exercising server trace paths.
type fixedService struct {
	id       uint16
	response []byte
}

func (f *fixedService) ServiceID() uint16 { return f.id }

func (f *fixedService) Handle(context.Context, *Conn, Message) ([]byte, error) {
	return f.response, nil
}

func (f *fixedService) Close() error { return nil }
