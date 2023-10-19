package simulation

import (
	"time"

	"example.com/scion-time/base/timebase"
)

type SimulationClock struct {
	seed int64 // still TODO if this is needed
}

func NewSimulationClock(seed int64) *SimulationClock {
	return &SimulationClock{seed: seed}
}

func (c *SimulationClock) Epoch() uint64 {
	//TODO implement me
	panic("SimulationClock.Epoch() implement me")
}

func (c *SimulationClock) Now() time.Time {
	//TODO implement me
	panic("SimulationClock.Now(): implement me")
}

func (c *SimulationClock) MaxDrift(duration time.Duration) time.Duration {
	//TODO implement me
	panic("SimulationClock.MaxDrift(): implement me")
}

func (c *SimulationClock) Step(offset time.Duration) {
	//TODO implement me
	panic("SimulationClock.Step(): implement me")
}

func (c *SimulationClock) Adjust(offset, duration time.Duration, frequency float64) {
	//TODO implement me
	panic("SimulationClock.Adjust(): implement me")
}

func (c *SimulationClock) Sleep(duration time.Duration) {
	//TODO implement me
	panic("SimulationClock.Sleep(): implement me")
}

var _ timebase.LocalClock = (*SimulationClock)(nil)
