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
	Web        WebFile                 `json:"web"`
	Socket     SocketFile              `json:"socket"`
	Video      VideoFile               `json:"video"`
	ConfigPath string                  `json:"config_path,omitempty"`
	Defaults   ServiceSettingsDefaults `json:"defaults"`
}

type ServiceSettingsDefaults struct {
	Web    WebFile    `json:"web"`
	Socket SocketFile `json:"socket"`
	Video  VideoFile  `json:"video"`
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
	return out, nil
}

// WriteServiceSettings patches web.*, socket.*, and video.* while preserving
// secrets, wireless/RCD configuration, and unknown future fields. Legacy keys
// are migrated and removed when the file is rewritten.
func WriteServiceSettings(configPath string, web WebFile, socket SocketFile, video VideoFile) (ServiceSettings, error) {
	path, err := resolveExistingConfigPath(configPath)
	if err != nil {
		return ServiceSettings{}, err
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
