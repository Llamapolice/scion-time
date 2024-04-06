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
	simClock.Log.Debug("context created")
	atomicFalse := atomic.Bool{}
	atomicFalse.Store(false)
	c := &CustomContext{
		log:      simClock.Log,
		deadline: simClock.Now().Add(timeout),
		lclk:     simClock,
		canceled: &atomicFalse,
		mu:       sync.Mutex{},
		done:     atomic.Value{},
	}
	simClock.WaitRequest <- WaitRequest{
		Id:           "context deadline",
		WaitDuration: timeout,
		Action: func(_, _ time.Time) {
			c.cancel()
		},
	}
	return c, func() {
		c.cancel()
	}
}

// CustomContext is a stripped down version of cancelCtx from Go's context package.
// It enables setting a timeout based on a SimClock instead of time.Now() as Go's context does.
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
