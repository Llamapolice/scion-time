package netprovider

import (
	"context"
	"github.com/scionproto/scion/pkg/daemon"
	"net"
	"net/netip"
	"time"
)

//type Connection struct {
//	net.UDPConn
//}

type Connection interface {
	Close() error
	Write(b []byte) (n int, err error)
	ReadMsgUDPAddrPort(buf []byte, oob []byte) (n int, oobn int, flags int, addr netip.AddrPort, err error)
	WriteToUDPAddrPort(b []byte, addr netip.AddrPort) (int, error)
	SetDeadline(t time.Time) error
	LocalAddr() net.Addr // based on core/client/client_scion.go:138, maybe change it to straight up give out the port?
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
