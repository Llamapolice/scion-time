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
	s.log.Info("Opening a new sim connection")
	return &SimConnection{Log: s.log}, nil
}

func (s *SimConnector) EnableTimestamping(n netprovider.Connection, localhostIface string) error {
	//TODO does this need more code?
	s.log.Info("Enabling timestamping")
	if _, ok := n.(*SimConnection); !ok {
		s.log.Error("SimConnector method EnableTimestamping called on a non-simulated connection")
	}
	return nil
}

func (s *SimConnector) SetDSCP(n netprovider.Connection, dscp uint8) error {
	//TODO implement me
	sconn, ok := n.(*SimConnection)
	if !ok {
		s.log.Error("SimConnector method SetDSCP called on a non-simulated connection")
		return nil
	}
	sconn.DSCP = dscp // This is optional but might be useful for debugging later
	return nil
}

func (s *SimConnector) ReadTXTimestamp(n netprovider.Connection) (time.Time, uint32, error) {
	//TODO implement me
	// Just return error, upstreams will find another one
	return time.Time{}, 0, SimConnectorError{}
}

func (s *SimConnector) ListenPacket(network string, address string) (netprovider.Connection, error) {
	return s.ListenUDP(network, nil)
}

var _ netprovider.ConnProvider = (*SimConnector)(nil)

type SimConnectorError struct {
	// Currently empty
}

func (e SimConnectorError) Error() string {
	return "this is a simulated connection, find another timestamp"
}
