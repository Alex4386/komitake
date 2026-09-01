package komitake

import (
	"encoding/hex"
	"errors"
	"fmt"
)

const (
	pairingSeedSize = 16
	ssidFieldSize   = 0x20
	qrPayloadSize   = 0x3e
)

// QRPayload builds the byte payload a kart scans to join a pairing network,
// from a Pairing session. The channel is fixed at 0 ("use default") per the
// LP2P pairing QR format; the live AP runs on its own concrete channel.
func (p *Pairing) QRPayload() ([]byte, error) {
	seed, err := hex.DecodeString(p.Seed)
	if err != nil {
		return nil, fmt.Errorf("invalid pairing seed: %w", err)
	}
	return BuildQRPayload(seed, p.SSID, 0)
}

// BuildQRPayload builds the 0x3e-byte pairing payload: 0x10-byte seed,
// 0x20-byte zero-padded SSID, little-endian channel, then zero padding.
func BuildQRPayload(seed []byte, ssid string, channel uint16) ([]byte, error) {
	if len(seed) != pairingSeedSize {
		return nil, fmt.Errorf("pairing seed must be %d bytes", pairingSeedSize)
	}
	if ssid == "" {
		return nil, errors.New("pairing ssid must not be empty")
	}
	if len(ssid) >= ssidFieldSize {
		return nil, fmt.Errorf("pairing ssid must be < %d bytes", ssidFieldSize)
	}
	payload := make([]byte, qrPayloadSize)
	copy(payload[:pairingSeedSize], seed)
	copy(payload[0x10:0x10+ssidFieldSize], []byte(ssid))
	payload[0x30] = byte(channel)
	payload[0x31] = byte(channel >> 8)
	return payload, nil
}
