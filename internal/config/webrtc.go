package config

import (
	"fmt"
	"strings"
)

// DefaultSTUNServer is used when webrtc.stun_servers is unset, so WebRTC video
// can traverse NATs out of the box when the viewer is off the kart's network.
const DefaultSTUNServer = "stun:stun.l.google.com:19302"

// WebRTCFile configures the browser-facing WebRTC video path. STUNServers lists
// STUN URLs (e.g. stun:stun.l.google.com:19302) offered to both the pion server
// peer and the browser client so ICE can traverse NATs when the viewer is not
// on the same L2 segment as the host.
type WebRTCFile struct {
	STUNServers []string `json:"stun_servers,omitempty"`
}

// ResolvedSTUNServers returns the configured STUN servers, falling back to the
// Google default when none are set.
func (w WebRTCFile) ResolvedSTUNServers() []string {
	servers := w.NormalizedSTUNServers()
	if len(servers) == 0 {
		return []string{DefaultSTUNServer}
	}
	return servers
}

// NormalizedSTUNServers trims blank entries and duplicates while preserving order.
func (w WebRTCFile) NormalizedSTUNServers() []string {
	seen := make(map[string]struct{}, len(w.STUNServers))
	out := make([]string, 0, len(w.STUNServers))
	for _, server := range w.STUNServers {
		trimmed := strings.TrimSpace(server)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

// ValidateSTUNServer accepts stun:/stuns: URLs with a host, optionally a port.
func ValidateSTUNServer(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fmt.Errorf("webrtc.stun_servers entries must not be empty")
	}
	scheme, rest, ok := strings.Cut(value, ":")
	if !ok {
		return fmt.Errorf("webrtc.stun_servers %q: want stun:host[:port]", raw)
	}
	switch strings.ToLower(scheme) {
	case "stun", "stuns":
	default:
		return fmt.Errorf("webrtc.stun_servers %q: scheme must be stun or stuns", raw)
	}
	if strings.TrimSpace(rest) == "" {
		return fmt.Errorf("webrtc.stun_servers %q: missing host", raw)
	}
	return nil
}

// ValidateWebRTC checks every configured STUN server URL.
func ValidateWebRTC(webrtc WebRTCFile) error {
	for _, server := range webrtc.NormalizedSTUNServers() {
		if err := ValidateSTUNServer(server); err != nil {
			return err
		}
	}
	return nil
}
