package simulation

import (
	"bufio"
	"context"
	"example.com/scion-time/core"
	"example.com/scion-time/core/client"
	"example.com/scion-time/core/cryptocore"
	"example.com/scion-time/core/server"
	"example.com/scion-time/core/sync"
	"example.com/scion-time/net/ntp"
	"example.com/scion-time/net/ntske"
	"example.com/scion-time/net/udp"
	"example.com/scion-time/simulation/simutils"
	"github.com/scionproto/scion/pkg/snet"
	"github.com/scionproto/scion/pkg/snet/path"
	"go.uber.org/zap"
	"os"
	"strconv"
	"time"
)

const maxNumberOfInstances = 30

type SimConfigFile struct {
	// TODO, WIP
	Servers []core.SvcConfig `toml:"servers"`
	Relays  []core.SvcConfig `toml:"relays"`
	Clients []core.SvcConfig `toml:"clients"`
	Tools   []core.SvcConfig `toml:"tools"`
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
	log           *zap.Logger
	receiver      chan simutils.SimPacket
	simConnectors []*simutils.SimConnector
	//handleConnectionSetup func(
	//	id string,
	//	receiveFromInstance chan simutils.SimPacket,
	//	sendToInstance chan simutils.SimPacket,
	//) *simutils.SimConnection
	scanner          *bufio.Scanner
	timeRequests     chan simutils.TimeRequest
	waitRequests     chan simutils.WaitRequest
	deadlineRequests chan simutils.DeadlineRequest
)

func RunSimulation(
	configFile string,
	seed int64,
	logger *zap.Logger,
) {
	log = logger
	log.Info("\u001B[34mStarting simulation\u001B[0m")

	// Some logic to read a config file and fill a settings struct
	log.Debug("Reading config file", zap.String("config location", configFile))
	var cfg SimConfigFile
	core.LoadConfig(&cfg, configFile, log)

	// Set up file package vars
	receiver = make(chan simutils.SimPacket)
	simConnectors = make([]*simutils.SimConnector, 0, len(cfg.Clients)+len(cfg.Relays)+len(cfg.Servers)+1)
	scanner = bufio.NewScanner(os.Stdin)
	timeRequests = make(chan simutils.TimeRequest)
	waitRequests = make(chan simutils.WaitRequest)
	deadlineRequests = make(chan simutils.DeadlineRequest)

	lcrypt := simutils.NewSimCrypto(seed, log)
	cryptocore.RegisterCrypto(lcrypt)

	//netcore.RegisterNetProvider(simConnector)

	// Some set up to build the simulated network and start instances
	// Register some channels into the sims
	// Size 2 as to not block since the connection is opened within the main routine
	//simConnectionListener := make(chan *simutils.SimConnection, 2)
	//simConnector.CallBack = simConnectionListener

	//receivingInstances := make(map[string]chan simutils.SimPacket)

	// Bare-bones message handler to pass messages around
	go func() {
		log.Info("\u001B[34mMessage handler started\u001B[0m")
	skip:
		for msg := range receiver {
			log.Debug(
				"Message handler received message",
				zap.Binary("msg", msg.B),
				zap.String("target", msg.TargetAddr.String()),
				zap.String("source", msg.SourceAddr.String()),
			)
			//receivingInstance, exists := receivingInstances[msg.TargetAddr.String()]
			//if exists {
			//	receivingInstance <- msg
			//	log.Debug("Passed message on to instance")
			//} else {
			targetAddr := msg.TargetAddr.Addr().WithZone("").Unmap()
			for _, simConn := range simConnectors {
				laddrPort := simConn.LocalAddress.Host.AddrPort()
				simConnLocalAddress := laddrPort.Addr().WithZone("").Unmap()
				if simConnLocalAddress == targetAddr {
					passOn := func() {
						simConn.Input <- msg
						log.Debug("Passed message on to instance", zap.Duration("after", msg.Latency))
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
		}
		log.Info("\u001B[34mMessage handler terminating\u001B[0m")
	}()

	// Sketch for a ground time provider
	go func() {
		log.Info("Time handler started")
		now := time.Unix(10000, 0)
		currentlyWaiting := make([]simutils.WaitRequest, 0, maxNumberOfInstances)
		deadlines := make([]simutils.DeadlineRequest, 0, maxNumberOfInstances*3) // 3 connections max per instance sounds reasonable?
		for {
			select {
			case req := <-timeRequests:
				log.Debug("Time has been requested", zap.String("by", req.Id))
				req.ReturnChan <- now
			case req := <-waitRequests:
				log.Debug("Wait has been requested", zap.String("by", req.Id), zap.Duration("duration", req.SleepDuration))
				currentlyWaiting = append(currentlyWaiting, req)
			case req := <-deadlineRequests:
				log.Debug("Deadline has been requested", zap.String("by", req.Id))
				req.RequestTime = now
				deadlines = append(deadlines, req)
			default:
				time.Sleep(time.Second / 10) // TODO just for development
				//if len(currentlyWaiting) == simutils.NumberOfClocks {
				if len(currentlyWaiting) > 0 {
					log.Info("\u001B[41m======== TIME HANDLER ========\u001B[0m", zap.Int("currently waiting", len(currentlyWaiting)))
					if len(currentlyWaiting) < 5 { // TODO how to adjust this number dynamically?
						continue
					}
					//log.Info("Enter 'w' to wait one loop for other goroutines or enter 'p' to process the next request in the queue")
					//scanner.Scan()
					//if scanner.Text() == "q" {
					//	os.Exit(0)
					//}
					//if scanner.Text() != "p" {
					//	continue
					//}
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
					for _, request := range currentlyWaiting {
						request.SleepDuration -= minDuration
					}
					now = now.Add(minDuration)
					log.Debug("Time handler unblocks a sleeper", zap.String("id", shortestRequest.Id))
					shortestRequest.Action()
				}
				is := make([]int, 0)
				for i, deadline := range deadlines {
					if deadline.Deadline.Before(now) {
						deadline.Unblock <- now.Sub(deadline.RequestTime)
						is = append(is, i)
					}
				}
				adj := 0
				for _, i := range is {
					deadlines = append(deadlines[:i-adj], deadlines[i-adj+1:]...)
					adj += 1 // This is to adjust the indices when multiple are to be removed
				}
			}
		}
	}()

	log.Info("\u001B[34mSetting up Servers\u001B[0m", zap.Int("amount", len(cfg.Servers)))
	simServers := make([]Server, len(cfg.Servers))

	for i, simServer := range cfg.Servers {
		tmp := serverSetUp(i, simServer)
		simServers[i] = tmp
		log.Debug("\u001B[34mDone setting up server, press Enter to continue\u001B[0m",
			zap.String("server id", tmp.Id))
		//scanner.Scan()
	}

	// Relays
	// Currently not tested, just basically copy/pasted from timeservice.go
	log.Info("\u001B[34mServers are set up, continuing with Relays\u001B[0m",
		zap.Int("amount", len(cfg.Relays)))
	simRelays := make([]Relay, len(cfg.Relays))

	for i, relay := range cfg.Relays {
		tmp := relaySetUp(i, relay)

		simRelays[i] = tmp
		log.Debug("\u001B[34mDone setting up relay, press Enter to continue\u001B[0m",
			zap.String("relay id", tmp.Id))
		//scanner.Scan()
	}

	// Clients
	// turns out I did some weird stuff here, now basically using what is in timeservice.go
	log.Info("\u001B[34mServers and Relays are set up, now setting up Clients\u001B[0m",
		zap.Int("amount", len(cfg.Clients)))
	simClients := make([]Client, len(cfg.Clients))
	for i, clnt := range cfg.Clients {
		tmp := clientSetUp(i, clnt)

		simClients[i] = tmp
		log.Debug("\u001B[34mDone setting up client, press Enter to continue\u001B[0m",
			zap.String("client id", tmp.Id))
		//scanner.Scan()
	}

	log.Info("\u001B[34mSetup completed\u001B[0m")

	for i, tool := range cfg.Tools {
		log.Info("\u001B[34mPress Enter to run tool\u001B[0m", zap.Int("tool", i))
		//scanner.Scan()
		runTool(i, tool)
	}

	// Main loop of simulation
	for condition := true; condition; {
		// Pass messages around between instances
		// Drop, corrupt, duplicate, kill, start, disconnect connections and instances as needed
		condition = false
	}

	select {}
}

func runTool(i int, tool core.SvcConfig) {
	id := "tool_" + strconv.Itoa(i)
	ctxClient := context.Background()
	lclk := simutils.NewSimulationClock(log, id, int64(i), timeRequests, waitRequests)
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

	simConnector := simutils.NewSimConnector(log, id, &laddrSNET, receiver, deadlineRequests)
	simConnectors = append(simConnectors, simConnector)

	ntpcs := []*client.SCIONClient{
		{Lclk: lclk, ConnectionProvider: simConnector, DSCP: 0, InterleavedMode: false},
	}
	ps := []snet.Path{
		path.Path{Src: laddrSNET.IA, Dst: raddrSNET.IA, DataplanePath: path.Empty{}},
	}

	go func() {
		log.Debug("Tool setup complete, running offset measurement now", zap.String("id", id))
		medianDuration, err := client.MeasureClockOffsetSCION(ctxClient, log, ntpcs, laddr, raddr, ps)
		if err != nil {
			log.Fatal("Tool had an error", zap.Error(err))
		}
		log.Debug("\u001B[31mMedian Duration measured by tool\u001B[0m",
			zap.Duration("duration", medianDuration))
		//log.Info("\u001B[34mPress Enter to exit simulation\u001B[0m")
		////scanner.Scan()
		//os.Exit(0)
	}()
}

func clientSetUp(i int, clnt core.SvcConfig) Client {
	log.Debug("Setting up client", zap.Int("client", i))
	tmp := newClient(receiver)
	tmp.Id += strconv.Itoa(i)

	laddr := core.LocalAddress(clnt)
	simClk := simutils.NewSimulationClock(log, tmp.Id, int64(i), timeRequests, waitRequests)
	tmp.LocalClk = simClk
	simNet := simutils.NewSimConnector(log, tmp.Id, laddr, receiver, deadlineRequests)
	simConnectors = append(simConnectors, simNet)

	laddr.Host.Port = 0
	refClocks, netClocks := core.CreateClocks(clnt, laddr, simClk, simNet, log)
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
		//tmp.Conn = handleConnectionSetup(tmp.Id+"_conn", tmp.ReceiveFromInstance, tmp.SendToInstance)
		//log.Debug("Simulator received connection",
		//	zap.String("relay id", tmp.Id), zap.String("connection id", tmp.Conn.Id))
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

func relaySetUp(i int, relay core.SvcConfig) Relay {
	log.Debug("\u001B[34mSetting up relay\u001B[0m", zap.Int("relay", i))
	tmp := newRelay(receiver)
	tmp.Id = tmp.Id + strconv.Itoa(i)

	localAddr := core.LocalAddress(relay)
	simClk := simutils.NewSimulationClock(log, tmp.Id, int64(i), timeRequests, waitRequests)
	tmp.LocalClk = simClk
	simNet := simutils.NewSimConnector(log, tmp.Id, localAddr, receiver, deadlineRequests)
	simConnectors = append(simConnectors, simNet)

	// Clock Sync
	log.Debug("Starting clock sync")
	localAddr.Host.Port = 0
	refClocks, netClocks := core.CreateClocks(relay, localAddr, simClk, simNet, log)
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
	dscp := core.Dscp(relay)
	daemonAddr := core.DaemonAddress(relay)
	server.StartSCIONServer(tmp.Ctx, log, simClk, simNet, daemonAddr, snet.CopyUDPAddr(localAddr.Host), dscp, tmp.Provider)

	//tmp.Conn = handleConnectionSetup(tmp.Id+"_conn", tmp.ReceiveFromInstance, tmp.SendToInstance)
	log.Debug("Simulator received connection",
		zap.String("relay id", tmp.Id), zap.String("connection id", tmp.Conn.Id))
	return tmp
}

func serverSetUp(i int, simServer core.SvcConfig) Server {
	log.Debug("\u001B[34mSetting up server\u001B[0m", zap.Int("server", i))
	tmp := newServer(receiver)
	tmp.Id = tmp.Id + strconv.Itoa(i)

	localAddr := core.LocalAddress(simServer)
	simClk := simutils.NewSimulationClock(log, tmp.Id, int64(i), timeRequests, waitRequests)
	tmp.LocalClk = simClk
	simNet := simutils.NewSimConnector(log, tmp.Id, localAddr, receiver, deadlineRequests)
	simConnectors = append(simConnectors, simNet)

	// Clock Sync
	log.Debug("Starting clock sync")
	localAddr.Host.Port = 0
	refClocks, netClocks := core.CreateClocks(simServer, localAddr, simClk, simNet, log)
	syncClks := sync.RegisterClocks(refClocks, netClocks)
	syncClks.Id = tmp.Id
	tmp.SyncClks = syncClks
	if len(refClocks) != 0 {
		log.Debug("Found reference clocks", zap.Int("amount", len(refClocks)))
		sync.SyncToRefClocks(log, simClk, syncClks)
		go sync.RunLocalClockSync(log, simClk, syncClks)
	}

	if len(netClocks) != 0 {
		log.Debug("Found net clocks", zap.Int("amount", len(netClocks)))
		go sync.RunGlobalClockSync(log, simClk, syncClks)
		//tmpSyncConn := handleConnectionSetup(tmp.Id+"_netSync", tmp.ReceiveFromInstance, nil)
		//log.Debug("Received sync connection", zap.String("local addr", tmpSyncConn.LAddr.String()))
	}
	log.Debug("Clock sync active")
	log.Debug("\u001B[34mPress Enter to continue to server start\u001B[0m")
	//scanner.Scan()
	// Server starting
	log.Info("Starting server", zap.String("id", tmp.Id))
	localAddr.Host.Port = ntp.ServerPortSCION
	dscp := core.Dscp(simServer)
	daemonAddr := core.DaemonAddress(simServer)
	server.StartSCIONServer(tmp.Ctx, log, simClk, simNet, daemonAddr, snet.CopyUDPAddr(localAddr.Host), dscp, tmp.Provider)

	//tmp.Conn = handleConnectionSetup(tmp.Id+"_conn", tmp.ReceiveFromInstance, tmp.SendToInstance)
	//log.Debug("Simulator received connection",
	//	zap.String("server id", tmp.Id), zap.String("connection id", tmp.Conn.Id),
	//	zap.String("conn laddr", tmp.Conn.LAddr.String()))
	return tmp
}
