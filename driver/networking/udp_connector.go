package networking

import (
	"example.com/scion-time/base/netprovider"
	"example.com/scion-time/net/udp"
	"fmt"
	"github.com/libp2p/go-reuseport"
	"net"
	"net/netip"
	"time"
)

type UDPConnector struct {
	// Nothing yet
}

type InterceptedConn struct {
	*net.UDPConn
}

func (receiver InterceptedConn) ReadMsgUDPAddrPort(buf []byte, oob []byte) (n int, oobn int, flags int, addr netip.AddrPort, err error) {
	n, oobn, flags, addr, err = receiver.UDPConn.ReadMsgUDPAddrPort(buf, oob)
	fmt.Printf("Reading from Connection, n: %d, oobn: %d, flags: %d, addr raw: %v, addr string: %s\n", n, oobn, flags, addr, addr.String())
	fmt.Printf("In first n bytes of buf: %v, first oobn bytes of oob: %v\n", buf[:n], oob[:oobn])
	return
}

func (receiver InterceptedConn) WriteToUDPAddrPort(b []byte, addr netip.AddrPort) (int, error) {
	n, err := receiver.UDPConn.WriteToUDPAddrPort(b, addr)
	fmt.Printf("Writing to Connetion, addr raw: %v, addr string: %s\n", addr, addr.String())
	fmt.Printf("Buffer to be written: %v, n bytes written(?): %d", b, n)
	return n, err
}

func (U *UDPConnector) ListenUDP(network string, laddr *net.UDPAddr) (netprovider.Connection, error) {
	conn, err := net.ListenUDP(network, laddr)
	return InterceptedConn{conn}, err
}

func (U *UDPConnector) EnableTimestamping(n netprovider.Connection, localHostIface string) error {
	return udp.EnableTimestamping(n.(*net.UDPConn), localHostIface)
}

func (U *UDPConnector) SetDSCP(n netprovider.Connection, dscp uint8) error {
	return udp.SetDSCP(n.(*net.UDPConn), dscp)
}

func (U *UDPConnector) ReadTXTimestamp(n netprovider.Connection) (time.Time, uint32, error) {
	return udp.ReadTXTimestamp(n.(*net.UDPConn))
}

func (U *UDPConnector) ListenPacket(network string, address string) (netprovider.Connection, error) {
	conn, err := reuseport.ListenPacket(network, address)
	return conn.(netprovider.Connection), err
}

var _ netprovider.ConnProvider = (*UDPConnector)(nil)
