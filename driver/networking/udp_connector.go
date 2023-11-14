package networking

import (
	"example.com/scion-time/base/netprovider"
	"example.com/scion-time/net/udp"
	"github.com/libp2p/go-reuseport"
	"net"
	"time"
)

type UDPConnector struct {
	// Nothing yet
}

func (U *UDPConnector) ListenUDP(network string, laddr *net.UDPAddr) (netprovider.Connection, error) {
	return net.ListenUDP(network, laddr)
}

func (U *UDPConnector) EnableTimestamping(n netprovider.Connection, localHostIface string) error {
	return udp.EnableTimestamping(n.(*net.UDPConn), localHostIface)
}

func (U *UDPConnector) SetDSCP(n netprovider.Connection, dscp uint8) error {
	return udp.SetDSCP(n.(*net.UDPConn), dscp)
}

func (U *UDPConnector) ReadTXTimestamp(n netprovider.Connection) (time.Time, uint32, error) {
	return udp.ReadTXTimestamp(n.(*net.UDPConn))
}

func (U *UDPConnector) ListenPacket(network string, address string) (netprovider.Connection, error) {
	conn, err := reuseport.ListenPacket(network, address)
	return conn.(netprovider.Connection), err
}

var _ netprovider.ConnProvider = (*UDPConnector)(nil)
