package simutils

import (
	"context"
	"example.com/scion-time/base/timebase"
	"go.uber.org/zap"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"
)

type PortReleaseMsg struct {
	Owner string
	Port  int
}

type SimConnectionError struct {
	errString string
}

func (e SimConnectionError) Error() string {
	return e.errString
}

type SimConnectorError struct {
	errString string
}

func (e SimConnectorError) Error() string {
	return e.errString
}

type SimPacket struct {
	B          []byte
	TargetAddr netip.AddrPort
	SourceAddr netip.AddrPort
	Latency    time.Duration
}

type TimeRequest struct {
	Id         string
	ReturnChan chan time.Time
}

type WaitRequest struct {
	Id            string
	SleepDuration time.Duration
	Action        func()
	//Unblock       chan interface{}
}

type DeadlineRequest struct {
	Id          string
	Deadline    time.Time
	Unblock     chan time.Duration
	RequestTime time.Time
}

// RequestFromMapHandler is used to circumvent concurrent read/write errors on the connections map in SimConnector.
// SimConnection will pass an instance into the corresponding channel upon SimConnection.Close() with a function that returns its port number,
// which the handler goroutine will then delete from the map. SimConnector.ListenUDP() passes a function that returns -1,
// which the handler ignores, but the passed function itself registers a new port->chan mapping.
// After the map operation has returned, the handler will unblock the calling goroutine waiting on the ReturnBack channel
type RequestFromMapHandler struct {
	Todo       func() int
	ReturnBack chan interface{}
}

// TODO if works, move to separate file

func WithDeadline(lclk timebase.LocalClock, timeout time.Duration) (context.Context, func()) {
	simClock := lclk.(*SimClock)
	simClock.ExpectedWaitQueueSize.Add(1)
	simClock.log.Debug("context created")
	atomicFalse := atomic.Bool{}
	atomicFalse.Store(false)
	c := &CustomContext{
		log:       simClock.log,
		deadline:  lclk.Now().Add(timeout),
		lclk:      lclk,
		canceled:  &atomicFalse,
		waitQueue: simClock.ExpectedWaitQueueSize,
		mu:        sync.Mutex{},
		done:      atomic.Value{},
	}
	simClock.WaitRequest <- WaitRequest{
		Id:            "context deadline",
		SleepDuration: timeout,
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
