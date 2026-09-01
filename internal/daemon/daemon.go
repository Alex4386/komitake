// Package daemon runs komitake's connectivity control plane: it brings up the
// LP2P access point, pairs karts, and holds their RCD handshake connections
// open. It also receives telemetry and sends kart drive control.
package daemon

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/Alex4386/komitake/internal/config"
	"github.com/Alex4386/komitake/internal/fuji"
	"github.com/Alex4386/komitake/internal/rcd"
	"github.com/Alex4386/komitake/internal/wireless"
)

const (
	// PairingPort accepts the kart's handshake while in pairing mode.
	PairingPort = 5201
	// HandshakePort accepts the kart's handshake while in normal mode.
	HandshakePort = 5202
)

type State string

const (
	StateDown    State = "DOWN"
	StateRunning State = "RUNNING"
	StatePairing State = "PAIRING"
)

type PairingRecord struct {
	State       State  `json:"state"`
	SeedHex     string `json:"seed,omitempty"`
	SSID        string `json:"ssid,omitempty"`
	Channel     uint16 `json:"channel,omitempty"`
	GeneratedAt string `json:"generated_at,omitempty"`
	FilePath    string `json:"file_path,omitempty"`
}

// WirelessInfo is non-secret AP/runtime networking for operators. Exposed via
// status so config.json can stay unreadable for the secret without blocking
// diagnostics.
type WirelessInfo struct {
	Interface   string `json:"interface,omitempty"`
	Address     string `json:"address,omitempty"`
	Subnet      string `json:"subnet,omitempty"`
	Channel     uint16 `json:"channel,omitempty"`
	SSID        string `json:"ssid,omitempty"`
	HostapdPath string `json:"hostapd_path,omitempty"`
}

// DeviceSummary describes a kart whose handshake is currently held open.
// Serial is filled from Fuji GetParam("product_code") once the control port
// answers; it may be empty briefly after connect.
type DeviceSummary struct {
	Kind  string `json:"kind"`
	Ident string `json:"ident"`
	// Serial is the serial number reported by the device (product_code).
	Serial     string `json:"serial,omitempty"`
	Address    string `json:"address"`
	MACAddress string `json:"mac_address"`
	// SignalDBM is the station's last-reported signal in dBm, from the AP. Nil
	// when unavailable (no AP, unknown station, or not on Linux).
	SignalDBM      *int               `json:"signal_dbm,omitempty"`
	Battery        *int               `json:"battery,omitempty"`
	CableConnected *bool              `json:"cable_connected,omitempty"`
	DriveArmed     bool               `json:"drive_armed"`
	IMU            *fuji.IMUTelemetry `json:"-"`
}

// deviceRecord is the internal, connectivity-only view of a connected kart.
type deviceRecord struct {
	kind           string
	ident          string
	serial         string
	address        string
	mac            string
	battery        *int
	cableConnected *bool
	armed          bool
	imu            *fuji.IMUTelemetry
}

func (d deviceRecord) summary() DeviceSummary {
	return DeviceSummary{
		Kind:           d.kind,
		Ident:          d.ident,
		Serial:         d.serial,
		Address:        d.address,
		MACAddress:     d.mac,
		Battery:        d.battery,
		CableConnected: d.cableConnected,
		DriveArmed:     d.armed,
		IMU:            d.imu,
	}
}

// pairer performs the pairing write. It is an interface so tests can inject a
// failing implementation without a real kart.
type pairer interface {
	Pair(context.Context, *rcd.Device, wireless.GroupInfo) error
}

type fujiPairer struct{}

func (fujiPairer) Pair(ctx context.Context, device *rcd.Device, group wireless.GroupInfo) error {
	return fuji.Pair(ctx, device, group)
}

type Manager struct {
	cfg           config.Runtime
	connector     pairer
	control       controlDialer
	newDrive      func(host string) (driveSender, error)
	newTranscoder func(context.Context, string, *videoHub, *slog.Logger) (videoEncoder, error)
	videoProfile  VideoProfile
	listen        func(network string, address string) (net.Listener, error)
	listenPacket  func(network string, address string) (net.PacketConn, error)
	ap            *wireless.AccessPoint
	logger        *slog.Logger
	events        Observer

	mu      sync.RWMutex
	state   State
	pairing *PairingRecord
	devices map[string]deviceRecord
	// controls holds one live Fuji control session per connected kart, keyed by
	// ident. The session must stay open for the kart's lifetime: closing it makes
	// the kart reset its network connection.
	controls map[string]*deviceControl
	video    *videoHub
	notify   chan struct{}

	runningLn    net.Listener
	pairingLn    net.Listener
	backgroundWG sync.WaitGroup
	pairingStop  context.CancelFunc

	// baseCtx bounds resources that must outlive the RPC that created them,
	// such as the hostapd process. Callers pass request-scoped contexts to
	// SetState, and those are cancelled as soon as the RPC returns.
	baseCtx    context.Context
	baseCancel context.CancelFunc
	closed     bool
}

var pairingModeTimeout = 2 * time.Minute

func NewManager(cfg config.Runtime) *Manager {
	logger := slog.Default().With("component", "daemon")
	baseCtx, baseCancel := context.WithCancel(context.Background())
	videoProfile, err := ResolveVideoProfile(cfg.Video)
	if err != nil {
		logger.Error("video hwaccel profile unavailable", "hwaccel", cfg.Video.NormalizedHwaccel(), "error", err)
	} else if videoProfile.Backend == config.VideoHwaccelNone {
		logger.Info("video transcoding disabled", "hwaccel", config.VideoHwaccelNone)
	}
	manager := &Manager{
		cfg:          cfg,
		connector:    fujiPairer{},
		control:      fujiControlDialer{},
		videoProfile: videoProfile,
		listen:       net.Listen,
		listenPacket: net.ListenPacket,
		ap:           buildAccessPoint(cfg, logger.With("subsystem", "ap")),
		logger:       logger,
		events:       NopObserver{},
		state:        StateDown,
		devices:      map[string]deviceRecord{},
		controls:     map[string]*deviceControl{},
		video:        newVideoHub(),
		notify:       make(chan struct{}),
		baseCtx:      baseCtx,
		baseCancel:   baseCancel,
	}
	manager.newTranscoder = func(ctx context.Context, deviceID string, hub *videoHub, logger *slog.Logger) (videoEncoder, error) {
		if manager.videoProfile.Backend == config.VideoHwaccelNone {
			logger.Info("video transcoding disabled", "ident", deviceID, "hwaccel", config.VideoHwaccelNone)
			return disabledVideoEncoder{}, nil
		}
		if manager.videoProfile.Backend == "" {
			profile, profileErr := ResolveVideoProfile(cfg.Video)
			if profileErr != nil {
				return nil, profileErr
			}
			if profile.Backend == config.VideoHwaccelNone {
				manager.videoProfile = profile
				logger.Info("video transcoding disabled", "ident", deviceID, "hwaccel", config.VideoHwaccelNone)
				return disabledVideoEncoder{}, nil
			}
			manager.videoProfile = profile
		}
		return startVideoTranscoder(ctx, deviceID, hub, logger, manager.videoProfile)
	}
	manager.newDrive = func(host string) (driveSender, error) {
		return fuji.NewDriveSender(host)
	}

	return manager
}

// SetObserver installs an event observer. It must be called before Start. The
// observer is the single seam for future external integrations (event streams,
// webhooks, exec hooks); the state machine already funnels every transition and
// device connect/disconnect through it.
func (m *Manager) SetObserver(o Observer) {
	if o == nil {
		o = NopObserver{}
	}
	m.mu.Lock()
	m.events = o
	m.mu.Unlock()
}

func (m *Manager) Start(ctx context.Context) error {
	m.logger.Info("starting manager", "auto_start", m.cfg.AutoStart, "address", m.cfg.Address, "pairing_file", m.cfg.PairingFile)
	if m.cfg.AutoStart {
		return m.SetState(ctx, StateRunning)
	}
	return m.persistPairingLocked(nil)
}

func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true

	m.logger.Info("stopping manager")

	_ = m.stopListenersLocked()
	m.clearDevicesLocked()
	if m.ap != nil {
		_ = m.ap.Stop()
	}
	m.stopPairingTimeoutLocked()

	// Wake any WaitForDevice callers so in-flight RPCs return instead of
	// blocking GracefulStop forever.
	m.signalLocked()
	m.mu.Unlock()

	// Cancel outside the lock: background goroutines may need it to finish.
	m.baseCancel()
	m.backgroundWG.Wait()

	m.logger.Info("manager stopped")
	return nil
}

func (m *Manager) CurrentState() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

func (m *Manager) CurrentPairing() *PairingRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.pairing == nil {
		return nil
	}
	cp := *m.pairing
	return &cp
}

func (m *Manager) CurrentWireless() *WirelessInfo {
	info := &WirelessInfo{
		Interface:   m.cfg.Wireless.Interface,
		Address:     m.cfg.Address,
		Channel:     m.cfg.GroupInfo.Channel,
		SSID:        m.cfg.GroupInfo.SSID,
		HostapdPath: m.cfg.Wireless.HostapdPath,
	}
	if m.cfg.Subnet != nil {
		info.Subnet = m.cfg.Subnet.String()
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.pairing != nil {
		info.SSID = m.pairing.SSID
		info.Channel = m.pairing.Channel
	}
	return info
}

func buildAccessPoint(cfg config.Runtime, logger *slog.Logger) *wireless.AccessPoint {
	if cfg.Wireless.Interface == "" {
		return nil
	}
	return wireless.NewAccessPoint(wireless.APConfig{
		Interface:   cfg.Wireless.Interface,
		HostapdPath: cfg.Wireless.HostapdPath,
		Subnet:      cfg.Subnet,
		Gateway:     cfg.Gateway,
		Logger:      logger,
	})
}
