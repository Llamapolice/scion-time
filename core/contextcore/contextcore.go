package contextcore

import (
	"context"
	"example.com/scion-time/base/timebase"
	"example.com/scion-time/simulation/simutils"
	"time"
)

// TODO The context handling might need a rework, currently we're doing simulation detection here

func WithTimeout(lclk timebase.LocalClock, parent context.Context, timeout time.Duration) (context.Context, func()) {
	if simClk, ok := lclk.(*simutils.SimClock); ok {
		return simutils.WithTimeout(simClk, timeout)
	}
	return context.WithTimeout(parent, timeout)
}
