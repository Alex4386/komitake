package daemon

// Observer receives connectivity events from the daemon. It is the single seam
// for external integrations: a future `komitake events` stream, outbound
// webhooks, or config-driven exec hooks all implement this interface and are
// installed with Manager.SetObserver. The state machine funnels every state
// transition and device connect/disconnect through it, so integrations never
// need to reach into daemon internals.
//
// Implementations must be safe for concurrent use and must not block: the
// daemon calls these from state-transition and handshake goroutines.
type Observer interface {
	// StateChanged fires after a successful transition between modes.
	StateChanged(from, to State)
	// DeviceConnected fires when a kart's handshake completes in normal mode.
	DeviceConnected(DeviceSummary)
	// DeviceDisconnected fires when a tracked kart drops its connection.
	DeviceDisconnected(DeviceSummary)
	// PairingCompleted fires when a kart accepts group info during pairing.
	PairingCompleted(address, ident string)
}

// NopObserver is the default Observer. It ignores every event, so the daemon
// runs identically whether or not an integration is attached.
type NopObserver struct{}

func (NopObserver) StateChanged(State, State)          {}
func (NopObserver) DeviceConnected(DeviceSummary)      {}
func (NopObserver) DeviceDisconnected(DeviceSummary)   {}
func (NopObserver) PairingCompleted(string, string)    {}

// emitStateLocked notifies the observer of a transition. It is called with the
// lock held but invokes the observer without releasing it; observers are
// contractually non-blocking, and the default is a no-op.
func (m *Manager) emitStateLocked(from, to State) {
	if from == to {
		return
	}
	m.events.StateChanged(from, to)
}
