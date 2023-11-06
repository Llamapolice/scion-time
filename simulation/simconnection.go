package simulation

import (
	"example.com/scion-time/base/netprovider"
	"go.uber.org/zap"
	"net"
	"net/netip"
	"time"
)

type SimConnection struct {
	Log *zap.Logger
}

func (S *SimConnection) Close() error {
	//TODO implement me
	panic("implement me")
}

func (S *SimConnection) Write(b []byte) (n int, err error) {
	//TODO implement me
	panic("implement me")
}

func (S *SimConnection) ReadMsgUDPAddrPort(buf []byte, oob []byte) (n int, oobn int, flags int, addr netip.AddrPort, err error) {
	//TODO implement me
	panic("implement me")
}

func (S *SimConnection) WriteToUDPAddrPort(b []byte, addr netip.AddrPort) (int, error) {
	//TODO implement me
	panic("implement me")
}

func (S *SimConnection) SetDeadline(t time.Time) error {
	//TODO implement me
	panic("implement me")
}

func (S *SimConnection) LocalAddr() net.Addr {
	//TODO implement me
	panic("implement me")
}

var _ netprovider.Connection = (*SimConnection)(nil)
