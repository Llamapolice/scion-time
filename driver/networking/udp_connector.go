package networking

import (
	"example.com/scion-time/base/netbase"
	"example.com/scion-time/net/udp"
	"net"
	"time"
)

type UDPConnector struct {
	// Nothing yet
}

func (U *UDPConnector) ListenUDP(network string, laddr *net.UDPAddr) (*net.UDPConn, error) {
	return net.ListenUDP(network, laddr)
}

func (U *UDPConnector) EnableTimestamping(n *net.UDPConn, localHostIface string) error {
	return udp.EnableTimestamping(n, localHostIface)
}

func (U *UDPConnector) SetDSCP(n *net.UDPConn, dscp uint8) error {
	return udp.SetDSCP(n, dscp)
}

func (U *UDPConnector) ReadTXTimestamp(n *net.UDPConn) (time.Time, uint32, error) {
	return udp.ReadTXTimestamp(n)
}

var _ netbase.ConnProvider = (*UDPConnector)(nil)
