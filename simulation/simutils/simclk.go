package simutils

import (
	"go.uber.org/zap"
	"golang.org/x/net/context"
	"math"
	"time"

	"example.com/scion-time/base/timebase"
)

type SimClock struct {
	Id    string
	seed  int64 // still TODO if this is needed
	log   *zap.Logger
	epoch uint64
	time  time.Time

	counter int // See Now() below, temporary hack to stop unlimited execution
}

func NewSimulationClock(seed int64, log *zap.Logger) *SimClock {
	return &SimClock{seed: seed, log: log, counter: 0, time: time.Unix(0, 0)}
}

func (c SimClock) Epoch() uint64 {
	//TODO implement me
	return c.epoch
}

func (c SimClock) Now() time.Time {
	//TODO implement me
	if c.counter > 1000 { // Hack to stop endless looping
		panic("Hard cutoff point reached")
	}
	c.counter++
	var ns int64 = c.time.UnixNano()
	c.time = c.time.Add(time.Duration(1e9))
	c.log.Debug("Time is now", zap.Int64("ns", ns), zap.String("clock id", c.Id))
	return time.Unix(0, ns)
}

func (c SimClock) MaxDrift(duration time.Duration) time.Duration {
	// Copied straight from sysclk_linux.go
	return math.MaxInt64
}

func (c SimClock) Step(offset time.Duration) {
	//TODO implement me

	// epoch part copied from sysclk_linux.go, not 100% what that does but probably not necessary
	if c.epoch == math.MaxUint64 {
		c.log.Error("SimClock.Step() has been called MaxUint64 times, should never be the case")
		panic("epoch overflow")
	}
	c.epoch++
}

func (c SimClock) Adjust(offset, duration time.Duration, frequency float64) {
	//TODO implement me
	panic("SimClock.Adjust(): implement me")
}

func (c SimClock) Sleep(duration time.Duration) {
	//TODO implement me correctly
	duration = time.Second
	duration *= 15 // slowed down for debugging/construction purposes
	c.log.Debug("SimClock sleeping", zap.Duration("duration", duration))
	time.Sleep(duration)
}

var _ timebase.LocalClock = (*SimClock)(nil)

type SimReferenceClock struct {
	Id string
}

func (s *SimReferenceClock) MeasureClockOffset(ctx context.Context, log *zap.Logger) (time.Duration, error) {
	return time.Second, nil
}
