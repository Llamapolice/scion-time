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

var (
	log                   *zap.Logger
	receiver              chan SimPacket
	handleConnectionSetup func(id string, receiveFromInstance chan SimPacket, sendToInstance chan SimPacket) *SimConnection
	scanner               *bufio.Scanner
)

func RunSimulation(
	configFile string,
	lclk timebase.LocalClock,
	lcrypt cryptobase.CryptoProvider,
	lnet netprovider.ConnProvider,
	logger *zap.Logger,
) {
	log = logger
	log.Info("\u001B[34mStarting simulation\u001B[0m")
	scanner = bufio.NewScanner(os.Stdin)

	// Some logic to read a config file and fill a settings struct
	log.Debug("Reading config file", zap.String("config location", configFile))
	var cfg SimConfigFile
	core.LoadConfig(&cfg, configFile, log)

	// Some set up to build the simulated network and start instances
	// Register some channels into the sims
	// Size 2 as to not block since the connection is opened within the main routine
	simConnectionListener := make(chan *SimConnection, 2)
	simConnector, ok := lnet.(*SimConnector)
	if !ok {
		log.Fatal("Non-simulated connector passed into simulation")
	}
	simConnector.CallBack = simConnectionListener

	receivingInstances := make(map[string]chan SimPacket)
	receiver = make(chan SimPacket)

	// Bare-bones message handler to pass messages around
	go func() {
		log.Info("\u001B[34mMessage handler started\u001B[0m")
		for msg := range receiver {
			log.Debug("Message handler received message", zap.Binary("msg", msg.B),
				zap.String("target", msg.Addr.String()))
			receivingInstance, exists := receivingInstances[msg.Addr.String()]
			if exists {
				receivingInstance <- msg
				log.Debug("Passed message on to instance")
			} else {
				log.Warn("Targeted address does not exist in the map (yet), message dropped",
					zap.String("target", msg.Addr.String()))
			}
		}
		log.Info("\u001B[34mMessage handler terminating\u001B[0m")
	}()

	// Helper func to do the repetitive part of handling connections
	// Not a standalone function to not have to pass the connection listener and receiver map all the time
	handleConnectionSetup = func(id string, receiveFromInstance, sendToInstance chan SimPacket) *SimConnection {
		tmpConn := <-simConnectionListener
		tmpConn.Id = id
		senderChan := make(chan SimPacket)
		tmpConn.WriteTo = receiveFromInstance
		if sendToInstance != nil {
			senderChan = sendToInstance
		}
		tmpConn.ReadFrom = senderChan
		log.Debug("Subscribing address", zap.String("LocalAddr()", tmpConn.LocalAddr().String()))
		receivingInstances[tmpConn.LocalAddr().String()] = senderChan
		return tmpConn
	}

	log.Info("\u001B[34mSetting up Servers\u001B[0m", zap.Int("amount", len(cfg.Servers)))
	simServers := make([]Server, len(cfg.Servers))

	for i, simServer := range cfg.Servers {
		tmp := serverSetUp(i, simServer, lclk)
		simServers[i] = tmp
		log.Debug("\u001B[34mDone setting up server, press Enter to continue\u001B[0m",
			zap.String("server id", tmp.Id))
		scanner.Scan()
	}

	// Relays
	// Currently not tested, just basically copy/pasted from timeservice.go
	log.Info("\u001B[34mServers are set up, continuing with Relays\u001B[0m",
		zap.Int("amount", len(cfg.Relays)))
	simRelays := make([]Relay, len(cfg.Relays))

	for i, relay := range cfg.Relays {
		tmp := relaySetUp(i, relay, lclk)

		simRelays[i] = tmp
		log.Debug("\u001B[34mDone setting up relay, press Enter to continue\u001B[0m",
			zap.String("relay id", tmp.Id))
		scanner.Scan()
	}

	// Clients
	// turns out I did some weird stuff here, now basically using what is in timeservice.go
	log.Info("\u001B[34mServers and Relays are set up, now setting up Clients\u001B[0m",
		zap.Int("amount", len(cfg.Clients)))
	simClients := make([]Client, len(cfg.Clients))
	for i, clnt := range cfg.Clients {
		tmp := clientSetUp(i, clnt, lclk)

		simClients[i] = tmp
		log.Debug("\u001B[34mDone setting up client, press Enter to continue\u001B[0m",
			zap.String("client id", tmp.Id))
		scanner.Scan()
	}

	log.Info("\u001B[34mSetup completed\u001B[0m")

	log.Info("\u001B[34mPress Enter to run tool\u001B[0m")
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
		medianDuration, err := client.MeasureClockOffsetSCION(ctxClient, log, ntpcs, laddr, raddr, ps)
		if err != nil {
			log.Fatal("Tool had an error", zap.Error(err))
		}
		log.Debug("\u001B[31mMedian Duration measured by tool\u001B[0m",
			zap.Duration("duration", medianDuration))
		log.Info("\u001B[34mPress Enter to exit simulation\u001B[0m")
		scanner.Scan()
		os.Exit(0)
	}()

	// Main loop of simulation
	for condition := true; condition; {
		// Pass messages around between instances
		// Drop, corrupt, duplicate, kill, start, disconnect connections and instances as needed
		condition = false
	}

	select {}
}

func clientSetUp(i int, clnt core.SvcConfig, lclk timebase.LocalClock) Client {
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
		tmp.Conn = handleConnectionSetup(tmp.Id+"_conn", tmp.ReceiveFromInstance, tmp.SendToInstance)
		log.Debug("Simulator received connection",
			zap.String("relay id", tmp.Id), zap.String("connection id", tmp.Conn.Id))
	}

	if len(refClocks) != 0 {
		sync.SyncToRefClocks(log, lclk, syncClks)
		go sync.RunLocalClockSync(log, lclk, syncClks)
	}

	if len(netClocks) != 0 {
		log.Fatal("unexpected configuration", zap.Int("number of peers", len(netClocks)))
	}
	return tmp
}

func relaySetUp(i int, relay core.SvcConfig, lclk timebase.LocalClock) Relay {
	log.Debug("\u001B[34mSetting up relay\u001B[0m", zap.Int("relay", i))
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
		log.Fatal("Unexpected simulation relay configuration",
			zap.Int("number of peers", len(netClocks)))
	}
	log.Debug("Clock syncs active")
	log.Debug("\u001B[34mPress Enter to continue to relay start\u001B[0m")
	scanner.Scan()
	// Relay starting
	log.Info("Starting relay", zap.String("id", tmp.Id))
	localAddr.Host.Port = ntp.ServerPortSCION
	dscp := core.Dscp(relay)
	daemonAddr := core.DaemonAddress(relay)
	server.StartSCIONServer(tmp.Ctx, log, daemonAddr, snet.CopyUDPAddr(localAddr.Host), dscp, tmp.Provider)

	tmp.Conn = handleConnectionSetup(tmp.Id+"_conn", tmp.ReceiveFromInstance, tmp.SendToInstance)
	log.Debug("Simulator received connection",
		zap.String("relay id", tmp.Id), zap.String("connection id", tmp.Conn.Id))
	return tmp
}

func serverSetUp(i int, simServer core.SvcConfig, lclk timebase.LocalClock) Server {
	log.Debug("\u001B[34mSetting up server\u001B[0m", zap.Int("server", i))
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
		log.Debug("Found reference clocks", zap.Int("amount", len(refClocks)))
		sync.SyncToRefClocks(log, lclk, syncClks)
		go sync.RunLocalClockSync(log, lclk, syncClks)
	}

	if len(netClocks) != 0 {
		log.Debug("Found net clocks", zap.Int("amount", len(netClocks)))
		go sync.RunGlobalClockSync(log, lclk, syncClks)
		tmpSyncConn := handleConnectionSetup(tmp.Id+"_netSync", tmp.ReceiveFromInstance, nil)
		log.Debug("Received sync connection", zap.String("local addr", tmpSyncConn.LAddr.String()))
	}
	log.Debug("Clock sync active")
	log.Debug("\u001B[34mPress Enter to continue to server start\u001B[0m")
	scanner.Scan()
	// Server starting
	log.Info("Starting server", zap.String("id", tmp.Id))
	localAddr.Host.Port = ntp.ServerPortSCION
	dscp := core.Dscp(simServer)
	daemonAddr := core.DaemonAddress(simServer)
	server.StartSCIONServer(tmp.Ctx, log, daemonAddr, snet.CopyUDPAddr(localAddr.Host), dscp, tmp.Provider)

	tmp.Conn = handleConnectionSetup(tmp.Id+"_conn", tmp.ReceiveFromInstance, tmp.SendToInstance)
	log.Debug("Simulator received connection",
		zap.String("server id", tmp.Id), zap.String("connection id", tmp.Conn.Id),
		zap.String("conn laddr", tmp.Conn.LAddr.String()))
	return tmp
}
