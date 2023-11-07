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
	S.Log.Debug("Closed simulated connection")
	return nil
}

func (S *SimConnection) Write(b []byte) (n int, err error) {
	//TODO implement me
	panic("Write: implement me")
}

func (S *SimConnection) ReadMsgUDPAddrPort(buf []byte, oob []byte) (n int, oobn int, flags int, addr netip.AddrPort, err error) {
	//TODO implement me
	panic("ReadMsgUDPAddrPort: implement me")
}

func (S *SimConnection) WriteToUDPAddrPort(b []byte, addr netip.AddrPort) (int, error) {
	//TODO implement me
	panic("WriteToUDPAddrPort: implement me")
}

func (S *SimConnection) SetDeadline(t time.Time) error {
	//TODO implement me
	panic("SetDeadline: implement me")
}

func (S *SimConnection) LocalAddr() net.Addr {
	//TODO implement me
	panic("LocalAddr: implement me")
}

var _ netprovider.Connection = (*SimConnection)(nil)
