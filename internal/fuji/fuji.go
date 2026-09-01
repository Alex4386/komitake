// Package fuji implements the Fuji kart protocol subset that komitake needs:
// pairing (group info), control (product_code, connection_info, state,
// shutdown), drive, and telemetry.
package fuji

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/Alex4386/komitake/internal/logging"
	"github.com/Alex4386/komitake/internal/rcd"
	"github.com/Alex4386/komitake/internal/wireless"
)

const (
	// PairingPort is the kart's pairing service TCP port, per switchbrew.
	PairingPort = 5106

	pairingServiceID = 0x102

	// Match openkartd util.retry_connect: the kart opens its service ports only
	// after the handshake finishes, so the first dials are often refused.
	dialAttempts    = 10
	dialAttemptWait = 200 * time.Millisecond
	dialRetryDelay  = 100 * time.Millisecond
)

// PairingClient writes group info to a kart over the pairing service.
type PairingClient struct {
	client *rcd.Client
}

func DialPairing(ctx context.Context, host string) (*PairingClient, error) {
	client, err := dialKart(ctx, host, PairingPort)
	if err != nil {
		return nil, err
	}
	// The SetGroupInfo request carries the raw PSK, so its payload must never
	// be dumped by the RCD tracer.
	client.MarkSensitiveService(pairingServiceID)
	return &PairingClient{client: client}, nil
}

func (c *PairingClient) Close() error {
	return c.client.Close()
}

func (c *PairingClient) SetGroupInfo(ctx context.Context, group wireless.GroupInfo) error {
	if err := group.Validate(); err != nil {
		return err
	}

	// The PSK is the link-layer key, so it is fingerprinted rather than dumped.
	// Note the request payload itself is deliberately not traced for the same
	// reason: its second half is the raw PSK.
	logging.New(nil).With("component", "fuji-pairing").Info("writing group info to kart",
		"ssid", group.SSID, "channel", group.Channel,
		logging.Secret("psk", group.PSK))

	req := padTo([]byte(group.SSID), 0x20)
	req = append(req, group.PSK...)
	_, err := c.client.Invoke(ctx, pairingServiceID, 1, req)
	return err
}

// Pair writes the game group info to a freshly-handshaked kart.
//
// The device is deliberately not closed here. After SetGroupInfo the kart
// resets onto the game SSID; the caller must bring the game AP up before
// dropping the handshake socket (another reset trigger) or the first join
// window is spent against a dead or still-pairing BSS.
func Pair(ctx context.Context, device *rcd.Device, group wireless.GroupInfo) error {
	client, err := DialPairing(ctx, device.Address)
	if err != nil {
		return err
	}
	defer client.Close()
	return client.SetGroupInfo(ctx, group)
}

// dialKart retries TCP dials to the kart the way openkartd's retry_connect
// does. switchbrew gives a short post-handshake window to open follow-up ports;
// connection refused on the first attempt is normal.
func dialKart(ctx context.Context, host string, port int) (*rcd.Client, error) {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	log := logging.New(nil).With("component", "fuji-dial", "address", addr)

	var last error
	for attempt := 1; attempt <= dialAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		attemptCtx, cancel := context.WithTimeout(ctx, dialAttemptWait)
		client, err := rcd.DialClient(attemptCtx, "tcp", addr)
		cancel()
		if err == nil {
			if attempt > 1 {
				log.Debug("kart port open after retry", "attempt", attempt)
			}
			return client, nil
		}
		last = err
		log.Debug("kart dial refused, retrying", "attempt", attempt, "error", err)
		if attempt == dialAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(dialRetryDelay):
		}
	}
	return nil, fmt.Errorf("dial %s after %d attempts: %w", addr, dialAttempts, last)
}

func padTo(data []byte, size int) []byte {
	if len(data) >= size {
		return append([]byte(nil), data...)
	}
	out := make([]byte, size)
	copy(out, data)
	return out
}

// FormatMAC renders the trailing six bytes of a device ident as a MAC address.
func FormatMAC(ident []byte) string {
	if len(ident) < 6 {
		return ""
	}
	start := len(ident) - 6
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x",
		ident[start], ident[start+1], ident[start+2], ident[start+3], ident[start+4], ident[start+5],
	)
}
