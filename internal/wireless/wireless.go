package wireless

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	mathrand "math/rand"
	"time"
)

const (
	PSKSize         = 32
	PairingSeedSize = 16

	// SSIDFieldSize is the on-wire SSID field width in the pairing QR payload
	// and the SetGroupInfo request.
	SSIDFieldSize = 0x20

	// PairingSSIDMaxSize leaves one byte for the zero terminator the kart
	// expects within the SSID field.
	PairingSSIDMaxSize = SSIDFieldSize - 1

	// PairingSSIDSize is the generated SSID length. lp2p ServiceName's simple
	// (no-underscore) validator accepts at most 19 characters; a longer stored
	// SSID can be rejected on Join after SetGroupInfo.
	// https://switchbrew.org/wiki/LDN_services#GroupInfo
	PairingSSIDSize = 19

	QRPayloadSize = 0x3e
)

type GroupInfo struct {
	SSID    string
	PSK     []byte
	Channel uint16
}

func (g GroupInfo) Validate() error {
	if g.SSID == "" {
		return errors.New("wireless ssid must not be empty")
	}
	if len(g.SSID) > PairingSSIDMaxSize {
		return fmt.Errorf("wireless ssid must be <= %d bytes", PairingSSIDMaxSize)
	}
	if len(g.PSK) != PSKSize {
		return fmt.Errorf("wireless psk must be %d bytes", PSKSize)
	}
	switch g.Channel {
	case 1, 6, 11:
	default:
		return errors.New("wireless channel must be 1, 6, or 11")
	}
	return nil
}

type PairingCredentials struct {
	Seed    []byte
	SSID    string
	Channel uint16
	PSK     []byte
}

func NewGameGroup(channel uint16) (GroupInfo, error) {
	if channel == 0 {
		channel = defaultChannel()
	}

	psk := make([]byte, PSKSize)
	if _, err := rand.Read(psk); err != nil {
		return GroupInfo{}, err
	}

	group := GroupInfo{
		SSID:    randomSSID("G"),
		PSK:     psk,
		Channel: channel,
	}
	return group, group.Validate()
}

func NewPairingCredentials(channel uint16) (PairingCredentials, error) {
	if channel == 0 {
		channel = defaultChannel()
	}

	seed := make([]byte, PairingSeedSize)
	if _, err := rand.Read(seed); err != nil {
		return PairingCredentials{}, err
	}

	psk := sha256.Sum256(seed)
	creds := PairingCredentials{
		Seed:    seed,
		SSID:    randomSSID("P"),
		Channel: channel,
		PSK:     psk[:],
	}
	return creds, validatePairingCredentials(creds)
}

func defaultChannel() uint16 {
	r := mathrand.New(mathrand.NewSource(time.Now().UnixNano()))
	choices := []uint16{1, 6, 11}
	return choices[r.Intn(len(choices))]
}

// BuildPairingQRCodePayload builds the 0x3e-byte pairing payload the kart
// scans: 0x10-byte seed, 0x20-byte zero-padded SSID, little-endian channel,
// then zero padding. Channel 0 means "use the default"; the AP still runs on a
// concrete 1/6/11 channel, so callers normally pass 0 here.
// https://switchbrew.org/wiki/Mario_Kart_Live:_Home_Circuit
func BuildPairingQRCodePayload(seed []byte, ssid string, channel uint16) ([]byte, error) {
	if len(seed) != PairingSeedSize {
		return nil, fmt.Errorf("pairing seed must be %d bytes", PairingSeedSize)
	}
	if ssid == "" {
		return nil, errors.New("pairing ssid must not be empty")
	}
	if len(ssid) > PairingSSIDMaxSize {
		return nil, fmt.Errorf("pairing ssid must be <= %d bytes", PairingSSIDMaxSize)
	}

	payload := make([]byte, QRPayloadSize)
	copy(payload[:PairingSeedSize], seed)
	copy(payload[0x10:0x10+SSIDFieldSize], []byte(ssid))
	binary.LittleEndian.PutUint16(payload[0x30:0x32], channel)
	return payload, nil
}

func validatePairingCredentials(creds PairingCredentials) error {
	if len(creds.Seed) != PairingSeedSize {
		return fmt.Errorf("pairing seed must be %d bytes", PairingSeedSize)
	}
	if len(creds.SSID) > PairingSSIDMaxSize {
		return fmt.Errorf("pairing ssid must be <= %d bytes", PairingSSIDMaxSize)
	}
	group := GroupInfo{
		SSID:    creds.SSID,
		PSK:     creds.PSK,
		Channel: creds.Channel,
	}
	return group.Validate()
}

func randomSSID(prefix string) string {
	if len(prefix) >= PairingSSIDSize {
		panic("ssid prefix is too long")
	}
	buf := make([]byte, (PairingSSIDSize-len(prefix))/2)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(buf)
}
