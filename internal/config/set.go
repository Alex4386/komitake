package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Alex4386/komitake/internal/wireless"
)

// SettingsChanges lists operator-editable settings. Only non-nil fields are
// patched; everything else in config.json is preserved.
type SettingsChanges struct {
	WebBind        *string
	WebTLSEnabled  *bool
	WebTLSCertFile *string
	WebTLSKeyFile  *string
	SocketBind     *string
	SocketChmod    *string

	WirelessInterface   *string
	WirelessAddress     *string
	WirelessChannel     *string
	WirelessHostapdPath *string
	WirelessSSID        *string
	WirelessPSK         *string

	PairingFile *string
	Autostart   *bool

	VideoHwaccel          *string
	VideoFFmpegPath       *string
	VideoFFmpegProfile    *string
	VideoFFmpegArgsInput  *[]string
	VideoFFmpegArgsOutput *[]string

	RCDName *string

	// Secret is written to the sibling secret file (mode 0600), not config.json.
	Secret *string
}

// ApplySettingsChanges patches config.json atomically, migrates legacy keys,
// validates the result, and optionally updates the secret file.
func ApplySettingsChanges(configPath string, changes SettingsChanges) error {
	if changes == (SettingsChanges{}) {
		return fmt.Errorf("no settings to apply")
	}

	path, err := resolveExistingConfigPath(configPath)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}
	if raw == nil {
		raw = map[string]any{}
	}
	migrateRawConfig(raw)
	existing, err := decodeFileFromRaw(raw)
	if err != nil {
		return err
	}

	if err := validateSettingsChanges(existing, changes); err != nil {
		return err
	}

	applySettingsChanges(raw, changes)

	file, err := decodeFileFromRaw(raw)
	if err != nil {
		return err
	}
	if err := resolveSecret(&file, path); err != nil {
		return err
	}
	if changes.Secret != nil {
		if pending := strings.TrimSpace(*changes.Secret); pending != "" {
			file.Secret = pending
		} else {
			file.Secret = ""
		}
	}
	if _, err := BuildRuntime(file, path, Options{}); err != nil {
		return err
	}

	encoded, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	perm := os.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		perm = info.Mode().Perm()
	}
	if err := writeFileAtomic(path, append(encoded, '\n'), perm); err != nil {
		return err
	}

	if changes.Secret != nil {
		secretPath := DefaultSecretFile(path)
		secret := strings.TrimSpace(*changes.Secret)
		if secret == "" {
			if err := os.Remove(secretPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove secret file: %w", err)
			}
			return nil
		}
		if err := writeFileAtomic(secretPath, append([]byte(secret), '\n'), 0o600); err != nil {
			return fmt.Errorf("write secret file: %w", err)
		}
	}
	return nil
}

func validateSettingsChanges(existing File, changes SettingsChanges) error {
	if changes.WebBind != nil {
		value := strings.TrimSpace(*changes.WebBind)
		if value != "" {
			if err := validateWebAddr(value); err != nil {
				return err
			}
		}
	}
	if changes.WebTLSEnabled != nil || changes.WebTLSCertFile != nil || changes.WebTLSKeyFile != nil {
		tls := existing.Web.TLS
		if changes.WebTLSEnabled != nil {
			tls.Enabled = *changes.WebTLSEnabled
		}
		if changes.WebTLSCertFile != nil {
			tls.CertFile = strings.TrimSpace(*changes.WebTLSCertFile)
		}
		if changes.WebTLSKeyFile != nil {
			tls.KeyFile = strings.TrimSpace(*changes.WebTLSKeyFile)
		}
		if err := validateWebTLS(tls); err != nil {
			return err
		}
	}
	if changes.SocketBind != nil {
		value := strings.TrimSpace(*changes.SocketBind)
		if value != "" {
			if _, err := ParseIPCAddress(value); err != nil {
				return fmt.Errorf("socket.bind: %w", err)
			}
		}
	}
	if changes.SocketChmod != nil {
		value := strings.TrimSpace(*changes.SocketChmod)
		if value != "" {
			if _, err := ParseSocketPerms(value); err != nil {
				return err
			}
		}
	}
	if changes.WirelessAddress != nil {
		value := strings.TrimSpace(*changes.WirelessAddress)
		if value != "" {
			iface := existing.Wireless.Interface
			if changes.WirelessInterface != nil {
				iface = strings.TrimSpace(*changes.WirelessInterface)
			}
			if iface == "" {
				iface = "wlan0"
			}
			if _, _, err := buildNetwork(WirelessFile{Interface: iface, Address: value}); err != nil {
				return err
			}
		}
	}
	if changes.WirelessChannel != nil {
		value := strings.TrimSpace(*changes.WirelessChannel)
		if value != "" {
			if _, err := parseWirelessChannel(value); err != nil {
				return err
			}
		}
	}
	if changes.WirelessPSK != nil {
		value := strings.TrimSpace(*changes.WirelessPSK)
		if value != "" {
			if _, err := decodeHex("wireless.psk", value, wireless.PSKSize); err != nil {
				return err
			}
		}
	}
	if changes.VideoHwaccel != nil {
		if err := ValidateVideoHwaccel(strings.ToLower(strings.TrimSpace(*changes.VideoHwaccel))); err != nil {
			return err
		}
	}
	if changes.VideoFFmpegPath != nil {
		value := strings.TrimSpace(*changes.VideoFFmpegPath)
		if value != "" {
			if _, err := os.Stat(value); err != nil {
				return fmt.Errorf("video.ffmpeg_path %q: %w", value, err)
			}
		}
	}
	if changes.VideoFFmpegProfile != nil {
		if err := ValidateVideoFFmpegProfile(strings.ToLower(strings.TrimSpace(*changes.VideoFFmpegProfile))); err != nil {
			return err
		}
	}
	if changes.VideoHwaccel != nil || changes.VideoFFmpegPath != nil || changes.VideoFFmpegProfile != nil || changes.VideoFFmpegArgsInput != nil || changes.VideoFFmpegArgsOutput != nil {
		merged := existing.Video
		if changes.VideoHwaccel != nil {
			merged.Hwaccel = strings.ToLower(strings.TrimSpace(*changes.VideoHwaccel))
		}
		if changes.VideoFFmpegPath != nil {
			merged.FFmpegPath = strings.TrimSpace(*changes.VideoFFmpegPath)
		}
		if changes.VideoFFmpegProfile != nil {
			merged.FFmpegProfile = strings.ToLower(strings.TrimSpace(*changes.VideoFFmpegProfile))
		}
		if changes.VideoFFmpegArgsInput != nil {
			merged.FFmpegArgs.Input = append([]string(nil), (*changes.VideoFFmpegArgsInput)...)
		}
		if changes.VideoFFmpegArgsOutput != nil {
			merged.FFmpegArgs.Output = append([]string(nil), (*changes.VideoFFmpegArgsOutput)...)
		}
		if err := ValidateVideo(merged); err != nil {
			return err
		}
	}
	if (changes.WirelessSSID != nil) != (changes.WirelessPSK != nil) {
		return fmt.Errorf("wireless.ssid and wireless.psk must be set together")
	}
	return nil
}

func parseWirelessChannel(raw string) (uint16, error) {
	value, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 16)
	if err != nil || value == 0 {
		return 0, fmt.Errorf("wireless.channel must be a positive integer")
	}
	return uint16(value), nil
}

func migrateRawConfig(raw map[string]any) {
	legacyAddress, _ := raw["address"].(string)
	wirelessObject, _ := raw["wireless"].(map[string]any)
	if wirelessObject == nil {
		wirelessObject = map[string]any{}
	}
	legacySubnet, _ := wirelessObject["subnet"].(string)
	currentAddress, _ := wirelessObject["address"].(string)
	if strings.TrimSpace(currentAddress) == "" {
		if migrated := migrateWirelessAddress(legacyAddress, legacySubnet); migrated != "" {
			wirelessObject["address"] = migrated
		}
	}
	delete(wirelessObject, "subnet")
	if len(wirelessObject) > 0 {
		raw["wireless"] = wirelessObject
	}
	for _, legacyKey := range []string{"address", "listen", "web_addr", "socket_perms"} {
		delete(raw, legacyKey)
	}
	if socketObject, ok := raw["socket"].(map[string]any); ok {
		delete(socketObject, "perm")
		if len(socketObject) == 0 {
			delete(raw, "socket")
		}
	}
}

func applySettingsChanges(raw map[string]any, changes SettingsChanges) {
	if changes.WebBind != nil {
		setNestedString(raw, []string{"web", "bind"}, strings.TrimSpace(*changes.WebBind))
	}
	if changes.WebTLSEnabled != nil || changes.WebTLSCertFile != nil || changes.WebTLSKeyFile != nil {
		webObject, _ := raw["web"].(map[string]any)
		if webObject == nil {
			webObject = map[string]any{}
		}
		tlsObject, _ := webObject["tls"].(map[string]any)
		if tlsObject == nil {
			tlsObject = map[string]any{}
		}
		if changes.WebTLSEnabled != nil {
			if *changes.WebTLSEnabled {
				tlsObject["enabled"] = true
			} else {
				delete(tlsObject, "enabled")
			}
		}
		if changes.WebTLSCertFile != nil {
			setMapString(tlsObject, "cert_file", strings.TrimSpace(*changes.WebTLSCertFile))
		}
		if changes.WebTLSKeyFile != nil {
			setMapString(tlsObject, "key_file", strings.TrimSpace(*changes.WebTLSKeyFile))
		}
		if len(tlsObject) == 0 {
			delete(webObject, "tls")
		} else {
			webObject["tls"] = tlsObject
		}
		raw["web"] = webObject
	}
	if changes.SocketBind != nil {
		setNestedString(raw, []string{"socket", "bind"}, strings.TrimSpace(*changes.SocketBind))
	}
	if changes.SocketChmod != nil {
		value := strings.TrimSpace(*changes.SocketChmod)
		if value != "" {
			perm, err := ParseSocketPerms(value)
			if err == nil {
				value = FormatSocketPerms(perm)
			}
		}
		setNestedString(raw, []string{"socket", "chmod"}, value)
	}
	if changes.WirelessInterface != nil {
		setNestedString(raw, []string{"wireless", "interface"}, strings.TrimSpace(*changes.WirelessInterface))
	}
	if changes.WirelessAddress != nil {
		setNestedString(raw, []string{"wireless", "address"}, strings.TrimSpace(*changes.WirelessAddress))
	}
	if changes.WirelessChannel != nil {
		value := strings.TrimSpace(*changes.WirelessChannel)
		if value == "" {
			deleteNested(raw, []string{"wireless", "channel"})
		} else if channel, err := parseWirelessChannel(value); err == nil {
			setNestedNumber(raw, []string{"wireless", "channel"}, float64(channel))
		}
	}
	if changes.WirelessHostapdPath != nil {
		setNestedString(raw, []string{"wireless", "hostapd_path"}, strings.TrimSpace(*changes.WirelessHostapdPath))
	}
	if changes.WirelessSSID != nil {
		setNestedString(raw, []string{"wireless", "ssid"}, strings.TrimSpace(*changes.WirelessSSID))
	}
	if changes.WirelessPSK != nil {
		setNestedString(raw, []string{"wireless", "psk"}, strings.TrimSpace(*changes.WirelessPSK))
	}
	if changes.PairingFile != nil {
		setTopLevelString(raw, "pairing_file", strings.TrimSpace(*changes.PairingFile))
	}
	if changes.Autostart != nil {
		raw["autostart"] = *changes.Autostart
	}
	if changes.VideoHwaccel != nil {
		setNestedString(raw, []string{"video", "hwaccel"}, strings.ToLower(strings.TrimSpace(*changes.VideoHwaccel)))
	}
	if changes.VideoFFmpegPath != nil {
		setNestedString(raw, []string{"video", "ffmpeg_path"}, strings.TrimSpace(*changes.VideoFFmpegPath))
	}
	if changes.VideoFFmpegProfile != nil {
		setNestedString(raw, []string{"video", "ffmpeg_profile"}, strings.ToLower(strings.TrimSpace(*changes.VideoFFmpegProfile)))
	}
	if changes.VideoFFmpegArgsInput != nil || changes.VideoFFmpegArgsOutput != nil {
		videoObject := ensureNestedMap(raw, []string{"video"})
		argsObject, _ := videoObject["ffmpeg_args"].(map[string]any)
		if argsObject == nil {
			argsObject = map[string]any{}
		}
		if changes.VideoFFmpegArgsInput != nil {
			if len(*changes.VideoFFmpegArgsInput) == 0 {
				delete(argsObject, "input")
			} else {
				argsObject["input"] = *changes.VideoFFmpegArgsInput
			}
		}
		if changes.VideoFFmpegArgsOutput != nil {
			if len(*changes.VideoFFmpegArgsOutput) == 0 {
				delete(argsObject, "output")
			} else {
				argsObject["output"] = *changes.VideoFFmpegArgsOutput
			}
		}
		if len(argsObject) == 0 {
			delete(videoObject, "ffmpeg_args")
		} else {
			videoObject["ffmpeg_args"] = argsObject
		}
	}
	if changes.RCDName != nil {
		setNestedString(raw, []string{"rcd", "name"}, strings.TrimSpace(*changes.RCDName))
	}
}

func decodeFileFromRaw(raw map[string]any) (File, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return File{}, err
	}
	var file File
	if err := json.Unmarshal(data, &file); err != nil {
		return File{}, fmt.Errorf("decode config: %w", err)
	}
	file.normalizeLegacy()
	return file, nil
}

func setTopLevelString(raw map[string]any, key, value string) {
	if value == "" {
		delete(raw, key)
		return
	}
	raw[key] = value
}

func setNestedString(raw map[string]any, path []string, value string) {
	object := ensureNestedMap(raw, path[:len(path)-1])
	key := path[len(path)-1]
	setMapString(object, key, value)
	if len(object) == 0 {
		deleteNested(raw, path[:len(path)-1])
	}
}

func setNestedNumber(raw map[string]any, path []string, value float64) {
	object := ensureNestedMap(raw, path[:len(path)-1])
	object[path[len(path)-1]] = value
}

func setMapString(object map[string]any, key, value string) {
	if value == "" {
		delete(object, key)
		return
	}
	object[key] = value
}

func deleteNested(raw map[string]any, path []string) {
	if len(path) == 0 {
		return
	}
	if len(path) == 1 {
		delete(raw, path[0])
		return
	}
	object, ok := raw[path[0]].(map[string]any)
	if !ok {
		return
	}
	deleteNested(object, path[1:])
	if len(object) == 0 {
		delete(raw, path[0])
	}
}

func ensureNestedMap(raw map[string]any, path []string) map[string]any {
	current := raw
	for _, key := range path {
		next, ok := current[key].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[key] = next
		}
		current = next
	}
	return current
}
