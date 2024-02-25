package netcore

import (
	"context"
	"example.com/scion-time/base/netbase"
	"github.com/scionproto/scion/pkg/daemon"
	"net"
	"sync/atomic"
	"time"
)

// TODO: structure copied from timecore

var lnetprovider atomic.Value

func RegisterConnProvider(n netbase.ConnProvider) {
	if n == nil {
		panic("conn provider must not be nil")
	}
	if swapped := lnetprovider.CompareAndSwap(nil, n); !swapped {
		panic("conn provider already registered, can only register one")
	}
}

func getNetProvider() netbase.ConnProvider {
	c := lnetprovider.Load().(netbase.ConnProvider)
	if c == nil {
		panic("no net provider registered")
	}
	return c
}

func ListenUDP(network string, laddr *net.UDPAddr) (netbase.Connection, error) {
	return getNetProvider().ListenUDP(network, laddr)
}

func EnableTimestamping(n netbase.Connection, localHostIface string) error {
	return getNetProvider().EnableTimestamping(n, localHostIface)
}

func SetDSCP(n netbase.Connection, dscp uint8) error {
	return getNetProvider().SetDSCP(n, dscp)
}

func ReadTXTimestamp(n netbase.Connection) (time.Time, uint32, error) {
	return getNetProvider().ReadTXTimestamp(n)
}

func ListenPacket(network string, address string) (netbase.Connection, error) {
	return getNetProvider().ListenPacket(network, address)
}

func NewDaemonConnector(ctx context.Context, daemonAddr string) daemon.Connector {
	return getNetProvider().NewDaemonConnector(ctx, daemonAddr)
}
