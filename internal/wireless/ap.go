//go:build linux

package wireless

import (
	"bufio"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Alex4386/komitake/internal/logging"
	"github.com/Alex4386/komitake/internal/wireless/lp2p"
	"github.com/vishvananda/netlink"
)

type APConfig struct {
	Interface   string
	HostapdPath string
	Subnet      *net.IPNet
	Gateway     net.IP
	Logger      *slog.Logger
}

type AccessPoint struct {
	cfg         APConfig
	logger      *slog.Logger
	mu          sync.Mutex
	tempDir     string
	hostapd     *exec.Cmd
	hostapdWait chan struct{}
	dhcp        *DHCPServer
	cancelLogs  context.CancelFunc
	ctrlSocket  string
}

func NewAccessPoint(cfg APConfig) *AccessPoint {
	if cfg.HostapdPath == "" {
		cfg.HostapdPath = "./hostapd"
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default().With("component", "wireless-ap")
	}
	return &AccessPoint{cfg: cfg, logger: logger}
}

func (a *AccessPoint) Start(ctx context.Context, group GroupInfo, temporary bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.cfg.Interface == "" {
		return nil
	}
	if runtime.GOOS != "linux" {
		return errors.New("managed wireless AP mode is only supported on linux")
	}
	if a.cfg.Subnet == nil || a.cfg.Gateway == nil {
		return errors.New("managed wireless AP mode requires wireless.subnet")
	}
	hostapdPath, err := a.resolveHostapdPath()
	if err != nil {
		return err
	}
	if err := group.Validate(); err != nil {
		return err
	}
	a.logger.Info("starting access point",
		"interface", a.cfg.Interface,
		"temporary", temporary,
		"ssid", group.SSID,
		"channel", group.Channel,
		"hostapd_path", hostapdPath,
		logging.Secret("psk", group.PSK),
	)

	// Replace any previous AP under this lock so PAIRING→RUNNING is one
	// stop+start critical section (avoids a Stop()/Start() gap with the
	// mutex released and a misleading idle "stopping" log).
	if err := a.stopLocked(); err != nil {
		return err
	}
	// Cheap USB radios often leave the phy in a bad state after hostapd is
	// killed; bounce the link before the game BSS so the second spawn actually
	// beacons vendor IEs the kart can see.
	if !temporary {
		if err := bounceInterface(a.cfg.Interface); err != nil {
			a.logger.Warn("interface bounce failed", "interface", a.cfg.Interface, "error", err)
		}
	}
	if err := ensureInterfaceAddress(a.cfg.Interface, a.cfg.Gateway, a.cfg.Subnet); err != nil {
		return err
	}

	tempDir, err := os.MkdirTemp("", "komitake-hostapd-")
	if err != nil {
		return err
	}
	a.tempDir = tempDir

	configPath := filepath.Join(tempDir, "hostapd.conf")
	ctrlPath := filepath.Join(tempDir, "hostapd_ctrl")
	if err := os.MkdirAll(ctrlPath, 0o700); err != nil {
		return err
	}
	a.ctrlSocket = filepath.Join(ctrlPath, a.cfg.Interface)
	configData, err := hostapdConfig(group, a.cfg.Interface, temporary, ctrlPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(configPath, []byte(configData), 0o600); err != nil {
		return err
	}
	a.logger.Debug("wrote hostapd config", "path", configPath)

	logCtx, cancel := context.WithCancel(context.Background())
	a.cancelLogs = cancel

	// Deliberately not exec.CommandContext(ctx, ...): ctx is scoped to this
	// Start call, and binding hostapd to it kills the AP as soon as the caller
	// returns. Lifetime is owned by stopLocked instead.
	cmd := exec.Command(hostapdPath, configPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		cancel()
		return err
	}
	a.logger.Info("hostapd started", "pid", cmd.Process.Pid, "config", configPath)

	enabled := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		notified := false
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.Contains(line, "AP-STA-CONNECTED"),
				strings.Contains(line, "AP-STA-DISCONNECTED"),
				strings.Contains(line, "AP-STA-POSSIBLE-PSK-MISMATCH"),
				strings.Contains(line, "associated"),
				strings.Contains(line, "disassociated"),
				strings.Contains(line, "authenticated"),
				strings.Contains(line, "deauthenticated"):
				// Association is the game-network failure fork: empty DHCP/5202
				// with no STA events means the kart never joined the BSS.
				a.logger.Info("hostapd", "line", line)
			default:
				a.logger.Debug("hostapd", "line", line)
			}
			// Keep draining after signalling readiness. Returning early leaves
			// the pipe unread, and hostapd blocks on write once it fills.
			if !notified && strings.Contains(line, "AP-ENABLED") {
				enabled <- nil
				notified = true
			}
			if logCtx.Err() != nil {
				return
			}
		}
		if err := scanner.Err(); err != nil && !notified {
			enabled <- err
			return
		}
		if !notified {
			enabled <- nil
		}
	}()

	// Reap the process so its pipe descriptors and the copier goroutine that
	// os/exec starts for Stderr are released.
	waitDone := make(chan struct{})
	go func() {
		defer close(waitDone)
		if err := cmd.Wait(); err != nil {
			a.logger.Debug("hostapd exited", "error", err)
		}
	}()
	// Record the process now so any failure below tears it down through
	// stopLocked, which reaps it and cleans up the temp dir.
	a.hostapd = cmd
	a.hostapdWait = waitDone

	select {
	case err := <-enabled:
		if err != nil {
			_ = a.stopLocked()
			return err
		}
		a.logger.Info("access point enabled", "interface", a.cfg.Interface, "ssid", group.SSID)
		// Give the driver a moment to actually emit beacons before the kart's
		// post-SetGroupInfo scan window.
		select {
		case <-time.After(300 * time.Millisecond):
		case <-ctx.Done():
			_ = a.stopLocked()
			return ctx.Err()
		}
	case <-time.After(5 * time.Second):
		// hostapd never reported AP-ENABLED. Verify it is at least still alive
		// rather than reporting success on a process that died instantly.
		select {
		case <-waitDone:
			_ = a.stopLocked()
			return errors.New("hostapd exited during startup; check the interface is free and supports AP mode")
		default:
			a.logger.Warn("continuing after hostapd startup timeout", "interface", a.cfg.Interface)
		}
	case <-waitDone:
		_ = a.stopLocked()
		return errors.New("hostapd exited during startup; check the interface is free and supports AP mode")
	case <-ctx.Done():
		_ = a.stopLocked()
		return ctx.Err()
	}

	server, err := NewDHCPServer(DHCPConfig{
		Interface: a.cfg.Interface,
		Subnet:    a.cfg.Subnet,
		Gateway:   a.cfg.Gateway,
		ServerIP:  a.cfg.Gateway,
	})
	if err != nil {
		_ = a.stopLocked()
		return err
	}
	if err := server.Start(); err != nil {
		_ = a.stopLocked()
		return err
	}
	a.logger.Info("dhcp server started", "interface", a.cfg.Interface, "gateway", a.cfg.Gateway.String())

	a.dhcp = server
	_ = temporary
	return nil
}

func (a *AccessPoint) resolveHostapdPath() (string, error) {
	if a.cfg.HostapdPath == "" {
		return "", errors.New("managed wireless AP mode requires hostapd_path")
	}
	for _, candidate := range a.hostapdCandidates() {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	if path, err := exec.LookPath(a.cfg.HostapdPath); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("managed wireless AP mode requires a built OpenKart hostapd binary at %q; run ./build.sh or set wireless.hostapd_path", a.cfg.HostapdPath)
}

func (a *AccessPoint) hostapdCandidates() []string {
	candidates := []string{a.cfg.HostapdPath}
	if a.cfg.HostapdPath != "./hostapd" {
		return candidates
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "hostapd"),
			filepath.Join(exeDir, "komitake-hostapd"),
		)
	}
	candidates = append(candidates,
		"/usr/local/bin/komitake-hostapd",
		"/usr/bin/komitake-hostapd",
	)
	return candidates
}

func (a *AccessPoint) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.stopLocked()
}

func (a *AccessPoint) stopLocked() error {
	running := a.hostapd != nil || a.dhcp != nil || a.tempDir != "" || a.cancelLogs != nil
	if !running {
		return nil
	}
	a.logger.Info("stopping access point", "interface", a.cfg.Interface)
	if a.cancelLogs != nil {
		a.cancelLogs()
		a.cancelLogs = nil
	}
	if a.dhcp != nil {
		_ = a.dhcp.Close()
		a.dhcp = nil
		a.logger.Info("dhcp server stopped", "interface", a.cfg.Interface)
	}
	if a.hostapd != nil && a.hostapd.Process != nil {
		a.logger.Info("stopping hostapd", "pid", a.hostapd.Process.Pid)
		_ = a.hostapd.Process.Signal(os.Interrupt)

		// Wait on the cmd.Wait goroutine started in Start rather than calling
		// Process.Wait here, which would race it over the same child.
		// Keep this short: after pairing the kart immediately probes for the
		// game SSID, so a multi-second gap with no AP is painful.
		select {
		case <-a.hostapdWait:
		case <-time.After(300 * time.Millisecond):
			a.logger.Info("hostapd did not exit, killing", "pid", a.hostapd.Process.Pid)
			_ = a.hostapd.Process.Kill()
			select {
			case <-a.hostapdWait:
			case <-time.After(500 * time.Millisecond):
				a.logger.Warn("hostapd did not exit after kill", "pid", a.hostapd.Process.Pid)
			}
		}
		a.hostapd = nil
		a.hostapdWait = nil
	}
	if a.tempDir != "" {
		_ = os.RemoveAll(a.tempDir)
		a.tempDir = ""
	}
	a.ctrlSocket = ""
	a.logger.Info("access point stopped", "interface", a.cfg.Interface)
	return nil
}

func ensureInterfaceAddress(name string, gateway net.IP, subnet *net.IPNet) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return err
	}
	if err := netlink.LinkSetUp(link); err != nil {
		return err
	}

	addrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
	if err != nil {
		return err
	}
	for _, addr := range addrs {
		_ = netlink.AddrDel(link, &addr)
	}

	addr := &netlink.Addr{
		IPNet: &net.IPNet{
			IP:   gateway,
			Mask: subnet.Mask,
		},
	}
	return netlink.AddrAdd(link, addr)
}

func hostapdConfig(group GroupInfo, iface string, temporary bool, ctrlPath string) (string, error) {
	// Nonce selection does not change join math (kart only sees the IE), but
	// pure-random is what already works for pairing on flaky USB APs. Use it
	// for the game BSS too; sequential nonces are only an anti-reuse nicety.
	network, err := lp2p.Encrypt(group.PSK, true)
	if err != nil {
		return "", err
	}
	_ = temporary
	// ignore_broadcast_ssid=2: lp2p stealth (all-zero SSID, same length).
	return fmt.Sprintf(`interface=%s
driver=nl80211
hw_mode=g
channel=%d
ignore_broadcast_ssid=2
ssid=%s
static_ccmp_key=%s
vendor_elements=%s
ctrl_interface=%s
`,
		iface,
		group.Channel,
		group.SSID,
		hex.EncodeToString(network.StaticCCMPKey),
		hex.EncodeToString(network.VendorIEs),
		ctrlPath,
	), nil
}

func bounceInterface(name string) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return err
	}
	if err := netlink.LinkSetDown(link); err != nil {
		return err
	}
	return netlink.LinkSetUp(link)
}
