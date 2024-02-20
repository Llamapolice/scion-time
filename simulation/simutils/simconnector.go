package simutils

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
	"strconv"
	"strings"
	"time"
)

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

type SimConnector struct {
	Id           string
	LocalAddress *snet.UDPAddr
	// This channel is where all messages to this SimConnector's corresponding IP address are sent
	Input chan SimPacket

	// Every SimConnection created by this SimConnector will apply this function to messages before passing it to the handler
	ModifyOutgoing func(packet *SimPacket)
	// Every message received by this SimConnector gets this function applied to it before entering the local message handler logic
	ModifyIncoming func(packet *SimPacket)
	// This function provides the baseline latency to a created SimConnection
	DefineLatency func(laddr *net.UDPAddr) time.Duration

	log *zap.Logger
	// This channel is where all spawned SimConnection write to
	globalMessageBus chan SimPacket
	// Current port number to be assigned to the next SimConnection
	port int
	// Mapping from ports to the corresponding SimConnection;s input channel
	connections map[int]chan SimPacket
	// Channel to request deletion or insertion in connections map using RequestFromMapHandler structs
	connectionsHandler chan RequestFromMapHandler
	// This Channel gets passed to created SimConnection to enable their deadline functionality via the local clock
	requestDeadline chan DeadlineRequest

	// Counter for how many times the port has reached 60k and restarted at 10k (probably not useful)
	portCycles int
}

func (s *SimConnector) NewDaemonConnector(ctx context.Context, daemonAddr string) daemon.Connector {
	var laddrIA addr.IA
	s.log.Debug("New daemon connector being requested",
		zap.String("passed daemonAddr", daemonAddr))
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

func NewSimConnector(log *zap.Logger, id string, laddr *snet.UDPAddr, globalMessageBus chan SimPacket, requestDeadline chan DeadlineRequest, ModifyIncomingMsg, ModifyOutgoingMsg func(packet *SimPacket), DefineLatency func(laddr *net.UDPAddr) time.Duration) *SimConnector {
	id = id + "_SimConnector"
	log.Info("Creating a new sim connector", zap.String("id", id), zap.String("laddr", laddr.String()))

	connections := make(map[int]chan SimPacket)
	connectionsHandlerInput := make(chan RequestFromMapHandler)
	go func() {
		// See doc for RequestFromMapHandler for some explanations
		for request := range connectionsHandlerInput {
			port := request.Todo()
			if port >= 0 {
				delete(connections, port)
			}
			request.ReturnBack <- port
			close(request.ReturnBack)
		}
	}()

	input := make(chan SimPacket)
	go func() {
		// This goroutine distributes incoming messages to the corresponding connection based on ports
		for msg := range input {
			ModifyIncomingMsg(&msg)
			port := int(msg.TargetAddr.Port())
			log.Debug("Message received", zap.String("connector id", id), zap.Int("target port", port))
			conn, ok := connections[port]
			if !ok {
				log.Error("Received packet for unknown connection", zap.String("targetAddr", msg.TargetAddr.String()))
				continue
			}
			conn <- msg
		}
	}()

	return &SimConnector{
		Id:                 id,
		LocalAddress:       laddr,
		Input:              input,
		ModifyIncoming:     ModifyIncomingMsg,
		ModifyOutgoing:     ModifyOutgoingMsg,
		DefineLatency:      DefineLatency,
		log:                log,
		globalMessageBus:   globalMessageBus,
		port:               10000,
		connections:        connections,
		connectionsHandler: connectionsHandlerInput,
		requestDeadline:    requestDeadline,
		portCycles:         0,
	}
}

func (s *SimConnector) ListenUDP(network string, laddr_orig *net.UDPAddr) (netprovider.Connection, error) {
	s.log.Info("Opening a sim connection")
	laddr := *laddr_orig
	if laddr.Port == 0 {
		laddr.Port = s.port
		s.port += 1
		if s.port == 60000 {
			s.log.Warn("Created 50000 connections from one connector", zap.String("connector id", s.Id))
			s.port = 10000
			s.portCycles += 1
		}
		s.log.Debug("Incoming port is 0, assigned one by SimConnector",
			zap.Int("new port", laddr.Port))
	}
	connReadFrom := make(chan SimPacket)
	simConn := &SimConnection{
		Log:                s.log,
		Id:                 s.Id + "_connection_port" + strconv.Itoa(laddr.Port) + "_iter" + strconv.Itoa(s.portCycles),
		ReadFrom:           connReadFrom,
		WriteTo:            s.globalMessageBus,
		Latency:            s.DefineLatency(&laddr),
		ModifyOutgoing:     s.ModifyOutgoing,
		Network:            network,
		LAddr:              &laddr,
		ConnectionsHandler: s.connectionsHandler,
		RequestDeadline:    s.requestDeadline,
	}
	tmp := make(chan interface{})
	s.connectionsHandler <- RequestFromMapHandler{
		Todo: func() int {
			s.connections[laddr.Port] = connReadFrom
			return -1
		},
		ReturnBack: tmp,
	}
	<-tmp
	return simConn, nil
}

func (s *SimConnector) EnableTimestamping(n netprovider.Connection, localhostIface string) error {
	if _, ok := n.(*SimConnection); !ok {
		s.log.Fatal("SimConnector method EnableTimestamping called on a non-simulated connection")
	}
	return nil
}

func (s *SimConnector) SetDSCP(n netprovider.Connection, dscp uint8) error {
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
	panic("Multiple server goroutines listening on same port not supported by the simulator")
}

var _ netprovider.ConnProvider = (*SimConnector)(nil)
