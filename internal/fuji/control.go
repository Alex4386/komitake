package fuji

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/Alex4386/komitake/internal/rcd"
)

const (
	// ControlPort is the kart's Fuji control service TCP port (switchbrew / openkartd).
	ControlPort = 5103

	controlServiceID = 0x100

	setParamCommand  = 2
	getParamCommand  = 3
	setStateCommand  = 4
	shutdownCommand  = 9
	paramNameSize    = 0x80
)

// ProductCode is the decoded GetParam("product_code") value.
type ProductCode struct {
	Unk1      uint16
	Character uint16
	Unk2      uint8
	Serial    string
}

// ControlClient talks to the kart's application control service (port 5103).
type ControlClient struct {
	client *rcd.Client
}

// DialControl connects to the kart control port with the same dial retry as pairing.
func DialControl(ctx context.Context, host string) (*ControlClient, error) {
	client, err := dialKart(ctx, host, ControlPort)
	if err != nil {
		return nil, err
	}
	return &ControlClient{client: client}, nil
}

func (c *ControlClient) Close() error {
	return c.client.Close()
}

// GetParam reads a named Fuji parameter. The response value length is a
// big-endian u16 at offset 0; the value follows at offset 0x10.
func (c *ControlClient) GetParam(ctx context.Context, name string) ([]byte, error) {
	return getParam(ctx, c.client, name)
}

// GetProductCode fetches and decodes the kart's product_code parameter.
func (c *ControlClient) GetProductCode(ctx context.Context) (ProductCode, error) {
	raw, err := c.GetParam(ctx, "product_code")
	if err != nil {
		return ProductCode{}, err
	}
	return DecodeProductCode(raw)
}

// SetState selects Fuji sleep (0) or drive (1) mode. connection_info must be
// configured first or the kart rejects this command with PARAM_STATE.
func (c *ControlClient) SetState(ctx context.Context, state byte) error {
	request := padTo([]byte{state}, 0x10)
	_, err := c.client.Invoke(ctx, controlServiceID, setStateCommand, request)
	return err
}

// Shutdown sends Fuji Control command 0x09. Per switchbrew, this disables the
// kart's network-reset-on-channel-close behavior so the kart powers off when
// the RCD channels are subsequently closed.
func (c *ControlClient) Shutdown(ctx context.Context) error {
	_, err := c.client.Invoke(ctx, controlServiceID, shutdownCommand, nil)
	return err
}

// SetConnectionInfo completes control setup by writing the connection_info
// parameter. After the handshake the kart expects the daemon to finish setup on
// the control channel; if it does not, the kart resets its network connection
// after a short interval. The port arguments tell the kart where to send
// telemetry and two LSP endpoints (control + video). Ports follow the Fuji
// connection_info layout.
func (c *ControlClient) SetConnectionInfo(ctx context.Context, telemetryPort, lspControlPort, lspStreamPort int, unknown uint16, timestamp int64) error {
	if timestamp == 0 {
		timestamp = time.Now().Unix()
	}
	return c.SetParam(ctx, "connection_info",
		buildConnectionInfo(telemetryPort, lspControlPort, lspStreamPort, unknown, timestamp))
}

// SetParam writes a named Fuji parameter. The request is a 0x80-byte zero-padded
// name, a big-endian u16 value length padded to 0x10, then the value.
func (c *ControlClient) SetParam(ctx context.Context, name string, value []byte) error {
	if len(name) >= paramNameSize {
		return fmt.Errorf("param name %q too long", name)
	}
	if len(value) > 0xffff {
		return fmt.Errorf("param %q value too large (%d bytes)", name, len(value))
	}
	req := padTo([]byte(name), paramNameSize)
	req = append(req, padTo(uint16ToBigEndian(uint16(len(value))), 0x10)...)
	req = append(req, value...)
	_, err := c.client.Invoke(ctx, controlServiceID, setParamCommand, req)
	return err
}

// buildConnectionInfo lays out the connection_info value as little-endian
// <HHHHQ: a fixed marker, the telemetry, LSP-control, and LSP-stream ports,
// then the current network time.
func buildConnectionInfo(telemetryPort, lspControlPort, lspStreamPort int, unknown uint16, timestamp int64) []byte {
	payload := make([]byte, 16)
	binary.LittleEndian.PutUint16(payload[0:2], unknown)
	binary.LittleEndian.PutUint16(payload[2:4], uint16(telemetryPort))
	binary.LittleEndian.PutUint16(payload[4:6], uint16(lspControlPort))
	binary.LittleEndian.PutUint16(payload[6:8], uint16(lspStreamPort))
	binary.LittleEndian.PutUint64(payload[8:16], uint64(timestamp))
	return payload
}

func uint16ToBigEndian(v uint16) []byte {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, v)
	return buf
}

type invoker interface {
	Invoke(ctx context.Context, service uint16, command uint16, data []byte) ([]byte, error)
}

func getParam(ctx context.Context, inv invoker, name string) ([]byte, error) {
	if len(name) >= paramNameSize {
		return nil, fmt.Errorf("param name %q too long", name)
	}
	req := padTo([]byte(name), paramNameSize)
	resp, err := inv.Invoke(ctx, controlServiceID, getParamCommand, req)
	if err != nil {
		return nil, err
	}
	if len(resp) < 0x10+2 {
		return nil, fmt.Errorf("get_param %q: short response (%d bytes)", name, len(resp))
	}
	n := int(binary.BigEndian.Uint16(resp[:2]))
	body := resp[0x10:]
	if n > len(body) {
		return nil, fmt.Errorf("get_param %q: length %d exceeds body %d", name, n, len(body))
	}
	return append([]byte(nil), body[:n]...), nil
}

// DecodeProductCode parses the product_code blob. Layout matches openkartd
// (Python): LE u16 unk1, LE u16 character, u8 unk2, then a NUL-terminated
// latin1 serial. switchbrew places the serial elsewhere; we follow the
// implementation that works on hardware.
func DecodeProductCode(data []byte) (ProductCode, error) {
	if len(data) < 5 {
		return ProductCode{}, fmt.Errorf("product_code too short (%d bytes)", len(data))
	}
	pc := ProductCode{
		Unk1:      binary.LittleEndian.Uint16(data[0:2]),
		Character: binary.LittleEndian.Uint16(data[2:4]),
		Unk2:      data[4],
	}
	serial := data[5:]
	if i := bytes.IndexByte(serial, 0); i >= 0 {
		serial = serial[:i]
	}
	// Cap at the documented 0xF field width when no NUL is present.
	if len(serial) > 0xF {
		serial = serial[:0xF]
	}
	pc.Serial = strings.TrimSpace(string(serial))
	return pc, nil
}
