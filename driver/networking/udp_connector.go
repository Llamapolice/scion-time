package networking

import (
	"context"
	"example.com/scion-time/base/netbase"
	"example.com/scion-time/net/scion"
	"example.com/scion-time/net/udp"
	"github.com/libp2p/go-reuseport"
	"github.com/scionproto/scion/pkg/daemon"
	"log/slog"
	"net"
	"time"
)

type UDPConnector struct {
	Log *slog.Logger
}

func (U *UDPConnector) NewDaemonConnector(ctx context.Context, daemonAddr string) daemon.Connector {
	return scion.NewDaemonConnector(ctx, daemonAddr)
}

func (U *UDPConnector) ListenUDP(network string, laddr *net.UDPAddr) (netbase.Connection, error) {
	return net.ListenUDP(network, laddr)
}

func (U *UDPConnector) EnableTimestamping(n netbase.Connection, localHostIface string) error {
	return udp.EnableTimestamping(n.(*net.UDPConn), localHostIface)
}

func (U *UDPConnector) SetDSCP(n netbase.Connection, dscp uint8) error {
	return udp.SetDSCP(n.(*net.UDPConn), dscp)
}

func (U *UDPConnector) ReadTXTimestamp(n netbase.Connection) (time.Time, uint32, error) {
	return udp.ReadTXTimestamp(n.(*net.UDPConn))
}

func (U *UDPConnector) ListenPacket(network string, address string) (netbase.Connection, error) {
	conn, err := reuseport.ListenPacket(network, address)
	return conn.(netbase.Connection), err
}

var _ netbase.ConnProvider = (*UDPConnector)(nil)
