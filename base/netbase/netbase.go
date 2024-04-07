package netbase

import (
	"context"
	"github.com/scionproto/scion/pkg/daemon"
	"net"
	"net/netip"
	"time"
)

type Connection interface {
	Close() error
	ReadMsgUDPAddrPort(buf []byte, oob []byte) (n int, oobn int, flags int, addr netip.AddrPort, err error)
	WriteToUDPAddrPort(b []byte, addr netip.AddrPort) (int, error)
	SetDeadline(t time.Time) error
	LocalAddr() net.Addr
}

var _ Connection = (*net.UDPConn)(nil)

type ConnProvider interface {
	ListenUDP(network string, laddr *net.UDPAddr) (Connection, error)
	EnableTimestamping(n Connection, localHostIface string) error
	SetDSCP(n Connection, dscp uint8) error
	ReadTXTimestamp(n Connection) (time.Time, uint32, error)
	ListenPacket(network string, address string) (Connection, error)
	NewDaemonConnector(ctx context.Context, daemonAddr string) daemon.Connector
}
