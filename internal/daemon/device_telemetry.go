package daemon

import "net"

const (
	fujiTelemetryBasePort  = 0x13fc
	fujiLSPControlBasePort = 0x13a8
	fujiLSPVideoBasePort   = 0x1398
	maximumMediaSlots      = 4
)

func hostWithoutPort(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err == nil {
		return host
	}
	return address
}
