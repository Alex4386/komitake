//go:build linux

package wireless

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

func (s *DHCPServer) Start() error {
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var controlErr error
			if err := c.Control(func(fd uintptr) {
				controlErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_BROADCAST, 1)
				if controlErr == nil && s.cfg.Interface != "" {
					controlErr = unix.BindToDevice(int(fd), s.cfg.Interface)
				}
			}); err != nil {
				return err
			}
			return controlErr
		},
	}

	pc, err := lc.ListenPacket(context.Background(), "udp4", fmt.Sprintf(":%d", dhcpServerPort))
	if err != nil {
		return err
	}
	conn, ok := pc.(*net.UDPConn)
	if !ok {
		pc.Close()
		return errors.New("failed to create UDP conn for dhcp")
	}
	s.conn = conn
	go s.serve()
	return nil
}

func (s *DHCPServer) serve() {
	// Sized for a full IPv4 datagram so a large option block is not truncated
	// mid-parse.
	buf := make([]byte, 65535)
	for {
		n, _, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		resp, err := s.handlePacket(buf[:n])
		if err != nil || len(resp) == 0 {
			continue
		}
		_, _ = s.conn.WriteToUDP(resp, &net.UDPAddr{IP: net.IPv4bcast, Port: dhcpClientPort})
	}
}
