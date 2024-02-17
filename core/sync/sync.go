package sync

import (
	"context"
	"example.com/scion-time/simulation/simutils"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"go.uber.org/zap"

	"example.com/scion-time/base/metrics"
	"example.com/scion-time/base/timebase"
	"example.com/scion-time/base/timemath"

	"example.com/scion-time/core/client"
)

const (
	refClkImpact   = 1.25
	refClkCutoff   = 0
	refClkTimeout  = 1 * time.Second
	refClkInterval = 2 * time.Second
	netClkImpact   = 2.5
	netClkCutoff   = time.Microsecond
	netClkTimeout  = 5 * time.Second
	netClkInterval = 60 * time.Second
)

type localReferenceClock struct{}

//var (
//	refClks       []client.ReferenceClock
//	refClkOffsets []time.Duration
//	refClkClient  client.ReferenceClockClient
//	netClks       []client.ReferenceClock
//	netClkOffsets []time.Duration
//	netClkClient  client.ReferenceClockClient
//)

type SyncableClocks struct {
	refClks       []client.ReferenceClock
	refClkOffsets []time.Duration
	refClkClient  client.ReferenceClockClient
	netClks       []client.ReferenceClock
	netClkOffsets []time.Duration
	netClkClient  client.ReferenceClockClient
	Id            string
}

func (c *localReferenceClock) MeasureClockOffset(context.Context, *zap.Logger) (
	time.Duration, error) {
	return 0, nil
}

func RegisterClocks(refClocks, netClocks []client.ReferenceClock) *SyncableClocks {
	//if refClks != nil || netClks != nil {
	//	panic("reference clocks already registered")
	//}
	clks := SyncableClocks{}

	clks.refClks = refClocks
	clks.refClkOffsets = make([]time.Duration, len(clks.refClks))

	clks.netClks = netClocks
	if len(clks.netClks) != 0 {
		clks.netClks = append(clks.netClks, &localReferenceClock{})
	}
	clks.netClkOffsets = make([]time.Duration, len(clks.netClks))
	return &clks
}

func (c *SyncableClocks) measureOffsetToRefClocks(log *zap.Logger, lclk timebase.LocalClock, timeout time.Duration) time.Duration {
	log.Debug("Measuring offset to reference clocks", zap.String("clock holder id", c.Id))
	ctx, cancel := context.WithDeadline(context.Background(), lclk.Now().Add(timeout))
	defer cancel()
	c.refClkClient.MeasureClockOffsets(ctx, log, c.refClks, c.refClkOffsets)
	return timemath.Median(c.refClkOffsets)
}

func SyncToRefClocks(log *zap.Logger, lclk timebase.LocalClock, syncClks *SyncableClocks) {
	corr := syncClks.measureOffsetToRefClocks(log, lclk, refClkTimeout)
	if corr != 0 {
		lclk.Step(corr)
	}
}

func RunLocalClockSync(log *zap.Logger, lclk timebase.LocalClock, syncClks *SyncableClocks) {
	if refClkImpact <= 1.0 {
		panic("invalid reference clock impact factor")
	}
	if refClkInterval <= 0 {
		panic("invalid reference clock sync interval")
	}
	if refClkTimeout < 0 || refClkTimeout > refClkInterval/2 {
		panic("invalid reference clock sync timeout")
	}
	maxCorr := refClkImpact * float64(lclk.MaxDrift(refClkInterval))
	if maxCorr <= 0 {
		panic("invalid reference clock max correction")
	}
	name := metrics.SyncLocalCorrN
	if simClk, ok := lclk.(*simutils.SimClock); ok {
		log.Debug("SimClock detected, adding id to prometheus gauge", zap.String("id", simClk.Id))
		name += simClk.Id
	}
	corrGauge := promauto.NewGauge(prometheus.GaugeOpts{
		Name: name,
		Help: metrics.SyncLocalCorrH,
	})
	pll := newPLL(log, lclk)
	for {
		corrGauge.Set(0)
		corr := syncClks.measureOffsetToRefClocks(log, lclk, refClkTimeout)
		if timemath.Abs(corr) > refClkCutoff {
			if float64(timemath.Abs(corr)) > maxCorr {
				corr = time.Duration(float64(timemath.Sign(corr)) * maxCorr)
			}
			// lclk.Adjust(corr, refClkInterval, 0)
			pll.Do(corr, 1000.0 /* weight */)
			corrGauge.Set(float64(corr))
		}
		lclk.Sleep(refClkInterval)
	}
}

func (c *SyncableClocks) measureOffsetToNetClocks(log *zap.Logger, lclk timebase.LocalClock, timeout time.Duration) time.Duration {
	log.Debug("\033[47mMeasuring offset to net clocks\033[0m", zap.String("clock holder id", c.Id))
	ctx, cancel := context.WithDeadline(context.Background(), lclk.Now().Add(timeout))
	defer cancel()
	c.netClkClient.MeasureClockOffsets(ctx, log, c.netClks, c.netClkOffsets)
	log.Debug("\033[47mFinished measuring offset to net clocks\033[0m", zap.String("clock holder id", c.Id))
	return timemath.FaultTolerantMidpoint(c.netClkOffsets)
}

func RunGlobalClockSync(log *zap.Logger, lclk timebase.LocalClock, syncClks *SyncableClocks) {
	if netClkImpact <= 1.0 {
		panic("invalid network clock impact factor")
	}
	if netClkImpact-1.0 <= refClkImpact {
		panic("invalid network clock impact factor")
	}
	if netClkInterval < refClkInterval {
		panic("invalid network clock sync interval")
	}
	if netClkTimeout < 0 || netClkTimeout > netClkInterval/2 {
		panic("invalid network clock sync timeout")
	}
	maxCorr := netClkImpact * float64(lclk.MaxDrift(netClkInterval))
	if maxCorr <= 0 {
		panic("invalid network clock max correction")
	}
	name := metrics.SyncGlobalCorrN
	help := metrics.SyncGlobalCorrH
	if syncClks.Id != "" {
		name += syncClks.Id
		help += syncClks.Id
	}
	corrGauge := promauto.NewGauge(prometheus.GaugeOpts{
		Name: name,
		Help: help,
	})
	pll := newPLL(log, lclk)
	for {
		corrGauge.Set(0)
		corr := syncClks.measureOffsetToNetClocks(log, lclk, netClkTimeout)
		if timemath.Abs(corr) > netClkCutoff {
			if float64(timemath.Abs(corr)) > maxCorr {
				corr = time.Duration(float64(timemath.Sign(corr)) * maxCorr)
			}
			// lclk.Adjust(corr, netClkInterval, 0)
			pll.Do(corr, 1000.0 /* weight */)
			corrGauge.Set(float64(corr))
		}
		lclk.Sleep(netClkInterval)
	}
}
