package core

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/scionproto/scion/pkg/addr"
	"github.com/scionproto/scion/pkg/snet"

	"example.com/scion-time/base/cryptobase"
	"example.com/scion-time/base/netbase"
	"example.com/scion-time/base/timebase"
	"example.com/scion-time/base/zaplog"
	"example.com/scion-time/core/client"
	"example.com/scion-time/driver/mbg"
	"example.com/scion-time/driver/phc"
	"example.com/scion-time/driver/shm"
	"example.com/scion-time/net/scion"
	"example.com/scion-time/net/udp"
)

const (
	AuthModeNTS  = "nts"
	AuthModeSPAO = "spao"

	scionRefClockNumClient = 7
)

type SvcConfig struct {
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

type NtpReferenceClockSCION struct {
	crypt      cryptobase.CryptoProvider
	ntpcs      [scionRefClockNumClient]*client.SCIONClient
	localAddr  udp.UDPAddr
	remoteAddr udp.UDPAddr
	pather     *scion.Pather
}

func (c *NtpReferenceClockSCION) MeasureClockOffset(ctx context.Context) (
	time.Time, time.Duration, error) {
	// TODO: Only a temporary workaround to not panic here, some pather needs to be added for the simulation
	var paths []snet.Path
	if c.pather != nil {
		paths = c.pather.Paths(c.remoteAddr.IA)
	}
	return client.MeasureClockOffsetSCION(ctx, zaplog.Logger(), c.crypt, c.ntpcs[:], c.localAddr, c.remoteAddr, paths)
}

type NtpReferenceClockIP struct {
	ntpc       *client.IPClient
	localAddr  *net.UDPAddr
	remoteAddr *net.UDPAddr
}

func (c *NtpReferenceClockIP) MeasureClockOffset(ctx context.Context) (
	time.Time, time.Duration, error) {
	return client.MeasureClockOffsetIP(ctx, zaplog.Logger(), c.ntpc, c.localAddr, c.remoteAddr)
}

// Originally in timeservice.go, seems to be deprecated now
type MbgReferenceClock struct {
	Dev string
}

// Originally in timeservice.go, seems to be deprecated now
//func (c *MbgReferenceClock) MeasureClockOffset(ctx context.Context, log *zap.Logger) (
//	time.Duration, error) {
//	return mbg.MeasureClockOffset(ctx, log, c.Dev)
//}

// LoadConfig loads configuration from a file and decodes it into a struct.
// The cfgStruct parameter must be a pointer to the configuration struct to be filled.
// The configFile parameter specifies the file path from which to load the configuration.
// If an error occurs while loading or decoding the configuration, the function will log a fatal error.
// Originally in timeservice.go
func LoadConfig[T any](cfgStruct T, configFile string) { // T is pointer to config struct
	raw, err := os.ReadFile(configFile)
	if err != nil {
		LogFatal("failed to load configuration", slog.Any("error", err))
	}
	err = toml.NewDecoder(bytes.NewReader(raw)).DisallowUnknownFields().Decode(cfgStruct)
	if err != nil {
		fmt.Println(err.(*toml.StrictMissingError).String())
		LogFatal("failed to decode configuration", slog.Any("error", err))
	}
}

// Originally in timeservice.go
func CreateClocks(
	cfg SvcConfig,
	localAddr *snet.UDPAddr,
	lclk timebase.LocalClock,
	lnet netbase.ConnProvider,
	lcrypt cryptobase.CryptoProvider,
	log *slog.Logger,
) (
	refClocks, netClocks []client.ReferenceClock) {
	dscp := Dscp(cfg)

	// this for example could be a simulated gps clock
	// maybe implement this using something like SimReferenceClocks (also then in config)
	for _, s := range cfg.MBGReferenceClocks {
		refClocks = append(refClocks, mbg.NewReferenceClock(log, s))
	}

	for _, s := range cfg.PHCReferenceClocks {
		refClocks = append(refClocks, phc.NewReferenceClock(log, s))
	}

	for _, s := range cfg.SHMReferenceClocks {
		t := strings.Split(s, ":")
		if len(t) > 2 || t[0] != shm.ReferenceClockType {
			LogFatal("unexpected SHM reference clock id", slog.String("id", s))
		}
		var u int
		if len(t) > 1 {
			var err error
			u, err = strconv.Atoi(t[1])
			if err != nil {
				LogFatal("unexpected SHM reference clock id",
					slog.String("id", s), slog.Any("error", err))
			}
		}
		refClocks = append(refClocks, shm.NewReferenceClock(log, u))
	}

	var dstIAs []addr.IA
	for _, s := range cfg.NTPReferenceClocks {
		remoteAddr, err := snet.ParseUDPAddr(s)
		if err != nil {
			LogFatal("failed to parse reference clock address",
				slog.String("address", s), slog.Any("error", err))
		}
		ntskeServer := NtskeServerFromRemoteAddr(s)
		if !remoteAddr.IA.IsZero() {
			refClocks = append(refClocks, NewNTPReferenceClockSCION(
				cfg.DaemonAddr,
				udp.UDPAddrFromSnet(localAddr),
				udp.UDPAddrFromSnet(remoteAddr),
				lclk,
				lnet,
				lcrypt,
				dscp,
				cfg.AuthModes,
				ntskeServer,
				cfg.NTSKEInsecureSkipVerify,
				log,
			))
			dstIAs = append(dstIAs, remoteAddr.IA)
		} else {
			refClocks = append(refClocks, NewNTPReferenceClockIP(
				localAddr.Host,
				remoteAddr.Host,
				lclk,
				lnet,
				dscp,
				cfg.AuthModes,
				ntskeServer,
				cfg.NTSKEInsecureSkipVerify,
				log,
			))
		}
	}

	for _, s := range cfg.SCIONPeers {
		remoteAddr, err := snet.ParseUDPAddr(s)
		if err != nil {
			LogFatal("failed to parse peer address", slog.String("address", s), slog.Any("error", err))
		}
		if remoteAddr.IA.IsZero() {
			LogFatal("unexpected peer address", slog.String("address", s), slog.Any("error", err))
		}
		ntskeServer := NtskeServerFromRemoteAddr(s)
		netClocks = append(netClocks, NewNTPReferenceClockSCION(
			cfg.DaemonAddr,
			udp.UDPAddrFromSnet(localAddr),
			udp.UDPAddrFromSnet(remoteAddr),
			lclk,
			lnet,
			lcrypt,
			dscp,
			cfg.AuthModes,
			ntskeServer,
			cfg.NTSKEInsecureSkipVerify,
			log,
		))
		dstIAs = append(dstIAs, remoteAddr.IA)
	}

	daemonAddr := DaemonAddress(cfg)
	if daemonAddr != "" {
		ctx := context.Background()
		pather := scion.StartPather(ctx, log, lnet, daemonAddr, dstIAs)
		var drkeyFetcher *scion.Fetcher
		if Contains(cfg.AuthModes, AuthModeSPAO) {
			drkeyFetcher = scion.NewFetcher(scion.NewDaemonConnector(ctx, daemonAddr))
		}
		for _, c := range refClocks {
			scionclk, ok := c.(*NtpReferenceClockSCION)
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
			scionclk, ok := c.(*NtpReferenceClockSCION)
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

func LogFatal(msg string, attrs ...slog.Attr) {
	slog.LogAttrs(context.Background(), slog.LevelError, msg, attrs...)
	os.Exit(1)
}

func NtskeServerFromRemoteAddr(remoteAddr string) string {
	split := strings.Split(remoteAddr, ",")
	if len(split) < 2 {
		panic("remote address has wrong format")
	}
	return split[1]
}

func Contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func Dscp(cfg SvcConfig) uint8 {
	if cfg.DSCP > 63 {
		LogFatal("invalid differentiated services codepoint value specified in config")
	}
	return cfg.DSCP
}

func LocalAddress(cfg SvcConfig) *snet.UDPAddr {
	if cfg.LocalAddr == "" {
		log.Fatal("local_address not specified in config")
	}
	var localAddr snet.UDPAddr
	err := localAddr.Set(cfg.LocalAddr)
	if err != nil {
		log.Fatal("failed to parse local address")
	}
	return &localAddr
}

func RemoteAddress(cfg SvcConfig) *snet.UDPAddr {
	if cfg.RemoteAddr == "" {
		log.Fatal("remote_address not specified in config")
	}
	var remoteAddr snet.UDPAddr
	err := remoteAddr.Set(cfg.RemoteAddr)
	if err != nil {
		log.Fatal("failed to parse remote address")
	}
	return &remoteAddr
}

func DaemonAddress(cfg SvcConfig) string {
	da := cfg.DaemonAddr
	if da == "" {
		return da
	}
	if da[:3] == "sim" { // TODO is there a better way to do this?
		da += "@" + LocalAddress(cfg).IA.String()
	}
	return da
}

func NewNTPReferenceClockSCION(
	daemonAddr string,
	localAddr, remoteAddr udp.UDPAddr,
	lclk timebase.LocalClock,
	lnet netbase.ConnProvider,
	lcrypt cryptobase.CryptoProvider,
	dscp uint8,
	authModes []string,
	ntskeServer string,
	ntskeInsecureSkipVerify bool,
	log *slog.Logger,
) *NtpReferenceClockSCION {
	c := &NtpReferenceClockSCION{
		crypt:      lcrypt,
		localAddr:  localAddr,
		remoteAddr: remoteAddr,
	}
	for i := range len(c.ntpcs) {
		c.ntpcs[i] = &client.SCIONClient{
			Lclk:               lclk,
			ConnectionProvider: lnet,
			DSCP:               dscp,
			InterleavedMode:    true,
		}
		if Contains(authModes, AuthModeNTS) {
			ConfigureSCIONClientNTS(c.ntpcs[i], ntskeServer, ntskeInsecureSkipVerify, daemonAddr, localAddr, remoteAddr, log)
		}
	}
	return c
}

func NewNTPReferenceClockIP(
	localAddr, remoteAddr *net.UDPAddr,
	lclk timebase.LocalClock,
	connprov netbase.ConnProvider,
	dscp uint8,
	authModes []string,
	ntskeServer string,
	ntskeInsecureSkipVerify bool,
	log *slog.Logger,
) *NtpReferenceClockIP {
	c := &NtpReferenceClockIP{
		localAddr:  localAddr,
		remoteAddr: remoteAddr,
	}
	c.ntpc = &client.IPClient{
		Lclk:               lclk,
		ConnectionProvider: connprov,
		DSCP:               dscp,
		InterleavedMode:    true,
	}
	if Contains(authModes, AuthModeNTS) {
		ConfigureIPClientNTS(c.ntpc, ntskeServer, ntskeInsecureSkipVerify, log)
	}
	return c
}

func ConfigureIPClientNTS(c *client.IPClient, ntskeServer string, ntskeInsecureSkipVerify bool, log *slog.Logger) {
	ntskeHost, ntskePort, err := net.SplitHostPort(ntskeServer)
	if err != nil {
		LogFatal("failed to split NTS-KE host and port", slog.Any("error", err))
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

func ConfigureSCIONClientNTS(c *client.SCIONClient, ntskeServer string, ntskeInsecureSkipVerify bool,
	daemonAddr string, localAddr, remoteAddr udp.UDPAddr, log *slog.Logger) {
	ntskeHost, ntskePort, err := net.SplitHostPort(ntskeServer)
	if err != nil {
		LogFatal("failed to split NTS-KE host and port", slog.Any("error", err))
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
