package client

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"time"

	"github.com/scionproto/scion/pkg/snet"

	"go.uber.org/zap"

	"example.com/scion-time/base/cryptobase"
	"example.com/scion-time/core/measurements"
	"example.com/scion-time/net/scion"
	"example.com/scion-time/net/udp"
)

type ReferenceClock interface {
	MeasureClockOffset(ctx context.Context) (time.Time, time.Duration, error)
}

type ReferenceClockClient struct {
	numOpsInProgress uint32
}

var (
	errNoPath             = errors.New("failed to measure clock offset: no path")
	errUnexpectedAddrType = errors.New("unexpected address type")

	ipMetrics    atomic.Pointer[ipClientMetrics]
	scionMetrics atomic.Pointer[scionClientMetrics]
)

func init() {
	ipMetrics.Store(newIPClientMetrics())
	scionMetrics.Store(newSCIONClientMetrics())
}

func MeasureClockOffsetIP(ctx context.Context, log *zap.Logger,
	ntpc *IPClient, localAddr, remoteAddr *net.UDPAddr) (
	ts time.Time, off time.Duration, err error) {
	mtrcs := ipMetrics.Load()

	var nerr, n int
	if ntpc.InterleavedMode {
		n = 3
	} else {
		n = 1
	}
	for i := range n {
		t, o, e := ntpc.measureClockOffsetIP(ctx, log, mtrcs, localAddr, remoteAddr)
		if e == nil {
			ts, off, err = t, o, e
			if ntpc.InInterleavedMode() {
				break
			}
		} else {
			if nerr == i {
				err = e
			}
			nerr++
			log.Info("failed to measure clock offset",
				zap.Stringer("to", remoteAddr), zap.Error(e))
		}
	}
	return
}

func collectMeasurements(ctx context.Context, ms []measurements.Measurement, msc chan measurements.Measurement) int {
	i := 0
	j := 0
	n := len(ms)
loop:
	for i != n {
		select {
		case m := <-msc:
			if m.Error == nil {
				if j != len(ms) {
					ms[j] = m
					j++
				}
			}
			i++
		case <-ctx.Done():
			break loop
		}
	}
	go func(n int) { // drain channel
		for n != 0 {
			<-msc
			n--
		}
	}(n - i)
	return j
}

func MeasureClockOffsetSCION(ctx context.Context, log *zap.Logger, crypt cryptobase.CryptoProvider,
	ntpcs []*SCIONClient, localAddr, remoteAddr udp.UDPAddr, ps []snet.Path) (
	time.Time, time.Duration, error) {
	mtrcs := scionMetrics.Load()

	sps := make([]snet.Path, len(ntpcs))
	nsps := 0
	for i, c := range ntpcs {
		pf := ntpcs[i].InterleavedModePath()
		for j := 0; j != len(ps); j++ {
			if p := ps[j]; snet.Fingerprint(p) == snet.PathFingerprint(pf) {
				ps[j] = ps[len(ps)-1]
				ps = ps[:len(ps)-1]
				sps[i] = p
				nsps++
				break
			}
		}
		if sps[i] == nil {
			c.ResetInterleavedMode()
		}
	}
	n, err := crypt.Sample(ctx, len(sps), len(ps), func(dst, src int) {
		sps[dst] = ps[src]
	})
	if err != nil {
		return time.Time{}, 0, err
	}
	if nsps+n == 0 {
		return time.Time{}, 0, errNoPath
	}
	for i, j := 0, 0; j != n; j++ {
		for sps[i] != nil {
			i++
		}
		sps[i] = ps[j]
		nsps++
	}

	ms := make([]measurements.Measurement, nsps)
	msc := make(chan measurements.Measurement)
	for i := range len(ntpcs) {
		if sps[i] == nil {
			continue
		}
		go func(ctx context.Context, log *zap.Logger, mtrcs *scionClientMetrics,
			ntpc *SCIONClient, localAddr, remoteAddr udp.UDPAddr, p snet.Path) {
			var err error
			var ts time.Time
			var off time.Duration
			var nerr, n int
			log.Debug("measuring clock offset",
				zap.Stringer("to IA", remoteAddr.IA),
				zap.Stringer("to", remoteAddr),
				zap.Stringer("from", localAddr),
				zap.Object("via", scion.PathMarshaler{Path: p}),
			)
			if ntpc.InterleavedMode {
				n = 3
			} else {
				n = 1
			}
			for j := range n {
				t, o, e := ntpc.measureClockOffsetSCION(ctx, log, mtrcs, localAddr, remoteAddr, p)
				if e == nil {
					ts, off, err = t, o, e
					if ntpc.InInterleavedMode() {
						break
					}
				} else {
					if nerr == j {
						err = e
					}
					nerr++
					log.Info("failed to measure clock offset",
						zap.Stringer("to", remoteAddr.IA),
						zap.Object("via", scion.PathMarshaler{Path: p}),
						zap.Error(e),
					)
				}
			}
			msc <- measurements.Measurement{
				Timestamp: ts,
				Offset:    off,
				Error:     err,
			}
		}(ctx, log, mtrcs, ntpcs[i], localAddr, remoteAddr, sps[i])
	}
	collectMeasurements(ctx, ms, msc)
	m := measurements.FaultTolerantMidpoint(ms)
	return m.Timestamp, m.Offset, m.Error
}

func (c *ReferenceClockClient) MeasureClockOffsets(ctx context.Context, log *zap.Logger,
	refclks []ReferenceClock, ms []measurements.Measurement) {
	if len(ms) != len(refclks) {
		panic("number of result offsets must be equal to the number of reference clocks")
	}
	swapped := atomic.CompareAndSwapUint32(&c.numOpsInProgress, 0, 1)
	if !swapped {
		panic("too many reference clock offset measurements in progress")
	}
	defer func(addr *uint32) {
		swapped := atomic.CompareAndSwapUint32(addr, 1, 0)
		if !swapped {
			panic("inconsistent count of reference clock offset measurements")
		}
	}(&c.numOpsInProgress)

	msc := make(chan measurements.Measurement)
	for _, refclk := range refclks {
		go func(ctx context.Context, log *zap.Logger, refclk ReferenceClock) {
			ts, off, err := refclk.MeasureClockOffset(ctx)
			msc <- measurements.Measurement{
				Timestamp: ts,
				Offset:    off,
				Error:     err,
			}
		}(ctx, log, refclk)
	}
	collectMeasurements(ctx, ms, msc)
}
