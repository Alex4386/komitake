package wireless

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestNewGameGroupUsesGamePrefix(t *testing.T) {
	t.Parallel()

	group, err := NewGameGroup(0)
	if err != nil {
		t.Fatalf("NewGameGroup() error = %v", err)
	}
	if group.SSID == "" || group.SSID[0] != 'G' {
		t.Fatalf("ssid = %q, want prefix G", group.SSID)
	}
	if len(group.SSID) != PairingSSIDSize {
		t.Fatalf("ssid len = %d, want %d", len(group.SSID), PairingSSIDSize)
	}
	if len(group.PSK) != PSKSize {
		t.Fatalf("psk len = %d", len(group.PSK))
	}
}

func TestNewPairingCredentialsUsesPairingPrefix(t *testing.T) {
	t.Parallel()

	creds, err := NewPairingCredentials(0)
	if err != nil {
		t.Fatalf("NewPairingCredentials() error = %v", err)
	}
	if creds.SSID == "" || creds.SSID[0] != 'P' {
		t.Fatalf("ssid = %q, want prefix P", creds.SSID)
	}
	if len(creds.SSID) != PairingSSIDSize {
		t.Fatalf("ssid len = %d, want %d", len(creds.SSID), PairingSSIDSize)
	}
}

func TestBuildPairingQRCodePayloadLayout(t *testing.T) {
	t.Parallel()

	seed := bytes.Repeat([]byte{0x11}, PairingSeedSize)
	payload, err := BuildPairingQRCodePayload(seed, "P1234567890123456789012", 6)
	if err != nil {
		t.Fatalf("BuildPairingQRCodePayload() error = %v", err)
	}
	if len(payload) != QRPayloadSize {
		t.Fatalf("payload len = %d", len(payload))
	}
	if !bytes.Equal(payload[:PairingSeedSize], seed) {
		t.Fatal("seed bytes were not copied at offset 0")
	}
	if got := string(bytes.TrimRight(payload[0x10:0x30], "\x00")); got != "P1234567890123456789012" {
		t.Fatalf("ssid bytes = %q", got)
	}
	if got := binary.LittleEndian.Uint16(payload[0x30:0x32]); got != 6 {
		t.Fatalf("channel = %d", got)
	}
	if !bytes.Equal(payload[0x32:], make([]byte, 0x0c)) {
		t.Fatal("expected trailing zero padding")
	}
}

// switchbrew documents the SSID field as 0x20 bytes with the remaining space
// zero-filled, so only the maximum is a wire constraint. A shorter SSID, such as
// one configured by hand, must be accepted.
func TestBuildPairingQRCodePayloadAcceptsShorterSSID(t *testing.T) {
	t.Parallel()

	seed := bytes.Repeat([]byte{0x11}, PairingSeedSize)
	payload, err := BuildPairingQRCodePayload(seed, "Pshort", 6)
	if err != nil {
		t.Fatalf("BuildPairingQRCodePayload() error = %v", err)
	}
	if got := string(bytes.TrimRight(payload[0x10:0x30], "\x00")); got != "Pshort" {
		t.Fatalf("ssid bytes = %q", got)
	}
	// The unused remainder of the field must be zero so the kart sees a
	// terminated string.
	if !bytes.Equal(payload[0x10+len("Pshort"):0x30], make([]byte, SSIDFieldSize-len("Pshort"))) {
		t.Fatal("ssid field was not zero-filled")
	}
}

// An SSID that fills the whole field leaves no room for the zero terminator.
func TestBuildPairingQRCodePayloadRejectsUnterminatableSSID(t *testing.T) {
	t.Parallel()

	seed := bytes.Repeat([]byte{0x11}, PairingSeedSize)

	if _, err := BuildPairingQRCodePayload(seed, string(bytes.Repeat([]byte{'x'}, SSIDFieldSize)), 6); err == nil {
		t.Fatalf("expected an error for a %d-byte ssid", SSIDFieldSize)
	}
	if _, err := BuildPairingQRCodePayload(seed, "", 6); err == nil {
		t.Fatal("expected an error for an empty ssid")
	}

	// One byte short of the field width is the largest acceptable SSID.
	maxSSID := string(bytes.Repeat([]byte{'x'}, PairingSSIDMaxSize))
	if _, err := BuildPairingQRCodePayload(seed, maxSSID, 6); err != nil {
		t.Fatalf("BuildPairingQRCodePayload(%d-byte ssid) error = %v", PairingSSIDMaxSize, err)
	}
}

// GroupInfo.Validate must apply the same terminator-aware limit, since the
// SetGroupInfo payload uses the same 0x20-byte field.
func TestGroupInfoValidateRejectsUnterminatableSSID(t *testing.T) {
	t.Parallel()

	group := GroupInfo{
		SSID:    string(bytes.Repeat([]byte{'x'}, SSIDFieldSize)),
		PSK:     make([]byte, PSKSize),
		Channel: 1,
	}
	if err := group.Validate(); err == nil {
		t.Fatalf("expected an error for a %d-byte ssid", SSIDFieldSize)
	}

	group.SSID = string(bytes.Repeat([]byte{'x'}, PairingSSIDMaxSize))
	if err := group.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
