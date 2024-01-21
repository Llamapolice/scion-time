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
	"github.com/scionproto/scion/pkg/snet/path"
	"go.uber.org/zap"
	"net"
	"strings"
	"time"
)

type SimConnector struct {
	CallBack chan *SimConnection

	log  *zap.Logger
	port int
}

type SimDaemonConnector struct {
	Ctx        context.Context
	DaemonAddr string
	CallerIA   addr.IA
}

func (s SimDaemonConnector) LocalIA(ctx context.Context) (addr.IA, error) {
	return s.CallerIA, nil
}

func (s SimDaemonConnector) Paths(ctx context.Context, dst, src addr.IA, f daemon.PathReqFlags) ([]snet.Path, error) {
	//TODO does this need more?
	paths := []snet.Path{
		path.Path{Src: s.CallerIA, Dst: s.CallerIA, DataplanePath: path.Empty{}},
	}
	return paths, nil
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
	var laddrIA addr.IA
	s.log.Debug("New daemon connector being requested", zap.String("passed daemonAddr", daemonAddr))
	err := laddrIA.Set(strings.Split(daemonAddr, "@")[1])
	if err != nil {
		s.log.Error("Couldn't set IA for daemon connector", zap.Error(err))
		laddrIA = 0
	}
	return SimDaemonConnector{
		Ctx:        ctx,
		DaemonAddr: daemonAddr,
		CallerIA:   laddrIA,
	}
}

func NewSimConnector(log *zap.Logger) *SimConnector {
	log.Info("Creating a new sim connector")
	return &SimConnector{log: log, port: 1000}
}

func (s *SimConnector) ListenUDP(network string, laddr *net.UDPAddr) (netprovider.Connection, error) {
	s.log.Info("Opening a new sim connection")
	if laddr.Port == 0 {
		p := s.port
		s.log.Debug("Incoming port is 0, assigned one by SimConnector", zap.Int("new port", p))
		s.port += 1
		laddr.Port = p
	}
	simConn := &SimConnection{Log: s.log, Network: network, LAddr: laddr}
	s.CallBack <- simConn
	s.log.Debug("Sim connection passed into channel", zap.String("network", network), zap.String("laddr", laddr.String()))
	return simConn, nil
}

func (s *SimConnector) EnableTimestamping(n netprovider.Connection, localhostIface string) error {
	//TODO does this need more code?
	s.log.Info("Enabling timestamping")
	if _, ok := n.(*SimConnection); !ok {
		s.log.Fatal("SimConnector method EnableTimestamping called on a non-simulated connection")
	}
	return nil
}

func (s *SimConnector) SetDSCP(n netprovider.Connection, dscp uint8) error {
	//TODO implement me
	sconn, ok := n.(*SimConnection)
	if !ok {
		s.log.Fatal("SimConnector method SetDSCP called on a non-simulated connection")
		return nil
	}
	sconn.DSCP = dscp // This is optional but might be useful for debugging later
	return nil
}

func (s *SimConnector) ReadTXTimestamp(n netprovider.Connection) (time.Time, uint32, error) {
	// Just return error, upstreams will find another one
	sconn, ok := n.(*SimConnection)
	if !ok {
		s.log.Fatal("SimConnector method ReadTXTimestamp called on a non-simulated connection")
		return time.Time{}, 0, nil
	}
	return time.Time{}, 0, SimConnectorError{errString: sconn.Id + " is a simulated connection, find another timestamp"}
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
