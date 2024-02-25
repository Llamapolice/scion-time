package simutils

import (
	"example.com/scion-time/core/client"
	"go.uber.org/zap"
	"golang.org/x/net/context"
	"math"
	"sync/atomic"
	"time"

	"example.com/scion-time/base/timebase"
)

type SimClock struct {
	Id                    string
	Log                   *zap.Logger
	ModifyTime            func(time time.Time) time.Time
	epoch                 uint64
	timeRequest           chan TimeRequest
	WaitRequest           chan WaitRequest
	ExpectedWaitQueueSize *atomic.Int32
}

func NewSimulationClock(log *zap.Logger, id string, ModifyTime func(t time.Time) time.Time, timeRequest chan TimeRequest, waitRequest chan WaitRequest, ExpectedWaitQueueSize *atomic.Int32) *SimClock {
	return &SimClock{
		Id:                    id + "_clk",
		Log:                   log,
		ModifyTime:            ModifyTime,
		timeRequest:           timeRequest,
		WaitRequest:           waitRequest,
		ExpectedWaitQueueSize: ExpectedWaitQueueSize,
	}
}

func (c SimClock) Epoch() uint64 {
	//TODO implement me
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
	// TODO this currently does nothing, SimCLk is still in skeleton phase
}

func (c SimClock) Sleep(duration time.Duration) {
	c.Log.Debug("SimClock sleeping", zap.String("id", c.Id), zap.Duration("duration", duration))
	// TODO maybe add a function ModifyDuration
	unblockChan := make(chan struct{})
	unblock := func() {
		unblockChan <- struct{}{}
	}
	c.WaitRequest <- WaitRequest{Id: c.Id, SleepDuration: duration, Action: unblock}
	<-unblockChan
	close(unblockChan)
}

var _ timebase.LocalClock = (*SimClock)(nil)

type SimReferenceClock struct {
	Id string
}

func (s *SimReferenceClock) MeasureClockOffset(ctx context.Context, log *zap.Logger) (time.Duration, error) {
	log.Debug("Measuring SimRefClk offset", zap.String("id", s.Id))
	return time.Second, nil
}

var _ client.ReferenceClock = (*SimReferenceClock)(nil)
