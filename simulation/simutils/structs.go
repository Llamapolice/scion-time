package simutils

import (
	"net/netip"
	"time"
)

type PortReleaseMsg struct {
	Owner string
	Port  int
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
}

type TimeRequest struct {
	ReturnChan chan time.Time
}
