package wireless

import (
	"net"
	"testing"
	"time"
)

func testDHCPServer(t *testing.T) *DHCPServer {
	t.Helper()
	_, subnet, err := net.ParseCIDR("192.168.137.0/24")
	if err != nil {
		t.Fatalf("ParseCIDR() error = %v", err)
	}
	s, err := NewDHCPServer(DHCPConfig{
		Subnet:   subnet,
		Gateway:  net.IPv4(192, 168, 137, 1),
		ServerIP: net.IPv4(192, 168, 137, 1),
	})
	if err != nil {
		t.Fatalf("NewDHCPServer() error = %v", err)
	}
	return s
}

// buildDHCPRequest assembles a minimal BOOTP packet with the given message type
// and optional requested-IP option.
func buildDHCPRequest(msgType byte, mac net.HardwareAddr, requested net.IP) []byte {
	buf := make([]byte, 240)
	buf[0] = 1
	buf[1] = 1
	buf[2] = byte(len(mac))
	copy(buf[28:], mac)
	copy(buf[236:240], []byte{99, 130, 83, 99})

	opts := []byte{53, 1, msgType}
	if requested != nil {
		opts = append(opts, 50, 4)
		opts = append(opts, requested.To4()...)
	}
	opts = append(opts, 255)
	return append(buf, opts...)
}

func replyYourIP(resp []byte) net.IP {
	return net.IP(resp[16:20])
}

func replyMsgType(resp []byte) byte {
	// Options start after the 240-byte header; option 53 is emitted first.
	if len(resp) < 243 {
		return 0
	}
	return resp[242]
}

func TestDHCPDiscoverOffersInSubnetAddress(t *testing.T) {
	s := testDHCPServer(t)
	mac := net.HardwareAddr{2, 0, 0, 0, 0, 1}

	resp, err := s.handlePacket(buildDHCPRequest(dhcpDiscover, mac, nil))
	if err != nil {
		t.Fatalf("handlePacket() error = %v", err)
	}
	if replyMsgType(resp) != dhcpOffer {
		t.Fatalf("message type = %d, want OFFER", replyMsgType(resp))
	}
	ip := replyYourIP(resp)
	if !s.cfg.Subnet.Contains(ip) {
		t.Fatalf("offered %s, which is outside the subnet", ip)
	}
	if ip.Equal(s.cfg.Gateway) {
		t.Fatal("offered the gateway address")
	}
}

// A client must not be able to claim the gateway by asking for it. Previously
// option 50 was echoed straight back into an ACK.
func TestDHCPRequestCannotClaimGateway(t *testing.T) {
	s := testDHCPServer(t)
	mac := net.HardwareAddr{2, 0, 0, 0, 0, 1}

	resp, err := s.handlePacket(buildDHCPRequest(dhcpRequest, mac, net.IPv4(192, 168, 137, 1)))
	if err != nil {
		t.Fatalf("handlePacket() error = %v", err)
	}
	if replyMsgType(resp) != dhcpNak {
		t.Fatalf("message type = %d, want NAK for a gateway claim", replyMsgType(resp))
	}
}

func TestDHCPRequestCannotClaimOffSubnetAddress(t *testing.T) {
	s := testDHCPServer(t)
	mac := net.HardwareAddr{2, 0, 0, 0, 0, 1}

	resp, err := s.handlePacket(buildDHCPRequest(dhcpRequest, mac, net.IPv4(10, 0, 0, 5)))
	if err != nil {
		t.Fatalf("handlePacket() error = %v", err)
	}
	if replyMsgType(resp) != dhcpNak {
		t.Fatalf("message type = %d, want NAK for an off-subnet claim", replyMsgType(resp))
	}
}

// One client must not be able to steal another's active lease.
func TestDHCPRequestCannotStealAnotherLease(t *testing.T) {
	s := testDHCPServer(t)
	first := net.HardwareAddr{2, 0, 0, 0, 0, 1}
	second := net.HardwareAddr{2, 0, 0, 0, 0, 2}

	resp, err := s.handlePacket(buildDHCPRequest(dhcpDiscover, first, nil))
	if err != nil {
		t.Fatalf("handlePacket() error = %v", err)
	}
	leased := append(net.IP(nil), replyYourIP(resp)...)

	resp, err = s.handlePacket(buildDHCPRequest(dhcpRequest, second, leased))
	if err != nil {
		t.Fatalf("handlePacket() error = %v", err)
	}
	if replyMsgType(resp) != dhcpNak {
		t.Fatalf("message type = %d, want NAK when claiming another client's lease", replyMsgType(resp))
	}
}

// Requesting the address previously offered to the same client must succeed,
// and the lease must be recorded so the allocator does not reissue it.
func TestDHCPRequestHonorsOwnLeaseAndRecordsIt(t *testing.T) {
	s := testDHCPServer(t)
	mac := net.HardwareAddr{2, 0, 0, 0, 0, 1}

	resp, err := s.handlePacket(buildDHCPRequest(dhcpDiscover, mac, nil))
	if err != nil {
		t.Fatalf("handlePacket() error = %v", err)
	}
	offered := append(net.IP(nil), replyYourIP(resp)...)

	resp, err = s.handlePacket(buildDHCPRequest(dhcpRequest, mac, offered))
	if err != nil {
		t.Fatalf("handlePacket() error = %v", err)
	}
	if replyMsgType(resp) != dhcpAck {
		t.Fatalf("message type = %d, want ACK", replyMsgType(resp))
	}
	if !replyYourIP(resp).Equal(offered) {
		t.Fatalf("ACK address = %s, want %s", replyYourIP(resp), offered)
	}

	s.mu.Lock()
	owner, tracked := s.inUse[offered.String()]
	s.mu.Unlock()
	if !tracked || owner != mac.String() {
		t.Fatalf("lease for %s not recorded against the client", offered)
	}
}

// An unclaimed in-subnet address may be requested without a prior DISCOVER.
func TestDHCPRequestAcceptsFreeAddress(t *testing.T) {
	s := testDHCPServer(t)
	mac := net.HardwareAddr{2, 0, 0, 0, 0, 7}
	want := net.IPv4(192, 168, 137, 50)

	resp, err := s.handlePacket(buildDHCPRequest(dhcpRequest, mac, want))
	if err != nil {
		t.Fatalf("handlePacket() error = %v", err)
	}
	if replyMsgType(resp) != dhcpAck {
		t.Fatalf("message type = %d, want ACK", replyMsgType(resp))
	}
	if !replyYourIP(resp).Equal(want.To4()) {
		t.Fatalf("ACK address = %s, want %s", replyYourIP(resp), want)
	}
}

func TestDHCPReleaseReturnsAddressToPool(t *testing.T) {
	s := testDHCPServer(t)
	mac := net.HardwareAddr{2, 0, 0, 0, 0, 1}

	resp, err := s.handlePacket(buildDHCPRequest(dhcpDiscover, mac, nil))
	if err != nil {
		t.Fatalf("handlePacket() error = %v", err)
	}
	leased := append(net.IP(nil), replyYourIP(resp)...)

	if _, err := s.handlePacket(buildDHCPRequest(dhcpRelease, mac, nil)); err != nil {
		t.Fatalf("handlePacket() error = %v", err)
	}

	s.mu.Lock()
	_, stillHeld := s.inUse[leased.String()]
	s.mu.Unlock()
	if stillHeld {
		t.Fatalf("%s still held after RELEASE", leased)
	}
}

func TestDHCPLeaseExpiryFreesAddress(t *testing.T) {
	s := testDHCPServer(t)
	mac := net.HardwareAddr{2, 0, 0, 0, 0, 1}

	ip := s.allocate(mac.String())
	if ip == nil {
		t.Fatal("allocate() returned nil")
	}

	// Backdate the lease so the next sweep collects it.
	s.mu.Lock()
	s.expiry[mac.String()] = time.Now().Add(-time.Minute)
	s.expireLocked(time.Now())
	_, stillHeld := s.inUse[ip.String()]
	s.mu.Unlock()

	if stillHeld {
		t.Fatalf("%s survived expiry", ip)
	}
}

// Exhaustion must yield no reply rather than handing out the server's own
// address, which is what the old code did.
func TestDHCPExhaustionReturnsNil(t *testing.T) {
	_, subnet, err := net.ParseCIDR("192.168.137.0/30")
	if err != nil {
		t.Fatalf("ParseCIDR() error = %v", err)
	}
	s, err := NewDHCPServer(DHCPConfig{
		Subnet:   subnet,
		Gateway:  net.IPv4(192, 168, 137, 1),
		ServerIP: net.IPv4(192, 168, 137, 1),
	})
	if err != nil {
		t.Fatalf("NewDHCPServer() error = %v", err)
	}

	// A /30 leaves only .2 usable once .0, .1 (gateway), and .3 (broadcast)
	// are excluded.
	if ip := s.allocate("aa:aa:aa:aa:aa:01"); ip == nil {
		t.Fatal("first allocation failed")
	}
	if ip := s.allocate("aa:aa:aa:aa:aa:02"); ip != nil {
		t.Fatalf("allocate() = %s, want nil once the pool is exhausted", ip)
	}
	if ip := s.allocate("aa:aa:aa:aa:aa:03"); ip != nil && ip.Equal(s.cfg.Gateway) {
		t.Fatal("allocate() handed out the server's own address")
	}
}

func TestDHCPNakOmitsLeaseOptions(t *testing.T) {
	s := testDHCPServer(t)
	mac := net.HardwareAddr{2, 0, 0, 0, 0, 1}

	resp := s.reply(dhcpPacket{htype: 1, hlen: 6, xid: []byte{1, 2, 3, 4},
		chaddr: make([]byte, 16), clientHWAddr: mac}, dhcpNak, net.IPv4zero)

	// Option 51 (lease time) is meaningless in a NAK.
	for i := 240; i+1 < len(resp); {
		code := resp[i]
		if code == 255 {
			break
		}
		if code == 0 {
			i++
			continue
		}
		if code == 51 {
			t.Fatal("NAK carried a lease-time option")
		}
		i += 2 + int(resp[i+1])
	}
}

// Every reply must meet the 300-byte BOOTP minimum.
func TestDHCPReplyMeetsMinimumSize(t *testing.T) {
	s := testDHCPServer(t)
	resp, err := s.handlePacket(buildDHCPRequest(dhcpDiscover, net.HardwareAddr{2, 0, 0, 0, 0, 1}, nil))
	if err != nil {
		t.Fatalf("handlePacket() error = %v", err)
	}
	if len(resp) < 300 {
		t.Fatalf("reply is %d bytes, want at least 300", len(resp))
	}
}

func FuzzParseDHCPPacket(f *testing.F) {
	f.Add(buildDHCPRequest(dhcpDiscover, net.HardwareAddr{2, 0, 0, 0, 0, 1}, nil))
	f.Add(buildDHCPRequest(dhcpRequest, net.HardwareAddr{2, 0, 0, 0, 0, 1}, net.IPv4(192, 168, 137, 9)))
	f.Add(make([]byte, 240))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		packet, err := parseDHCPPacket(data)
		if err != nil {
			return
		}
		if len(packet.chaddr) > 16 {
			t.Fatalf("chaddr is %d bytes, want at most 16", len(packet.chaddr))
		}
	})
}
