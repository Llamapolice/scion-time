package timemath

import (
	"math"
	"time"
)

func Duration(seconds float64) time.Duration {
	return time.Duration(seconds*float64(time.Second) + 0.5)
}

func Seconds(duration time.Duration) float64 {
	return float64(duration) / float64(time.Second)
}

func Abs(d time.Duration) time.Duration {
	if d == math.MinInt64 {
		panic("unexpected duration value (math.MinInt64)")
	}
	if d < 0 {
		d = -d
	}
	return d
}

func Sign(d time.Duration) int {
	if d < 0 {
		return -1
	}
	if d > 0 {
		return 1
	}
	return 0
}

func Inv(d time.Duration) time.Duration {
	if d == math.MinInt64 {
		panic("unexpected duration value (math.MinInt64)")
	}
	return -d
}
