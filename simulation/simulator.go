package simulation

import (
	"bufio"
	"context"
	"example.com/scion-time/core"
	"example.com/scion-time/core/client"
	"example.com/scion-time/core/server"
	"example.com/scion-time/core/sync"
	"example.com/scion-time/net/ntp"
	"example.com/scion-time/net/ntske"
	"example.com/scion-time/net/udp"
	"example.com/scion-time/simulation/simutils"
	"github.com/scionproto/scion/pkg/snet"
	"github.com/scionproto/scion/pkg/snet/path"
	"go.uber.org/zap"
	"net"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

const maxNumberOfInstances = 30

// FAKE CONSTANTS
var NOPModifyMsg = func(_ *simutils.SimPacket) {}
var DefineDefaultLatency = func(_ *net.UDPAddr) time.Duration { return 2 * time.Millisecond }
var NOPModifyTime = func(t time.Time) time.Time { return t }

type SimConfigFile struct {
	Servers []SimSvcConfig `toml:"servers"`
	Relays  []SimSvcConfig `toml:"relays"`
	Clients []SimSvcConfig `toml:"clients"`
	Tools   []SimSvcConfig `toml:"tools"`
}

type SimSvcConfig struct {
	core.SvcConfig
	Seed            int64  `toml:"random_seed,omitempty"`
	DelayAfterStart string `toml:"delay_after_start,omitempty"`
}

type Instance struct {
	Id                  string
	Ctx                 context.Context
	ReceiveFromInstance chan simutils.SimPacket
	SendToInstance      chan simutils.SimPacket
	Conn                *simutils.SimConnection
	LocalClk            *simutils.SimClock
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

func newInstance(receiver chan simutils.SimPacket) Instance {
	return Instance{
		Id:                  "sim_",
		Ctx:                 context.Background(),
		ReceiveFromInstance: receiver,
		SendToInstance:      make(chan simutils.SimPacket),
		Conn:                &simutils.SimConnection{},
	}
}

func newServer(receiver chan simutils.SimPacket) (s Server) {
	s = Server{
		Instance: newInstance(receiver),
		Provider: ntske.NewProvider(),
	}
	s.Id += "server_"
	return s
}

func newClient(receiver chan simutils.SimPacket) (c Client) {
	c = Client{
		Instance: newInstance(receiver),
	}
	c.Id += "client_"
	return c
}

func newRelay(receiver chan simutils.SimPacket) (r Relay) {
	r = Relay{newServer(receiver)}
	r.Id = "sim_relay_"
	return r
}

var (
	log                   *zap.Logger
	receiver              chan simutils.SimPacket
	globalModifyMsg       func(packet simutils.SimPacket) simutils.SimPacket
	simConnectors         []*simutils.SimConnector
	ExpectedWaitQueueSize *atomic.Int32
	waitingConnections    *atomic.Int32
	scanner               *bufio.Scanner
	timeRequests          chan simutils.TimeRequest
	waitRequests          chan simutils.WaitRequest
	deadlineRequests      chan simutils.DeadlineRequest
)

func RunSimulation(configFile string, logger *zap.Logger) {
	log = logger
	log.Info("\u001B[34mStarting simulation\u001B[0m")

	// Some logic to read a config file and fill a settings struct
	log.Debug("Reading config file", zap.String("config location", configFile))
	var cfg SimConfigFile
	core.LoadConfig(&cfg, configFile, log)

	// Set up file package vars
	simConnectors = make([]*simutils.SimConnector, 0, len(cfg.Clients)+len(cfg.Relays)+len(cfg.Servers)+len(cfg.Tools))
	scanner = bufio.NewScanner(os.Stdin)
	ExpectedWaitQueueSize = &atomic.Int32{}
	waitingConnections = &atomic.Int32{}

	// Standard NOP packet modification in message handler
	globalModifyMsg = func(packet simutils.SimPacket) simutils.SimPacket {
		return packet
	}

	// Bare-bones message handler to pass messages around
	go func() {
		log.Info("\u001B[34mMessage handler started\u001B[0m")
		receiver = make(chan simutils.SimPacket)
	skip:
		for msg := range receiver {
			log.Debug(
				"Message handler received message",
				zap.Binary("msg", msg.B),
				zap.String("target", msg.TargetAddr.String()),
				zap.String("source", msg.SourceAddr.String()),
			)
			targetAddr := msg.TargetAddr.Addr().WithZone("").Unmap()
			for _, simConn := range simConnectors {
				laddrPort := simConn.LocalAddress.Host.AddrPort()
				simConnLocalAddress := laddrPort.Addr().WithZone("").Unmap()
				if simConnLocalAddress == targetAddr {
					// necessary, otherwise the function will use the current simConn and msg during its execution instead of creation (what we want)
					tmp := *simConn
					tmpMsg := globalModifyMsg(msg)
					passOn := func() {
						tmp.Input <- tmpMsg
						log.Debug(
							"Passed message on to instance",
							zap.String("receiving connector id", tmp.Id),
							zap.Stringer("source addr", tmpMsg.SourceAddr),
							zap.Duration("after", tmpMsg.Latency),
						)
					}
					waitRequests <- simutils.WaitRequest{
						Id:            simConn.Id + "_msg",
						SleepDuration: msg.Latency,
						Action:        passOn,
					}
					continue skip
				}
			}
			log.Warn("Targeted address does not exist (yet), message dropped",
				zap.String("target", targetAddr.String()))
			//ExpectedWaitQueueSize.Add(-1)
		}
		log.Info("\u001B[34mMessage handler terminating\u001B[0m")
	}()

	for receiver == nil {
	} // Wait for message handler to be ready

	// Sketch for a ground time provider
	go func() {
		log.Info("Time handler started")
		now := time.Unix(10000, 0)
		currentlyWaiting := make([]simutils.WaitRequest, 0, maxNumberOfInstances)
		timeRequests = make(chan simutils.TimeRequest)
		waitRequests = make(chan simutils.WaitRequest)
		deadlineRequests = make(chan simutils.DeadlineRequest)
		for {
			select {
			case req := <-timeRequests:
				log.Debug("Time has been requested", zap.String("by", req.Id))
				req.ReturnChan <- now
			case req := <-waitRequests:
				log.Debug("Wait has been requested", zap.String("by", req.Id), zap.Duration("duration", req.SleepDuration))
				currentlyWaiting = append(currentlyWaiting, req)
			case req := <-deadlineRequests:
				ExpectedWaitQueueSize.Add(1)
				log.Debug("Deadline has been requested", zap.String("by", req.Id))
				log.Debug("adding waiter deadline request")
				req.RequestTime = now
				duration := req.Deadline.Sub(req.RequestTime)
				waitForDeadline := simutils.WaitRequest{
					Id:            req.Id,
					SleepDuration: duration,
					Action: func() {
						log.Debug("removing waiter deadline request")
						req.Unblock <- duration
						ExpectedWaitQueueSize.Add(-1)
					},
				}
				currentlyWaiting = append(currentlyWaiting, waitForDeadline)
			default:
				time.Sleep(time.Second / 2) // TODO just for development
				if len(currentlyWaiting) >= 0 {
					log.Info(
						"\u001B[41m======== TIME HANDLER ========\u001B[0m",
						zap.Int("waiting sleepers", len(currentlyWaiting)),
						zap.Int32("waiting connections", waitingConnections.Load()),
						zap.Int32("expected to wait", ExpectedWaitQueueSize.Load()),
					)
					if len(currentlyWaiting)+int(waitingConnections.Load()) < int(ExpectedWaitQueueSize.Load()) { // TODO how to adjust this number dynamically?
						continue
					}
					log.Info("When this message appears, 'waiting sleepers' + 'waiting connections' should be exactly equal to 'expected to wait'")
					log.Info("And when you enter w, nothing should happen until this log message appears again")
					log.Info("Enter 'w' to wait one loop for other goroutines or enter 'p' to process the next request in the queue (q for os.Exit(0))")
					scanner.Scan()
					if scanner.Text() == "q" {
						os.Exit(0)
					}
					if scanner.Text() != "p" {
						continue
					}
					log.Info("Handling the next waiting request")
					minDuration := time.Hour
					minIndex := 10000000
					for i, request := range currentlyWaiting {
						if request.SleepDuration < minDuration {
							minIndex = i
							minDuration = request.SleepDuration
						}
					}
					shortestRequest := currentlyWaiting[minIndex]
					currentlyWaiting = append(currentlyWaiting[:minIndex], currentlyWaiting[minIndex+1:]...)
					for i := range currentlyWaiting {
						if currentlyWaiting[i].SleepDuration > minDuration {
							currentlyWaiting[i].SleepDuration -= minDuration
						} else {
							currentlyWaiting[i].SleepDuration = 0
						}
					}
					now = now.Add(minDuration)
					log.Debug("Time handler unblocks a sleeper", zap.String("id", shortestRequest.Id), zap.Time("updated time", now))
					shortestRequest.Action()
				}
			}
		}
	}()

	for deadlineRequests == nil {
	} // Wait for time handler to be ready

	log.Info("\u001B[34mSetting up Servers, press c to time handler to start\u001B[0m", zap.Int("amount", len(cfg.Servers)))
	simServers := make([]Server, len(cfg.Servers))

	for i, simServer := range cfg.Servers {
		ExpectedWaitQueueSize.Add(1)
		log.Debug("adding waiter server setup")

		tmp := serverSetUp(i, simServer)
		simServers[i] = tmp

		log.Debug("\u001B[34mDone setting up server, waiting for deadline to continue\u001B[0m",
			zap.String("server id", tmp.Id))
		pauseSetUp(simServer, "server", i)
	}

	// Relays
	// Currently not tested, just basically copy/pasted from timeservice.go
	log.Info("\u001B[34mServers are set up, continuing with Relays\u001B[0m",
		zap.Int("amount", len(cfg.Relays)))
	simRelays := make([]Relay, len(cfg.Relays))

	for i, relay := range cfg.Relays {
		ExpectedWaitQueueSize.Add(1)
		log.Debug("adding waiter relay setup")

		tmp := relaySetUp(i, relay)

		simRelays[i] = tmp
		log.Debug("\u001B[34mDone setting up relay, waiting for deadline to continue\u001B[0m",
			zap.String("relay id", tmp.Id))
		pauseSetUp(relay, "relay", i)
	}

	// Clients
	// turns out I did some weird stuff here, now basically using what is in timeservice.go
	log.Info("\u001B[34mServers and Relays are set up, now setting up Clients\u001B[0m",
		zap.Int("amount", len(cfg.Clients)))
	simClients := make([]Client, len(cfg.Clients))
	for i, clnt := range cfg.Clients {
		ExpectedWaitQueueSize.Add(1)
		log.Debug("adding waiter client setup")

		tmp := clientSetUp(i, clnt)
		simClients[i] = tmp

		log.Debug("\u001B[34mDone setting up client, waiting for deadline to continue\u001B[0m",
			zap.String("client id", tmp.Id))
		pauseSetUp(clnt, "client", i)
	}

	log.Info("\u001B[34mSetup completed\u001B[0m")

	for i, tool := range cfg.Tools {
		ExpectedWaitQueueSize.Add(1)
		log.Debug("adding waiter tool setup")
		log.Info("\u001B[34mRunning Tool\u001B[0m", zap.Int("tool", i))

		runTool(i, tool)

		pauseSetUp(tool, "tool", i)
	}

	// Main loop of simulation
	for condition := true; condition; {
		// Pass messages around between instances
		// Drop, corrupt, duplicate, kill, start, disconnect connections and instances as needed
		condition = false
	}

	select {}
}

func checkEmptyAuthMode(cfg *SimSvcConfig) {
	if len(cfg.AuthModes) > 0 {
		log.Warn("Auth modes are currently not supported by the simulation, will be ignored")
	}
	cfg.AuthModes = cfg.AuthModes[:0]
}

func pauseSetUp(instance SimSvcConfig, instanceType string, i int) {
	unblockChan := make(chan struct{})
	waitDuration, err := time.ParseDuration(instance.DelayAfterStart)
	if err != nil {
		log.Fatal("delay_after_start failed to parse from config", zap.Int(instanceType+" number", i))
	}
	waitRequests <- simutils.WaitRequest{
		Id:            "afterStartDelay_" + instanceType + strconv.Itoa(i),
		SleepDuration: waitDuration,
		Action:        func() { unblockChan <- struct{}{} },
	}
	<-unblockChan
	close(unblockChan)
	log.Debug("removing waiter in setup for " + instanceType)
	//ExpectedWaitQueueSize.Add(-1)
}

func runTool(i int, tool SimSvcConfig) {
	id := "tool_" + strconv.Itoa(i)
	ctxClient := context.Background()

	checkEmptyAuthMode(&tool)

	lclk := simutils.NewSimulationClock(log, id, NOPModifyTime, timeRequests, waitRequests, ExpectedWaitQueueSize)

	var laddr udp.UDPAddr
	var raddr udp.UDPAddr
	var laddrSNET snet.UDPAddr
	var raddrSNET snet.UDPAddr
	err := laddrSNET.Set(tool.LocalAddr)
	if err != nil {
		log.Fatal("Tool local address failed to parse")
	}
	laddr = udp.UDPAddrFromSnet(&laddrSNET)
	err = raddrSNET.Set(tool.RemoteAddr)
	if err != nil {
		log.Fatal("Tool remote address failed to parse")
	}
	raddr = udp.UDPAddrFromSnet(&raddrSNET)

	simConnector := simutils.NewSimConnector(log, id, &laddrSNET, receiver, deadlineRequests, NOPModifyMsg, NOPModifyMsg, DefineDefaultLatency, ExpectedWaitQueueSize, waitingConnections)
	simConnectors = append(simConnectors, simConnector)

	simCrypt := simutils.NewSimCrypto(tool.Seed, log)

	ntpcs := []*client.SCIONClient{
		{Lclk: lclk, ConnectionProvider: simConnector, DSCP: 0, InterleavedMode: false},
	}
	ps := []snet.Path{
		path.Path{Src: laddrSNET.IA, Dst: raddrSNET.IA, DataplanePath: path.Empty{}},
	}

	go func() { // TODO does this still need to be in a goroutine?
		log.Debug("Tool setup complete, running offset measurement now", zap.String("id", id))
		medianDuration, err := client.MeasureClockOffsetSCION(ctxClient, log, simCrypt, ntpcs, laddr, raddr, ps)
		if err != nil {
			log.Fatal("Tool had an error", zap.Error(err))
		}
		log.Debug("\u001B[31mMedian Duration measured by tool\u001B[0m",
			zap.Duration("duration", medianDuration))
	}()
}

func clientSetUp(i int, clnt SimSvcConfig) Client {
	log.Debug("Setting up client", zap.Int("client", i))
	tmp := newClient(receiver)
	tmp.Id += strconv.Itoa(i)

	checkEmptyAuthMode(&clnt)

	simClk := simutils.NewSimulationClock(log, tmp.Id, NOPModifyTime, timeRequests, waitRequests, ExpectedWaitQueueSize)
	tmp.LocalClk = simClk

	laddr := core.LocalAddress(clnt.SvcConfig)
	simNet := simutils.NewSimConnector(log, tmp.Id, laddr, receiver, deadlineRequests, NOPModifyMsg, NOPModifyMsg, DefineDefaultLatency, ExpectedWaitQueueSize, waitingConnections)
	simConnectors = append(simConnectors, simNet)

	simCrypt := simutils.NewSimCrypto(clnt.Seed, log)

	laddr.Host.Port = 0
	refClocks, netClocks := core.CreateClocks(clnt.SvcConfig, laddr, simClk, simNet, simCrypt, log)
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
		server.StartSCIONDispatcher(tmp.Ctx, log, simClk, simNet, snet.CopyUDPAddr(laddr.Host))
	}

	if len(refClocks) != 0 {
		sync.SyncToRefClocks(log, simClk, syncClks)
		go sync.RunLocalClockSync(log, simClk, syncClks)
	}

	if len(netClocks) != 0 {
		log.Fatal("unexpected configuration", zap.Int("number of peers", len(netClocks)))
	}
	return tmp
}

func relaySetUp(i int, relay SimSvcConfig) Relay {
	log.Debug("\u001B[34mSetting up relay\u001B[0m", zap.Int("relay", i))
	tmp := newRelay(receiver)
	tmp.Id = tmp.Id + strconv.Itoa(i)

	checkEmptyAuthMode(&relay)

	simClk := simutils.NewSimulationClock(log, tmp.Id, NOPModifyTime, timeRequests, waitRequests, ExpectedWaitQueueSize)
	tmp.LocalClk = simClk

	localAddr := core.LocalAddress(relay.SvcConfig)
	simNet := simutils.NewSimConnector(log, tmp.Id, localAddr, receiver, deadlineRequests, NOPModifyMsg, NOPModifyMsg, DefineDefaultLatency, ExpectedWaitQueueSize, waitingConnections)
	simConnectors = append(simConnectors, simNet)

	simCrypt := simutils.NewSimCrypto(relay.Seed, log)

	// Clock Sync
	log.Debug("Starting clock sync")
	localAddr.Host.Port = 0
	refClocks, netClocks := core.CreateClocks(relay.SvcConfig, localAddr, simClk, simNet, simCrypt, log)
	syncClks := sync.RegisterClocks(refClocks, netClocks)
	syncClks.Id = tmp.Id
	tmp.SyncClks = syncClks
	if len(refClocks) != 0 {
		sync.SyncToRefClocks(log, simClk, syncClks)
		go sync.RunLocalClockSync(log, simClk, syncClks)
	}

	if len(netClocks) != 0 {
		log.Fatal("Unexpected simulation relay configuration",
			zap.Int("number of peers", len(netClocks)))
	}
	log.Debug("Clock syncs active")
	log.Debug("\u001B[34mPress Enter to continue to relay start\u001B[0m")
	//scanner.Scan()
	// Relay starting
	log.Info("Starting relay", zap.String("id", tmp.Id))
	localAddr.Host.Port = ntp.ServerPortSCION
	dscp := core.Dscp(relay.SvcConfig)
	daemonAddr := core.DaemonAddress(relay.SvcConfig)
	server.StartSCIONServer(tmp.Ctx, log, simClk, simNet, daemonAddr, snet.CopyUDPAddr(localAddr.Host), dscp, tmp.Provider)

	return tmp
}

func serverSetUp(i int, simServer SimSvcConfig) Server {
	log.Debug("\u001B[34mSetting up server\u001B[0m", zap.Int("server", i))
	tmp := newServer(receiver)
	tmp.Id = tmp.Id + strconv.Itoa(i)

	checkEmptyAuthMode(&simServer)

	simClk := simutils.NewSimulationClock(log, tmp.Id, NOPModifyTime, timeRequests, waitRequests, ExpectedWaitQueueSize)
	tmp.LocalClk = simClk

	localAddr := core.LocalAddress(simServer.SvcConfig)
	simNet := simutils.NewSimConnector(log, tmp.Id, localAddr, receiver, deadlineRequests, NOPModifyMsg, NOPModifyMsg, DefineDefaultLatency, ExpectedWaitQueueSize, waitingConnections)
	simConnectors = append(simConnectors, simNet)

	simCrypt := simutils.NewSimCrypto(simServer.Seed, log)

	// Clock Sync
	log.Debug("Starting clock sync")
	localAddr.Host.Port = 0
	refClocks, netClocks := core.CreateClocks(simServer.SvcConfig, localAddr, simClk, simNet, simCrypt, log)
	syncClks := sync.RegisterClocks(refClocks, netClocks)
	syncClks.Id = tmp.Id
	tmp.SyncClks = syncClks
	if len(refClocks) != 0 {
		log.Debug("Found reference clocks, adding waiters", zap.Int("amount", len(refClocks)))
		//ExpectedWaitQueueSize.Add(int32(len(refClocks)))
		sync.SyncToRefClocks(log, simClk, syncClks)
		//ExpectedWaitQueueSize.Add(int32(-len(refClocks)))
		go sync.RunLocalClockSync(log, simClk, syncClks)
	}

	if len(netClocks) != 0 {
		log.Debug("Found net clocks, NOT adding waiters", zap.Int("amount", len(netClocks)))
		//ExpectedWaitQueueSize.Add(int32(len(netClocks)))
		go sync.RunGlobalClockSync(log, simClk, syncClks)
	}
	log.Debug("Clock sync active")
	log.Debug("\u001B[34mPress Enter to continue to server start\u001B[0m")
	//scanner.Scan()
	// Server starting
	log.Info("Starting server", zap.String("id", tmp.Id))
	localAddr.Host.Port = ntp.ServerPortSCION
	dscp := core.Dscp(simServer.SvcConfig)
	daemonAddr := core.DaemonAddress(simServer.SvcConfig)
	server.StartSCIONServer(tmp.Ctx, log, simClk, simNet, daemonAddr, snet.CopyUDPAddr(localAddr.Host), dscp, tmp.Provider)

	return tmp
}
