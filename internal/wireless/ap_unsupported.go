//go:build !linux

package wireless

import (
	"context"
	"errors"
	"log/slog"
	"net"
)

type APConfig struct {
	Interface   string
	HostapdPath string
	Subnet      *net.IPNet
	Gateway     net.IP
	Logger      *slog.Logger
}

type AccessPoint struct {
	cfg    APConfig
	logger *slog.Logger
}

func NewAccessPoint(cfg APConfig) *AccessPoint {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default().With("component", "wireless-ap")
	}
	return &AccessPoint{cfg: cfg, logger: logger}
}

func (a *AccessPoint) Start(context.Context, GroupInfo, bool) error {
	if a.cfg.Interface == "" {
		return nil
	}
	a.logger.Warn("managed wireless AP mode is unavailable on this platform", "interface", a.cfg.Interface)
	return errors.New("managed wireless AP mode is only implemented on linux")
}

func (a *AccessPoint) Stop() error {
	return nil
}

func (a *AccessPoint) StationSignalDBM(string) (int, bool) {
	return 0, false
}
