package client

import (
	"example.com/scion-time/base/timebase"
	"math"
	"time"

	"go.uber.org/zap"

	"example.com/scion-time/base/timemath"
)

type filterState struct {
	epoch          uint64
	alo, amid, ahi float64
	alolo, ahihi   float64
	navg           float64
}

type filter struct {
	state map[string]filterState
}

func combine(lo, mid, hi time.Duration, trust float64) (offset time.Duration, weight float64) {
	offset = mid
	weight = 0.001 + trust*2.0/timemath.Seconds(hi-lo)
	if weight < 1.0 {
		weight = 1.0
	}
	return
}

func (f *filter) do(log *zap.Logger, reference string, lclk timebase.LocalClock, cTxTime, sRxTime, sTxTime, cRxTime time.Time) (
	offset time.Duration) {

	// Based on Ntimed by Poul-Henning Kamp, https://github.com/bsdphk/Ntimed

	var weight float64

	fs := f.state[reference]

	lo := timemath.Seconds(cTxTime.Sub(sRxTime))
	hi := timemath.Seconds(cRxTime.Sub(sTxTime))
	mid := (lo + hi) / 2

	if fs.epoch != lclk.Epoch() {
		fs.epoch = lclk.Epoch()
		fs.alo = 0.0
		fs.amid = 0.0
		fs.ahi = 0.0
		fs.alolo = 0.0
		fs.ahihi = 0.0
		fs.navg = 0.0
	}

	const (
		filterAverage   = 20.0
		filterThreshold = 3.0
	)

	if fs.navg < filterAverage {
		fs.navg += 1.0
	}

	var loNoise, hiNoise float64
	if fs.navg > 2.0 {
		loNoise = math.Sqrt(fs.alolo - fs.alo*fs.alo)
		hiNoise = math.Sqrt(fs.ahihi - fs.ahi*fs.ahi)
	}

	loLim := fs.alo - loNoise*filterThreshold
	hiLim := fs.ahi + hiNoise*filterThreshold

	var branch int
	failLo := lo < loLim
	failHi := hi > hiLim
	if failLo && failHi {
		branch = 1
	} else if fs.navg > 3.0 && failLo {
		mid = fs.amid + (hi - fs.ahi)
		branch = 2
	} else if fs.navg > 3.0 && failHi {
		mid = fs.amid + (lo - fs.alo)
		branch = 3
	} else {
		branch = 4
	}

	r := fs.navg
	if fs.navg > 2.0 && branch != 4 {
		r *= r
	}

	fs.alo += (lo - fs.alo) / r
	fs.amid += (mid - fs.amid) / r
	fs.ahi += (hi - fs.ahi) / r
	fs.alolo += (lo*lo - fs.alolo) / r
	fs.ahihi += (hi*hi - fs.ahihi) / r

	if f.state == nil {
		f.state = make(map[string]filterState)
	}
	f.state[reference] = fs

	trust := 1.0

	offset, weight = combine(timemath.Duration(lo), timemath.Duration(mid), timemath.Duration(hi), trust)

	log.Debug("filtered response",
		zap.String("from", reference),
		zap.Int("branch", branch),
		zap.Float64("lo [s]", lo),
		zap.Float64("mid [s]", mid),
		zap.Float64("hi [s]", hi),
		zap.Float64("loLim [s]", loLim),
		zap.Float64("amid [s]", fs.amid),
		zap.Float64("hiLim [s]", hiLim),
		zap.Float64("offset [s]", timemath.Seconds(offset)),
		zap.Float64("weight", weight),
	)

	return timemath.Inv(offset)
}
