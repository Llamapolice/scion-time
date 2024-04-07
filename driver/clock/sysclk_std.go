//go:build !linux

package clock

import (
	"context"
	"log/slog"
	"math"
	"time"

	"example.com/scion-time/base/timebase"
)

type SystemClock struct {
	Log *slog.Logger
}

var _ timebase.LocalClock = (*SystemClock)(nil)

func (c *SystemClock) Epoch() uint64 {
	return 0
}

func (c *SystemClock) Now() time.Time {
	return time.Now().UTC()
}

func (c *SystemClock) MaxDrift(duration time.Duration) time.Duration {
	return math.MaxInt64
}

func (c *SystemClock) Step(offset time.Duration) {
	c.Log.LogAttrs(context.Background(), slog.LevelDebug,
		"SystemClock.Step, not yet implemented",
		slog.Duration("offset", offset),
	)
}

func (c *SystemClock) Adjust(offset, duration time.Duration, frequency float64) {
	c.Log.LogAttrs(context.Background(), slog.LevelDebug,
		"SystemClock.Adjust, not yet implemented",
		slog.Duration("offset", offset),
		slog.Duration("duration", duration),
		slog.Float64("frequency", frequency),
	)
}

func (c *SystemClock) Sleep(duration time.Duration) {
	c.Log.LogAttrs(context.Background(), slog.LevelDebug,
		"SystemClock.Sleep",
		slog.Duration("duration", duration),
	)
	time.Sleep(duration)
}
