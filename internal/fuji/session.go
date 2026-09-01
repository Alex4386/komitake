package fuji

import (
	"context"

	"github.com/Alex4386/komitake/internal/rcd"
)

// EventPort is the kart's Fuji event service TCP port. The kart opens it after
// the handshake and expects the daemon to connect. komitake holds the channel
// open but never uses it; dropping it makes the kart reset its whole network
// connection.
const EventPort = 5107

// Session is a connectivity-only control session to a kart. It holds the two
// post-handshake channels the kart requires open (control 5103 and event 5107)
// and exposes the control parameter operations komitake needs.
//
// The kart resets its whole network connection if either channel is dropped, so
// a Session must stay open for the lifetime of the connection.
type Session struct {
	control *ControlClient
	event   *rcd.Client
}

// Connect opens the control and event channels concurrently, mirroring
// openkartd. Both must succeed: the kart expects both open before it considers
// itself connected.
func Connect(ctx context.Context, host string) (*Session, error) {
	type ctrlRes struct {
		c   *ControlClient
		err error
	}
	type evtRes struct {
		c   *rcd.Client
		err error
	}
	ctrlCh := make(chan ctrlRes, 1)
	evtCh := make(chan evtRes, 1)

	go func() {
		c, err := DialControl(ctx, host)
		ctrlCh <- ctrlRes{c: c, err: err}
	}()
	go func() {
		c, err := dialKart(ctx, host, EventPort)
		evtCh <- evtRes{c: c, err: err}
	}()

	ctrl := <-ctrlCh
	evt := <-evtCh
	if ctrl.err != nil {
		if evt.c != nil {
			_ = evt.c.Close()
		}
		return nil, ctrl.err
	}
	if evt.err != nil {
		_ = ctrl.c.Close()
		return nil, evt.err
	}
	return &Session{control: ctrl.c, event: evt.c}, nil
}

// GetParam reads a named control parameter over the control channel.
func (s *Session) GetParam(ctx context.Context, name string) ([]byte, error) {
	return s.control.GetParam(ctx, name)
}

// SetConnectionInfo completes control setup so the kart does not reset.
func (s *Session) SetConnectionInfo(ctx context.Context, telemetryPort, lspControlPort, lspStreamPort int, unknown uint16, timestamp int64) error {
	return s.control.SetConnectionInfo(ctx, telemetryPort, lspControlPort, lspStreamPort, unknown, timestamp)
}

// SetState arms (1) or sleeps (0) the kart drive state.
func (s *Session) SetState(ctx context.Context, state byte) error {
	return s.control.SetState(ctx, state)
}

// Shutdown prepares the kart to power off when channels are closed.
func (s *Session) Shutdown(ctx context.Context) error {
	return s.control.Shutdown(ctx)
}

// Close tears down both channels.
func (s *Session) Close() error {
	var err error
	if s.control != nil {
		if e := s.control.Close(); e != nil {
			err = e
		}
	}
	if s.event != nil {
		if e := s.event.Close(); e != nil && err == nil {
			err = e
		}
	}
	return err
}
