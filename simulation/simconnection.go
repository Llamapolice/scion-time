package simulation

import (
	"example.com/scion-time/base/netprovider"
	"go.uber.org/zap"
	"net"
	"net/netip"
	"time"
)

type SimConnection struct {
	Log      *zap.Logger
	Id       string // maybe change to Int but start with string for easier debugging
	ReadFrom chan SimPacket
	WriteTo  chan SimPacket

	Deadline time.Time
	// Following are temporary, might be nice for debugging, but might change
	Network string
	LAddr   *net.UDPAddr
	DSCP    uint8
}

type SimPacket struct {
	B    []byte
	Addr netip.AddrPort
}

func (S *SimConnection) Close() error {
	//TODO implement me
	S.Log.Debug("Closed simulated connection", zap.String("connection id", S.Id), zap.String("network", S.Network))
	return nil
}

func (S *SimConnection) Write(b []byte) (n int, err error) {
	//TODO implement me
	panic("Write: implement me")
}

func (S *SimConnection) ReadMsgUDPAddrPort(buf []byte, oob []byte) (
	n int,
	oobn int,
	flags int,
	addr netip.AddrPort,
	err error,
) {
	// TODO this is still temporary and just returns empty messages
	// Maybe we just need to spin here until we actually get a message? Behavior of original ReadMsg is not clear to me
	// Answer: yes this blocks until deadline or we get a packet

	// Following some example packets:
	// Reading from Connection, n: 204, oobn: 64, flags: 0, addr raw: 10.0.0.73:31024, addr string: 10.0.0.73:31024
	// buf: [0, 0, 0, 0, 17, 37, 0, 56, 1, 0, 0, 0, 0, 1, 255, 0, 0, 0, 1, 17, 0, 1, 255, 0, 0, 0, 1, 18, 10, 1, 1, 11, 10, 1, 1, 12, 134, 0, 32, 194, 0, 0, 202, 140, 101, 83, 159, 144, 0, 0, 90, 193, 101, 83, 158, 241, 1, 0, 180, 181, 101, 83, 158, 241, 0, 63, 1, 239, 0, 0, 75, 159, 32, 232, 237, 118, 0, 63, 0, 0, 0, 113, 129, 0, 17, 7, 90, 179, 0, 63, 0, 104, 0, 0, 69, 36, 39, 49, 169, 121, 0, 63, 0, 1, 0, 2, 42, 142, 143, 113, 237, 10, 0, 63, 0, 0, 0, 6, 229, 182, 163, 25, 228, 86, 0, 63, 0, 0, 0, 5, 4, 196, 245, 238, 138, 110, 0, 63, 0, 104, 0, 0, 78, 242, 29, 211, 219, 222, 183, 172, 39, 139, 0, 56, 25, 25, 35, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 232, 254, 30, 22, 10, 241, 187, 231]
	// oob: [64, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 65, 0, 0, 0, 150, 159, 83, 101, 0, 0, 0, 0, 83, 222, 176, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
	//
	// Reading from Connection, n: 204, oobn: 64, flags: 0, addr raw: 10.0.0.73:31024, addr string: 10.0.0.73:31024
	// buf: [0, 0, 0, 0, 17, 37, 0, 56, 1, 0, 0, 0, 0, 1, 255, 0, 0, 0, 1, 17, 0, 1, 255, 0, 0, 0, 1, 18, 10, 1, 1, 11, 10, 1, 1, 12, 134, 0, 32, 194, 0, 0, 202, 140, 101, 83, 159, 144, 0, 0, 90, 193, 101, 83, 158, 241, 1, 0, 180, 181, 101, 83, 158, 241, 0, 63, 1, 239, 0, 0, 75, 159, 32, 232, 237, 118, 0, 63, 0, 0, 0, 113, 129, 0, 17, 7, 90, 179, 0, 63, 0, 104, 0, 0, 69, 36, 39, 49, 169, 121, 0, 63, 0, 1, 0, 2, 42, 142, 143, 113, 237, 10, 0, 63, 0, 0, 0, 6, 229, 182, 163, 25, 228, 86, 0, 63, 0, 0, 0, 5, 4, 196, 245, 238, 138, 110, 0, 63, 0, 104, 0, 0, 78, 242, 29, 211, 219, 222, 235, 43, 39, 139, 0, 56, 78, 67, 35, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 232, 254, 30, 22, 11, 142, 170, 224, 232, 254, 30, 22, 12, 37, 239, 14, 232, 254, 30, 22, 10, 244, 147, 110]
	// oob: [64, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 65, 0, 0, 0, 150, 159, 83, 101, 0, 0, 0, 0, 189, 189, 0, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
	//
	// Reading from Connection, n: 204, oobn: 64, flags: 0, addr raw: 10.0.0.73:31024, addr string: 10.0.0.73:31024
	// buf: [0, 0, 0, 0, 17, 37, 0, 56, 1, 0, 0, 0, 0, 1, 255, 0, 0, 0, 1, 17, 0, 1, 255, 0, 0, 0, 1, 18, 10, 1, 1, 11, 10, 1, 1, 12, 134, 0, 32, 194, 0, 0, 202, 140, 101, 83, 159, 144, 0, 0, 90, 193, 101, 83, 158, 241, 1, 0, 180, 181, 101, 83, 158, 241, 0, 63, 1, 239, 0, 0, 75, 159, 32, 232, 237, 118, 0, 63, 0, 0, 0, 113, 129, 0, 17, 7, 90, 179, 0, 63, 0, 104, 0, 0, 69, 36, 39, 49, 169, 121, 0, 63, 0, 1, 0, 2, 42, 142, 143, 113, 237, 10, 0, 63, 0, 0, 0, 6, 229, 182, 163, 25, 228, 86, 0, 63, 0, 0, 0, 5, 4, 196, 245, 238, 138, 110, 0, 63, 0, 104, 0, 0, 78, 242, 29, 211, 219, 222, 161, 223, 39, 139, 0, 56, 162, 65, 35, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 232, 254, 30, 22, 12, 229, 183, 218, 232, 254, 30, 22, 13, 115, 101, 185, 232, 254, 30, 22, 12, 121, 0, 238]
	// oob: [64, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 65, 0, 0, 0, 150, 159, 83, 101, 0, 0, 0, 0, 238, 208, 73, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]

	S.Log.Debug("Connection was asked to ReadMsgUDPAddrPort, waiting for something to come in on the channel", zap.String("Server ID", S.Id))
	//n, oobn, flags = 204, 64, 0
	//data := []byte{0, 0, 0, 0, 17, 37, 0, 56, 1, 0, 0, 0, 0, 1, 255, 0, 0, 0, 1, 17, 0, 1, 255, 0, 0, 0, 1, 18, 10, 1, 1, 11, 10, 1, 1, 12, 134, 0, 32, 194, 0, 0, 202, 140, 101, 83, 159, 144, 0, 0, 90, 193, 101, 83, 158, 241, 1, 0, 180, 181, 101, 83, 158, 241, 0, 63, 1, 239, 0, 0, 75, 159, 32, 232, 237, 118, 0, 63, 0, 0, 0, 113, 129, 0, 17, 7, 90, 179, 0, 63, 0, 104, 0, 0, 69, 36, 39, 49, 169, 121, 0, 63, 0, 1, 0, 2, 42, 142, 143, 113, 237, 10, 0, 63, 0, 0, 0, 6, 229, 182, 163, 25, 228, 86, 0, 63, 0, 0, 0, 5, 4, 196, 245, 238, 138, 110, 0, 63, 0, 104, 0, 0, 78, 242, 29, 211, 219, 222, 183, 172, 39, 139, 0, 56, 25, 25, 35, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 232, 254, 30, 22, 10, 241, 187, 231}
	//oodata := []byte{64, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 65, 0, 0, 0, 150, 159, 83, 101, 0, 0, 0, 0, 83, 222, 176, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}

	msg := <-S.ReadFrom
	S.Log.Debug("Received message", zap.String("connection id", S.Id))
	data := msg.B
	if len(data) > cap(buf) {
		S.Log.Error("Buffer passed to ReadMsgUDPAddrPort is too small", zap.Int("data length", len(data)), zap.Int("buffer capacity", cap(buf)))
		return 0, 0, 0, netip.AddrPort{}, SimConnectorError{"buffer too small"}
	}
	for i, item := range data {
		buf[i] = item
	}
	n = len(data)

	// We don't generate ood cause no actual NIC touches our message
	//if len(oodata) > cap(oob) {
	//	S.Log.Error("OOBuffer passed to ReadMsgUDPAddrPort is too small", zap.Int("oodata length", len(oodata)), zap.Int("oob capacity", cap(oob)))
	//}

	//addr = netip.MustParseAddrPort("10.0.0.73:31024")
	addr = msg.Addr
	return n, 0, 0, addr, nil
}

func (S *SimConnection) WriteToUDPAddrPort(b []byte, addr netip.AddrPort) (int, error) {
	//TODO implement me
	S.Log.Debug("Message to be written", zap.String("connection id", S.Id), zap.Binary("msg", b), zap.String("target addr", addr.String()))
	for S.WriteTo == nil {
		// Wait for main simulator routine to initialize channel
		time.Sleep(0)
	}
	S.WriteTo <- SimPacket{B: b, Addr: addr}
	return len(b), nil
}

func (S *SimConnection) SetDeadline(t time.Time) error {
	//TODO implement me
	//if S.Deadline != time.Unix(0, 0) {
	//	panic("Previous deadline not resolved")
	//}
	S.Log.Debug("Connection getting a deadline", zap.String("conn id", S.Id), zap.Time("deadline", t))
	S.Deadline = t
	return nil
}

func (S *SimConnection) LocalAddr() net.Addr {
	//TODO implement me
	return S.LAddr
}

var _ netprovider.Connection = (*SimConnection)(nil)
