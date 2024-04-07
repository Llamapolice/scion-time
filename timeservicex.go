// Driver for quick experiments

package main

import (
	"context"
	"log/slog"
	"time"

	"example.com/scion-time/driver/clock"
	"example.com/scion-time/net/ntp"
)

func runX() {
	initLogger(true, "", true)

	log := slog.Default()

	clk := &clock.SystemClock{Log: log}
	log.Debug("local clock", slog.Time("now", clk.Now()))
	clk.Step(-1 * time.Second)
	log.Debug("local clock", slog.Time("now", clk.Now()))

	now64 := ntp.Time64FromTime(time.Now())
	log.LogAttrs(context.Background(), slog.LevelDebug, "test",
		slog.Any("now", ntp.Time64LogValuer{T: now64}))

	var pkt ntp.Packet
	log.LogAttrs(context.Background(), slog.LevelDebug, "test",
		slog.Any("pkt", ntp.PacketLogValuer{Pkt: &pkt}))
}
