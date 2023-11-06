package netbase

import (
	"example.com/scion-time/base/netprovider"
	"net"
	"sync/atomic"
	"time"
)

// TODO: structure copied from timebase

var lnetprovider atomic.Value

func RegisterNetProvider(n netprovider.ConnProvider) {
	if n == nil {
		panic("net provider must not be nil")
	}
	if swapped := lnetprovider.CompareAndSwap(nil, n); !swapped {
		panic("net provider already registered, can only register one")
	}
}

func getNetProvider() netprovider.ConnProvider {
	c := lnetprovider.Load().(netprovider.ConnProvider)
	if c == nil {
		panic("no net provider registered")
	}
	return c
}

func ListenUDP(network string, laddr *net.UDPAddr) (netprovider.Connection, error) {
	return getNetProvider().ListenUDP(network, laddr)
}

func EnableTimestamping(n netprovider.Connection, localHostIface string) error {
	return getNetProvider().EnableTimestamping(n, localHostIface)
}

func SetDSCP(n netprovider.Connection, dscp uint8) error {
	return getNetProvider().SetDSCP(n, dscp)
}

func ReadTXTimestamp(n netprovider.Connection) (time.Time, uint32, error) {
	return getNetProvider().ReadTXTimestamp(n)
}
