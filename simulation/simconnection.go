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

	// Following are temporary, might be nice for debugging, but might change
	DSCP uint8
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
	// TODO this is still temporary and just returns empty messages
	// Maybe we just need to spin here until we actually get a message? Behavior of original ReadMsg is not clear to me
	S.Log.Debug("Connection was asked to ReadMsgUDPAddrPort")
	return 0, 0, 0, netip.AddrPort{}, nil
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
