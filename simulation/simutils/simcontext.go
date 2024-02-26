package simutils

import (
	"context"
	"example.com/scion-time/base/timebase"
	"go.uber.org/zap"
	"sync"
	"sync/atomic"
	"time"
)

func WithTimeout(simClock *SimClock, timeout time.Duration) (context.Context, func()) {
	simClock.ExpectedWaitQueueSize.Add(1)
	simClock.Log.Debug("context created")
	atomicFalse := atomic.Bool{}
	atomicFalse.Store(false)
	c := &CustomContext{
		log:       simClock.Log,
		deadline:  simClock.Now().Add(timeout),
		lclk:      simClock,
		canceled:  &atomicFalse,
		waitQueue: simClock.ExpectedWaitQueueSize,
		mu:        sync.Mutex{},
		done:      atomic.Value{},
	}
	simClock.WaitRequest <- WaitRequest{
		Id:           "context deadline",
		WaitDuration: timeout,
		Action: func() {
			c.cancel()
			c.waitQueue.Add(-1)
		},
	}
	return c, func() {
		c.cancel()
	}
}

type CustomContext struct {
	log       *zap.Logger
	deadline  time.Time
	lclk      timebase.LocalClock
	canceled  *atomic.Bool
	waitQueue *atomic.Int32
	// copied from context.go cancelCtx
	mu   sync.Mutex   // protects following fields
	done atomic.Value // of chan struct{}, created lazily, closed by first cancel call
}

func (c *CustomContext) Done() <-chan struct{} {
	// copied from context.go cancelCtx
	d := c.done.Load()
	if d != nil {
		return d.(chan struct{})
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	d = c.done.Load()
	if d == nil {
		d = make(chan struct{})
		c.done.Store(d)
	}
	return d.(chan struct{})
}

func (c *CustomContext) cancel() {
	// inspired by context.go
	if c.canceled.Load() {
		c.waitQueue.Add(1) // needed because this will be called by the deferred cancel() meaning something else will continue running after this
		c.log.Debug("context double canceled, incrementing queue size again")
		return
	}
	c.log.Debug("context got canceled")
	c.canceled.Store(true)
	d, _ := c.done.Load().(chan struct{})
	if d == nil {
		closedchan := make(chan struct{})
		close(closedchan)
		c.done.Store(closedchan)
	} else {
		close(d)
	}
}

func (c *CustomContext) Err() error {
	return nil
}

func (c *CustomContext) Value(key any) any {
	return nil
}

func (c *CustomContext) Deadline() (deadline time.Time, ok bool) {
	return c.deadline, true
}

var _ context.Context = (*CustomContext)(nil)
