package daemon

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/Alex4386/komitake/internal/wireless"
)

func (m *Manager) SetState(ctx context.Context, state State) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return errors.New("daemon is shutting down")
	}
	if state == m.state {
		m.logger.Debug("state unchanged", "state", state)
		return nil
	}
	previous := m.state
	if state == StatePairing && !m.cfg.HasGroupInfo {
		return errors.New("pairing requires wireless group info")
	}

	// Long-lived resources are owned by baseCtx, not by the caller's
	// request-scoped ctx, which is cancelled the moment the RPC returns. The
	// caller's ctx still bounds the blocking startup work below via startCtx.
	startCtx, cancelStart := mergeCancel(m.baseCtx, ctx)
	defer cancelStart()

	m.logger.Info("changing state", "from", previous, "to", state)

	m.stopPairingTimeoutLocked()
	if err := m.stopListenersLocked(); err != nil {
		m.logger.Error("failed stopping listeners", "error", err)
		return err
	}
	m.clearDevicesLocked()
	// For RUNNING/PAIRING, leave the AP up until Start() replaces it under one
	// lock (shorter no-beacon window than Stop()+Start() with the mutex released).
	// Only tear the AP down here when leaving wireless service entirely.
	if state == StateDown && m.ap != nil {
		if err := m.ap.Stop(); err != nil {
			m.logger.Error("failed stopping access point", "error", err)
			return err
		}
	}
	m.state = StateDown
	m.pairing = nil
	if err := m.persistPairingLocked(nil); err != nil {
		m.logger.Error("failed writing pairing state", "error", err)
		return err
	}

	switch state {
	case StateDown:
		m.logger.Info("daemon services stopped")
		m.emitStateLocked(previous, StateDown)
		m.signalLocked()
		return nil
	case StateRunning:
		if m.ap != nil {
			if err := m.ap.Start(startCtx, m.cfg.GroupInfo, false); err != nil {
				m.logger.Error("failed starting normal access point", "error", err, "ssid", m.cfg.GroupInfo.SSID, "channel", m.cfg.GroupInfo.Channel)
				return err
			}
		}
		ln, err := m.listen("tcp", net.JoinHostPort(m.cfg.Address, fmt.Sprintf("%d", HandshakePort)))
		if err != nil {
			m.logger.Error("failed starting handshake listener", "error", err, "port", HandshakePort)
			return err
		}
		m.runningLn = ln
		m.state = StateRunning
		if err := m.persistPairingLocked(nil); err != nil {
			m.logger.Error("failed writing pairing state", "error", err)
			return err
		}
		m.logger.Info("daemon ready in normal mode", "handshake_port", HandshakePort)
		m.emitStateLocked(previous, StateRunning)
		m.backgroundWG.Add(1)
		go m.acceptLoop(ln, false)
	case StatePairing:
		channel := m.cfg.GroupInfo.Channel
		creds, err := wireless.NewPairingCredentials(channel)
		if err != nil {
			return err
		}
		pairingGroup := wireless.GroupInfo{
			SSID:    creds.SSID,
			PSK:     creds.PSK,
			Channel: creds.Channel,
		}
		if m.ap != nil {
			if err := m.ap.Start(startCtx, pairingGroup, true); err != nil {
				m.logger.Error("failed starting pairing access point", "error", err, "ssid", pairingGroup.SSID, "channel", pairingGroup.Channel)
				return err
			}
		}
		record := &PairingRecord{
			State:       StatePairing,
			SeedHex:     hex.EncodeToString(creds.Seed),
			SSID:        creds.SSID,
			Channel:     creds.Channel,
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			FilePath:    m.cfg.PairingFile,
		}
		m.pairing = record
		if err := m.persistPairingLocked(record); err != nil {
			m.logger.Error("failed writing pairing record", "error", err)
			return err
		}
		ln, err := m.listen("tcp", net.JoinHostPort(m.cfg.Address, fmt.Sprintf("%d", PairingPort)))
		if err != nil {
			m.logger.Error("failed starting pairing listener", "error", err, "port", PairingPort)
			return err
		}
		m.pairingLn = ln
		m.state = StatePairing
		m.logger.Info("daemon ready in pairing mode", "pairing_port", PairingPort, "ssid", record.SSID, "channel", record.Channel)
		m.emitStateLocked(previous, StatePairing)
		m.startPairingTimeoutLocked()
		m.backgroundWG.Add(1)
		go m.acceptLoop(ln, true)
	default:
		return fmt.Errorf("unknown state %q", state)
	}

	m.signalLocked()
	return nil
}

// mergeCancel returns a context cancelled when either parent is cancelled. It
// lets startup work respect both daemon shutdown and the caller's deadline
// without leaking either into the lifetime of the resources being created.
func mergeCancel(base, caller context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(base)
	if caller == nil || caller.Done() == nil {
		return ctx, cancel
	}

	stop := make(chan struct{})
	go func() {
		select {
		case <-caller.Done():
			cancel()
		case <-stop:
		}
	}()
	return ctx, func() {
		close(stop)
		cancel()
	}
}

func (m *Manager) stopListenersLocked() error {
	if m.runningLn != nil {
		_ = m.runningLn.Close()
		m.runningLn = nil
	}
	if m.pairingLn != nil {
		_ = m.pairingLn.Close()
		m.pairingLn = nil
	}
	return nil
}

func (m *Manager) startPairingTimeoutLocked() {
	ctx, cancel := context.WithCancel(context.Background())
	m.pairingStop = cancel
	m.backgroundWG.Add(1)
	go func() {
		defer m.backgroundWG.Done()
		timer := time.NewTimer(pairingModeTimeout)
		defer timer.Stop()
		m.logger.Info("pairing timeout armed", "timeout", pairingModeTimeout.String())
		select {
		case <-ctx.Done():
			m.logger.Debug("pairing timeout canceled")
			return
		case <-timer.C:
			m.logger.Info("pairing timed out, returning to normal mode")
			_ = m.SetState(context.Background(), StateRunning)
		}
	}()
}

func (m *Manager) stopPairingTimeoutLocked() {
	if m.pairingStop != nil {
		m.pairingStop()
		m.pairingStop = nil
	}
}
