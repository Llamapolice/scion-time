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
	Id          string                                           // Identifier string, ends in "_clk"
	Log         *zap.Logger                                      // Logger
	ModifyTime  func(time time.Time) time.Time                   // Called to modify the true time before returning for Now()
	ModifySleep func(d time.Duration) time.Duration              // Called to modify the sleep duration
	AdjustFunc  func(c *SimClock, o, d time.Duration, f float64) // Function called within Adjust() method
	timeRequest chan TimeRequest                                 // Channel to send TimeRequest to
	WaitRequest chan WaitRequest                                 // Channel to send WaitRequest to
	epoch       uint64                                           // Used internally for the Epoch() method
}

func NewSimulationClock(log *zap.Logger, id string, ModifyTime func(t time.Time) time.Time, ModifySleep func(d time.Duration) time.Duration, AdjustFunc func(c *SimClock, o time.Duration, d time.Duration, f float64), timeRequest chan TimeRequest, waitRequest chan WaitRequest) *SimClock {
	return &SimClock{
		Id:          id + "_clk",
		Log:         log,
		ModifyTime:  ModifyTime,
		ModifySleep: ModifySleep,
		AdjustFunc:  AdjustFunc,
		timeRequest: timeRequest,
		WaitRequest: waitRequest,
	}
}

func (c SimClock) Epoch() uint64 {
	return c.epoch
}

func (c SimClock) Now() time.Time {
	ans := make(chan time.Time)
	c.timeRequest <- TimeRequest{Id: c.Id, ReturnChan: ans}
	trueTime := <-ans
	close(ans)
	return c.ModifyTime(trueTime)
}

func (c SimClock) MaxDrift(duration time.Duration) time.Duration {
	// Copied straight from sysclk_linux.go
	return math.MaxInt64
}

func (c SimClock) Step(offset time.Duration) {
	// epoch part copied from sysclk_linux.go, not 100% what that does but probably not necessary
	if c.epoch == math.MaxUint64 {
		c.Log.Error("SimClock.Step() has been called MaxUint64 times, should never be the case")
		panic("epoch overflow")
	}
	c.epoch++
}

func (c SimClock) Adjust(offset, duration time.Duration, frequency float64) {
	c.Log.Debug(
		"Adjusting SimClock (currently NOP)",
		zap.String("clock id", c.Id),
		zap.Duration("offset", offset),
		zap.Duration("duration", duration),
		zap.Float64("frequency", frequency),
	)
	c.AdjustFunc(&c, offset, duration, frequency)
}

func (c SimClock) Sleep(duration time.Duration) {
	c.Log.Debug("SimClock sleeping", zap.String("id", c.Id), zap.Duration("duration", duration))
	unblockChan := make(chan struct{})
	unblock := func(_, _ time.Time) {
		unblockChan <- struct{}{}
	}
	sleepDuration := c.ModifySleep(duration)
	c.WaitRequest <- WaitRequest{Id: c.Id, WaitDuration: sleepDuration, Action: unblock}
	<-unblockChan
	close(unblockChan)
}

var _ timebase.LocalClock = (*SimClock)(nil)

type SimReferenceClock struct {
	Id  string
	Log *zap.Logger
}

func (s *SimReferenceClock) MeasureClockOffset(ctx context.Context) (time.Time, time.Duration, error) {
	// TODO this is an empty implementation currently, same output as localReferenceClock
	s.Log.Debug("Measuring SimRefClk offset", zap.String("id", s.Id))
	return time.Unix(0, 0), 0, nil
}

var _ client.ReferenceClock = (*SimReferenceClock)(nil)
