package config

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/Alex4386/komitake/internal/logging"
	"github.com/Alex4386/komitake/internal/rcd"
	"github.com/Alex4386/komitake/internal/wireless"
)

// DefaultSocketPath is the unix socket the daemon serves and clients dial when
// no address is configured.
const DefaultSocketPath = "/run/komitake.sock"

// DefaultIPCAddress is the default admin-API address in the unified form used
// by both the daemon (listen) and clients (dial).
const DefaultIPCAddress = "unix:" + DefaultSocketPath

// DefaultWebAddr is the host:port used by `komitake web` when web.bind is unset.
const DefaultWebAddr = "127.0.0.1:8080"

// DefaultSocketPerm protects the unauthenticated admin API by default.
const DefaultSocketPerm os.FileMode = 0o600

var DefaultConfigCandidates = []string{
	"./config.json",
	"/etc/komitake/config.json",
}

type File struct {
	Web    WebFile    `json:"web"`
	Socket SocketFile `json:"socket"`
	Video  VideoFile  `json:"video"`

	// Legacy flat keys are accepted on read and removed on settings rewrites.
	Address     string `json:"address,omitempty"`
	Listen      string `json:"listen,omitempty"`
	WebAddr     string `json:"web_addr,omitempty"`
	SocketPerms string `json:"socket_perms,omitempty"`

	PairingFile string       `json:"pairing_file"`
	Autostart   *bool        `json:"autostart"`
	Secret      string       `json:"secret"`
	Wireless    WirelessFile `json:"wireless"`
	RCD         RCDFile      `json:"rcd"`
}

type WebFile struct {
	Bind string     `json:"bind,omitempty"`
	TLS  WebTLSFile `json:"tls,omitempty"`
}

type WebTLSFile struct {
	Enabled  bool   `json:"enabled,omitempty"`
	CertFile string `json:"cert_file,omitempty"`
	KeyFile  string `json:"key_file,omitempty"`
}

type SocketFile struct {
	Bind  string `json:"bind,omitempty"`
	Chmod string `json:"chmod,omitempty"`
	Perm  string `json:"perm,omitempty"`
}

func (file *File) normalizeLegacy() {
	if file.Socket.Bind == "" && file.Listen != "" {
		file.Socket.Bind = file.Listen
	}
	if file.Web.Bind == "" && file.WebAddr != "" {
		file.Web.Bind = file.WebAddr
	}
	if file.Socket.Chmod == "" && file.Socket.Perm != "" {
		file.Socket.Chmod = file.Socket.Perm
	}
	if file.Socket.Chmod == "" && file.SocketPerms != "" {
		file.Socket.Chmod = file.SocketPerms
	}
	if file.Wireless.Address == "" {
		file.Wireless.Address = migrateWirelessAddress(file.Address, file.Wireless.Subnet)
	}
	file.Address = ""
	file.Listen = ""
	file.WebAddr = ""
	file.SocketPerms = ""
	file.Socket.Perm = ""
	file.Wireless.Subnet = ""
}

func migrateWirelessAddress(legacyAddress, legacySubnet string) string {
	legacyAddress = strings.TrimSpace(legacyAddress)
	legacySubnet = strings.TrimSpace(legacySubnet)
	if legacyAddress == "" && legacySubnet == "" {
		return ""
	}
	if legacySubnet != "" {
		_, subnet, err := net.ParseCIDR(legacySubnet)
		if err == nil {
			ones, bits := subnet.Mask.Size()
			if bits == 32 {
				host := legacyAddress
				if host == "" {
					if firstHost := firstHostIP(&net.IPNet{IP: subnet.IP.Mask(subnet.Mask), Mask: subnet.Mask}); firstHost != nil {
						host = firstHost.String()
					}
				}
				if host != "" {
					return fmt.Sprintf("%s/%d", host, ones)
				}
			}
		}
	}
	if legacyAddress != "" {
		if strings.Contains(legacyAddress, "/") {
			return legacyAddress
		}
		return legacyAddress + "/24"
	}
	return ""
}

// IPCAddress is a parsed admin-API address: a network ("unix" or "tcp") and the
// address within it (a socket path or host:port).
type IPCAddress struct {
	Network string
	Address string
}

// String renders the address back into the unified "unix:/path" / "tcp:host:port"
// form.
func (a IPCAddress) String() string {
	return a.Network + ":" + a.Address
}

// ParseIPCAddress interprets the unified address form:
//   - "unix:/run/komitake.sock" -> unix socket at that path
//   - "tcp:127.0.0.1:5252"      -> TCP at that host:port
//   - "/run/komitake.sock"      -> unix (leading slash, dot, or @ means a path)
//   - "192.168.1.2:5252"        -> TCP (a bare host:port)
//
// An empty string resolves to DefaultIPCAddress.
func ParseIPCAddress(addr string) (IPCAddress, error) {
	if addr == "" {
		addr = DefaultIPCAddress
	}
	switch {
	case strings.HasPrefix(addr, "unix:"):
		path := strings.TrimPrefix(addr, "unix:")
		if path == "" {
			return IPCAddress{}, fmt.Errorf("ipc address %q has an empty unix path", addr)
		}
		return IPCAddress{Network: "unix", Address: path}, nil
	case strings.HasPrefix(addr, "tcp:"):
		hostport := strings.TrimPrefix(addr, "tcp:")
		if _, _, err := net.SplitHostPort(hostport); err != nil {
			return IPCAddress{}, fmt.Errorf("ipc address %q is not a valid tcp host:port: %w", addr, err)
		}
		return IPCAddress{Network: "tcp", Address: hostport}, nil
	case strings.HasPrefix(addr, "/"), strings.HasPrefix(addr, "."), strings.HasPrefix(addr, "@"):
		return IPCAddress{Network: "unix", Address: addr}, nil
	default:
		// A bare token with a port is TCP; anything else is treated as a path.
		if _, _, err := net.SplitHostPort(addr); err == nil {
			return IPCAddress{Network: "tcp", Address: addr}, nil
		}
		return IPCAddress{Network: "unix", Address: addr}, nil
	}
}

type WirelessFile struct {
	Interface string `json:"interface"`
	SSID      string `json:"ssid"`
	PSKHex    string `json:"psk"`
	Channel   uint16 `json:"channel"`
	// Address is the AP host address with prefix, e.g. 192.168.137.1/24.
	Address string `json:"address,omitempty"`
	// Subnet is the legacy network CIDR migrated into Address.
	Subnet      string `json:"subnet,omitempty"`
	HostapdPath string `json:"hostapd_path"`
}

type RCDFile struct {
	Name          string  `json:"name"`
	IdentHex      string  `json:"ident"`
	MasterKeyHex  string  `json:"master_key"`
	SupportedVers []uint8 `json:"versions"`
}

type Options struct {
	ConfigPath  string
	Interface   string
	Listen      string
	PairingFile string
}

type Runtime struct {
	ConfigPath    string
	Address       string
	Listen        IPCAddress
	WebAddr       string
	SocketPerm    os.FileMode
	HasSocketPerm bool
	StateFile     string
	PairingFile   string
	AutoStart     bool
	Wireless      WirelessFile
	Subnet        *net.IPNet
	Gateway       net.IP
	GroupInfo     wireless.GroupInfo
	ServerInfo    rcd.ServerInfo
	HasGroupInfo  bool
	Video         VideoFile
}

type PersistentState struct {
	GameNetwork *GameNetworkRecord `json:"game_network,omitempty"`
}

type GameNetworkRecord struct {
	SSID    string `json:"ssid"`
	PSKHex  string `json:"psk"`
	Channel uint16 `json:"channel,omitempty"`
}

func Load(path string, opts Options) (Runtime, error) {
	resolvedPath, err := ResolveConfigPath(path)
	if err != nil {
		return Runtime{}, err
	}

	log := logging.New(nil).With("component", "config")
	log.Debug("resolved config path", "path", resolvedPath, "explicit", path != "")

	file, err := readConfigFile(resolvedPath)
	if err != nil {
		return Runtime{}, err
	}
	if err := resolveSecret(&file, resolvedPath); err != nil {
		return Runtime{}, err
	}

	statePath := DefaultStateFile(resolvedPath)
	state, err := readPersistentState(statePath)
	if err != nil {
		return Runtime{}, err
	}
	log.Debug("loaded persistent state",
		"path", statePath, "has_game_network", state.GameNetwork != nil)

	rt, nextState, dirty, err := buildRuntime(file, resolvedPath, statePath, opts, state)
	if err != nil {
		return Runtime{}, err
	}
	if dirty {
		log.Info("persisting generated credentials", "path", statePath)
		if err := writePersistentState(statePath, nextState); err != nil {
			return Runtime{}, err
		}
	}

	// The effective configuration is the first thing worth knowing when a
	// deployment misbehaves. Secrets are fingerprinted, never printed.
	log.Debug("effective configuration",
		"address", rt.Address,
		"listen", rt.Listen.String(),
		"pairing_file", rt.PairingFile,
		"autostart", rt.AutoStart,
		"interface", rt.Wireless.Interface,
		"hostapd_path", rt.Wireless.HostapdPath,
		"subnet", cidrString(rt.Subnet),
		"gateway", ipString(rt.Gateway),
		"has_group_info", rt.HasGroupInfo,
		"ssid", rt.GroupInfo.SSID,
		"channel", rt.GroupInfo.Channel,
		"rcd_name", rt.ServerInfo.Name,
		"rcd_versions", rt.ServerInfo.Versions,
		logging.Secret("wireless_psk", rt.GroupInfo.PSK),
		logging.Secret("rcd_ident", rt.ServerInfo.Ident),
		logging.Secret("rcd_master_key", rt.ServerInfo.MasterKey))

	return rt, nil
}

func cidrString(n *net.IPNet) string {
	if n == nil {
		return ""
	}
	return n.String()
}

func ipString(ip net.IP) string {
	if ip == nil {
		return ""
	}
	return ip.String()
}

func ResolveConfigPath(path string) (string, error) {
	if path != "" {
		return path, nil
	}

	for _, candidate := range DefaultConfigCandidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no config found in %v", DefaultConfigCandidates)
}

// ResolveListenAddress finds where the daemon's admin API is, for a client.
// An explicit override wins; otherwise it reads socket.bind (or legacy listen)
// from config and falls back to the default.
func ResolveListenAddress(override string, configPath string) string {
	if override != "" {
		return override
	}
	if configPath == "" {
		return DefaultIPCAddress
	}
	file, err := readConfigFile(configPath)
	if err != nil {
		return DefaultIPCAddress
	}
	if file.Socket.Bind != "" {
		return file.Socket.Bind
	}
	return DefaultIPCAddress
}

func DefaultPairingFile(configPath string) string {
	if configPath == "" {
		return filepath.Join(".", "pairing.json")
	}
	return filepath.Join(filepath.Dir(configPath), "pairing.json")
}

func DefaultStateFile(configPath string) string {
	if configPath == "" {
		return filepath.Join(".", "state.json")
	}
	return filepath.Join(filepath.Dir(configPath), "state.json")
}

// DefaultSecretFile is the sibling path for the root secret. Keeping it out of
// config.json lets that file stay world-readable for operators while the secret
// remains mode 0600.
func DefaultSecretFile(configPath string) string {
	if configPath == "" {
		return filepath.Join(".", "secret")
	}
	return filepath.Join(filepath.Dir(configPath), "secret")
}

func BuildRuntime(file File, configPath string, opts Options) (Runtime, error) {
	rt, _, _, err := buildRuntime(file, configPath, DefaultStateFile(configPath), opts, PersistentState{})
	return rt, err
}

func buildRuntime(file File, configPath string, statePath string, opts Options, state PersistentState) (Runtime, PersistentState, bool, error) {
	file.normalizeLegacy()

	listenRaw := file.Socket.Bind
	if opts.Listen != "" {
		listenRaw = opts.Listen
	}
	listen, err := ParseIPCAddress(listenRaw)
	if err != nil {
		return Runtime{}, PersistentState{}, false, err
	}

	pairingFile := file.PairingFile
	if pairingFile == "" {
		pairingFile = DefaultPairingFile(configPath)
	}
	if opts.PairingFile != "" {
		pairingFile = opts.PairingFile
	}

	wirelessConfig := file.Wireless
	if opts.Interface != "" {
		wirelessConfig.Interface = opts.Interface
	}

	autoStart := true
	if file.Autostart != nil {
		autoStart = *file.Autostart
	}

	subnet, gateway, err := buildNetwork(wirelessConfig)
	if err != nil {
		return Runtime{}, PersistentState{}, false, err
	}
	address := "0.0.0.0"
	if gateway != nil {
		address = gateway.String()
	}

	groupInfo, hasGroupInfo, nextState, dirty, err := buildGroupInfo(wirelessConfig, state)
	if err != nil {
		return Runtime{}, PersistentState{}, false, err
	}
	serverInfo, err := buildServerInfo(file.Secret, file.RCD)
	if err != nil {
		return Runtime{}, PersistentState{}, false, err
	}

	socketPerm := DefaultSocketPerm
	hasSocketPerm := false
	if strings.TrimSpace(file.Socket.Chmod) != "" {
		socketPerm, err = ParseSocketPerms(file.Socket.Chmod)
		if err != nil {
			return Runtime{}, PersistentState{}, false, err
		}
		hasSocketPerm = true
	}
	webAddr := strings.TrimSpace(file.Web.Bind)
	if webAddr == "" {
		webAddr = DefaultWebAddr
	}
	video := file.Video
	if err := ValidateVideo(video); err != nil {
		return Runtime{}, PersistentState{}, false, err
	}

	return Runtime{
		ConfigPath:    configPath,
		Address:       address,
		Listen:        listen,
		WebAddr:       webAddr,
		SocketPerm:    socketPerm,
		HasSocketPerm: hasSocketPerm,
		StateFile:     statePath,
		PairingFile:   pairingFile,
		AutoStart:     autoStart,
		Wireless:      wirelessConfig,
		Subnet:        subnet,
		Gateway:       gateway,
		GroupInfo:     groupInfo,
		ServerInfo:    serverInfo,
		HasGroupInfo:  hasGroupInfo,
		Video:         video,
	}, nextState, dirty, nil
}

func buildGroupInfo(cfg WirelessFile, state PersistentState) (wireless.GroupInfo, bool, PersistentState, bool, error) {
	nextState := state
	explicitSSID := cfg.SSID != ""
	explicitPSK := cfg.PSKHex != ""
	if explicitSSID != explicitPSK {
		return wireless.GroupInfo{}, false, PersistentState{}, false, errors.New("wireless.ssid and wireless.psk must be set together")
	}

	if explicitSSID {
		psk, err := decodeHex("wireless.psk", cfg.PSKHex, wireless.PSKSize)
		if err != nil {
			return wireless.GroupInfo{}, false, PersistentState{}, false, err
		}
		channel := cfg.Channel
		if channel == 0 {
			channel = 1
		}
		group := wireless.GroupInfo{
			SSID:    cfg.SSID,
			PSK:     psk,
			Channel: channel,
		}
		if err := group.Validate(); err != nil {
			return wireless.GroupInfo{}, false, PersistentState{}, false, err
		}
		return group, true, nextState, false, nil
	}

	if state.GameNetwork != nil {
		psk, err := decodeHex("state.game_network.psk", state.GameNetwork.PSKHex, wireless.PSKSize)
		if err != nil {
			return wireless.GroupInfo{}, false, PersistentState{}, false, err
		}
		channel := cfg.Channel
		if channel == 0 {
			channel = state.GameNetwork.Channel
		}
		if channel == 0 {
			channel = 1
		}
		group := wireless.GroupInfo{
			SSID:    state.GameNetwork.SSID,
			PSK:     psk,
			Channel: channel,
		}
		if err := group.Validate(); err != nil {
			return wireless.GroupInfo{}, false, PersistentState{}, false, err
		}
		// Persisted SSIDs from older builds were 23 bytes and can fail lp2p
		// Join after SetGroupInfo. Mint a pairing-sized name and rewrite state.
		if len(group.SSID) > wireless.PairingSSIDSize {
			group, err = wireless.NewGameGroup(channel)
			if err != nil {
				return wireless.GroupInfo{}, false, PersistentState{}, false, err
			}
			nextState.GameNetwork = &GameNetworkRecord{
				SSID:    group.SSID,
				PSKHex:  hex.EncodeToString(group.PSK),
				Channel: group.Channel,
			}
			return group, true, nextState, true, nil
		}
		return group, true, nextState, false, nil
	}

	group, err := wireless.NewGameGroup(cfg.Channel)
	if err != nil {
		return wireless.GroupInfo{}, false, PersistentState{}, false, err
	}
	nextState.GameNetwork = &GameNetworkRecord{
		SSID:    group.SSID,
		PSKHex:  hex.EncodeToString(group.PSK),
		Channel: group.Channel,
	}
	return group, true, nextState, true, nil
}

func buildNetwork(config WirelessFile) (*net.IPNet, net.IP, error) {
	if config.Interface == "" {
		return nil, nil, nil
	}
	addressCIDR := strings.TrimSpace(config.Address)
	if addressCIDR == "" {
		addressCIDR = "192.168.137.1/24"
	}
	ip, parsedNetwork, err := net.ParseCIDR(addressCIDR)
	if err != nil {
		return nil, nil, fmt.Errorf("wireless.address must be a host CIDR (e.g. 192.168.137.1/24): %w", err)
	}
	gateway := ip.To4()
	if gateway == nil {
		return nil, nil, errors.New("wireless.address must be IPv4")
	}
	network := &net.IPNet{IP: gateway.Mask(parsedNetwork.Mask), Mask: parsedNetwork.Mask}
	if gateway.Equal(network.IP) {
		return nil, nil, errors.New("wireless.address must be a host address, not the network address")
	}
	if isBroadcast(gateway, network) {
		return nil, nil, errors.New("wireless.address must not be the broadcast address")
	}
	return network, gateway, nil
}

func isBroadcast(ip net.IP, network *net.IPNet) bool {
	ipv4 := ip.To4()
	if ipv4 == nil {
		return false
	}
	broadcast := make(net.IP, 4)
	networkIP := network.IP.To4()
	for index := range broadcast {
		broadcast[index] = networkIP[index] | ^network.Mask[index]
	}
	return ipv4.Equal(broadcast)
}

func firstHostIP(network *net.IPNet) net.IP {
	ip := network.IP.To4()
	if ip == nil {
		return nil
	}
	host := append(net.IP(nil), ip...)
	host[3]++
	if !network.Contains(host) {
		return nil
	}
	return host
}

func buildServerInfo(secret string, cfg RCDFile) (rcd.ServerInfo, error) {
	name := cfg.Name
	if name == "" {
		name = "Komitake"
	}

	var ident []byte
	if cfg.IdentHex == "" {
		if secret == "" {
			return rcd.ServerInfo{}, errors.New("secret is required to derive rcd.ident")
		}
		ident = derive(secret, "rcd/ident", 16)
	} else {
		var err error
		ident, err = decodeHex("rcd.ident", cfg.IdentHex, 16)
		if err != nil {
			return rcd.ServerInfo{}, err
		}
	}

	var masterKey []byte
	if cfg.MasterKeyHex == "" {
		if secret == "" {
			return rcd.ServerInfo{}, errors.New("secret is required to derive rcd.master_key")
		}
		masterKey = derive(secret, "rcd/master_key", 64)
	} else {
		var err error
		masterKey, err = decodeHex("rcd.master_key", cfg.MasterKeyHex, 0)
		if err != nil {
			return rcd.ServerInfo{}, err
		}
	}

	versions := cfg.SupportedVers
	if len(versions) == 0 {
		versions = []uint8{2, 1}
	}

	info := rcd.ServerInfo{
		Name:      name,
		Ident:     ident,
		MasterKey: masterKey,
		Versions:  versions,
	}
	return info, info.Validate()
}

func derive(secret string, name string, length int) []byte {
	h := hmac.New(sha512.New, []byte(secret))
	_, _ = h.Write([]byte(name))
	return h.Sum(nil)[:length]
}

func decodeHex(name string, value string, expectedLen int) ([]byte, error) {
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("%s must be hexadecimal: %w", name, err)
	}
	if expectedLen > 0 && len(decoded) != expectedLen {
		return nil, fmt.Errorf("%s must be %d bytes", name, expectedLen)
	}
	return decoded, nil
}

func readConfigFile(path string) (File, error) {
	data, err := os.ReadFile(path)
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

// resolveSecret prefers /etc/komitake/secret (or a sibling of config.json) so
// config.json can be mode 0644. A non-empty "secret" field in JSON still works
// for older installs and local ./config.json checkouts.
func resolveSecret(file *File, configPath string) error {
	secretPath := DefaultSecretFile(configPath)
	data, err := os.ReadFile(secretPath)
	if err == nil {
		secret := string(bytes.TrimSpace(data))
		if secret != "" {
			file.Secret = secret
			return nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read secret file %s: %w", secretPath, err)
	}
	return nil
}

func readPersistentState(path string) (PersistentState, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return PersistentState{}, nil
	}
	if err != nil {
		return PersistentState{}, err
	}

	var state PersistentState
	if err := json.Unmarshal(data, &state); err != nil {
		return PersistentState{}, fmt.Errorf("decode state: %w", err)
	}
	return state, nil
}

func writePersistentState(path string, state PersistentState) error {
	// 0700 / 0600: state.json holds the generated game-network PSK, which is
	// the input to the link-layer CCMP key.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(data, '\n'), 0o600)
}

// writeFileAtomic writes via a temp file and rename so a crash mid-write cannot
// leave a truncated file, which would be fatal on the next load.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	tmpName = ""
	return nil
}
