package fuji

import (
	"context"
	"encoding/binary"
	"testing"
)

func TestDecodeProductCode(t *testing.T) {
	t.Parallel()

	raw := make([]byte, 5+8)
	binary.LittleEndian.PutUint16(raw[0:2], 1)
	binary.LittleEndian.PutUint16(raw[2:4], 2)
	raw[4] = 3
	copy(raw[5:], []byte("XKW12345\x00"))

	pc, err := DecodeProductCode(raw)
	if err != nil {
		t.Fatalf("DecodeProductCode: %v", err)
	}
	if pc.Unk1 != 1 || pc.Character != 2 || pc.Unk2 != 3 {
		t.Fatalf("header = %+v", pc)
	}
	if pc.Serial != "XKW12345" {
		t.Fatalf("serial = %q", pc.Serial)
	}
}

func TestDecodeProductCodeShort(t *testing.T) {
	t.Parallel()
	if _, err := DecodeProductCode([]byte{1, 2, 3}); err == nil {
		t.Fatal("expected error")
	}
}

func TestGetParamParsesLength(t *testing.T) {
	t.Parallel()

	stub := &invokeStub{resp: make([]byte, 0x10+5)}
	binary.BigEndian.PutUint16(stub.resp[0:2], 5)
	copy(stub.resp[0x10:], []byte("hello"))

	got, err := getParam(t.Context(), stub, "product_code")
	if err != nil {
		t.Fatalf("GetParam: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("got %q", got)
	}
	if stub.service != controlServiceID || stub.command != getParamCommand {
		t.Fatalf("invoke = %d/%d", stub.service, stub.command)
	}
	if len(stub.req) != paramNameSize || string(stub.req[:12]) != "product_code" {
		t.Fatalf("req = %q (len %d)", stub.req, len(stub.req))
	}
}

type invokeStub struct {
	service, command uint16
	req, resp        []byte
}

func (s *invokeStub) Invoke(_ context.Context, service, command uint16, data []byte) ([]byte, error) {
	s.service, s.command = service, command
	s.req = append([]byte(nil), data...)
	return append([]byte(nil), s.resp...), nil
}

func TestBuildConnectionInfoPhysicalLayout(t *testing.T) {
	t.Parallel()
	payload := buildConnectionInfo(3335, 3333, 8888, 1, 0x0102030405060708)
	if len(payload) != 16 {
		t.Fatalf("length = %d", len(payload))
	}
	want := []uint16{1, 3335, 3333, 8888}
	for index, value := range want {
		if got := binary.LittleEndian.Uint16(payload[index*2 : index*2+2]); got != value {
			t.Fatalf("word %d = %d, want %d", index, got, value)
		}
	}
	if got := binary.LittleEndian.Uint64(payload[8:16]); got != 0x0102030405060708 {
		t.Fatalf("timestamp = %#x", got)
	}
}
