package simutils

import (
	"example.com/scion-time/base/netbase"
	"go.uber.org/zap"
	"net"
	"net/netip"
	"sync/atomic"
	"time"
)

type SimConnection struct {
	Log            *zap.Logger
	Id             string // maybe change to Int but start with string for easier debugging
	ReadFrom       chan SimPacket
	WriteTo        chan SimPacket
	Latency        time.Duration
	ModifyOutgoing func(packet *SimPacket)

	// Internal usage
	Deadline      time.Time
	StopListening chan struct{}
	// Following are temporary, might be nice for debugging, but might change
	DSCP uint8
	// These are used by the SimConnector to handle connections
	Closed             bool
	Network            string
	LAddr              *net.UDPAddr
	ConnectionsHandler chan RequestFromMapHandler
	RequestDeadline    chan DeadlineRequest
	WaitCounter        *atomic.Int32
	expired            bool
}

func (S *SimConnection) Close() error {
	if S.Closed {
		if S.expired {
			S.WaitCounter.Add(1)
		}
		S.Log.Error("Trying to close already closed connection",
			zap.String("conn id", S.Id), zap.String("conn laddr", S.LAddr.String()))
		return nil
	}
	if S.expired {
		S.WaitCounter.Add(-1)
	}
	S.Closed = true
	//S.expired = false
	tmp := make(chan interface{})
	S.Log.Debug("Removing simconnection from map", zap.String("id", S.Id))
	S.ConnectionsHandler <- RequestFromMapHandler{
		Todo: func() int {
			return S.LAddr.Port
		},
		ReturnBack: tmp,
	}
	<-tmp
	S.Log.Debug("Closed simulated connection",
		zap.String("connection id", S.Id), zap.String("network", S.Network))
	S.StopListening <- struct{}{}
	close(S.ReadFrom)
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
	S.WaitCounter.Add(1)
	var msg SimPacket
	select {
	case msg = <-S.ReadFrom:
		S.Log.Debug("Received message", zap.String("connection id", S.Id), zap.Stringer("from", msg.SourceAddr))
	case <-S.StopListening:
		S.WaitCounter.Add(-1)
		S.Log.Debug("Connection was closed, stopping the listener", zap.String("id", S.Id))
		return 0, 0, 0, netip.AddrPort{}, SimConnectionError{"connection timed out"}
	}
	S.WaitCounter.Add(-1)
	data := msg.B
	if len(data) > cap(buf) {
		S.Log.Error("Buffer passed to ReadMsgUDPAddrPort is too small",
			zap.Int("data length", len(data)), zap.Int("buffer capacity", cap(buf)))
		return 0, 0, 0, netip.AddrPort{}, SimConnectionError{"buffer too small"}
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
	S.Log.Debug("Message to be written", zap.String("connection id", S.Id), zap.Binary("msg", b),
		zap.Stringer("target addr", addr), zap.Stringer("originating addr", S.LAddr.AddrPort()))
	if addr.Port() == 0 {
		S.Log.Fatal("Writing to port 0 is not possible")
	}
	packet := SimPacket{B: b, TargetAddr: addr, SourceAddr: S.LAddr.AddrPort(), Latency: S.Latency}
	S.ModifyOutgoing(&packet)
	S.WriteTo <- packet
	return len(b), nil
}

func (S *SimConnection) SetDeadline(t time.Time) error {
	S.Log.Debug("Connection getting a deadline",
		zap.String("conn id", S.Id), zap.Time("deadline", t))
	S.Deadline = t
	go func() {
		unblock := make(chan time.Duration)
		S.RequestDeadline <- DeadlineRequest{Id: S.Id, Deadline: t, Unblock: unblock}
		sleepDuration := <-unblock
		if S.Closed {
			S.Log.Debug("Already closed connection timed out",
				zap.String("conn id", S.Id))
			return
		}
		S.Log.Debug("Connection timed out",
			zap.String("conn id", S.Id), zap.Duration("after", sleepDuration))
		S.expired = true
		err := S.Close()
		if err != nil {
			S.Log.Error("Deadline sleeping did not work",
				zap.String("connection id", S.Id), zap.Error(err))
		}
	}()
	return nil
}

func (S *SimConnection) LocalAddr() net.Addr {
	return S.LAddr
}

var _ netbase.Connection = (*SimConnection)(nil)
