// SCION time service

package main

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"example.com/scion-time/core"
	"example.com/scion-time/driver/networking"
	"flag"
	"fmt"
	"log/slog"
	"github.com/mmcloughlin/profile"
	"net"
	"net/http"
	"os"
	"strconv"
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

	"example.com/scion-time/base/logbase"
	"example.com/scion-time/base/zaplog"

	"example.com/scion-time/benchmark"

	"example.com/scion-time/core/client"
	"example.com/scion-time/core/server"
	"example.com/scion-time/core/sync"
	"example.com/scion-time/core/timecore"

	"example.com/scion-time/driver/clock"
	"example.com/scion-time/driver/mbg"
	"example.com/scion-time/driver/phc"
	"example.com/scion-time/driver/shm"

	"example.com/scion-time/net/ntp"
	"example.com/scion-time/net/ntske"
	"example.com/scion-time/net/scion"
	"example.com/scion-time/net/udp"
)

const (
	dispatcherModeExternal = "external"
	dispatcherModeInternal = "internal"

	tlsCertReloadInterval = time.Minute * 10

	scionRefClockNumClient = 7
)

type svcConfig struct {
	LocalAddr               string   `toml:"local_address,omitempty"`
	DaemonAddr              string   `toml:"daemon_address,omitempty"`
	RemoteAddr              string   `toml:"remote_address,omitempty"`
	MBGReferenceClocks      []string `toml:"mbg_reference_clocks,omitempty"`
	PHCReferenceClocks      []string `toml:"phc_reference_clocks,omitempty"`
	SHMReferenceClocks      []string `toml:"shm_reference_clocks,omitempty"`
	NTPReferenceClocks      []string `toml:"ntp_reference_clocks,omitempty"`
	SCIONPeers              []string `toml:"scion_peers,omitempty"`
	NTSKECertFile           string   `toml:"ntske_cert_file,omitempty"`
	NTSKEKeyFile            string   `toml:"ntske_key_file,omitempty"`
	NTSKEServerName         string   `toml:"ntske_server_name,omitempty"`
	AuthModes               []string `toml:"auth_modes,omitempty"`
	NTSKEInsecureSkipVerify bool     `toml:"ntske_insecure_skip_verify,omitempty"`
	DSCP                    uint8    `toml:"dscp,omitempty"` // must be in range [0, 63]
}

type ntpReferenceClockIP struct {
	ntpc       *client.IPClient
	localAddr  *net.UDPAddr
	remoteAddr *net.UDPAddr
}

type ntpReferenceClockSCION struct {
	ntpcs      [scionRefClockNumClient]*client.SCIONClient
	localAddr  udp.UDPAddr
	remoteAddr udp.UDPAddr
	pather     *scion.Pather
}

type tlsCertCache struct {
	cert       *tls.Certificate
	reloadedAt time.Time
	certFile   string
	keyFile    string
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func initLogger(verbose bool, outputFile string, console bool) {
	zapcfg := zap.NewDevelopmentConfig()
	zapcfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder // Add some color to the output based on log level
	zapcfg.DisableStacktrace = true
	zapcfg.EncoderConfig.EncodeCaller = func(
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
		zapcfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}
	zaplogger, err := zapcfg.Build()
	if err != nil {
		panic(err)
	}
	if outputFile != "" {
		file, err := os.Create(outputFile)
		if err == nil {
			zapcfg.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
			fileEncoder := zapcore.NewJSONEncoder(c.EncoderConfig)
			fileCore := zapcore.NewCore(fileEncoder, zapcore.Lock(zapcore.AddSync(file)), zapcore.DebugLevel)
			if console {
				zaplogger = zap.New(zapcore.NewTee(fileCore, log.Core()))
			} else {
				zaplogger = zap.New(fileCore)
			}
		}
	}
	zaplog.SetLogger(zaplogger)

	var level slog.Leveler
	if verbose {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr,
		&slog.HandlerOptions{Level: level})))
}

func logFatal(msg string, attrs ...slog.Attr) {
	slog.LogAttrs(context.Background(), slog.LevelError, msg, attrs...)
	os.Exit(1)
}

func runMonitor() {
	http.Handle("/metrics", promhttp.Handler())
	err := http.ListenAndServe("127.0.0.1:8080", nil)
	logFatal("failed to serve metrics", slog.Any("error", err))
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

func configureIPClientNTS(c *client.IPClient, ntskeServer string, ntskeInsecureSkipVerify bool, log *slog.Logger) {
	ntskeHost, ntskePort, err := net.SplitHostPort(ntskeServer)
	if err != nil {
		logFatal("failed to split NTS-KE host and port", slog.Any("error", err))
	}
	c.Auth.Enabled = true
	c.Auth.NTSKEFetcher.TLSConfig = tls.Config{
		NextProtos:         []string{"ntske/1"},
		InsecureSkipVerify: ntskeInsecureSkipVerify,
		ServerName:         ntskeHost,
		MinVersion:         tls.VersionTLS13,
	}
	c.Auth.NTSKEFetcher.Port = ntskePort
	c.Auth.NTSKEFetcher.Log = log
}

func newNTPReferenceClockIP(log *slog.Logger, localAddr, remoteAddr *net.UDPAddr, dscp uint8,
	authModes []string, ntskeServer string, ntskeInsecureSkipVerify bool) *ntpReferenceClockIP {
	c := &ntpReferenceClockIP{
		localAddr:  localAddr,
		remoteAddr: remoteAddr,
	}
	c.ntpc = &client.IPClient{
		DSCP:            dscp,
		InterleavedMode: true,
	}
	if contains(authModes, authModeNTS) {
		configureIPClientNTS(c.ntpc, ntskeServer, ntskeInsecureSkipVerify, log)
	}
	return c
}

func (c *ntpReferenceClockIP) MeasureClockOffset(ctx context.Context) (
	time.Time, time.Duration, error) {
	return client.MeasureClockOffsetIP(ctx, zaplog.Logger(), c.ntpc, c.localAddr, c.remoteAddr)
}

func configureSCIONClientNTS(c *client.SCIONClient, ntskeServer string, ntskeInsecureSkipVerify bool,
	daemonAddr string, localAddr, remoteAddr udp.UDPAddr, log *slog.Logger) {
	ntskeHost, ntskePort, err := net.SplitHostPort(ntskeServer)
	if err != nil {
		logFatal("failed to split NTS-KE host and port", slog.Any("error", err))
	}
	c.Auth.NTSEnabled = true
	c.Auth.NTSKEFetcher.TLSConfig = tls.Config{
		NextProtos:         []string{"ntske/1"},
		InsecureSkipVerify: ntskeInsecureSkipVerify,
		ServerName:         ntskeHost,
		MinVersion:         tls.VersionTLS13,
	}
	c.Auth.NTSKEFetcher.Port = ntskePort
	c.Auth.NTSKEFetcher.Log = log
	c.Auth.NTSKEFetcher.QUIC.Enabled = true
	c.Auth.NTSKEFetcher.QUIC.DaemonAddr = daemonAddr
	c.Auth.NTSKEFetcher.QUIC.LocalAddr = localAddr
	c.Auth.NTSKEFetcher.QUIC.RemoteAddr = remoteAddr
}

func newNTPReferenceClockSCION(log *slog.Logger, daemonAddr string, localAddr, remoteAddr udp.UDPAddr, dscp uint8,
	authModes []string, ntskeServer string, ntskeInsecureSkipVerify bool) *ntpReferenceClockSCION {
	c := &ntpReferenceClockSCION{
		localAddr:  localAddr,
		remoteAddr: remoteAddr,
	}
	for i := range len(c.ntpcs) {
		c.ntpcs[i] = &client.SCIONClient{
			DSCP:            dscp,
			InterleavedMode: true,
		}
		if contains(authModes, authModeNTS) {
			configureSCIONClientNTS(c.ntpcs[i], ntskeServer, ntskeInsecureSkipVerify, daemonAddr, localAddr, remoteAddr, log)
		}
	}
	return c
}

func (c *ntpReferenceClockSCION) MeasureClockOffset(ctx context.Context) (
	time.Time, time.Duration, error) {
	paths := c.pather.Paths(c.remoteAddr.IA)
	return client.MeasureClockOffsetSCION(ctx, zaplog.Logger(), c.ntpcs[:], c.localAddr, c.remoteAddr, paths)
}

func loadConfig(configFile string) svcConfig {
	raw, err := os.ReadFile(configFile)
	if err != nil {
		logFatal("failed to load configuration", slog.Any("error", err))
	}
	var cfg svcConfig
	err = toml.NewDecoder(bytes.NewReader(raw)).DisallowUnknownFields().Decode(&cfg)
	if err != nil {
		logFatal("failed to decode configuration", slog.Any("error", err))
	}
	return cfg
}

func localAddress(cfg svcConfig) *snet.UDPAddr {
	if cfg.LocalAddr == "" {
		logFatal("local_address not specified in config")
	}
	var localAddr snet.UDPAddr
	err := localAddr.Set(cfg.LocalAddr)
	if err != nil {
		logFatal("failed to parse local address")
	}
	return &localAddr
}

func remoteAddress(cfg svcConfig) *snet.UDPAddr {
	if cfg.RemoteAddr == "" {
		logFatal("remote_address not specified in config")
	}
	var remoteAddr snet.UDPAddr
	err := remoteAddr.Set(cfg.RemoteAddr)
	if err != nil {
		logFatal("failed to parse remote address")
	}
	return &remoteAddr
}

func daemonAddress(cfg svcConfig) string {
	return cfg.DaemonAddr
}

func dscp(cfg svcConfig) uint8 {
	if cfg.DSCP > 63 {
		logFatal("invalid differentiated services codepoint value specified in config")
	}
	return cfg.DSCP
}



func tlsConfig(cfg core.SvcConfig) *tls.Config {
	if cfg.NTSKEServerName == "" || cfg.NTSKECertFile == "" || cfg.NTSKEKeyFile == "" {
		logFatal("missing parameters in configuration for NTSKE server")
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

func createClocks(cfg svcConfig, localAddr *snet.UDPAddr, log *slog.Logger) (
	refClocks, netClocks []client.ReferenceClock) {
	dscp := dscp(cfg)

	for _, s := range cfg.MBGReferenceClocks {
		refClocks = append(refClocks, mbg.NewReferenceClock(log, s))
	}

	for _, s := range cfg.PHCReferenceClocks {
		refClocks = append(refClocks, phc.NewReferenceClock(log, s))
	}

	for _, s := range cfg.SHMReferenceClocks {
		t := strings.Split(s, ":")
		if len(t) > 2 || t[0] != shm.ReferenceClockType {
			logFatal("unexpected SHM reference clock id", slog.String("id", s))
		}
		var u int
		if len(t) > 1 {
			var err error
			u, err = strconv.Atoi(t[1])
			if err != nil {
				logFatal("unexpected SHM reference clock id",
					slog.String("id", s), slog.Any("error", err))
			}
		}
		refClocks = append(refClocks, shm.NewReferenceClock(log, u))
	}

	var dstIAs []addr.IA
	for _, s := range cfg.NTPReferenceClocks {
		remoteAddr, err := snet.ParseUDPAddr(s)
		if err != nil {
			logFatal("failed to parse reference clock address",
				slog.String("address", s), slog.Any("error", err))
		}
		ntskeServer := ntskeServerFromRemoteAddr(s)
		if !remoteAddr.IA.IsZero() {
			refClocks = append(refClocks, newNTPReferenceClockSCION(
				log,
				cfg.DaemonAddr,
				udp.UDPAddrFromSnet(localAddr),
				udp.UDPAddrFromSnet(remoteAddr),
				dscp,
				cfg.AuthModes,
				ntskeServer,
				cfg.NTSKEInsecureSkipVerify,
			))
			dstIAs = append(dstIAs, remoteAddr.IA)
		} else {
			refClocks = append(refClocks, newNTPReferenceClockIP(
				log,
				localAddr.Host,
				remoteAddr.Host,
				dscp,
				cfg.AuthModes,
				ntskeServer,
				cfg.NTSKEInsecureSkipVerify,
			))
		}
	}

	for _, s := range cfg.SCIONPeers {
		remoteAddr, err := snet.ParseUDPAddr(s)
		if err != nil {
			logFatal("failed to parse peer address", slog.String("address", s), slog.Any("error", err))
		}
		if remoteAddr.IA.IsZero() {
			logFatal("unexpected peer address", slog.String("address", s), slog.Any("error", err))
		}
		ntskeServer := ntskeServerFromRemoteAddr(s)
		netClocks = append(netClocks, newNTPReferenceClockSCION(
			log,
			cfg.DaemonAddr,
			udp.UDPAddrFromSnet(localAddr),
			udp.UDPAddrFromSnet(remoteAddr),
			dscp,
			cfg.AuthModes,
			ntskeServer,
			cfg.NTSKEInsecureSkipVerify,
		))
		dstIAs = append(dstIAs, remoteAddr.IA)
	}

	daemonAddr := daemonAddress(cfg)
	if daemonAddr != "" {
		ctx := context.Background()
		pather := scion.StartPather(ctx, log, daemonAddr, dstIAs)
		var drkeyFetcher *scion.Fetcher
		if contains(cfg.AuthModes, authModeSPAO) {
			drkeyFetcher = scion.NewFetcher(scion.NewDaemonConnector(ctx, daemonAddr))
		}
		for _, c := range refClocks {
			scionclk, ok := c.(*ntpReferenceClockSCION)
			if ok {
				scionclk.pather = pather
				if drkeyFetcher != nil {
					for i := 0; i != len(scionclk.ntpcs); i++ {
						scionclk.ntpcs[i].Auth.Enabled = true
						scionclk.ntpcs[i].Auth.DRKeyFetcher = drkeyFetcher
					}
				}
			}
		}
		for _, c := range netClocks {
			scionclk, ok := c.(*ntpReferenceClockSCION)
			if ok {
				scionclk.pather = pather
				if drkeyFetcher != nil {
					for i := 0; i != len(scionclk.ntpcs); i++ {
						scionclk.ntpcs[i].Auth.Enabled = true
						scionclk.ntpcs[i].Auth.DRKeyFetcher = drkeyFetcher
					}
				}
			}
		}
	}

	return
}

func copyIP(ip net.IP) net.IP {
	return append(ip[:0:0], ip...)
}

func runServer(configFile string) {
	ctx := context.Background()
	log := slog.Default()

	var cfg core.SvcConfig
	core.LoadConfig(&cfg, configFile, log)
	localAddr := core.LocalAddress(cfg)
	daemonAddr := core.DaemonAddress(cfg)

	lclk := &clock.SystemClock{Log: log}
	timecore.RegisterClock(lclk)

	lcrypt := &crypto.SafeCrypto{Log: log}

	lnet := &networking.UDPConnector{Log: log}

	localAddr.Host.Port = 0
	refClocks, netClocks := core.CreateClocks(cfg, localAddr, lclk, lnet, lcrypt, log)
	syncClks := sync.RegisterClocks(refClocks, netClocks)

	if len(refClocks) != 0 {
		sync.SyncToRefClocks(zaplog.Logger(), lclk, syncClks)
		go sync.RunLocalClockSync(zaplog.Logger(), lclk, syncClks)
	}

	if len(netClocks) != 0 {
		go sync.RunGlobalClockSync(zaplog.Logger(), lclk, syncClks)
	}

	dscp := core.Dscp(cfg)
	tlsConfig := tlsConfig(cfg)
	provider := ntske.NewProvider()

	localAddr.Host.Port = ntp.ServerPortIP
	server.StartNTSKEServerIP(ctx, log, copyIP(localAddr.Host.IP), localAddr.Host.Port, tlsConfig, provider)
	server.StartIPServer(ctx, zaplog.Logger(), lclk, lnet, snet.CopyUDPAddr(localAddr.Host), dscp, provider)

	localAddr.Host.Port = ntp.ServerPortSCION
	server.StartNTSKEServerSCION(ctx, log, udp.UDPAddrFromSnet(localAddr), tlsConfig, provider)
	server.StartSCIONServer(ctx, zaplog.Logger(), lclk, lnet, daemonAddr, snet.CopyUDPAddr(localAddr.Host), dscp, provider)

	runMonitor()
}

func runRelay(configFile string) {
	ctx := context.Background()
	log := slog.Default()

	var cfg core.SvcConfig
	core.LoadConfig(&cfg, configFile, log)
	localAddr := core.LocalAddress(cfg)
	daemonAddr := core.DaemonAddress(cfg)

	lclk := &clock.SystemClock{Log: log}
	timecore.RegisterClock(lclk)

	lcrypt := &crypto.SafeCrypto{Log: log}

	lnet := &networking.UDPConnector{Log: log}

	localAddr.Host.Port = 0
	refClocks, netClocks := core.CreateClocks(cfg, localAddr, lclk, lnet, lcrypt, log)
	syncClks := sync.RegisterClocks(refClocks, netClocks)

	if len(refClocks) != 0 {
		sync.SyncToRefClocks(zaplog.Logger(), lclk, syncClks)
		go sync.RunLocalClockSync(zaplog.Logger(), lclk, syncClks)
	}

	if len(netClocks) != 0 {
		logFatal("unexpected configuration", slog.Int("number of peers", len(netClocks)))
	}

	dscp := core.Dscp(cfg)
	tlsConfig := tlsConfig(cfg)
	provider := ntske.NewProvider()

	localAddr.Host.Port = ntp.ServerPortIP
	server.StartNTSKEServerIP(ctx, log, copyIP(localAddr.Host.IP), localAddr.Host.Port, tlsConfig, provider)
	server.StartIPServer(ctx, zaplog.Logger(), lclk, lnet, snet.CopyUDPAddr(localAddr.Host), dscp, provider)

	localAddr.Host.Port = ntp.ServerPortSCION
	server.StartNTSKEServerSCION(ctx, log, udp.UDPAddrFromSnet(localAddr), tlsConfig, provider)
	server.StartSCIONServer(ctx, zaplog.Logger(), lclk, lnet, daemonAddr, snet.CopyUDPAddr(localAddr.Host), dscp, provider)

	runMonitor()
}

func runClient(configFile string) {
	ctx := context.Background()
	log := slog.Default()

	var cfg core.SvcConfig
	core.LoadConfig(&cfg, configFile, log)
	localAddr := core.LocalAddress(cfg)

	lclk := &clock.SystemClock{Log: log}
	timecore.RegisterClock(lclk)

	lcrypt := &crypto.SafeCrypto{Log: log}

	lnet := &networking.UDPConnector{Log: log}

	localAddr.Host.Port = 0
	refClocks, netClocks := core.CreateClocks(cfg, localAddr, lclk, lnet, lcrypt, log)
	syncClks := sync.RegisterClocks(refClocks, netClocks)

	scionClocksAvailable := false
	for _, c := range refClocks {
		_, ok := c.(*core.NtpReferenceClockSCION)
		if ok {
			scionClocksAvailable = true
			break
		}
	}
	if scionClocksAvailable {
		server.StartSCIONDispatcher(ctx, zaplog.Logger(), lclk, lnet, snet.CopyUDPAddr(localAddr.Host))
	}

	if len(refClocks) != 0 {
		sync.SyncToRefClocks(zaplog.Logger(), lclk, syncClks)
		go sync.RunLocalClockSync(zaplog.Logger(), lclk, syncClks)
	}

	if len(netClocks) != 0 {
		logFatal("unexpected configuration", slog.Int("number of peers", len(netClocks)))
	}

	runMonitor()
}

func runIPTool(localAddr, remoteAddr *snet.UDPAddr, dscp uint8,
	authModes []string, ntskeServer string, ntskeInsecureSkipVerify, periodic bool) {
	ctx := context.Background()
	log := slog.Default()

	lclk := &clock.SystemClock{Log: log}
	timecore.RegisterClock(lclk)

	lnet := &networking.UDPConnector{Log: log}

	laddr := localAddr.Host
	raddr := remoteAddr.Host
	c := &client.IPClient{
		Lclk:               lclk,
		ConnectionProvider: lnet,
		DSCP:               dscp,
		InterleavedMode:    true,
	}
	if core.Contains(authModes, core.AuthModeNTS) {
		core.ConfigureIPClientNTS(c, ntskeServer, ntskeInsecureSkipVerify, log)
	}

	for {
		ts, off, err := client.MeasureClockOffsetIP(ctx, zaplog.Logger(), c, laddr, raddr)
		if err != nil {
			logFatal("failed to measure clock offset", slog.Any("remote", raddr), slog.Any("error", err))
		}
		if !periodic {
			break
		}
		fmt.Printf("%s,%+.9f,%t\n", ts.UTC().Format(time.RFC3339), off.Seconds(), c.InInterleavedMode())
		lclk.Sleep(1 * time.Second)
	}
}

func runSCIONTool(daemonAddr, dispatcherMode string, localAddr, remoteAddr *snet.UDPAddr,
	dscp uint8, authModes []string, ntskeServer string, ntskeInsecureSkipVerify bool) {
	var err error
	ctx := context.Background()
	log := slog.Default()

	lclk := &clock.SystemClock{Log: log}
	timecore.RegisterClock(lclk)

	lcrypt := &crypto.SafeCrypto{Log: log}

	lnet := &networking.UDPConnector{Log: log}

	if dispatcherMode == dispatcherModeInternal {
		server.StartSCIONDispatcher(ctx, zaplog.Logger(), lclk, lnet, snet.CopyUDPAddr(localAddr.Host))
	}

	dc := lnet.NewDaemonConnector(ctx, daemonAddr)

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
			logFatal("failed to lookup paths", slog.Any("remote", remoteAddr), slog.Any("error", err))
		}
		if len(ps) == 0 {
			logFatal("no paths available", slog.Any("remote", remoteAddr))
		}
	}
	log.LogAttrs(ctx, slog.LevelDebug,
		"available paths",
		slog.Any("remote", remoteAddr),
		slog.Any("via", ps),
	)

	laddr := udp.UDPAddrFromSnet(localAddr)
	raddr := udp.UDPAddrFromSnet(remoteAddr)
	c := &client.SCIONClient{
		Lclk:               lclk,
		ConnectionProvider: lnet,
		DSCP:               dscp,
		InterleavedMode:    true,
	}
	if core.Contains(authModes, core.AuthModeSPAO) {
		c.Auth.Enabled = true
		c.Auth.DRKeyFetcher = scion.NewFetcher(dc)
	}
	if core.Contains(authModes, core.AuthModeNTS) {
		core.ConfigureSCIONClientNTS(c, ntskeServer, ntskeInsecureSkipVerify, daemonAddr, laddr, raddr, log)
	}

	_, _, err = client.MeasureClockOffsetSCION(ctx, zaplog.Logger(), lcrypt, []*client.SCIONClient{c}, laddr, raddr, ps)
	if err != nil {
		logFatal("failed to measure clock offset",
			slog.Any("remote", remoteAddr),
			slog.Any("error", err),
		)
	}
}

func runBenchmark(configFile string) {
	var cfg core.SvcConfig
	core.LoadConfig(&cfg, configFile, log)
	log := slog.Default()

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

func runIPBenchmark(localAddr, remoteAddr *snet.UDPAddr, authModes []string, ntskeServer string, log *slog.Logger) {
	lclk := &clock.SystemClock{Log: slog.New(logbase.NewNopHandler())}
	timecore.RegisterClock(lclk)

	lnet := &networking.UDPConnector{Log: zap.NewNop()}

	benchmark.RunIPBenchmark(localAddr.Host, remoteAddr.Host, lclk, lnet, authModes, ntskeServer, log)
}

func runSCIONBenchmark(daemonAddr string, localAddr, remoteAddr *snet.UDPAddr, authModes []string, ntskeServer string, log *slog.Logger) {
	lclk := &clock.SystemClock{Log: slog.New(logbase.NewNopHandler())}
	timecore.RegisterClock(lclk)

	lcrypt := &crypto.SafeCrypto{Log: zap.NewNop()}

	lnet := &networking.UDPConnector{Log: zap.NewNop()}

	benchmark.RunSCIONBenchmark(daemonAddr, localAddr, remoteAddr, lclk, lnet, lcrypt, authModes, ntskeServer, log)
}

func runSimulation(configFile string) {
	simulation.RunSimulation(configFile, log)
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
		authModesStr            string
		ntskeInsecureSkipVerify bool
		profileCPU              bool
		periodic                bool
		console                 bool
		logfile                 string
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
	simulationFlags.BoolVar(&console, "console", true, "Print log messages to stdout")
	simulationFlags.StringVar(&logfile, "logfile", "", "Save log messages to this file")

	if len(os.Args) < 2 {
		exitWithUsage()
	}

	switch os.Args[1] {
	case simulationFlags.Name():
		err := simulationFlags.Parse(os.Args[2:])
		if err != nil || simulationFlags.NArg() != 0 {
			exitWithUsage()
		}
		if configFile == "" {
			exitWithUsage()
		}
		initLogger(true, logfile, console)
		runSimulation(configFile)
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
		initLogger(verbose, "", true)
		runServer(configFile)
	case relayFlags.Name():
		err := relayFlags.Parse(os.Args[2:])
		if err != nil || relayFlags.NArg() != 0 {
			exitWithUsage()
		}
		if configFile == "" {
			exitWithUsage()
		}
		initLogger(verbose, "", true)
		runRelay(configFile)
	case clientFlags.Name():
		err := clientFlags.Parse(os.Args[2:])
		if err != nil || clientFlags.NArg() != 0 {
			exitWithUsage()
		}
		if configFile == "" {
			exitWithUsage()
		}
		initLogger(verbose, "", true)
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
			initLogger(verbose, "", true)
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
			initLogger(verbose, "", true)
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
		initLogger(verbose, "", true)
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
		initLogger(verbose, "", true)
		runDRKeyDemo(daemonAddr, serverMode, &drkeyServerAddr, &drkeyClientAddr)
	case "x":
		runX()
	default:
		exitWithUsage()
	}
}
