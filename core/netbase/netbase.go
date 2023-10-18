package netbase

import (
	"example.com/scion-time/base/netbase"
	"example.com/scion-time/net/udp"
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

func ListenUDP(network string, laddr *udp.UDPAddr) (*netbase.ConnProvider, error) {
	panic("ListenUDP not implemented yet")
}

func EnableTimestamping(n *netbase.ConnProvider, localHostIface string) error {
	panic("EnableTimestamping not yet implemented")
}

func SetDSCP(n *netbase.ConnProvider, dscp uint8) error {
	panic("SetDSCP not yet implemented")
}

func ReadTXTimestamp(n *netbase.ConnProvider) (time.Time, uint32, error) {
	panic("ReadTXTimestamp not implemented yet")
}

func Close() error {
	return getNetProvider().Close()
}
