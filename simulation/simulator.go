package simulation

import (
	"example.com/scion-time/base/cryptobase"
	"example.com/scion-time/base/timebase"
	"time"
)

type SimConfig struct {
	// TODO, just an example for now
	instances             []instance
	connections           [][]connection
	maxSimulationDuration time.Duration
}

type instance struct {
	// TODO, just an example for now
	instanceType        int8
	malicious           int8
	failureChance       float64
	meanFailureDuration time.Duration
	minFailureDuration  time.Duration
	maxFailureDuration  time.Duration
}

type connection struct {
	failureChance                   float64
	dropChance                      float64
	duplicationChance               float64
	maxDuplicates                   int32
	multipleDuplicateChanceModifier float64
	corruptionChance                float64
	corruptionSeverity              float64
	meanLatency                     time.Duration
	minLatency                      time.Duration
	maxLatency                      time.Duration
}

func RunSimulation(lclk timebase.LocalClock, lcrypt cryptobase.CryptoProvider) {
	// Some logic to read a config file and fill a settings struct

	// Some set up to build the simulated network and start instances

	// Main loop of simulation
	for condition := true; condition; {
		// Pass messages around between instances
		// Drop, corrupt, duplicate, kill, start, disconnect connections and instances as needed
		condition = false
	}
}
