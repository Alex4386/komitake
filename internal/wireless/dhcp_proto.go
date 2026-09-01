// DHCP protocol handling: packet parsing, lease management, and reply
// construction. Kept free of build tags so the logic that processes untrusted
// network input is testable on any platform; socket setup lives in dhcp.go.

package wireless

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
	"sync"
	"time"
)

const (
	dhcpServerPort = 67
	dhcpClientPort = 68
	bootReply      = 2
)

// DHCP message types (RFC 2132 section 9.6).
const (
	dhcpDiscover = 1
	dhcpOffer    = 2
	dhcpRequest  = 3
	dhcpDecline  = 4
	dhcpAck      = 5
	dhcpNak      = 6
	dhcpRelease  = 7
)

type DHCPConfig struct {
	Interface string
	Subnet    *net.IPNet
	Gateway   net.IP
	ServerIP  net.IP
}

type DHCPServer struct {
	cfg    DHCPConfig
	conn   *net.UDPConn
	mu     sync.Mutex
	leases map[string]net.IP
	inUse  map[string]string
	// expiry tracks when each client's lease lapses, keyed like leases. Without
	// it the pool never recovers from client churn.
	expiry    map[string]time.Time
	stopCh    chan struct{}
	closeOnce sync.Once
}

// Close is safe to call concurrently and more than once.
func (s *DHCPServer) Close() error {
	s.closeOnce.Do(func() {
		close(s.stopCh)
	})
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

func NewDHCPServer(cfg DHCPConfig) (*DHCPServer, error) {
	if cfg.Subnet == nil || cfg.Gateway == nil || cfg.ServerIP == nil {
		return nil, errors.New("dhcp requires subnet, gateway, and server IP")
	}
	return &DHCPServer{
		cfg:    cfg,
		leases: map[string]net.IP{},
		inUse:  map[string]string{},
		expiry: map[string]time.Time{},
		stopCh: make(chan struct{}),
	}, nil
}

func (s *DHCPServer) handlePacket(data []byte) ([]byte, error) {
	packet, err := parseDHCPPacket(data)
	if err != nil {
		return nil, err
	}
	msgType := packet.options[53]
	if len(msgType) == 0 {
		return nil, nil
	}

	clientID := packet.clientHWAddr.String()
	switch msgType[0] {
	case dhcpDiscover:
		ip := s.allocate(clientID)
		if ip == nil {
			return nil, nil
		}
		return s.reply(packet, dhcpOffer, ip), nil
	case dhcpRequest:
		// A requested address is only honored if this client already owns it.
		// Previously option 50 was echoed back verbatim, so any client could
		// claim the gateway or another kart's lease and get an ACK.
		ip := s.claim(clientID, packet.requestedIP())
		if ip == nil {
			return s.reply(packet, dhcpNak, net.IPv4zero), nil
		}
		return s.reply(packet, dhcpAck, ip), nil
	case dhcpRelease, dhcpDecline:
		s.release(clientID)
		return nil, nil
	default:
		return nil, nil
	}
}

// claim resolves the address to ACK for a REQUEST. It returns nil when the
// request must be NAK'd.
func (s *DHCPServer) claim(clientID string, requested net.IP) net.IP {
	s.mu.Lock()
	s.expireLocked(time.Now())
	current, hasLease := s.leases[clientID]
	s.mu.Unlock()

	if requested == nil {
		if hasLease {
			return current
		}
		return s.allocate(clientID)
	}

	if hasLease && current.Equal(requested) {
		s.mu.Lock()
		s.expiry[clientID] = time.Now().Add(leaseDuration)
		s.mu.Unlock()
		return current
	}
	// Only hand out an unclaimed in-subnet address that is not the gateway or
	// the broadcast address.
	if !s.cfg.Subnet.Contains(requested) ||
		requested.Equal(s.cfg.Gateway) ||
		isBroadcast(requested, s.cfg.Subnet) {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if owner, taken := s.inUse[requested.String()]; taken && owner != clientID {
		return nil
	}
	if hasLease {
		delete(s.inUse, current.String())
	}
	ip := append(net.IP(nil), requested...)
	s.leases[clientID] = ip
	s.inUse[ip.String()] = clientID
	s.expiry[clientID] = time.Now().Add(leaseDuration)
	return append(net.IP(nil), ip...)
}

// release frees a client's lease so the address returns to the pool.
func (s *DHCPServer) release(clientID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ip, ok := s.leases[clientID]; ok {
		delete(s.inUse, ip.String())
		delete(s.leases, clientID)
	}
	delete(s.expiry, clientID)
}

// allocate returns this client's existing lease, or the next free address.
// It returns nil when the pool is exhausted; handing out the server's own
// address, as it did previously, is worse than not replying.
func (s *DHCPServer) allocate(clientID string) net.IP {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(time.Now())

	if ip, ok := s.leases[clientID]; ok {
		s.expiry[clientID] = time.Now().Add(leaseDuration)
		return append(net.IP(nil), ip...)
	}

	for candidate := s.firstLeaseIP(); candidate != nil && s.cfg.Subnet.Contains(candidate); candidate = nextIPv4(candidate) {
		if candidate.Equal(s.cfg.Gateway) || isBroadcast(candidate, s.cfg.Subnet) {
			continue
		}
		key := candidate.String()
		if _, ok := s.inUse[key]; ok {
			continue
		}
		s.leases[clientID] = append(net.IP(nil), candidate...)
		s.inUse[key] = clientID
		s.expiry[clientID] = time.Now().Add(leaseDuration)
		return append(net.IP(nil), candidate...)
	}
	return nil
}

// expireLocked drops leases past their expiry so their addresses return to the
// pool. Callers must hold s.mu.
func (s *DHCPServer) expireLocked(now time.Time) {
	for clientID, deadline := range s.expiry {
		if now.Before(deadline) {
			continue
		}
		if ip, ok := s.leases[clientID]; ok {
			delete(s.inUse, ip.String())
			delete(s.leases, clientID)
		}
		delete(s.expiry, clientID)
	}
}

func (s *DHCPServer) firstLeaseIP() net.IP {
	ip := append(net.IP(nil), s.cfg.Subnet.IP.To4()...)
	ip[3] += 2
	return ip
}

// leaseDuration is advertised in option 51. Retail lp2p uses 5 seconds; that
// is hostile for debugging and for our in-process server, so we keep a long
// lease while matching the rest of the Offer option set (see reply()).
const leaseDuration = 8 * time.Hour

func broadcastAddr(subnet *net.IPNet) net.IP {
	ip := subnet.IP.To4()
	mask := subnet.Mask
	if ip == nil || len(mask) != 4 {
		return nil
	}
	out := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		out[i] = ip[i] | ^mask[i]
	}
	return out
}

func (s *DHCPServer) reply(req dhcpPacket, msgType byte, yourIP net.IP) []byte {
	resp := make([]byte, 240)
	resp[0] = bootReply
	resp[1] = req.htype
	resp[2] = req.hlen
	copy(resp[4:8], req.xid)
	copy(resp[16:20], yourIP.To4())
	copy(resp[20:24], s.cfg.ServerIP.To4())
	copy(resp[28:28+len(req.chaddr)], req.chaddr)
	copy(resp[236:240], []byte{99, 130, 83, 99})

	options := []byte{53, 1, msgType}
	options = appendIPOption(options, 54, s.cfg.ServerIP)

	// A NAK carries only the message type and server identifier; lease
	// parameters are meaningless when the request is being refused.
	if msgType != dhcpNak {
		// Prefer switchbrew lp2p DHCP Offer shape over openkartd/udhcpd:
		// subnet, server id, broadcast, lease, MTU. No router/DNS.
		// https://switchbrew.org/wiki/LDN_services#lp2p
		options = appendIPOption(options, 1, net.IP(s.cfg.Subnet.Mask))
		options = appendIPOption(options, 28, broadcastAddr(s.cfg.Subnet))
		options = append(options, 26, 2, 0x05, 0xdc) // interface MTU 1500

		lease := make([]byte, 4)
		binary.BigEndian.PutUint32(lease, uint32(leaseDuration.Seconds()))
		options = append(options, 51, 4)
		options = append(options, lease...)
		// Retail uses T1=0 / T2=0; advertise the same so the kart's renew
		// logic matches.
		options = append(options, 58, 4, 0, 0, 0, 0)
		options = append(options, 59, 4, 0, 0, 0, 0)
	}
	options = append(options, 255)

	resp = append(resp, options...)

	// Pad to the 300-byte BOOTP minimum; some clients reject shorter replies.
	for len(resp) < 300 {
		resp = append(resp, 0)
	}
	return resp
}

// appendIPOption writes a 4-byte IPv4 option, skipping it entirely when the
// value will not convert. Emitting a length of 4 with no payload, as the
// previous code could, produces a malformed packet.
func appendIPOption(options []byte, code byte, ip net.IP) []byte {
	v4 := ip.To4()
	if v4 == nil {
		return options
	}
	options = append(options, code, 4)
	return append(options, v4...)
}

type dhcpPacket struct {
	htype        byte
	hlen         byte
	xid          []byte
	chaddr       []byte
	clientHWAddr net.HardwareAddr
	options      map[byte][]byte
}

func parseDHCPPacket(data []byte) (dhcpPacket, error) {
	if len(data) < 240 {
		return dhcpPacket{}, errors.New("dhcp packet too short")
	}
	if !bytes.Equal(data[236:240], []byte{99, 130, 83, 99}) {
		return dhcpPacket{}, errors.New("missing dhcp magic cookie")
	}
	hlen := int(data[2])
	if hlen <= 0 || hlen > 16 {
		return dhcpPacket{}, errors.New("invalid dhcp hardware address length")
	}
	packet := dhcpPacket{
		htype:        data[1],
		hlen:         data[2],
		xid:          append([]byte(nil), data[4:8]...),
		chaddr:       append([]byte(nil), data[28:44]...),
		clientHWAddr: append(net.HardwareAddr(nil), data[28:28+hlen]...),
		options:      map[byte][]byte{},
	}

	options := data[240:]
	for i := 0; i < len(options); {
		code := options[i]
		i++
		if code == 0 {
			continue
		}
		if code == 255 {
			break
		}
		if i >= len(options) {
			break
		}
		length := int(options[i])
		i++
		if i+length > len(options) {
			break
		}
		packet.options[code] = append([]byte(nil), options[i:i+length]...)
		i += length
	}
	return packet, nil
}

func (p dhcpPacket) requestedIP() net.IP {
	if raw := p.options[50]; len(raw) == 4 {
		return net.IPv4(raw[0], raw[1], raw[2], raw[3]).To4()
	}
	return nil
}

func nextIPv4(ip net.IP) net.IP {
	next := append(net.IP(nil), ip.To4()...)
	next[3]++
	for i := 3; i > 0 && next[i] == 0; i-- {
		next[i-1]++
	}
	return next
}

func isBroadcast(ip net.IP, subnet *net.IPNet) bool {
	broadcast := append(net.IP(nil), subnet.IP.To4()...)
	mask := subnet.Mask
	for i := range broadcast {
		broadcast[i] |= ^mask[i]
	}
	return broadcast.Equal(ip.To4())
}
