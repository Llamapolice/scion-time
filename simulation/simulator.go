package simulation

import (
	"context"
	"example.com/scion-time/base/cryptobase"
	"example.com/scion-time/base/netprovider"
	"example.com/scion-time/base/timebase"
	"example.com/scion-time/core/server"
	"example.com/scion-time/net/ntske"
	"github.com/scionproto/scion/pkg/snet"
	"go.uber.org/zap"
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

func RunSimulation(lclk timebase.LocalClock, lcrypt cryptobase.CryptoProvider, lnet netprovider.ConnProvider, log *zap.Logger) {
	log.Info("Starting simulation")

	// Some logic to read a config file and fill a settings struct

	// Some set up to build the simulated network and start instances
	ctx := context.Background()
	var localAddr snet.UDPAddr
	err := localAddr.Set("[1-ff00:0:110,192.0.2.1]:80") // Copied example addr from snet/udpaddr.go
	if err != nil {
		log.Fatal("Local address failed to parse")
	}

	server.StartSCIONServer(ctx, log, "sim_daemonAddr", snet.CopyUDPAddr(localAddr.Host), 0, ntske.NewProvider())

	// Main loop of simulation
	for condition := true; condition; {
		// Pass messages around between instances
		// Drop, corrupt, duplicate, kill, start, disconnect connections and instances as needed
		condition = false
	}
	log.Info("Ended simulation (successfully?)")
}
