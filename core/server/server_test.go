package server_test

import (
	"log/slog"
	"testing"
	"time"

	"example.com/scion-time/base/logbase"

	"example.com/scion-time/core/server"
	"example.com/scion-time/driver/clock"

	"example.com/scion-time/net/ntp"
)

func TestSimpleRequest(t *testing.T) {
	server.LogTSS(t, "pre")
	lclk := &clock.SystemClock{Log: slog.New(logbase.NewNopHandler())}

	cTxTime := lclk.Now()
	ntpreq := ntp.Packet{}
	ntpreq.SetVersion(ntp.VersionMax)
	ntpreq.SetMode(ntp.ModeClient)
	ntpreq.TransmitTime = ntp.Time64FromTime(cTxTime)

	rxt := lclk.Now()
	clientID := "client-0"

	var txt0 time.Time
	var ntpresp ntp.Packet
	server.HandleRequest(clientID, lclk, &ntpreq, &rxt, &txt0, &ntpresp)

	server.LogTSS(t, "post")
}
