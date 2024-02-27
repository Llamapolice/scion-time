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
	Id  string      // Identifier string
	Log *zap.Logger // Logger

	ReadFrom       chan SimPacket          // Channel this connection receives messages from
	WriteTo        chan SimPacket          // Global channel the connection sends messages to
	Latency        time.Duration           // The base latency this connection assigns outgoing messages
	ModifyOutgoing func(packet *SimPacket) // Function modifying the packet before sending

	// Internal usage
	Deadline      time.Time     // The deadline this connection has received, if applicable
	StopListening chan struct{} // Channel to prematurely return from ReadMsgUDPAddrPort() when timed out
	// Following are temporary, might be nice for debugging, but might change
	DSCP uint8 // The connection's DSCP value. Currently not used
	// These are used by the SimConnector to handle connections
	Closed             bool                       // True if the connection has been closed
	Network            string                     // Will always be "udp"
	LAddr              *net.UDPAddr               // The address including port this connection is listening on
	ConnectionsHandler chan RequestFromMapHandler // Channel to notify the SimConnector when closing
	RequestDeadline    chan DeadlineRequest       // Channel to request a deadline from the time handler
	expired            bool                       // Set to true if the connection has timed out
	WaitCounter        *atomic.Int32              // TODO remove
}

func (S *SimConnection) Close() error {
	if S.Closed {
		if S.expired {
			S.WaitCounter.Add(1) // TODO remove
		}
		S.Log.Error("Trying to close already closed connection",
			zap.String("conn id", S.Id), zap.String("conn laddr", S.LAddr.String()))
		return nil
	}
	if S.expired {
		S.WaitCounter.Add(-1) // TODO remove
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
	<-S.StopListening
	close(S.StopListening)
	close(S.ReadFrom)
	return nil
}

//func (S *SimConnection) Write(b []byte) (n int, err error) { // TODO remove
//	panic("Write: implement me")
//}

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
		S.StopListening <- struct{}{}
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
