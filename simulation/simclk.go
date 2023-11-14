package simulation

import (
	"go.uber.org/zap"
	"time"

	"example.com/scion-time/base/timebase"
)

type SimClock struct {
	seed int64 // still TODO if this is needed
	log  *zap.Logger

	counter int // See Now() below
}

func NewSimulationClock(seed int64, log *zap.Logger) *SimClock {
	return &SimClock{seed: seed, log: log, counter: 0}
}

func (c *SimClock) Epoch() uint64 {
	//TODO implement me
	panic("SimClock.Epoch() implement me")
}

func (c *SimClock) Now() time.Time {
	//TODO implement me
	if c.counter > 10 { // Hack to stop endless looping
		panic("Hard cutoff point reached")
	}
	c.counter++

	var s int64 = 0
	var ns int64 = 0
	c.log.Debug("Time is now", zap.Int64("s", s), zap.Int64("ns", ns))
	return time.Unix(s, ns)
}

func (c *SimClock) MaxDrift(duration time.Duration) time.Duration {
	//TODO implement me
	panic("SimClock.MaxDrift(): implement me")
}

func (c *SimClock) Step(offset time.Duration) {
	//TODO implement me
	panic("SimClock.Step(): implement me")
}

func (c *SimClock) Adjust(offset, duration time.Duration, frequency float64) {
	//TODO implement me
	panic("SimClock.Adjust(): implement me")
}

func (c *SimClock) Sleep(duration time.Duration) {
	//TODO implement me
	panic("SimClock.Sleep(): implement me")
}

var _ timebase.LocalClock = (*SimClock)(nil)
