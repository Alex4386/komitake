package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

// ServiceSettings are safe, operator-editable settings persisted in config.json.
type ServiceSettings struct {
	Web    WebFile    `json:"web"`
	Socket SocketFile `json:"socket"`
	Video  VideoFile  `json:"video"`
	WebRTC WebRTCFile `json:"webrtc"`
	// AllowConfig reports whether editing settings from the Web UI is permitted.
	// It is read-only over the API (never accepted on write) so a locked-down
	// deployment cannot be unlocked from the browser.
	AllowConfig bool                    `json:"allow_config"`
	General     GeneralSettings         `json:"general"`
	Wireless    WirelessSettings        `json:"wireless"`
	ConfigPath  string                  `json:"config_path,omitempty"`
	Defaults    ServiceSettingsDefaults `json:"defaults"`
}

// GeneralSettings are operator-editable top-level daemon settings.
type GeneralSettings struct {
	Autostart   bool   `json:"autostart"`
	RCDName     string `json:"rcd_name,omitempty"`
	PairingFile string `json:"pairing_file,omitempty"`
}

// WirelessSettings are the non-secret, operator-editable wireless fields.
// SSID and PSK are intentionally excluded (PSK is effectively the network key).
type WirelessSettings struct {
	Interface   string `json:"interface,omitempty"`
	Address     string `json:"address,omitempty"`
	Channel     uint16 `json:"channel,omitempty"`
	HostapdPath string `json:"hostapd_path,omitempty"`
}

type ServiceSettingsDefaults struct {
	Web    WebFile    `json:"web"`
	Socket SocketFile `json:"socket"`
	Video  VideoFile  `json:"video"`
	WebRTC WebRTCFile `json:"webrtc"`
}

func ParseSocketPerms(raw string) (os.FileMode, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return DefaultSocketPerm, nil
	}
	value = strings.TrimPrefix(strings.ToLower(value), "0o")
	value = strings.TrimPrefix(value, "0")
	if value == "" {
		value = "0"
	}
	parsed, err := strconv.ParseUint(value, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("socket.chmod %q: %w", raw, err)
	}
	if parsed > 0o777 {
		return 0, fmt.Errorf("socket.chmod %q out of range", raw)
	}
	return os.FileMode(parsed), nil
}

func FormatSocketPerms(perm os.FileMode) string {
	return fmt.Sprintf("%04o", perm&0o777)
}

func ResolveWebSettings(configPath string) (WebFile, error) {
	path, err := resolveExistingConfigPath(configPath)
	if err != nil {
		if configPath == "" {
			return WebFile{}, nil
		}
		return WebFile{}, err
	}
	file, err := readConfigFile(path)
	if err != nil {
		return WebFile{}, err
	}
	file.Web.Bind = strings.TrimSpace(file.Web.Bind)
	file.Web.TLS.CertFile = strings.TrimSpace(file.Web.TLS.CertFile)
	file.Web.TLS.KeyFile = strings.TrimSpace(file.Web.TLS.KeyFile)
	if err := validateWebTLS(file.Web.TLS); err != nil {
		return WebFile{}, err
	}
	return file.Web, nil
}

func ResolveWebAddr(configPath string) string {
	web, err := ResolveWebSettings(configPath)
	if err != nil {
		return ""
	}
	return web.Bind
}

func ReadServiceSettings(configPath string) (ServiceSettings, error) {
	out := ServiceSettings{Defaults: ServiceSettingsDefaults{
		Web:    WebFile{Bind: DefaultWebAddr},
		Socket: SocketFile{Bind: DefaultIPCAddress, Chmod: FormatSocketPerms(DefaultSocketPerm)},
		Video:  VideoFile{Hwaccel: VideoHwaccelAuto},
	}}
	path, err := resolveExistingConfigPath(configPath)
	if err != nil {
		return out, err
	}
	out.ConfigPath = path
	file, err := readConfigFile(path)
	if err != nil {
		return out, err
	}
	out.Web.Bind = strings.TrimSpace(file.Web.Bind)
	out.Web.TLS.Enabled = file.Web.TLS.Enabled
	out.Web.TLS.CertFile = strings.TrimSpace(file.Web.TLS.CertFile)
	out.Web.TLS.KeyFile = strings.TrimSpace(file.Web.TLS.KeyFile)
	out.Socket.Bind = strings.TrimSpace(file.Socket.Bind)
	out.Socket.Chmod = strings.TrimSpace(file.Socket.Chmod)
	out.Video = file.Video
	if out.Video.NormalizedHwaccel() == "" {
		out.Video.Hwaccel = VideoHwaccelAuto
	}
	out.WebRTC.STUNServers = file.WebRTC.NormalizedSTUNServers()
	out.AllowConfig = file.Web.WebAllowConfig()
	autostart := true
	if file.Autostart != nil {
		autostart = *file.Autostart
	}
	out.General = GeneralSettings{
		Autostart:   autostart,
		RCDName:     strings.TrimSpace(file.RCD.Name),
		PairingFile: strings.TrimSpace(file.PairingFile),
	}
	out.Wireless = WirelessSettings{
		Interface:   strings.TrimSpace(file.Wireless.Interface),
		Address:     strings.TrimSpace(file.Wireless.Address),
		Channel:     file.Wireless.Channel,
		HostapdPath: strings.TrimSpace(file.Wireless.HostapdPath),
	}
	return out, nil
}

// WriteServiceSettings patches web.*, socket.*, and video.* while preserving
// secrets, wireless/RCD configuration, and unknown future fields. Legacy keys
// are migrated and removed when the file is rewritten.
func WriteServiceSettings(configPath string, web WebFile, socket SocketFile, video VideoFile, webrtc WebRTCFile, general *GeneralSettings, wireless *WirelessSettings) (ServiceSettings, error) {
	path, err := resolveExistingConfigPath(configPath)
	if err != nil {
		return ServiceSettings{}, err
	}
	// Enforce the lock server-side: a config with allow_config=false cannot be
	// edited through this API, and the flag itself is never writable here.
	existing, err := readConfigFile(path)
	if err != nil {
		return ServiceSettings{}, err
	}
	if !existing.Web.WebAllowConfig() {
		return ServiceSettings{}, fmt.Errorf("web configuration editing is disabled (web.allow_config is false)")
	}
	webBind := strings.TrimSpace(web.Bind)
	webTLSEnabled := web.TLS.Enabled
	webTLSCertFile := strings.TrimSpace(web.TLS.CertFile)
	webTLSKeyFile := strings.TrimSpace(web.TLS.KeyFile)
	socketBind := strings.TrimSpace(socket.Bind)
	socketChmod := strings.TrimSpace(socket.Chmod)
	if socketChmod != "" {
		perm, err := ParseSocketPerms(socketChmod)
		if err != nil {
			return ServiceSettings{}, err
		}
		socketChmod = FormatSocketPerms(perm)
	}

	hwaccel := video.NormalizedHwaccel()
	ffmpegPath := strings.TrimSpace(video.FFmpegPath)
	ffmpegProfile := video.NormalizedFFmpegProfile()
	ffmpegArgsInput := append([]string(nil), video.FFmpegArgs.Input...)
	ffmpegArgsOutput := append([]string(nil), video.FFmpegArgs.Output...)
	stunServers := webrtc.NormalizedSTUNServers()
	changes := SettingsChanges{
		WebBind:               &webBind,
		WebTLSEnabled:         &webTLSEnabled,
		WebTLSCertFile:        &webTLSCertFile,
		WebTLSKeyFile:         &webTLSKeyFile,
		SocketBind:            &socketBind,
		SocketChmod:           &socketChmod,
		VideoHwaccel:          &hwaccel,
		VideoFFmpegPath:       &ffmpegPath,
		VideoFFmpegProfile:    &ffmpegProfile,
		VideoFFmpegArgsInput:  &ffmpegArgsInput,
		VideoFFmpegArgsOutput: &ffmpegArgsOutput,
		WebRTCSTUNServers:     &stunServers,
	}
	// General and wireless are only patched when the caller provides them, so a
	// partial PUT never clobbers existing values it did not send.
	if general != nil {
		autostart := general.Autostart
		rcdName := strings.TrimSpace(general.RCDName)
		pairingFile := strings.TrimSpace(general.PairingFile)
		changes.Autostart = &autostart
		changes.RCDName = &rcdName
		changes.PairingFile = &pairingFile
	}
	if wireless != nil {
		wirelessInterface := strings.TrimSpace(wireless.Interface)
		wirelessAddress := strings.TrimSpace(wireless.Address)
		wirelessChannel := ""
		if wireless.Channel != 0 {
			wirelessChannel = strconv.FormatUint(uint64(wireless.Channel), 10)
		}
		wirelessHostapdPath := strings.TrimSpace(wireless.HostapdPath)
		changes.WirelessInterface = &wirelessInterface
		changes.WirelessAddress = &wirelessAddress
		changes.WirelessChannel = &wirelessChannel
		changes.WirelessHostapdPath = &wirelessHostapdPath
	}
	if err := ApplySettingsChanges(path, changes); err != nil {
		return ServiceSettings{}, err
	}
	return ReadServiceSettings(path)
}

func resolveExistingConfigPath(explicit string) (string, error) {
	if explicit != "" {
		if _, err := os.Stat(explicit); err != nil {
			return "", err
		}
		return explicit, nil
	}
	return ResolveConfigPath("")
}

func validateWebTLS(webTLS WebTLSFile) error {
	hasCertFile := strings.TrimSpace(webTLS.CertFile) != ""
	hasKeyFile := strings.TrimSpace(webTLS.KeyFile) != ""
	if hasCertFile != hasKeyFile {
		return fmt.Errorf("web.tls.cert_file and web.tls.key_file must be set together")
	}
	return nil
}

func validateWebAddr(address string) error {
	if strings.HasPrefix(address, ":") {
		if strings.TrimPrefix(address, ":") == "" {
			return fmt.Errorf("web.bind %q missing port", address)
		}
		return nil
	}
	if _, _, err := net.SplitHostPort(address); err != nil {
		return fmt.Errorf("web.bind %q: %w", address, err)
	}
	return nil
}
