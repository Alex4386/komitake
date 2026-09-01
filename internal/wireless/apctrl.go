//go:build linux

package wireless

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// StationSignalDBM returns the last-reported signal strength in dBm for the
// station with the given MAC, read from hostapd's control interface. ok is
// false when the AP is down, the station is unknown, or hostapd reports no
// signal yet.
func (a *AccessPoint) StationSignalDBM(mac string) (dbm int, ok bool) {
	a.mu.Lock()
	sock := a.ctrlSocket
	a.mu.Unlock()
	if sock == "" || mac == "" {
		return 0, false
	}

	reply, err := hostapdCtrlCommand(sock, "STA "+strings.ToLower(mac))
	if err != nil {
		a.logger.Debug("hostapd sta query failed", "mac", mac, "error", err)
		return 0, false
	}
	for _, line := range strings.Split(reply, "\n") {
		key, value, found := strings.Cut(line, "=")
		if !found || key != "signal" {
			continue
		}
		v, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return 0, false
		}
		return v, true
	}
	return 0, false
}

// hostapdCtrlCommand sends one command to hostapd's unixgram control socket and
// returns the reply. It binds a private client socket in the same directory so
// hostapd can reply to it.
func hostapdCtrlCommand(serverSocket, command string) (string, error) {
	raddr := &net.UnixAddr{Name: serverSocket, Net: "unixgram"}
	localPath := filepath.Join(filepath.Dir(serverSocket),
		"komitake-ctrl-"+strconv.Itoa(os.Getpid())+"-"+strconv.FormatInt(time.Now().UnixNano(), 36))
	laddr := &net.UnixAddr{Name: localPath, Net: "unixgram"}

	conn, err := net.DialUnix("unixgram", laddr, raddr)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = conn.Close()
		_ = os.Remove(localPath)
	}()

	_ = conn.SetDeadline(time.Now().Add(time.Second))
	if _, err := conn.Write([]byte(command)); err != nil {
		return "", err
	}

	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return "", err
	}
	reply := string(buf[:n])
	if strings.HasPrefix(reply, "FAIL") {
		return "", errors.New("hostapd returned FAIL")
	}
	return reply, nil
}
