package simutils

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
	DSCP uint8
	// These are used by the SimConnector to handle connections
	UseCounter         int
	Closed             bool
	Network            string
	LAddr              *net.UDPAddr
	PortReleaseMsgChan chan PortReleaseMsg
}

type SimPacket struct {
	B          []byte
	TargetAddr netip.AddrPort
	SourceAddr netip.AddrPort
}

func (S *SimConnection) Close() error {
	//TODO implement me
	S.Log.Debug("Closing SimConnection", zap.String("conn id", S.Id))
	S.UseCounter += 1
	if S.Closed {
		S.Log.Error("Trying to close already closed connection",
			zap.String("conn id", S.Id), zap.String("conn laddr", S.LAddr.String()))
		return nil
	}
	S.Closed = true
	usedPort := S.LAddr.Port
	S.Log.Debug("Releasing port for further use",
		zap.Int("released port", usedPort), zap.String("laddr", S.LAddr.String()))
	S.LAddr.Port = 0
	S.PortReleaseMsgChan <- PortReleaseMsg{
		Owner: S.Network + S.LAddr.String(),
		Port:  usedPort,
	}
	S.Log.Debug("Closed simulated connection",
		zap.String("connection id", S.Id), zap.String("network", S.Network))
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
	S.Log.Debug("Connection was asked to ReadMsgUDPAddrPort, waiting for something to come in on the channel",
		zap.String("Server ID", S.Id))

	msg := <-S.ReadFrom
	S.Log.Debug("Received message", zap.String("connection id", S.Id))
	data := msg.B
	if len(data) > cap(buf) {
		S.Log.Error("Buffer passed to ReadMsgUDPAddrPort is too small",
			zap.Int("data length", len(data)), zap.Int("buffer capacity", cap(buf)))
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

	addr = msg.SourceAddr
	return n, 0, 0, addr, nil
}

func (S *SimConnection) WriteToUDPAddrPort(b []byte, addr netip.AddrPort) (int, error) {
	//TODO implement me
	S.Log.Debug("Message to be written", zap.String("connection id", S.Id), zap.Binary("msg", b),
		zap.String("target addr", addr.String()), zap.String("originating addr", S.LAddr.AddrPort().String()))
	if addr.Port() == 0 {
		S.Log.Fatal("Writing to port 0 is not possible")
	}
	for S.WriteTo == nil {
		// Wait for main simulator routine to initialize channel
		time.Sleep(0)
	}
	S.WriteTo <- SimPacket{B: b, TargetAddr: addr, SourceAddr: S.LAddr.AddrPort()}
	return len(b), nil
}

func (S *SimConnection) SetDeadline(t time.Time) error {
	//TODO implement me correctly
	S.Log.Debug("Connection getting a deadline",
		zap.String("conn id", S.Id), zap.Time("deadline", t))
	S.Deadline = t
	sleepDuration := time.Until(t)
	useCounter := S.UseCounter
	go func() {
		time.Sleep(sleepDuration)
		if S.UseCounter != useCounter {
			S.Log.Debug("Already closed connection timed out",
				zap.String("conn id", S.Id))
			return
		}
		S.Log.Debug("Connection timed out",
			zap.String("conn id", S.Id), zap.Duration("after", sleepDuration))
		err := S.Close()
		if err != nil {
			S.Log.Error("Deadline sleeping did not work",
				zap.String("connection id", S.Id), zap.Error(err))
		}
	}()
	return nil
}

func (S *SimConnection) LocalAddr() net.Addr {
	//TODO implement me
	return S.LAddr
}

var _ netprovider.Connection = (*SimConnection)(nil)
