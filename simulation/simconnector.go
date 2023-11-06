package simulation

import (
	"example.com/scion-time/base/netprovider"
	"go.uber.org/zap"
	"net"
	"time"
)

type SimConnector struct {
	log *zap.Logger
}

func NewSimConnector(log *zap.Logger) *SimConnector {
	log.Info("Creating a new sim connector")
	return &SimConnector{log: log}
}

func (s *SimConnector) ListenUDP(network string, laddr *net.UDPAddr) (netprovider.Connection, error) {
	//TODO implement me
	panic("implement me")
}

func (s *SimConnector) EnableTimestamping(n netprovider.Connection, localHostIface string) error {
	//TODO implement me
	panic("implement me")
}

func (s *SimConnector) SetDSCP(n netprovider.Connection, dscp uint8) error {
	//TODO implement me
	panic("implement me")
}

func (s *SimConnector) ReadTXTimestamp(n netprovider.Connection) (time.Time, uint32, error) {
	//TODO implement me
	panic("implement me")
}

func (s *SimConnector) ListenPacket(network string, address string) (netprovider.Connection, error) {
	// TODO implement me
	panic("implement me")
}

var _ netprovider.ConnProvider = (*SimConnector)(nil)
