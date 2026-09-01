package config

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildRuntimeAppliesDefaultsAndOverrides(t *testing.T) {
	t.Parallel()

	cfg := File{
		Wireless: WirelessFile{
			Interface: "wlan0",
		},
		RCD: RCDFile{
			IdentHex:     hex.EncodeToString(make([]byte, 16)),
			MasterKeyHex: hex.EncodeToString(make([]byte, 64)),
		},
	}

	rt, err := BuildRuntime(cfg, "/tmp/komitake/config.json", Options{
		Listen:      "unix:/tmp/komitake.sock",
		PairingFile: "/tmp/pairing.json",
		Interface:   "wlan1",
	})
	if err != nil {
		t.Fatalf("BuildRuntime() error = %v", err)
	}

	if rt.Listen.Network != "unix" || rt.Listen.Address != "/tmp/komitake.sock" {
		t.Fatalf("Listen = %+v", rt.Listen)
	}
	if rt.PairingFile != "/tmp/pairing.json" {
		t.Fatalf("PairingFile = %q", rt.PairingFile)
	}
	if rt.Wireless.Interface != "wlan1" {
		t.Fatalf("Interface = %q", rt.Wireless.Interface)
	}
	if !rt.HasGroupInfo {
		t.Fatal("expected generated group info")
	}
	if len(rt.GroupInfo.PSK) != 32 {
		t.Fatalf("generated PSK len = %d", len(rt.GroupInfo.PSK))
	}
	if rt.GroupInfo.SSID == "" || rt.GroupInfo.SSID[0] != 'G' {
		t.Fatalf("SSID = %q, want prefix G", rt.GroupInfo.SSID)
	}
	if len(rt.ServerInfo.Ident) != 16 {
		t.Fatalf("ident len = %d", len(rt.ServerInfo.Ident))
	}
}

func TestParseIPCAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in      string
		network string
		address string
		wantErr bool
	}{
		{in: "", network: "unix", address: DefaultSocketPath},
		{in: "unix:/run/komitake.sock", network: "unix", address: "/run/komitake.sock"},
		{in: "/var/run/foo.sock", network: "unix", address: "/var/run/foo.sock"},
		{in: "./local.sock", network: "unix", address: "./local.sock"},
		{in: "@abstract", network: "unix", address: "@abstract"},
		{in: "tcp:127.0.0.1:5252", network: "tcp", address: "127.0.0.1:5252"},
		{in: "192.168.1.2:5252", network: "tcp", address: "192.168.1.2:5252"},
		{in: "0.0.0.0:5252", network: "tcp", address: "0.0.0.0:5252"},
		{in: "unix:", wantErr: true},
		{in: "tcp:not-a-hostport", wantErr: true},
	}

	for _, tt := range tests {
		got, err := ParseIPCAddress(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseIPCAddress(%q) = %+v, want error", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseIPCAddress(%q) error = %v", tt.in, err)
			continue
		}
		if got.Network != tt.network || got.Address != tt.address {
			t.Errorf("ParseIPCAddress(%q) = {%s %s}, want {%s %s}",
				tt.in, got.Network, got.Address, tt.network, tt.address)
		}
	}
}

func TestResolveConfigPathPrefersCandidatesInOrder(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first := filepath.Join(dir, "project.json")
	second := filepath.Join(dir, "system.json")
	if err := os.WriteFile(first, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := DefaultConfigCandidates
	DefaultConfigCandidates = []string{first, second}
	t.Cleanup(func() { DefaultConfigCandidates = orig })

	path, err := ResolveConfigPath("")
	if err != nil {
		t.Fatalf("ResolveConfigPath() error = %v", err)
	}
	if path != first {
		t.Fatalf("ResolveConfigPath() = %q, want %q", path, first)
	}
}

func TestBuildRuntimeUsesExplicitRCDValues(t *testing.T) {
	t.Parallel()

	ident := make([]byte, 16)
	masterKey := make([]byte, 64)
	cfg := File{
		RCD: RCDFile{
			Name:         "Komitake",
			IdentHex:     hex.EncodeToString(ident),
			MasterKeyHex: hex.EncodeToString(masterKey),
		},
	}

	rt, err := BuildRuntime(cfg, "/tmp/config.json", Options{})
	if err != nil {
		t.Fatalf("BuildRuntime() error = %v", err)
	}

	if got := hex.EncodeToString(rt.ServerInfo.Ident); got != hex.EncodeToString(ident) {
		t.Fatalf("ident = %s", got)
	}
}

func TestLoadPersistsGeneratedGameNetwork(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := File{
		RCD: RCDFile{
			IdentHex:     hex.EncodeToString(make([]byte, 16)),
			MasterKeyHex: hex.EncodeToString(make([]byte, 64)),
		},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	first, err := Load(configPath, Options{})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	second, err := Load(configPath, Options{})
	if err != nil {
		t.Fatalf("Load() second error = %v", err)
	}

	if first.GroupInfo.SSID != second.GroupInfo.SSID {
		t.Fatalf("SSID changed across loads: %q != %q", first.GroupInfo.SSID, second.GroupInfo.SSID)
	}
	if got := hex.EncodeToString(first.GroupInfo.PSK); got != hex.EncodeToString(second.GroupInfo.PSK) {
		t.Fatalf("PSK changed across loads: %s != %s", got, hex.EncodeToString(second.GroupInfo.PSK))
	}

	stateData, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("ReadFile(state.json) error = %v", err)
	}
	var state PersistentState
	if err := json.Unmarshal(stateData, &state); err != nil {
		t.Fatalf("json.Unmarshal(state.json) error = %v", err)
	}
	if state.GameNetwork == nil || state.GameNetwork.SSID == "" {
		t.Fatalf("state missing game network: %+v", state)
	}
}

func TestLoadPrefersSecretFileOverJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	cfg := File{
		Secret: "from-json",
		RCD:    RCDFile{},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secret"), []byte("from-file\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(secret) error = %v", err)
	}

	rt, err := Load(configPath, Options{})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	// Ident is derived from the secret; matching the file-derived value proves
	// the sibling secret file won over the JSON field.
	want, err := BuildRuntime(File{Secret: "from-file", RCD: RCDFile{}}, configPath, Options{})
	if err != nil {
		t.Fatalf("BuildRuntime() error = %v", err)
	}
	if hex.EncodeToString(rt.ServerInfo.Ident) != hex.EncodeToString(want.ServerInfo.Ident) {
		t.Fatal("secret file was not used to derive rcd.ident")
	}
}

func TestLoadExplicitWirelessOverridesPersistedState(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	statePath := filepath.Join(dir, "state.json")
	cfg := File{
		Wireless: WirelessFile{
			SSID:    "Gexplicit",
			PSKHex:  hex.EncodeToString(make([]byte, 32)),
			Channel: 6,
		},
		RCD: RCDFile{
			IdentHex:     hex.EncodeToString(make([]byte, 16)),
			MasterKeyHex: hex.EncodeToString(make([]byte, 64)),
		},
	}
	configData, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal(config) error = %v", err)
	}
	if err := os.WriteFile(configPath, configData, 0o644); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}

	state := PersistentState{
		GameNetwork: &GameNetworkRecord{
			SSID:    "Gpersisted",
			PSKHex:  hex.EncodeToString(bytesOf(0x42, 32)),
			Channel: 11,
		},
	}
	stateData, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("json.Marshal(state) error = %v", err)
	}
	if err := os.WriteFile(statePath, stateData, 0o644); err != nil {
		t.Fatalf("WriteFile(state) error = %v", err)
	}

	rt, err := Load(configPath, Options{})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if rt.GroupInfo.SSID != "Gexplicit" {
		t.Fatalf("SSID = %q", rt.GroupInfo.SSID)
	}
	if rt.GroupInfo.Channel != 6 {
		t.Fatalf("Channel = %d", rt.GroupInfo.Channel)
	}
}

func bytesOf(v byte, count int) []byte {
	out := make([]byte, count)
	for i := range out {
		out[i] = v
	}
	return out
}
