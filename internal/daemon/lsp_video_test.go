package daemon

import (
	"encoding/binary"
	"os"
	"testing"
)

func TestCapturedFirstIDRFixtureReconstructsFrame(t *testing.T) {
	data, err := os.ReadFile("../../re/captures/2026-08-31-successful-lsp/fixture-first-idr-43.datagrams.bin")
	if err != nil {
		t.Skipf("capture fixture unavailable: %v", err)
	}
	assembler := newVideoAssembler("kart")
	var frames []VideoFrame
	offset := 0
	for offset < len(data) {
		size := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		offset += 4
		packet, parseErr := parseNiffinPacket(data[offset : offset+size])
		offset += size
		if parseErr != nil {
			t.Fatalf("parseNiffinPacket: %v", parseErr)
		}
		currentFrames, consumeErr := assembler.consume(packet.packet)
		if consumeErr != nil {
			t.Fatalf("consume: %v", consumeErr)
		}
		frames = append(frames, currentFrames...)
	}
	if len(frames) != 1 {
		t.Fatalf("frames = %d, want 1", len(frames))
	}
	frame := frames[0]
	if frame.DeviceID != "kart" || !frame.KeyFrame {
		t.Fatalf("frame = %+v", frame)
	}
	if len(frame.Data) != 59490-framVideoHeaderSize {
		t.Fatalf("data length = %d", len(frame.Data))
	}
	if !containsNALType(frame.Data, 7) || !containsNALType(frame.Data, 8) || !containsNALType(frame.Data, 5) {
		t.Fatal("first frame lacks SPS/PPS/IDR")
	}
}

func TestParseNiffinPacketRejectsParityWithoutFixture(t *testing.T) {
	packet := make([]byte, niffinDatagramSize)
	packet[0], packet[1], packet[2], packet[6] = 0xEC, 0xC1, 0x80, 1
	if _, err := parseNiffinPacket(packet); err != errUnsupportedParity {
		t.Fatalf("error = %v", err)
	}
}
