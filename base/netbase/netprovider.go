package netbase

import (
	"example.com/scion-time/net/udp"
	"net"
	"net/netip"
	"time"
)

type ConnProvider interface {
	Close() error
	Write(b []byte) (n int, err error)
	ReadMsgUDPAddrPort(buf []byte, oob []byte) (n int, oobn int, flags int, addr netip.AddrPort, err error)
	WriteToUDPAddrPort(b []byte, addr netip.AddrPort) (int, error)
	SetDeadline(t time.Time) error
	LocalAddr() net.Addr // based on core/client/client_scion.go:138, maybe change it to straight up give out the port?

	ListenUDP(network string, laddr *udp.UDPAddr) (*ConnProvider, error)
	EnableTimestamping(n ConnProvider, localHostIface string) error
	SetDSCP(n ConnProvider, dscp uint8) error
	ReadTXTimestamp(n ConnProvider) (time.Time, uint32, error)
}
