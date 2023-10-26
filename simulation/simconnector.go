package simulation

import (
	"example.com/scion-time/base/netbase"
	"net"
	"time"
)

type SimConnector struct {
	// Nothing yet
}

func (s *SimConnector) ListenUDP(network string, laddr *net.UDPAddr) (*net.UDPConn, error) {
	//TODO implement me
	panic("implement me")
}

func (s *SimConnector) EnableTimestamping(n *net.UDPConn, localHostIface string) error {
	//TODO implement me
	panic("implement me")
}

func (s *SimConnector) SetDSCP(n *net.UDPConn, dscp uint8) error {
	//TODO implement me
	panic("implement me")
}

func (s *SimConnector) ReadTXTimestamp(n *net.UDPConn) (time.Time, uint32, error) {
	//TODO implement me
	panic("implement me")
}

var _ netbase.ConnProvider = (*SimConnector)(nil)
