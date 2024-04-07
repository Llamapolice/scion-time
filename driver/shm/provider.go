package shm

import (
	"log/slog"
	"time"
)

type Provider struct {
	log  *slog.Logger
	unit int
	shm  segment
}

func NewProvider(log *slog.Logger, unit int) *Provider {
	return &Provider{log: log, unit: unit}
}

func (p *Provider) StoreClockSample(refTime, sysTime time.Time) error {
	if !p.shm.initialized {
		err := initSegment(&p.shm, p.unit)
		if err != nil {
			return err
		}
	}

	*p.shm.time = shmTime{
		mode:                 0,
		clockTimeStampSec:    refTime.Unix(),
		clockTimeStampUSec:   int32(refTime.Nanosecond() / 1e3),
		receiveTimeStampSec:  sysTime.Unix(),
		receiveTimeStampUSec: int32(sysTime.Nanosecond() / 1e3),
		leap:                 0,
		precision:            0,
		nSamples:             0,
		clockTimeStampNSec:   uint32(refTime.Nanosecond()),
		receiveTimeStampNSec: uint32(sysTime.Nanosecond()),
		count:                p.shm.time.count + 1,
		valid:                1,
	}

	return nil
}
