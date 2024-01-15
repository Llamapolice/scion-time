package simulation

import (
	"bufio"
	"context"
	"example.com/scion-time/base/cryptobase"
	"example.com/scion-time/base/netprovider"
	"example.com/scion-time/base/timebase"
	"example.com/scion-time/core"
	"example.com/scion-time/core/client"
	"example.com/scion-time/core/server"
	"example.com/scion-time/core/sync"
	"example.com/scion-time/net/ntp"
	"example.com/scion-time/net/ntske"
	"example.com/scion-time/net/udp"
	"github.com/scionproto/scion/pkg/snet"
	"github.com/scionproto/scion/pkg/snet/path"
	"go.uber.org/zap"
	"os"
	"strconv"
	"time"
)

type SimConfigFile struct {
	// TODO, WIP
	Servers []core.SvcConfig `toml:"servers"`
	Relays  []core.SvcConfig `toml:"relays"`
	Clients []core.SvcConfig `toml:"clients"`
}

type Instance struct {
	Id                  string
	Ctx                 context.Context
	ReceiveFromInstance chan SimPacket
	SendToInstance      chan SimPacket
	Conn                *SimConnection
	SyncClks            *sync.SyncableClocks
}

type Server struct {
	Instance
	Provider *ntske.Provider
}

type Client struct {
	Instance
}

type Relay struct {
	Server
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

func newInstance(receiver chan SimPacket) Instance {
	return Instance{
		Id:                  "sim_",
		Ctx:                 context.Background(),
		ReceiveFromInstance: receiver,
		SendToInstance:      make(chan SimPacket),
		Conn:                &SimConnection{},
	}
}

func newServer(receiver chan SimPacket) (s Server) {
	s = Server{
		Instance: newInstance(receiver),
		Provider: ntske.NewProvider(),
	}
	s.Id += "server_"
	return s
}

func newClient(receiver chan SimPacket) (c Client) {
	c = Client{
		Instance: newInstance(receiver),
	}
	c.Id += "client_"
	return c
}

func newRelay(receiver chan SimPacket) (r Relay) {
	r = Relay{newServer(receiver)}
	r.Id = "sim_relay_"
	return r
}

func RunSimulation(
	configFile string,
	lclk timebase.LocalClock,
	lcrypt cryptobase.CryptoProvider,
	lnet netprovider.ConnProvider,
	log *zap.Logger,
) {
	log.Info("Starting simulation")
	scanner := bufio.NewScanner(os.Stdin)

	// Some logic to read a config file and fill a settings struct
	log.Debug("Reading config file", zap.String("config location", configFile))
	var cfg SimConfigFile
	core.LoadConfig(&cfg, configFile, log)

	// Some set up to build the simulated network and start instances
	// Register some channels into the sims
	simConnectionListener := make(chan *SimConnection, 2) // Size 2 as to not block since the connection is opened within the main routine
	simConnector, ok := lnet.(*SimConnector)
	if !ok {
		log.Fatal("Non-simulated connector passed into simulation")
	}
	simConnector.CallBack = simConnectionListener

	receiver := make(chan SimPacket)

	log.Info("Setting up Servers", zap.Int("amount", len(cfg.Servers)))
	simServers := make([]Server, len(cfg.Servers))

	for i, simServer := range cfg.Servers {
		log.Debug("Setting up server", zap.Int("server", i))
		tmp := newServer(receiver)
		tmp.Id = tmp.Id + strconv.Itoa(i)
		localAddr := core.LocalAddress(simServer)

		// Clock Sync
		log.Debug("Starting clock sync")
		localAddr.Host.Port = 0
		refClocks, netClocks := core.CreateClocks(simServer, localAddr, log)
		syncClks := sync.RegisterClocks(refClocks, netClocks)
		syncClks.Id = tmp.Id
		tmp.SyncClks = syncClks
		if len(refClocks) != 0 {
			sync.SyncToRefClocks(log, lclk, syncClks)
			go sync.RunLocalClockSync(log, lclk, syncClks)
		}

		if len(netClocks) != 0 {
			go sync.RunGlobalClockSync(log, lclk, syncClks)
		}
		log.Debug("Clock syncs started")
		log.Debug("Press Enter to continue to server start")
		scanner.Scan()
		// Server starting
		log.Info("Starting server", zap.String("id", tmp.Id))
		localAddr.Host.Port = ntp.ServerPortSCION
		dscp := core.Dscp(simServer)
		daemonAddr := core.DaemonAddress(simServer)
		server.StartSCIONServer(tmp.Ctx, log, daemonAddr, snet.CopyUDPAddr(localAddr.Host), dscp, tmp.Provider)

		tmpConn := <-simConnectionListener
		tmp.Conn = tmpConn
		tmp.Conn.Id = tmp.Id + "_conn"
		log.Debug("Simulator received connection", zap.String("server id", tmp.Id), zap.String("connection id", tmp.Conn.Id))
		tmp.Conn.ReadFrom = tmp.SendToInstance
		tmp.Conn.WriteTo = tmp.ReceiveFromInstance

		simServers[i] = tmp
		log.Debug("Done setting up server, press Enter to continue", zap.String("server id", tmp.Id))
		scanner.Scan()
	}

	// Relays
	// Currently not tested, just basically copy/pasted from timeservice.go
	log.Info("Servers are set up, continuing with Relays", zap.Int("amount", len(cfg.Relays)))
	simRelays := make([]Relay, len(cfg.Relays))

	for i, relay := range cfg.Relays {
		log.Debug("Setting up relay", zap.Int("relay", i))
		tmp := newRelay(receiver)
		tmp.Id = tmp.Id + strconv.Itoa(i)
		localAddr := core.LocalAddress(relay)

		// Clock Sync
		log.Debug("Starting clock sync")
		localAddr.Host.Port = 0
		refClocks, netClocks := core.CreateClocks(relay, localAddr, log)
		syncClks := sync.RegisterClocks(refClocks, netClocks)
		syncClks.Id = tmp.Id
		tmp.SyncClks = syncClks
		if len(refClocks) != 0 {
			sync.SyncToRefClocks(log, lclk, syncClks)
			go sync.RunLocalClockSync(log, lclk, syncClks)
		}

		if len(netClocks) != 0 {
			log.Fatal("Unexpected simulation relay configuration", zap.Int("number of peers", len(netClocks)))
		}
		log.Debug("Clock syncs started")
		log.Debug("Press Enter to continue to relay start")
		scanner.Scan()
		// Relay starting
		log.Info("Starting relay", zap.String("id", tmp.Id))
		localAddr.Host.Port = ntp.ServerPortSCION
		dscp := core.Dscp(relay)
		daemonAddr := core.DaemonAddress(relay)
		server.StartSCIONServer(tmp.Ctx, log, daemonAddr, snet.CopyUDPAddr(localAddr.Host), dscp, tmp.Provider)

		tmpConn := <-simConnectionListener
		tmp.Conn = tmpConn
		tmp.Conn.Id = tmp.Id + "_conn"
		log.Debug("Simulator received connection", zap.String("relay id", tmp.Id), zap.String("connection id", tmp.Conn.Id))
		tmp.Conn.ReadFrom = tmp.SendToInstance
		tmp.Conn.WriteTo = tmp.ReceiveFromInstance

		simRelays[i] = tmp
		log.Debug("Done setting up relay, press Enter to continue", zap.String("relay id", tmp.Id))
		scanner.Scan()
	}

	// Clients
	// turns out I did some weird stuff here, now basically using what is in timeservice.go
	log.Info("Servers and Relays are set up, now setting up Clients", zap.Int("amount", len(cfg.Clients)))
	simClients := make([]Client, len(cfg.Clients))
	for i, clnt := range cfg.Clients {
		log.Debug("Setting up client", zap.Int("client", i))
		tmp := newClient(receiver)
		tmp.Id += strconv.Itoa(i)

		laddr := core.LocalAddress(clnt)
		laddr.Host.Port = 0
		refClocks, netClocks := core.CreateClocks(clnt, laddr, log)
		syncClks := sync.RegisterClocks(refClocks, netClocks)
		syncClks.Id = tmp.Id
		tmp.SyncClks = syncClks

		scionClocksAvailable := false
		for _, c := range refClocks {
			_, ok := c.(*core.NtpReferenceClockSCION)
			if ok {
				scionClocksAvailable = true
				break
			}
		}
		if scionClocksAvailable {
			server.StartSCIONDispatcher(tmp.Ctx, log, snet.CopyUDPAddr(laddr.Host))
			tmpConn := <-simConnectionListener
			tmp.Conn = tmpConn
			tmp.Conn.Id = tmp.Id + "_conn"
			log.Debug("Simulator received connection", zap.String("relay id", tmp.Id), zap.String("connection id", tmp.Conn.Id))
			tmp.Conn.ReadFrom = tmp.SendToInstance
			tmp.Conn.WriteTo = tmp.ReceiveFromInstance
		}

		if len(refClocks) != 0 {
			sync.SyncToRefClocks(log, lclk, syncClks)
			go sync.RunLocalClockSync(log, lclk, syncClks)
		}

		if len(netClocks) != 0 {
			log.Fatal("unexpected configuration", zap.Int("number of peers", len(netClocks)))
		}

		simClients[i] = tmp
	}

	log.Info("Setup completed")

	log.Info("Press Enter to run tool")
	scanner.Scan()
	ctxClient := context.Background()
	var laddr udp.UDPAddr
	var raddr udp.UDPAddr
	var laddrSNET snet.UDPAddr
	var raddrSNET snet.UDPAddr
	err := laddrSNET.Set("1-ff00:0:111,10.1.1.12")
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
		if err != nil {
			log.Fatal("Tool had an error", zap.Error(err))
		}
		log.Debug("Median Duration measured by tool", zap.Duration("duration", medianDuration))
		log.Info("Press Enter to exit simulation")
		scanner.Scan()
		os.Exit(0)
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

	simServers[0].SendToInstance <- toolMsg
	log.Debug("Forwarded packet to server 1, waiting for response now")

	// Receive response from server 1
	server1Response := <-simServers[0].ReceiveFromInstance
	log.Debug("Received response from server 1", zap.String("target addr", server1Response.Addr.String()))
	// Forward response to client
	cSendTo <- server1Response

	// Main loop of simulation
	for condition := true; condition; {
		// Pass messages around between instances
		// Drop, corrupt, duplicate, kill, start, disconnect connections and instances as needed
		condition = false
	}

	select {}

	log.Info("Ended simulation (successfully?)")
}
