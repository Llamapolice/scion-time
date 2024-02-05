// SCION time service

package main

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"example.com/scion-time/core"
	"example.com/scion-time/core/netbase"
	"example.com/scion-time/driver/networking"
	"example.com/scion-time/simulation/simutils"
	"flag"
	"fmt"
	"github.com/mmcloughlin/profile"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/scionproto/scion/pkg/daemon"
	"github.com/scionproto/scion/pkg/drkey"
	"github.com/scionproto/scion/pkg/snet"
	"github.com/scionproto/scion/pkg/snet/path"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"example.com/scion-time/base/crypto"

	"example.com/scion-time/simulation"

	"example.com/scion-time/benchmark"

	"example.com/scion-time/core/client"
	"example.com/scion-time/core/cryptobase"
	"example.com/scion-time/core/server"
	"example.com/scion-time/core/sync"
	"example.com/scion-time/core/timebase"

	"example.com/scion-time/driver/clock"
	"example.com/scion-time/net/ntp"
	"example.com/scion-time/net/ntske"
	"example.com/scion-time/net/scion"
	"example.com/scion-time/net/udp"
)

const (
	dispatcherModeExternal = "external"
	dispatcherModeInternal = "internal"

	tlsCertReloadInterval = time.Minute * 10
)

type tlsCertCache struct {
	cert       *tls.Certificate
	reloadedAt time.Time
	certFile   string
	keyFile    string
}

var (
	log *zap.Logger
)

func initLogger(verbose bool) {
	c := zap.NewDevelopmentConfig()
	c.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder // Add some color to the output based on log level
	c.DisableStacktrace = true
	c.EncoderConfig.EncodeCaller = func(
		caller zapcore.EntryCaller, enc zapcore.PrimitiveArrayEncoder) {
		// See https://github.com/scionproto/scion/blob/master/pkg/log/log.go
		// TODO: revert to old, shorter version
		//p := caller.TrimmedPath()
		//if len(p) > 30 {
		//	p = "..." + p[len(p)-27:]
		//}
		p := caller.FullPath() // Full path so I can click it in the IDE output :)
		cwd, err := os.Getwd()
		if err != nil {
			enc.AppendString(fmt.Sprintf("%30s", p))
			return
		}
		p = strings.TrimPrefix(p, cwd+"/")
		enc.AppendString(fmt.Sprintf("%30s", p))
	}
	if !verbose {
		c.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}
	var err error
	log, err = c.Build()
	if err != nil {
		panic(err)
	}
}

func runMonitor(log *zap.Logger) {
	http.Handle("/metrics", promhttp.Handler())
	err := http.ListenAndServe("127.0.0.1:8080", nil)
	log.Fatal("failed to serve metrics", zap.Error(err))
}

func (c *tlsCertCache) loadCert(chi *tls.ClientHelloInfo) (*tls.Certificate, error) {
	now := time.Now()
	if now.Before(c.reloadedAt) || !now.Before(c.reloadedAt.Add(tlsCertReloadInterval)) {
		cert, err := tls.LoadX509KeyPair(c.certFile, c.keyFile)
		if err != nil {
			return &tls.Certificate{}, err
		}
		c.cert = &cert
		c.reloadedAt = now
	}
	return c.cert, nil
}

func tlsConfig(cfg core.SvcConfig) *tls.Config {
	if cfg.NTSKEServerName == "" || cfg.NTSKECertFile == "" || cfg.NTSKEKeyFile == "" {
		log.Fatal("missing parameters in configuration for NTSKE server")
	}
	certCache := tlsCertCache{
		certFile: cfg.NTSKECertFile,
		keyFile:  cfg.NTSKEKeyFile,
	}
	return &tls.Config{
		ServerName:     cfg.NTSKEServerName,
		NextProtos:     []string{"ntske/1"},
		GetCertificate: certCache.loadCert,
		MinVersion:     tls.VersionTLS13,
	}
}

func copyIP(ip net.IP) net.IP {
	return append(ip[:0:0], ip...)
}

func runServer(configFile string) {
	ctx := context.Background()

	var cfg core.SvcConfig
	core.LoadConfig(&cfg, configFile, log)
	localAddr := core.LocalAddress(cfg)
	daemonAddr := core.DaemonAddress(cfg)

	localAddr.Host.Port = 0
	refClocks, netClocks := core.CreateClocks(cfg, localAddr, log)
	syncClks := sync.RegisterClocks(refClocks, netClocks)

	lclk := &clock.SystemClock{Log: log}
	timebase.RegisterClock(lclk)

	lcrypt := &crypto.SafeCrypto{Log: log}
	cryptobase.RegisterCrypto(lcrypt)

	lnet := &networking.UDPConnector{Log: log}
	netbase.RegisterNetProvider(lnet)

	if len(refClocks) != 0 {
		sync.SyncToRefClocks(log, lclk, syncClks)
		go sync.RunLocalClockSync(log, lclk, syncClks)
	}

	if len(netClocks) != 0 {
		go sync.RunGlobalClockSync(log, lclk, syncClks)
	}

	dscp := core.Dscp(cfg)
	tlsConfig := tlsConfig(cfg)
	provider := ntske.NewProvider()

	localAddr.Host.Port = ntp.ServerPortIP
	server.StartNTSKEServerIP(ctx, log, copyIP(localAddr.Host.IP), localAddr.Host.Port, tlsConfig, provider)
	server.StartIPServer(ctx, log, snet.CopyUDPAddr(localAddr.Host), dscp, provider)

	localAddr.Host.Port = ntp.ServerPortSCION
	server.StartNTSKEServerSCION(ctx, log, udp.UDPAddrFromSnet(localAddr), tlsConfig, provider)
	server.StartSCIONServer(ctx, log, daemonAddr, snet.CopyUDPAddr(localAddr.Host), dscp, provider)

	runMonitor(log)
}

func runRelay(configFile string) {
	ctx := context.Background()

	var cfg core.SvcConfig
	core.LoadConfig(&cfg, configFile, log)
	localAddr := core.LocalAddress(cfg)
	daemonAddr := core.DaemonAddress(cfg)

	localAddr.Host.Port = 0
	refClocks, netClocks := core.CreateClocks(cfg, localAddr, log)
	syncClks := sync.RegisterClocks(refClocks, netClocks)

	lclk := &clock.SystemClock{Log: log}
	timebase.RegisterClock(lclk)

	lcrypt := &crypto.SafeCrypto{Log: log}
	cryptobase.RegisterCrypto(lcrypt)

	lnet := &networking.UDPConnector{Log: log}
	netbase.RegisterNetProvider(lnet)

	if len(refClocks) != 0 {
		sync.SyncToRefClocks(log, lclk, syncClks)
		go sync.RunLocalClockSync(log, lclk, syncClks)
	}

	if len(netClocks) != 0 {
		log.Fatal("unexpected configuration", zap.Int("number of peers", len(netClocks)))
	}

	dscp := core.Dscp(cfg)
	tlsConfig := tlsConfig(cfg)
	provider := ntske.NewProvider()

	localAddr.Host.Port = ntp.ServerPortIP
	server.StartNTSKEServerIP(ctx, log, copyIP(localAddr.Host.IP), localAddr.Host.Port, tlsConfig, provider)
	server.StartIPServer(ctx, log, snet.CopyUDPAddr(localAddr.Host), dscp, provider)

	localAddr.Host.Port = ntp.ServerPortSCION
	server.StartNTSKEServerSCION(ctx, log, udp.UDPAddrFromSnet(localAddr), tlsConfig, provider)
	server.StartSCIONServer(ctx, log, daemonAddr, snet.CopyUDPAddr(localAddr.Host), dscp, provider)

	runMonitor(log)
}

func runClient(configFile string) {
	ctx := context.Background()

	var cfg core.SvcConfig
	core.LoadConfig(&cfg, configFile, log)
	localAddr := core.LocalAddress(cfg)

	localAddr.Host.Port = 0
	refClocks, netClocks := core.CreateClocks(cfg, localAddr, log)
	syncClks := sync.RegisterClocks(refClocks, netClocks)

	lclk := &clock.SystemClock{Log: log}
	timebase.RegisterClock(lclk)

	lcrypt := &crypto.SafeCrypto{Log: log}
	cryptobase.RegisterCrypto(lcrypt)

	lnet := &networking.UDPConnector{Log: log}
	netbase.RegisterNetProvider(lnet)

	scionClocksAvailable := false
	for _, c := range refClocks {
		_, ok := c.(*core.NtpReferenceClockSCION)
		if ok {
			scionClocksAvailable = true
			break
		}
	}
	if scionClocksAvailable {
		server.StartSCIONDispatcher(ctx, log, snet.CopyUDPAddr(localAddr.Host))
	}

	if len(refClocks) != 0 {
		sync.SyncToRefClocks(log, lclk, syncClks)
		go sync.RunLocalClockSync(log, lclk, syncClks)
	}

	if len(netClocks) != 0 {
		log.Fatal("unexpected configuration", zap.Int("number of peers", len(netClocks)))
	}

	runMonitor(log)
}

func runIPTool(localAddr, remoteAddr *snet.UDPAddr, dscp uint8,
	authModes []string, ntskeServer string, ntskeInsecureSkipVerify, periodic bool) {
	ctx := context.Background()

	lclk := &clock.SystemClock{Log: log}
	timebase.RegisterClock(lclk)

	lcrypt := &crypto.SafeCrypto{Log: log}
	cryptobase.RegisterCrypto(lcrypt)

	lnet := &networking.UDPConnector{Log: log}
	netbase.RegisterNetProvider(lnet)

	laddr := localAddr.Host
	raddr := remoteAddr.Host
	c := &client.IPClient{
		DSCP:            dscp,
		InterleavedMode: true,
	}
	if core.Contains(authModes, core.AuthModeNTS) {
		core.ConfigureIPClientNTS(c, ntskeServer, ntskeInsecureSkipVerify, log)
	}

	for {
		at, off, err := client.MeasureClockOffsetIP(ctx, log, c, laddr, raddr)
		if err != nil {
			log.Fatal("failed to measure clock offset", zap.Stringer("to", raddr), zap.Error(err))
		}
		if !periodic {
			break
		}
		fmt.Printf("%s,%+.9f,%t\n", at.UTC().Format(time.RFC3339), off.Seconds(), c.InInterleavedMode())
		lclk.Sleep(1 * time.Second)
	}
}

func runSCIONTool(daemonAddr, dispatcherMode string, localAddr, remoteAddr *snet.UDPAddr,
	dscp uint8, authModes []string, ntskeServer string, ntskeInsecureSkipVerify bool) {
	var err error
	ctx := context.Background()

	lclk := &clock.SystemClock{Log: log}
	timebase.RegisterClock(lclk)

	lcrypt := &crypto.SafeCrypto{Log: log}
	cryptobase.RegisterCrypto(lcrypt)

	lnet := &networking.UDPConnector{Log: log}
	netbase.RegisterNetProvider(lnet)

	if dispatcherMode == dispatcherModeInternal {
		server.StartSCIONDispatcher(ctx, log, snet.CopyUDPAddr(localAddr.Host))
	}

	dc := netbase.NewDaemonConnector(ctx, daemonAddr)

	var ps []snet.Path
	if remoteAddr.IA.Equal(localAddr.IA) {
		ps = []snet.Path{path.Path{
			Src:           remoteAddr.IA,
			Dst:           remoteAddr.IA,
			DataplanePath: path.Empty{},
		}}
	} else {
		ps, err = dc.Paths(ctx, remoteAddr.IA, localAddr.IA, daemon.PathReqFlags{Refresh: true})
		if err != nil {
			log.Fatal("failed to lookup paths", zap.Stringer("to", remoteAddr.IA), zap.Error(err))
		}
		if len(ps) == 0 {
			log.Fatal("no paths available", zap.Stringer("to", remoteAddr.IA))
		}
	}
	log.Debug("available paths", zap.Stringer("to", remoteAddr.IA), zap.Array("via", scion.PathArrayMarshaler{Paths: ps}))

	laddr := udp.UDPAddrFromSnet(localAddr)
	raddr := udp.UDPAddrFromSnet(remoteAddr)
	c := &client.SCIONClient{
		DSCP:            dscp,
		InterleavedMode: true,
	}
	if core.Contains(authModes, core.AuthModeSPAO) {
		c.Auth.Enabled = true
		c.Auth.DRKeyFetcher = scion.NewFetcher(dc)
	}
	if core.Contains(authModes, core.AuthModeNTS) {
		core.ConfigureSCIONClientNTS(c, ntskeServer, ntskeInsecureSkipVerify, daemonAddr, laddr, raddr, log)
	}

	_, err = client.MeasureClockOffsetSCION(ctx, log, []*client.SCIONClient{c}, laddr, raddr, ps)
	if err != nil {
		log.Fatal("failed to measure clock offset",
			zap.Stringer("remoteIA", raddr.IA),
			zap.Stringer("remoteHost", raddr.Host),
			zap.Error(err),
		)
	}
}

func runBenchmark(configFile string) {
	var cfg core.SvcConfig
	core.LoadConfig(&cfg, configFile, log)
	localAddr := core.LocalAddress(cfg)
	daemonAddr := core.DaemonAddress(cfg)
	remoteAddr := core.RemoteAddress(cfg)

	localAddr.Host.Port = 0
	ntskeServer := core.NtskeServerFromRemoteAddr(cfg.RemoteAddr)

	if !remoteAddr.IA.IsZero() {
		runSCIONBenchmark(daemonAddr, localAddr, remoteAddr, cfg.AuthModes, ntskeServer, log)
	} else {
		if daemonAddr != "" {
			exitWithUsage()
		}
		runIPBenchmark(localAddr, remoteAddr, cfg.AuthModes, ntskeServer, log)
	}
}

func runIPBenchmark(localAddr, remoteAddr *snet.UDPAddr, authModes []string, ntskeServer string, log *zap.Logger) {
	lclk := &clock.SystemClock{Log: zap.NewNop()}
	timebase.RegisterClock(lclk)

	lcrypt := &crypto.SafeCrypto{Log: zap.NewNop()}
	cryptobase.RegisterCrypto(lcrypt)

	lnet := &networking.UDPConnector{Log: zap.NewNop()}
	netbase.RegisterNetProvider(lnet)

	benchmark.RunIPBenchmark(localAddr.Host, remoteAddr.Host, authModes, ntskeServer, log)
}

func runSCIONBenchmark(daemonAddr string, localAddr, remoteAddr *snet.UDPAddr, authModes []string, ntskeServer string, log *zap.Logger) {
	lclk := &clock.SystemClock{Log: zap.NewNop()}
	timebase.RegisterClock(lclk)

	lcrypt := &crypto.SafeCrypto{Log: zap.NewNop()}
	cryptobase.RegisterCrypto(lcrypt)

	lnet := &networking.UDPConnector{Log: zap.NewNop()}
	netbase.RegisterNetProvider(lnet)

	benchmark.RunSCIONBenchmark(daemonAddr, localAddr, remoteAddr, authModes, ntskeServer, log)
}

func runSimulation(seed int64, configFile string) {
	lclk := simutils.NewSimulationClock(seed, log)
	timebase.RegisterClock(lclk)

	lcrypt := simutils.NewSimCrypto(seed, log)
	cryptobase.RegisterCrypto(lcrypt)

	lnet := simutils.NewSimConnector(log)
	netbase.RegisterNetProvider(lnet)

	simulation.RunSimulation(configFile, lclk, lcrypt, lnet, log)
}

func runDRKeyDemo(daemonAddr string, serverMode bool, serverAddr, clientAddr *snet.UDPAddr) {
	ctx := context.Background()
	dc := scion.NewDaemonConnector(ctx, daemonAddr)

	if serverMode {
		hostASMeta := drkey.HostASMeta{
			ProtoId:  123,
			Validity: time.Now(),
			SrcIA:    serverAddr.IA,
			DstIA:    clientAddr.IA,
			SrcHost:  serverAddr.Host.IP.String(),
		}
		hostASKey, err := scion.FetchHostASKey(ctx, dc, hostASMeta)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error fetching host-AS key:", err)
			return
		}
		t0 := time.Now()
		serverKey, err := scion.DeriveHostHostKey(hostASKey, clientAddr.Host.IP.String())
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error deriving host-host key:", err)
		}
		durationServer := time.Since(t0)
		fmt.Printf(
			"Server\thost key = %s\tduration = %s\n",
			hex.EncodeToString(serverKey.Key[:]),
			durationServer,
		)
	} else {
		hostHostMeta := drkey.HostHostMeta{
			ProtoId:  123,
			Validity: time.Now(),
			SrcIA:    serverAddr.IA,
			DstIA:    clientAddr.IA,
			SrcHost:  serverAddr.Host.IP.String(),
			DstHost:  clientAddr.Host.IP.String(),
		}
		t0 := time.Now()
		clientKey, err := scion.FetchHostHostKey(ctx, dc, hostHostMeta)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error fetching host-host key:", err)
			return
		}
		durationClient := time.Since(t0)
		fmt.Printf(
			"Client,\thost key = %s\tduration = %s\n",
			hex.EncodeToString(clientKey.Key[:]),
			durationClient,
		)
	}
}

func exitWithUsage() {
	fmt.Println("<usage>")
	os.Exit(1)
}

func main() {
	var (
		verbose                 bool
		configFile              string
		daemonAddr              string
		localAddr               snet.UDPAddr
		remoteAddrStr           string
		dispatcherMode          string
		drkeyMode               string
		drkeyServerAddr         snet.UDPAddr
		drkeyClientAddr         snet.UDPAddr
		dscp                    uint
		seed                    int64
		authModesStr            string
		ntskeInsecureSkipVerify bool
		profileCPU              bool
		periodic                bool
	)

	serverFlags := flag.NewFlagSet("server", flag.ExitOnError)
	relayFlags := flag.NewFlagSet("relay", flag.ExitOnError)
	clientFlags := flag.NewFlagSet("client", flag.ExitOnError)
	toolFlags := flag.NewFlagSet("tool", flag.ExitOnError)
	benchmarkFlags := flag.NewFlagSet("benchmark", flag.ExitOnError)
	drkeyFlags := flag.NewFlagSet("drkey", flag.ExitOnError)
	simulationFlags := flag.NewFlagSet("simulation", flag.ExitOnError)

	serverFlags.BoolVar(&verbose, "verbose", false, "Verbose logging")
	serverFlags.StringVar(&configFile, "config", "", "Config file")
	serverFlags.BoolVar(&profileCPU, "profile-cpu", false, "Enable profiling")

	relayFlags.BoolVar(&verbose, "verbose", false, "Verbose logging")
	relayFlags.StringVar(&configFile, "config", "", "Config file")

	clientFlags.BoolVar(&verbose, "verbose", false, "Verbose logging")
	clientFlags.StringVar(&configFile, "config", "", "Config file")

	toolFlags.BoolVar(&verbose, "verbose", false, "Verbose logging")
	toolFlags.StringVar(&daemonAddr, "daemon", "", "Daemon address")
	toolFlags.StringVar(&dispatcherMode, "dispatcher", "", "Dispatcher mode")
	toolFlags.Var(&localAddr, "local", "Local address")
	toolFlags.StringVar(&remoteAddrStr, "remote", "", "Remote address")
	toolFlags.UintVar(&dscp, "dscp", 0, "Differentiated services codepoint, must be in range [0, 63]")
	toolFlags.StringVar(&authModesStr, "auth", "", "Authentication modes")
	toolFlags.BoolVar(&ntskeInsecureSkipVerify, "ntske-insecure-skip-verify", false, "Skip NTSKE verification")
	toolFlags.BoolVar(&periodic, "periodic", false, "Perform periodic offset measurements")

	benchmarkFlags.BoolVar(&verbose, "verbose", false, "Verbose logging")
	benchmarkFlags.StringVar(&configFile, "config", "", "Config file")

	drkeyFlags.BoolVar(&verbose, "verbose", false, "Verbose logging")
	drkeyFlags.StringVar(&daemonAddr, "daemon", "", "Daemon address")
	drkeyFlags.StringVar(&drkeyMode, "mode", "", "Mode")
	drkeyFlags.Var(&drkeyServerAddr, "server", "Server address")
	drkeyFlags.Var(&drkeyClientAddr, "client", "Client address")

	simulationFlags.StringVar(&configFile, "config", "", "Simulation config file")
	simulationFlags.Int64Var(&seed, "seed", 5, "Seed for the pseudorandom generation, defaults to 5")

	if len(os.Args) < 2 {
		exitWithUsage()
	}

	switch os.Args[1] {
	case simulationFlags.Name():
		err := simulationFlags.Parse(os.Args[2:])
		if err != nil || simulationFlags.NArg() != 0 {
			fmt.Println("NArg not 0")
			exitWithUsage()
		}
		if configFile == "" {
			fmt.Println("configFile empty?")
			exitWithUsage()
		}
		initLogger(true)
		runSimulation(seed, configFile)
	case serverFlags.Name():
		err := serverFlags.Parse(os.Args[2:])
		if err != nil || serverFlags.NArg() != 0 {
			exitWithUsage()
		}
		if configFile == "" {
			exitWithUsage()
		}
		if profileCPU {
			defer profile.Start(profile.CPUProfile).Stop()
		}
		initLogger(verbose)
		runServer(configFile)
	case relayFlags.Name():
		err := relayFlags.Parse(os.Args[2:])
		if err != nil || relayFlags.NArg() != 0 {
			exitWithUsage()
		}
		if configFile == "" {
			exitWithUsage()
		}
		initLogger(verbose)
		runRelay(configFile)
	case clientFlags.Name():
		err := clientFlags.Parse(os.Args[2:])
		if err != nil || clientFlags.NArg() != 0 {
			exitWithUsage()
		}
		if configFile == "" {
			exitWithUsage()
		}
		initLogger(verbose)
		runClient(configFile)
	case toolFlags.Name():
		err := toolFlags.Parse(os.Args[2:])
		if err != nil || toolFlags.NArg() != 0 {
			exitWithUsage()
		}
		var remoteAddr snet.UDPAddr
		err = remoteAddr.Set(remoteAddrStr)
		if err != nil {
			exitWithUsage()
		}
		if dscp > 63 {
			exitWithUsage()
		}
		authModes := strings.Split(authModesStr, ",")
		for i := range authModes {
			authModes[i] = strings.TrimSpace(authModes[i])
		}
		if !remoteAddr.IA.IsZero() {
			if dispatcherMode == "" {
				dispatcherMode = dispatcherModeExternal
			} else if dispatcherMode != dispatcherModeExternal &&
				dispatcherMode != dispatcherModeInternal {
				exitWithUsage()
			}
			ntskeServer := core.NtskeServerFromRemoteAddr(remoteAddrStr)
			initLogger(verbose)
			runSCIONTool(daemonAddr, dispatcherMode, &localAddr, &remoteAddr, uint8(dscp),
				authModes, ntskeServer, ntskeInsecureSkipVerify)
		} else {
			if daemonAddr != "" {
				exitWithUsage()
			}
			if dispatcherMode != "" {
				exitWithUsage()
			}
			ntskeServer := core.NtskeServerFromRemoteAddr(remoteAddrStr)
			initLogger(verbose)
			runIPTool(&localAddr, &remoteAddr, uint8(dscp),
				authModes, ntskeServer, ntskeInsecureSkipVerify, periodic)
		}
	case benchmarkFlags.Name():
		err := benchmarkFlags.Parse(os.Args[2:])
		if err != nil || benchmarkFlags.NArg() != 0 {
			exitWithUsage()
		}
		if configFile == "" {
			exitWithUsage()
		}
		initLogger(verbose)
		runBenchmark(configFile)
	case drkeyFlags.Name():
		err := drkeyFlags.Parse(os.Args[2:])
		if err != nil || drkeyFlags.NArg() != 0 {
			exitWithUsage()
		}
		if drkeyMode != "server" && drkeyMode != "client" {
			exitWithUsage()
		}
		serverMode := drkeyMode == "server"
		initLogger(verbose)
		runDRKeyDemo(daemonAddr, serverMode, &drkeyServerAddr, &drkeyClientAddr)
	case "x":
		runX()
	default:
		exitWithUsage()
	}
}
