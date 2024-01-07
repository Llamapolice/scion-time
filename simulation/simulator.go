package simulation

import (
	"context"
	"example.com/scion-time/base/cryptobase"
	"example.com/scion-time/base/netprovider"
	"example.com/scion-time/base/timebase"
	"example.com/scion-time/core"
	"example.com/scion-time/core/client"
	"example.com/scion-time/core/server"
	"example.com/scion-time/net/ntske"
	"example.com/scion-time/net/udp"
	"github.com/scionproto/scion/pkg/snet"
	"github.com/scionproto/scion/pkg/snet/path"
	"go.uber.org/zap"
	"time"
)

type SimConfig struct {
	// TODO, WIP
	Servers []core.SvcConfig `toml:"servers"`
	Relays  []core.SvcConfig `toml:"relays"`
	Clients []core.SvcConfig `toml:"clients"`
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

func RunSimulation(
	configFile string,
	lclk timebase.LocalClock,
	lcrypt cryptobase.CryptoProvider,
	lnet netprovider.ConnProvider,
	log *zap.Logger,
) {
	log.Info("Starting simulation")

	// Some logic to read a config file and fill a settings struct

	// Some set up to build the simulated network and start instances
	// Register some channels into the sims
	simConnectionListener := make(chan *SimConnection, 2) // Size 2 as to not block since the connection is opened within the main routine

	simConnector, ok := lnet.(*SimConnector)
	if !ok {
		log.Fatal("Non-simulated connector passed into simulation")
	}
	simConnector.CallBack = simConnectionListener

	// SCION Server 1:
	ctx1, cancel1 := context.WithCancel(context.Background())
	provider := ntske.NewProvider()

	//localRefClks := []string{"/sim/simClk"}
	//ntpRefClks := []string{}
	//SCIONPeers := []string{"1-ff00:0:111,10.1.1.11:10123"}
	//
	//

	var localAddr snet.UDPAddr
	err := localAddr.Set("1-ff00:0:111,10.1.1.11:10123") // Using testnet/gen-eh/ASff00_0_111/ts1-ff00_0_111-1.toml for now
	if err != nil {
		log.Fatal("Local address failed to parse")
	}

	log.Info("Starting first server")
	// With daemon addr
	//server.StartSCIONServer(ctx1, log, "10.1.1.11:30255", snet.CopyUDPAddr(localAddr.Host), 0, provider)
	// Without daemon addr
	server.StartSCIONServer(ctx1, log, "", snet.CopyUDPAddr(localAddr.Host), 63, provider)
	server1Connection := <-simConnectionListener
	server1Connection.Id = "server_1"
	log.Debug("Simulator received connection of server 1")

	s1ReceiveFrom := make(chan SimPacket)
	s1SendTo := make(chan SimPacket)
	server1Connection.ReadFrom = s1SendTo
	server1Connection.WriteTo = s1ReceiveFrom

	// SCION Server 2:
	ctx2, cancel2 := context.WithCancel(context.Background())
	err = localAddr.Set("1-ff00:0:112,10.1.1.12:10123") // Using testnet/gen-eh/ASff00_0_112/ts1-ff00_0_112-1.toml for now
	if err != nil {
		log.Fatal("Local address failed to parse")
	}

	log.Info("Starting second server")
	//server.StartSCIONServer(ctx2, log, "10.1.1.12:30255", snet.CopyUDPAddr(localAddr.Host), 0, provider)
	server.StartSCIONServer(ctx2, log, "", snet.CopyUDPAddr(localAddr.Host), 63, provider)
	server2Connection := <-simConnectionListener
	server2Connection.Id = "server_2"
	log.Debug("Simulator received connection of server 2")

	s2ReceiveFrom := make(chan SimPacket)
	s2SendTo := make(chan SimPacket)
	server2Connection.ReadFrom = s2SendTo
	server2Connection.WriteTo = s2ReceiveFrom

	// Client
	// try with SCION version
	ctxClient, cancelClient := context.WithCancel(context.Background())
	var laddr udp.UDPAddr
	var raddr udp.UDPAddr
	var laddrSNET snet.UDPAddr
	var raddrSNET snet.UDPAddr
	err = laddrSNET.Set("1-ff00:0:112,10.1.1.12")
	if err != nil {
		log.Fatal("Tool local address failed to parse")
	}
	laddr = udp.UDPAddrFromSnet(&laddrSNET)
	err = raddrSNET.Set("1-ff00:0:111,10.1.1.11:10123")
	if err != nil {
		log.Fatal("Tool remote address failed to parse")
	}
	raddr = udp.UDPAddrFromSnet(&raddrSNET)
	ntpcs := []*client.SCIONClient{
		{DSCP: 0, InterleavedMode: false},
	}
	ps := []snet.Path{
		path.Path{Src: laddrSNET.IA, Dst: raddrSNET.IA, DataplanePath: path.Empty{}},
	}

	go func() {
		//_, err = client.MeasureClockOffsetIP(ctxClient, log, &client.IPClient{DSCP: 0, InterleavedMode: true}, laddrSNET.Host, raddrSNET.Host)
		medianDuration, err := client.MeasureClockOffsetSCION(ctxClient, log, ntpcs, laddr, raddr, ps)
		log.Debug("Median Duration measured by tool", zap.Duration("duration", medianDuration))
		if err != nil {
			log.Fatal("Tool had an error", zap.Error(err))
		}
	}()
	clientConnection := <-simConnectionListener
	clientConnection.Id = "client"
	log.Debug("Simulator received connection of client")

	cReceiveFrom := make(chan SimPacket)
	cSendTo := make(chan SimPacket)
	clientConnection.ReadFrom = cSendTo
	clientConnection.WriteTo = cReceiveFrom

	// Start communication from tool to server
	toolMsg := <-cReceiveFrom
	log.Debug("Received packet from tool", zap.String("target addr", toolMsg.Addr.String()))

	s1SendTo <- toolMsg
	log.Debug("Forwarded packet to server 1, waiting for response now")

	// Receive response from server 1
	log.Debug("Sending step")
	server1Response := <-s1ReceiveFrom
	log.Debug("Received response from server 1", zap.String("target addr", server1Response.Addr.String()))
	// Forward response to client
	cSendTo <- server1Response

	// Main loop of simulation
	for condition := true; condition; {
		// Pass messages around between instances
		// Drop, corrupt, duplicate, kill, start, disconnect connections and instances as needed
		condition = false
	}

	defer log.Debug("Canceled all contexts")
	defer cancel1()
	defer cancel2()
	defer cancelClient()

	select {}

	log.Info("Ended simulation (successfully?)")
}
