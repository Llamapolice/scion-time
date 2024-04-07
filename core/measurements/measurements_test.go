package measurements_test

import (
	"math"
	"testing"
	"time"

	"example.com/scion-time/core/measurements"
)

func TestMedian(t *testing.T) {
	ms := []measurements.Measurement{
		{Offset: time.Duration(math.MaxInt64)},
		{Offset: time.Duration(math.MaxInt64 - 1)},
	}
	m := ms[1]
	x := measurements.Median(ms)
	if x.Offset != m.Offset {
		t.Errorf("Median(%v) == %d; want %d", ms, x.Offset, m.Offset)
	}
}

func TestFaultTolerantMidpoint(t *testing.T) {
	ms := []measurements.Measurement{
		{Offset: time.Duration(math.MaxInt64)},
		{Offset: time.Duration(math.MaxInt64 - 1)},
		{Offset: time.Duration(math.MaxInt64 - 2)},
		{Offset: time.Duration(math.MaxInt64 - 3)},
	}
	m := ms[2]
	x := measurements.FaultTolerantMidpoint(ms)
	if x.Offset != m.Offset {
		t.Errorf("FaultTolerantMidpoint(%v) == %d; want %d", ms, x.Offset, m.Offset)
	}
}
