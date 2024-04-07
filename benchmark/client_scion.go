package benchmark

import (
	"context"
	"crypto/tls"
	"example.com/scion-time/base/cryptobase"
	"example.com/scion-time/base/netbase"
	"example.com/scion-time/base/timebase"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	"github.com/HdrHistogram/hdrhistogram-go"

	"github.com/scionproto/scion/pkg/daemon"
	"github.com/scionproto/scion/pkg/snet"
	"github.com/scionproto/scion/pkg/snet/path"

	"example.com/scion-time/base/zaplog"
	"example.com/scion-time/core/client"
	"example.com/scion-time/net/scion"
	"example.com/scion-time/net/udp"
)

func RunSCIONBenchmark(
	daemonAddr string,
	localAddr, remoteAddr *snet.UDPAddr,
	lclk timebase.LocalClock,
	lnet netbase.ConnProvider,
	lcrypt cryptobase.CryptoProvider,
	authModes []string,
	ntskeServer string,
	log *slog.Logger,
) {
	// const numClientGoroutine = 8
	// const numRequestPerClient = 10000
	const numClientGoroutine = 1
	const numRequestPerClient = 20_000

	ctx := context.Background()

	var mu sync.Mutex
	sg := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(numClientGoroutine)

	for range numClientGoroutine {
		go func() {
			var err error
			hg := hdrhistogram.New(1, 50000, 5)

			dc := scion.NewDaemonConnector(ctx, daemonAddr)

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
					logFatal(ctx, log, "failed to lookup paths", slog.Any("to", remoteAddr.IA), slog.Any("error", err))
				}
				if len(ps) == 0 {
					logFatal(ctx, log, "no paths available", slog.Any("to", remoteAddr.IA))
				}
			}
			log.LogAttrs(ctx, slog.LevelDebug,
				"available paths",
				slog.Any("to", remoteAddr.IA),
				slog.Any("via", ps),
			)

			laddr := udp.UDPAddrFromSnet(localAddr)
			raddr := udp.UDPAddrFromSnet(remoteAddr)
			c := &client.SCIONClient{
				Lclk:               lclk,
				ConnectionProvider: lnet,
				InterleavedMode:    true,
				Histo:              hg,
			}

			if contains(authModes, "spao") {
				c.Auth.Enabled = true
				c.Auth.DRKeyFetcher = scion.NewFetcher(dc)
			}

			if contains(authModes, "nts") {
				ntskeHost, ntskePort, err := net.SplitHostPort(ntskeServer)
				if err != nil {
					logFatal(ctx, log, "failed to split NTS-KE host and port", slog.Any("error", err))
				}
				c.Auth.NTSEnabled = true
				c.Auth.NTSKEFetcher.TLSConfig = tls.Config{
					InsecureSkipVerify: true,
					ServerName:         ntskeHost,
					MinVersion:         tls.VersionTLS13,
				}
				c.Auth.NTSKEFetcher.Port = ntskePort
				c.Auth.NTSKEFetcher.Log = log
				c.Auth.NTSKEFetcher.QUIC.Enabled = true
				c.Auth.NTSKEFetcher.QUIC.DaemonAddr = daemonAddr
				c.Auth.NTSKEFetcher.QUIC.LocalAddr = laddr
				c.Auth.NTSKEFetcher.QUIC.RemoteAddr = raddr
			}

			defer wg.Done()
			<-sg
			ntpcs := []*client.SCIONClient{c}
			for range numRequestPerClient {
				_, _, err = client.MeasureClockOffsetSCION(ctx, zaplog.Logger(), lcrypt, ntpcs, laddr, raddr, ps)
				if err != nil {
					log.LogAttrs(ctx, slog.LevelInfo,
						"failed to measure clock offset",
						slog.Any("remoteIA", raddr.IA),
						slog.Any("remoteHost", raddr.Host),
						slog.Any("error", err),
					)
				}
			}
			mu.Lock()
			defer mu.Unlock()
			hg.PercentilesPrint(os.Stdout, 1, 1.0)
		}()
	}
	t0 := time.Now()
	close(sg)
	wg.Wait()
	log.LogAttrs(ctx, slog.LevelInfo, "time elbasped", slog.Duration("duration", time.Since(t0)))
}
