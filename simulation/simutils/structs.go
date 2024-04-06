package simutils

import (
	"net/netip"
	"time"
)

type PortReleaseMsg struct {
	Owner string
	Port  int
}

type SimConnectionError struct {
	errString string
}

func (e SimConnectionError) Error() string {
	return e.errString
}

type SimConnectorError struct {
	errString string
}

func (e SimConnectorError) Error() string {
	return e.errString
}

type SimPacket struct {
	B          []byte
	TargetAddr netip.AddrPort
	SourceAddr netip.AddrPort
	Latency    time.Duration
}

type TimeRequest struct {
	Id         string
	ReturnChan chan time.Time
}

type WaitRequest struct {
	Id           string
	WaitDeadline time.Time // If this is set (i.e. WaitDeadline.IsZero() == false), WaitDuration is ignored
	WaitDuration time.Duration
	Action       func(receivedAt, now time.Time)

	ReceivedAt time.Time // Populated by the TimeHandler upon receiving the request
}

type DeadlineRequest struct {
	Id          string
	Deadline    time.Time
	Unblock     chan time.Duration
	RequestTime time.Time
}

// RequestFromMapHandler is used to circumvent concurrent read/write errors on the connections map in SimConnector.
// SimConnection will pass an instance into the corresponding channel upon SimConnection.Close() with a function that returns its port number,
// which the handler goroutine will then delete from the map. SimConnector.ListenUDP() passes a function that returns -1,
// which the handler ignores, but the passed function itself registers a new port->chan mapping.
// After the map operation has returned, the handler will unblock the calling goroutine waiting on the ReturnBack channel
type RequestFromMapHandler struct {
	Todo       func() int
	ReturnBack chan interface{}
}
