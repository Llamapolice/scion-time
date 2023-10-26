package netbase

import (
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

type ConnProvider interface {
	ListenUDP(network string, laddr *net.UDPAddr) (*net.UDPConn, error)
	EnableTimestamping(n *net.UDPConn, localHostIface string) error
	SetDSCP(n *net.UDPConn, dscp uint8) error
	ReadTXTimestamp(n *net.UDPConn) (time.Time, uint32, error)
}
