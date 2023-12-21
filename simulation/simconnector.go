package simulation

import (
	"context"
	"example.com/scion-time/base/netprovider"
	"github.com/scionproto/scion/pkg/addr"
	"github.com/scionproto/scion/pkg/daemon"
	"github.com/scionproto/scion/pkg/drkey"
	"github.com/scionproto/scion/pkg/private/common"
	"github.com/scionproto/scion/pkg/private/ctrl/path_mgmt"
	"github.com/scionproto/scion/pkg/snet"
	"go.uber.org/zap"
	"net"
	"time"
)

type SimConnector struct {
	CallBack chan *SimConnection

	log *zap.Logger
}

type SimDaemonConnector struct {
}

func (s SimDaemonConnector) LocalIA(ctx context.Context) (addr.IA, error) {
	//TODO implement me
	panic("implement me")
}

func (s SimDaemonConnector) Paths(ctx context.Context, dst, src addr.IA, f daemon.PathReqFlags) ([]snet.Path, error) {
	//TODO implement me
	panic("implement me")
}

func (s SimDaemonConnector) ASInfo(ctx context.Context, ia addr.IA) (daemon.ASInfo, error) {
	//TODO implement me
	panic("implement me")
}

func (s SimDaemonConnector) IFInfo(ctx context.Context, ifs []common.IFIDType) (map[common.IFIDType]*net.UDPAddr, error) {
	//TODO implement me
	panic("implement me")
}

func (s SimDaemonConnector) SVCInfo(ctx context.Context, svcTypes []addr.SVC) (map[addr.SVC][]string, error) {
	//TODO implement me
	panic("implement me")
}

func (s SimDaemonConnector) RevNotification(ctx context.Context, revInfo *path_mgmt.RevInfo) error {
	//TODO implement me
	panic("implement me")
}

func (s SimDaemonConnector) DRKeyGetASHostKey(ctx context.Context, meta drkey.ASHostMeta) (drkey.ASHostKey, error) {
	//TODO implement me
	panic("implement me")
}

func (s SimDaemonConnector) DRKeyGetHostASKey(ctx context.Context, meta drkey.HostASMeta) (drkey.HostASKey, error) {
	//TODO implement me
	panic("implement me")
}

func (s SimDaemonConnector) DRKeyGetHostHostKey(ctx context.Context, meta drkey.HostHostMeta) (drkey.HostHostKey, error) {
	//TODO implement me
	panic("implement me")
}

func (s SimDaemonConnector) Close() error {
	//TODO implement me
	panic("implement me")
}

var _ daemon.Connector = (*SimDaemonConnector)(nil)

func (s *SimConnector) NewDaemonConnector(ctx context.Context, daemonAddr string) daemon.Connector {
	//TODO implement me
	return SimDaemonConnector{}
}

func NewSimConnector(log *zap.Logger) *SimConnector {
	log.Info("Creating a new sim connector")
	return &SimConnector{log: log}
}

func (s *SimConnector) ListenUDP(network string, laddr *net.UDPAddr) (netprovider.Connection, error) {
	s.log.Info("Opening a new sim connection")
	simConn := &SimConnection{Log: s.log, Network: network, LAddr: laddr}
	s.CallBack <- simConn
	s.log.Debug("Sim connection passed into channel")
	return simConn, nil
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
	return time.Time{}, 0, SimConnectorError{errString: "this is a simulated connection, find another timestamp"}
}

func (s *SimConnector) ListenPacket(network string, address string) (netprovider.Connection, error) {
	return s.ListenUDP(network, nil)
}

var _ netprovider.ConnProvider = (*SimConnector)(nil)

type SimConnectorError struct {
	errString string
}

func (e SimConnectorError) Error() string {
	return e.errString
}
