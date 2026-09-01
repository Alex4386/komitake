package rcd

import (
	"context"
	"encoding/hex"
	"testing"
)

func TestEncodeDecodeMessageRoundTrip(t *testing.T) {
	t.Parallel()

	msg := Message{
		Service:    0x100,
		Command:    2,
		Status:     7,
		IsResponse: true,
		Data:       []byte("hello"),
	}

	encoded := EncodeMessage(msg)
	decoded, err := DecodeMessage(encoded)
	if err != nil {
		t.Fatalf("DecodeMessage() error = %v", err)
	}

	if decoded.Service != msg.Service || decoded.Command != msg.Command || decoded.Status != msg.Status || decoded.IsResponse != msg.IsResponse || string(decoded.Data) != string(msg.Data) {
		t.Fatalf("decoded message mismatch: %#v", decoded)
	}
}

func TestServerInfoPairingKeysMatchKnownVector(t *testing.T) {
	t.Parallel()

	info := ServerInfo{
		Name:      "Komitake",
		Ident:     mustHex(t, "000102030405060708090a0b0c0d0e0f"),
		MasterKey: mustHex(t, "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"),
		Versions:  []uint8{2, 1},
	}

	pairingID, secretKey := info.PairingKeys(mustHex(t, "101112131415161718191a1b1c1d1e1f"), "Fuji")
	if got := hex.EncodeToString(pairingID); got != "7172bba5c18d40d5d4063a95802eaa9d60ebbabc68f32467e6821d4fd6feb7c7" {
		t.Fatalf("pairingID = %s", got)
	}
	if got := hex.EncodeToString(secretKey); got != "b7a7b0e264e237cd7df0c840e3147dcbe8ee44f1664bf07c2c340f68938bd74c2a7e5b01f19f0e62273695088030d0b3a9e4f600b803c11d17df20cf5d4acbca" {
		t.Fatalf("secretKey = %s", got)
	}
}

func TestHandshakeServicePairingFlow(t *testing.T) {
	t.Parallel()

	service := NewHandshakeService(ServerInfo{
		Name:      "Komitake",
		Ident:     mustHex(t, "000102030405060708090a0b0c0d0e0f"),
		MasterKey: mustHex(t, "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"),
		Versions:  []uint8{2, 1},
	}, nil, true)

	begin := make([]byte, 0x50)
	begin[0] = 1
	copy(begin[0x10:0x20], []byte("Fuji"))
	copy(begin[0x20:0x30], mustHex(t, "101112131415161718191a1b1c1d1e1f"))

	resp1, err := service.Handle(context.Background(), nil, Message{Command: 1, Data: begin})
	if err != nil {
		t.Fatalf("begin handshake: %v", err)
	}
	if len(resp1) != 0x50 {
		t.Fatalf("begin response len = %d", len(resp1))
	}

	negotiate := append(make([]byte, 0x20), 2, 2, 1)
	resp2, err := service.Handle(context.Background(), nil, Message{Command: 2, Data: negotiate})
	if err != nil {
		t.Fatalf("negotiate: %v", err)
	}
	if len(resp2) != 0x30 {
		t.Fatalf("negotiate response len = %d", len(resp2))
	}

	resp3, err := service.Handle(context.Background(), nil, Message{Command: 3, Data: make([]byte, 0x20)})
	if err != nil {
		t.Fatalf("get secret: %v", err)
	}
	if len(resp3) != 64 {
		t.Fatalf("secret len = %d", len(resp3))
	}

	resp4, err := service.Handle(context.Background(), nil, Message{Command: 4, Data: service.transcript.FlushedDigest()})
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if len(resp4) != 32 {
		t.Fatalf("finalize digest len = %d", len(resp4))
	}
}

func mustHex(t *testing.T, value string) []byte {
	t.Helper()
	data, err := hex.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
