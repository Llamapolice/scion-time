package simutils

import (
	"example.com/scion-time/core/client"
	"go.uber.org/zap"
	"golang.org/x/net/context"
	"math"
	"time"

	"example.com/scion-time/base/timebase"
)

type SimClock struct {
	Id          string
	seed        int64 // still TODO if this is needed
	log         *zap.Logger
	epoch       uint64
	time        time.Time
	timeRequest chan TimeRequest
	waitRequest chan WaitRequest

	counter int // See Now() below, temporary hack to stop unlimited execution
}

var NumberOfClocks int

func NewSimulationClock(
	seed int64,
	log *zap.Logger,
	timeRequest chan TimeRequest,
	waitRequest chan WaitRequest,
) *SimClock {
	NumberOfClocks += 1
	return &SimClock{
		seed:        seed,
		log:         log,
		counter:     0,
		time:        time.Unix(0, 0),
		timeRequest: timeRequest,
		waitRequest: waitRequest,
	}
}

func (c SimClock) Epoch() uint64 {
	//TODO implement me
	return c.epoch
}

func (c SimClock) Now() time.Time {
	//if c.counter > 1000 { // Hack to stop endless looping
	//	panic("Hard cutoff point reached")
	//}
	//c.counter++
	//var ns int64 = c.time.UnixNano()
	//c.time = c.time.Add(time.Duration(1e9))
	//c.log.Debug("Time is now", zap.Int64("ns", ns), zap.String("clock id", c.Id))
	//return time.Unix(0, ns)
	ans := make(chan time.Time)
	c.timeRequest <- TimeRequest{Id: c.Id, ReturnChan: ans}
	trueTime := <-ans
	// TODO modify this true time to add variability
	close(ans)
	return trueTime
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
	c.log.Debug(
		"Adjusting SimClock (currently NOP)",
		zap.String("clock id", c.Id),
		zap.Duration("offset", offset),
		zap.Duration("duration", duration),
		zap.Float64("frequency", frequency),
	)
	// TODO actually do something lul
}

func (c SimClock) Sleep(duration time.Duration) {
	//TODO implement me correctly
	//duration = time.Second
	//duration *= 15 // slowed down for debugging/construction purposes
	c.log.Debug("SimClock sleeping", zap.Duration("duration", duration))
	//time.Sleep(duration)
	unblock := make(chan interface{})
	c.waitRequest <- WaitRequest{Id: c.Id, SleepDuration: duration, Unblock: unblock}
	<-unblock
	close(unblock)
}

var _ timebase.LocalClock = (*SimClock)(nil)

type SimReferenceClock struct {
	Id string
}

func (s *SimReferenceClock) MeasureClockOffset(ctx context.Context, log *zap.Logger) (time.Duration, error) {
	return time.Second, nil
}

var _ client.ReferenceClock = (*SimReferenceClock)(nil)
