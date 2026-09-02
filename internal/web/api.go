// Package web serves komitake's REST API and the bundled frontend, driving the
// daemon through the public pkg/komitake client SDK.
package web

import (
	"context"
	"net/http"

	"github.com/Alex4386/komitake/pkg/komitake"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

// Client is the subset of the komitake SDK the API needs. It is an interface so
// the API can be tested without a live daemon.
type Client interface {
	Status(ctx context.Context) (*komitake.Status, error)
	Karts(ctx context.Context) ([]komitake.Kart, error)
	StartPairing(ctx context.Context) (*komitake.Pairing, error)
	StopPairing(ctx context.Context) error
	AwaitPairing(ctx context.Context) error
	SetDrive(ctx context.Context, selector string, steer, throttle, brake float64) (*komitake.DriveState, error)
	GetDrive(ctx context.Context, selector string) (*komitake.DriveState, error)
	SetDriveMode(ctx context.Context, selector string, enabled bool) (*komitake.Kart, error)
	ShutdownKart(ctx context.Context, selector string) (*komitake.Kart, error)
	StreamVideo(ctx context.Context, selector string) (komitake.VideoReceiver, error)
	StreamVideoWithOptions(ctx context.Context, selector string, options komitake.VideoStreamOptions) (komitake.VideoReceiver, error)
	ReloadDaemon(ctx context.Context) error
	RestartDaemon(ctx context.Context) error
}

// RegisterAPI mounts the REST API on the given mux, backed by client. The API
// is composed as a hierarchy of plugins rooted at /v1.
func RegisterAPI(mux *http.ServeMux, client Client, options ...Options) huma.API {
	config := huma.DefaultConfig("Komitake", "1.0.0")
	// Disable huma's built-in Stoplight Elements page; we serve Scalar at /docs
	// instead (see registerDocs).
	config.DocsPath = ""
	api := humago.New(mux, config)
	resolvedOptions := Options{}
	if len(options) > 0 {
		resolvedOptions = options[0]
	}
	Mount(api, rootPlugins(client, resolvedOptions)...)
	registerKartSerialRedirects(mux, client)
	registerDocs(mux)
	return api
}

// rootPlugins is the top-level plugin tree. Everything hangs off the /v1
// version plugin; add sibling version plugins here to introduce a /v2.
func rootPlugins(client Client, options Options) []Plugin {
	return []Plugin{
		&versionPlugin{
			prefix: "/v1",
			children: []Plugin{
				&statusPlugin{client: client},
				&kartsPlugin{client: client},
				&settingsPlugin{configPath: options.ConfigPath},
				&daemonPlugin{client: client},
			},
		},
	}
}

// versionPlugin groups an API version and mounts its children under the version
// prefix.
type versionPlugin struct {
	prefix   string
	children []Plugin
}

func (v *versionPlugin) Prefix() string      { return v.prefix }
func (v *versionPlugin) Register(a huma.API) { Mount(a, v.children...) }

type kartDTO struct {
	Kind           string   `json:"kind" doc:"Device kind, always Fuji"`
	Ident          string   `json:"ident" doc:"RCD identity of the kart"`
	Serial         string   `json:"serial,omitempty" doc:"Device-reported serial number, when known"`
	Address        string   `json:"address" doc:"IP address on the AP subnet"`
	MACAddress     string   `json:"mac_address" doc:"Station MAC address"`
	SignalDBM      *int     `json:"signal_dbm,omitempty" doc:"Station signal in dBm, if known"`
	Battery        *int     `json:"battery,omitempty" doc:"Kart HUD battery bars (0-4)"`
	CableConnected *bool    `json:"cable_connected,omitempty" doc:"Charging cable connection from status telemetry"`
	DriveArmed     bool     `json:"drive_armed" doc:"Local daemon drive-session state"`
	AccelMPS2      *vec3DTO `json:"accel_mps2,omitempty"`
	GyroDPS        *vec3DTO `json:"gyro_dps,omitempty"`
	Orientation    *quatDTO `json:"orientation,omitempty"`
	IMUTimerUs     *uint32  `json:"imu_timer_us,omitempty"`
}
type vec3DTO struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}
type quatDTO struct {
	I float64 `json:"i"`
	J float64 `json:"j"`
	K float64 `json:"k"`
	R float64 `json:"r"`
}

type statusDTO struct {
	Mode     string       `json:"mode" doc:"Daemon mode: stopped, normal, or pairing"`
	Wireless *wirelessDTO `json:"wireless,omitempty"`
	Pairing  *pairingDTO  `json:"pairing,omitempty"`
}

type wirelessDTO struct {
	Interface string `json:"interface,omitempty"`
	Address   string `json:"address,omitempty"`
	Subnet    string `json:"subnet,omitempty"`
	Channel   uint16 `json:"channel,omitempty"`
	SSID      string `json:"ssid,omitempty"`
}

type pairingDTO struct {
	SSID      string `json:"ssid"`
	Channel   uint16 `json:"channel"`
	QRPayload []byte `json:"qr_payload" doc:"Raw bytes to encode as the pairing QR code"`
}

func daemonError(err error) error {
	return huma.Error502BadGateway("cannot reach the komitake daemon", err)
}

func statusToDTO(s *komitake.Status) statusDTO {
	out := statusDTO{Mode: string(s.Mode)}
	if s.Wireless != nil {
		out.Wireless = &wirelessDTO{
			Interface: s.Wireless.Interface,
			Address:   s.Wireless.Address,
			Subnet:    s.Wireless.Subnet,
			Channel:   s.Wireless.Channel,
			SSID:      s.Wireless.SSID,
		}
	}
	if s.Pairing != nil {
		if dto, err := pairingToDTO(s.Pairing); err == nil {
			out.Pairing = &dto
		}
	}
	return out
}

func kartToDTO(k komitake.Kart) kartDTO {
	out := kartDTO{Kind: k.Kind, Ident: k.Ident, Serial: k.Serial, Address: k.Address, MACAddress: k.MACAddress, SignalDBM: k.SignalDBM, Battery: k.Battery, CableConnected: k.CableConnected, DriveArmed: k.DriveArmed, IMUTimerUs: k.IMUTimerUs}
	if k.AccelMPS2 != nil {
		out.AccelMPS2 = &vec3DTO{X: k.AccelMPS2.X, Y: k.AccelMPS2.Y, Z: k.AccelMPS2.Z}
	}
	if k.GyroDPS != nil {
		out.GyroDPS = &vec3DTO{X: k.GyroDPS.X, Y: k.GyroDPS.Y, Z: k.GyroDPS.Z}
	}
	if k.Orientation != nil {
		out.Orientation = &quatDTO{I: k.Orientation.I, J: k.Orientation.J, K: k.Orientation.K, R: k.Orientation.R}
	}
	return out
}

func pairingToDTO(p *komitake.Pairing) (pairingDTO, error) {
	payload, err := p.QRPayload()
	if err != nil {
		return pairingDTO{}, err
	}
	return pairingDTO{SSID: p.SSID, Channel: p.Channel, QRPayload: payload}, nil
}
