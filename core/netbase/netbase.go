package netbase

import (
	"example.com/scion-time/base/netbase"
	"net"
	"sync/atomic"
	"time"
)

// TODO: structure copied from timebase

var lnetprovider atomic.Value

func RegisterNetProvider(n netbase.ConnProvider) {
	if n == nil {
		panic("net provider must not be nil")
	}
	if swapped := lnetprovider.CompareAndSwap(nil, n); !swapped {
		panic("net provider already registered, can only register one")
	}
}

func getNetProvider() netbase.ConnProvider {
	c := lnetprovider.Load().(netbase.ConnProvider)
	if c == nil {
		panic("no net provider registered")
	}
	return c
}

func ListenUDP(network string, laddr *net.UDPAddr) (*net.UDPConn, error) {
	return getNetProvider().ListenUDP(network, laddr)
}

func EnableTimestamping(n *net.UDPConn, localHostIface string) error {
	return getNetProvider().EnableTimestamping(n, localHostIface)
}

func SetDSCP(n *net.UDPConn, dscp uint8) error {
	return getNetProvider().SetDSCP(n, dscp)
}

func ReadTXTimestamp(n *net.UDPConn) (time.Time, uint32, error) {
	return getNetProvider().ReadTXTimestamp(n)
}
