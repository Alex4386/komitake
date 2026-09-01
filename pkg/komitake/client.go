// Package komitake is a client SDK for the komitake daemon's admin API. It
// wraps the gRPC service exposed over a unix socket or TCP so external tools
// can query state, list karts, and drive pairing.
package komitake

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	adminv1 "github.com/Alex4386/komitake/proto/komitake/admin/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// DefaultAddress is the daemon's default admin API address.
const DefaultAddress = "unix:/run/komitake.sock"

// Client talks to a komitake daemon.
type Client struct {
	conn   *grpc.ClientConn
	admin  adminv1.AdminServiceClient
	target string
}

// Dial connects to the daemon at the given address. The address is either
// "unix:/path", "tcp:host:port", a bare "host:port" (TCP), or a filesystem path
// (unix). An empty address uses DefaultAddress. No I/O happens until the first
// call.
func Dial(address string) (*Client, error) {
	network, addr, err := parseAddress(address)
	if err != nil {
		return nil, err
	}
	conn, err := grpc.NewClient(
		"passthrough:///"+addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, a string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, a)
		}),
	)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, admin: adminv1.NewAdminServiceClient(conn), target: network + ":" + addr}, nil
}

// Close releases the underlying connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// Address returns the resolved target the client dials.
func (c *Client) Address() string { return c.target }

// Status is the daemon's current mode and wireless/pairing state.
type Status struct {
	Mode     Mode
	Wireless *Wireless
	Pairing  *Pairing
}

// Mode is the daemon lifecycle state.
type Mode string

const (
	ModeStopped Mode = "stopped"
	ModeNormal  Mode = "normal"
	ModePairing Mode = "pairing"
	ModeUnknown Mode = "unknown"
)

// Wireless is non-secret AP/runtime networking info.
type Wireless struct {
	Interface   string
	Address     string
	Subnet      string
	Channel     uint16
	SSID        string
	HostapdPath string
}

// Pairing describes an active pairing session. Seed is equivalent to the
// network key; treat it as a secret.
type Pairing struct {
	SSID        string
	Channel     uint16
	Seed        string
	GeneratedAt string
}

// Kart is a connected kart.
type Kart struct {
	Kind       string
	Ident      string
	Serial     string
	Address    string
	MACAddress string
	// SignalDBM is the station signal in dBm, or nil when the AP reports none.
	SignalDBM *int
	// Battery is the documented HUD bar count from telemetry type 0x01.
	Battery        *int
	CableConnected *bool
	DriveArmed     bool
	AccelMPS2      *Vec3
	GyroDPS        *Vec3
	Orientation    *Quat
	IMUTimerUs     *uint32
}

type Vec3 struct{ X, Y, Z float64 }
type Quat struct{ I, J, K, R float64 }

// Status returns the daemon's current state.
func (c *Client) Status(ctx context.Context) (*Status, error) {
	resp, err := c.admin.GetState(ctx, &adminv1.GetStateRequest{})
	if err != nil {
		return nil, err
	}
	return statusFromProto(resp.State, resp.Wireless, resp.Pairing), nil
}

// Karts lists currently connected karts.
func (c *Client) Karts(ctx context.Context) ([]Kart, error) {
	resp, err := c.admin.ListDevices(ctx, &adminv1.ListDevicesRequest{})
	if err != nil {
		return nil, err
	}
	out := make([]Kart, 0, len(resp.Devices))
	for _, d := range resp.Devices {
		out = append(out, kartFromProto(d))
	}
	return out, nil
}

// PairingInfo returns the current pairing session, or nil when not pairing.
func (c *Client) PairingInfo(ctx context.Context) (*Pairing, error) {
	resp, err := c.admin.GetPairingInfo(ctx, &adminv1.GetPairingInfoRequest{})
	if err != nil {
		return nil, err
	}
	return pairingFromProto(resp.Pairing), nil
}

// StartPairing puts the daemon into pairing mode and returns the session info a
// caller renders into a QR code.
func (c *Client) StartPairing(ctx context.Context) (*Pairing, error) {
	if _, err := c.admin.SetState(ctx, &adminv1.SetStateRequest{State: adminv1.State_STATE_PAIRING}); err != nil {
		return nil, err
	}
	resp, err := c.admin.GetPairingInfo(ctx, &adminv1.GetPairingInfoRequest{})
	if err != nil {
		return nil, err
	}
	p := pairingFromProto(resp.Pairing)
	if p == nil {
		return nil, fmt.Errorf("daemon entered pairing mode but returned no pairing info")
	}
	return p, nil
}

// StopPairing returns the daemon to normal mode.
func (c *Client) StopPairing(ctx context.Context) error {
	_, err := c.admin.SetState(ctx, &adminv1.SetStateRequest{State: adminv1.State_STATE_RUNNING})
	return err
}

// WaitForKart blocks until a kart is connected. An empty ident matches any
// kart. The context bounds the wait.
func (c *Client) WaitForKart(ctx context.Context, ident string) (*Kart, error) {
	resp, err := c.admin.WaitForDevice(ctx, &adminv1.WaitForDeviceRequest{Ident: ident})
	if err != nil {
		return nil, err
	}
	k := kartFromProto(resp.Device)
	return &k, nil
}

// ProductCode is the decoded Fuji product_code parameter.
type ProductCode struct {
	Unk1      uint16
	Character uint16
	Unk2      uint8
	Serial    string
}

// GetDeviceParam reads a Fuji control GetParam value for a connected kart.
// selector is an ident or serial (exact or unique prefix).
func (c *Client) GetDeviceParam(ctx context.Context, selector, name string) (*Kart, []byte, error) {
	resp, err := c.admin.GetDeviceParam(ctx, &adminv1.GetDeviceParamRequest{
		Selector: selector,
		Name:     name,
	})
	if err != nil {
		return nil, nil, err
	}
	k := kartFromProto(resp.Device)
	return &k, append([]byte(nil), resp.Value...), nil
}

// GetProductCode reads product_code over the Fuji control port and refreshes
// the daemon's cached serial for that device.
func (c *Client) GetProductCode(ctx context.Context, selector string) (*Kart, *ProductCode, error) {
	resp, err := c.admin.GetProductCode(ctx, &adminv1.GetProductCodeRequest{Selector: selector})
	if err != nil {
		return nil, nil, err
	}
	k := kartFromProto(resp.Device)
	pc := resp.ProductCode
	if pc == nil {
		return &k, nil, fmt.Errorf("daemon returned no product_code")
	}
	return &k, &ProductCode{
		Unk1:      uint16(pc.Unk1),
		Character: uint16(pc.Character),
		Unk2:      uint8(pc.Unk2),
		Serial:    pc.Serial,
	}, nil
}

// VideoFrame is one complete Annex-B H.264 frame from a connected kart.
type VideoFrame struct {
	DeviceID      string
	Sequence      uint64
	Metadata0     uint64
	Metadata1     uint64
	Metadata2     uint64
	KeyFrame      bool
	Discontinuity bool
	AnnexB        []byte
}

// VideoStream receives video frames until its context is canceled or the daemon disconnects.
type VideoStream struct {
	stream grpc.ServerStreamingClient[adminv1.StreamVideoResponse]
}

// VideoReceiver receives complete video frames from a daemon stream.
type VideoReceiver interface {
	Recv() (VideoFrame, error)
}

type VideoStreamOptions struct {
	FreshKeyFrame bool
}

// StreamVideo opens a multiplexed Annex-B H.264 stream for a kart selector.
func (client *Client) StreamVideo(ctx context.Context, selector string) (VideoReceiver, error) {
	return client.StreamVideoWithOptions(ctx, selector, VideoStreamOptions{})
}

// StreamVideoWithOptions opens a video stream with explicit startup behavior.
func (client *Client) StreamVideoWithOptions(ctx context.Context, selector string, options VideoStreamOptions) (VideoReceiver, error) {
	stream, err := client.admin.StreamVideo(ctx, &adminv1.StreamVideoRequest{Selector: selector, FreshKeyFrame: options.FreshKeyFrame})
	if err != nil {
		return nil, err
	}
	return &VideoStream{stream: stream}, nil
}

// Recv blocks until the next complete video frame is available.
func (stream *VideoStream) Recv() (VideoFrame, error) {
	frame, err := stream.stream.Recv()
	if err != nil {
		return VideoFrame{}, err
	}
	return VideoFrame{DeviceID: frame.GetDeviceId(), Sequence: frame.GetSequence(),
		Metadata0: frame.GetMetadata_0(), Metadata1: frame.GetMetadata_1(), Metadata2: frame.GetMetadata_2(),
		KeyFrame: frame.GetKeyFrame(), Discontinuity: frame.GetDiscontinuity(),
		AnnexB: append([]byte(nil), frame.GetAnnexB()...)}, nil
}

// DriveState is the last teleop command for a kart.
type DriveState struct {
	DeviceID string
	Steer    float64
	Throttle float64
	// Brake controls the physical kart brake-light byte independently of throttle.
	Brake     float64
	Applied   bool
	Reason    string
	UpdatedAt time.Time
}

func (client *Client) SetDrive(
	ctx context.Context,
	selector string,
	steer, throttle, brake float64,
) (*DriveState, error) {
	response, err := client.admin.SetDrive(ctx, &adminv1.SetDriveRequest{
		Selector: selector,
		Steer:    steer,
		Throttle: throttle,
		Brake:    brake,
	})
	if err != nil {
		return nil, err
	}
	state := driveFromProto(response.GetDrive())
	return &state, nil
}

func (client *Client) GetDrive(ctx context.Context, selector string) (*DriveState, error) {
	response, err := client.admin.GetDrive(ctx, &adminv1.GetDriveRequest{Selector: selector})
	if err != nil {
		return nil, err
	}
	state := driveFromProto(response.GetDrive())
	return &state, nil
}

// SetDriveMode enables or disables teleoperation for one connected kart.
func (client *Client) SetDriveMode(ctx context.Context, selector string, enabled bool) (*Kart, error) {
	response, err := client.admin.SetDriveMode(ctx, &adminv1.SetDriveModeRequest{
		Selector: selector,
		Enabled:  enabled,
	})
	if err != nil {
		return nil, err
	}
	device := kartFromProto(response.GetDevice())
	return &device, nil
}

// ShutdownKart powers off one connected kart via Fuji Control Shutdown.
func (client *Client) ShutdownKart(ctx context.Context, selector string) (*Kart, error) {
	response, err := client.admin.ShutdownKart(ctx, &adminv1.ShutdownKartRequest{Selector: selector})
	if err != nil {
		return nil, err
	}
	device := kartFromProto(response.GetDevice())
	return &device, nil
}

func driveFromProto(state *adminv1.DriveState) DriveState {
	if state == nil {
		return DriveState{}
	}
	out := DriveState{
		DeviceID: state.GetDeviceId(),
		Steer:    state.GetSteer(),
		Throttle: state.GetThrottle(),
		Brake:    state.GetBrake(),
		Applied:  state.GetApplied(),
		Reason:   state.GetReason(),
	}
	if state.GetUpdatedAt() != "" {
		out.UpdatedAt, _ = time.Parse(time.RFC3339Nano, state.GetUpdatedAt())
	}
	return out
}

// AwaitPairing blocks until the daemon leaves pairing mode, polling status. It
// returns nil when pairing completes (daemon back in normal mode), or an error
// if it is canceled or ends in an unexpected state.
func (c *Client) AwaitPairing(ctx context.Context) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		st, err := c.Status(ctx)
		if err != nil {
			if ctx.Err() != nil {
				continue
			}
			return err
		}
		switch st.Mode {
		case ModePairing:
		case ModeNormal:
			return nil
		default:
			return fmt.Errorf("pairing ended with daemon in %s mode", st.Mode)
		}
	}
}

func parseAddress(addr string) (network, address string, err error) {
	if addr == "" {
		addr = DefaultAddress
	}
	switch {
	case strings.HasPrefix(addr, "unix:"):
		return "unix", strings.TrimPrefix(addr, "unix:"), nil
	case strings.HasPrefix(addr, "tcp:"):
		return "tcp", strings.TrimPrefix(addr, "tcp:"), nil
	case strings.HasPrefix(addr, "/"), strings.HasPrefix(addr, "."), strings.HasPrefix(addr, "@"):
		return "unix", addr, nil
	default:
		if _, _, e := net.SplitHostPort(addr); e == nil {
			return "tcp", addr, nil
		}
		return "unix", addr, nil
	}
}

func statusFromProto(state adminv1.State, w *adminv1.WirelessInfo, p *adminv1.PairingRecord) *Status {
	return &Status{
		Mode:     modeFromProto(state),
		Wireless: wirelessFromProto(w),
		Pairing:  pairingFromProto(p),
	}
}

func modeFromProto(state adminv1.State) Mode {
	switch state {
	case adminv1.State_STATE_RUNNING:
		return ModeNormal
	case adminv1.State_STATE_PAIRING:
		return ModePairing
	case adminv1.State_STATE_DOWN:
		return ModeStopped
	default:
		return ModeUnknown
	}
}

func wirelessFromProto(w *adminv1.WirelessInfo) *Wireless {
	if w == nil {
		return nil
	}
	return &Wireless{
		Interface:   w.Interface,
		Address:     w.Address,
		Subnet:      w.Subnet,
		Channel:     uint16(w.Channel),
		SSID:        w.Ssid,
		HostapdPath: w.HostapdPath,
	}
}

func pairingFromProto(p *adminv1.PairingRecord) *Pairing {
	if p == nil {
		return nil
	}
	return &Pairing{
		SSID:        p.Ssid,
		Channel:     uint16(p.Channel),
		Seed:        p.SeedHex,
		GeneratedAt: p.GeneratedAt,
	}
}

func kartFromProto(d *adminv1.DeviceSummary) Kart {
	k := Kart{
		Kind:       d.Kind,
		Ident:      d.Ident,
		Serial:     d.Serial,
		Address:    d.Address,
		MACAddress: d.MacAddress,
	}
	if d.SignalDbm != nil {
		v := int(d.GetSignalDbm())
		k.SignalDBM = &v
	}
	if d.Battery != nil {
		v := int(d.GetBattery())
		k.Battery = &v
	}
	if d.CableConnected != nil {
		v := d.GetCableConnected()
		k.CableConnected = &v
	}
	k.DriveArmed = d.GetDriveArmed()
	if value := d.GetAccelMps2(); value != nil {
		k.AccelMPS2 = &Vec3{X: value.X, Y: value.Y, Z: value.Z}
	}
	if value := d.GetGyroDps(); value != nil {
		k.GyroDPS = &Vec3{X: value.X, Y: value.Y, Z: value.Z}
	}
	if value := d.GetOrientation(); value != nil {
		k.Orientation = &Quat{I: value.I, J: value.J, K: value.K, R: value.R}
	}
	if d.ImuTimerUs != nil {
		v := d.GetImuTimerUs()
		k.IMUTimerUs = &v
	}
	return k
}
