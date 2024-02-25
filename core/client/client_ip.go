package client

import (
	"context"
	"example.com/scion-time/base/netbase"
	"example.com/scion-time/base/timebase"
	"net"
	"net/netip"
	"time"

	"github.com/HdrHistogram/hdrhistogram-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"go.uber.org/zap"

	"example.com/scion-time/base/metrics"

	"example.com/scion-time/net/ntp"
	"example.com/scion-time/net/nts"
	"example.com/scion-time/net/ntske"
	"example.com/scion-time/net/udp"
)

type IPClient struct {
	Lclk               timebase.LocalClock
	ConnectionProvider netbase.ConnProvider
	DSCP               uint8
	InterleavedMode    bool
	Auth               struct {
		Enabled      bool
		NTSKEFetcher ntske.Fetcher
	}
	Raw   bool
	Histo *hdrhistogram.Histogram
	prev  struct {
		reference   string
		interleaved bool
		cTxTime     ntp.Time64
		cRxTime     ntp.Time64
		sRxTime     ntp.Time64
	}
}

type ipClientMetrics struct {
	reqsSent                 prometheus.Counter
	reqsSentInterleaved      prometheus.Counter
	pktsReceived             prometheus.Counter
	pktsAuthenticated        prometheus.Counter
	respsAccepted            prometheus.Counter
	respsAcceptedInterleaved prometheus.Counter
}

func newIPClientMetrics() *ipClientMetrics {
	return &ipClientMetrics{
		reqsSent: promauto.NewCounter(prometheus.CounterOpts{
			Name: metrics.IPClientReqsSentN,
			Help: metrics.IPClientReqsSentH,
		}),
		reqsSentInterleaved: promauto.NewCounter(prometheus.CounterOpts{
			Name: metrics.IPClientReqsSentInterleavedN,
			Help: metrics.IPClientReqsSentInterleavedH,
		}),
		pktsReceived: promauto.NewCounter(prometheus.CounterOpts{
			Name: metrics.IPClientPktsReceivedN,
			Help: metrics.IPClientPktsReceivedH,
		}),
		pktsAuthenticated: promauto.NewCounter(prometheus.CounterOpts{
			Name: metrics.IPClientPktsAuthenticatedN,
			Help: metrics.IPClientPktsAuthenticatedH,
		}),
		respsAccepted: promauto.NewCounter(prometheus.CounterOpts{
			Name: metrics.IPClientRespsAcceptedN,
			Help: metrics.IPClientRespsAcceptedH,
		}),
		respsAcceptedInterleaved: promauto.NewCounter(prometheus.CounterOpts{
			Name: metrics.IPClientRespsAcceptedInterleavedN,
			Help: metrics.IPClientRespsAcceptedInterleavedH,
		}),
	}
}

func compareAddrs(x, y netip.Addr) int {
	return x.Unmap().Compare(y.Unmap())
}

func (c *IPClient) InInterleavedMode() bool {
	return c.InterleavedMode && c.prev.reference != "" && c.prev.interleaved
}

func (c *IPClient) ResetInterleavedMode() {
	c.prev.reference = ""
}

func (c *IPClient) measureClockOffsetIP(ctx context.Context, log *zap.Logger, mtrcs *ipClientMetrics,
	localAddr, remoteAddr *net.UDPAddr) (
	at time.Time, offset time.Duration, weight float64, err error) {
	conn, err := c.ConnectionProvider.ListenUDP("udp", &net.UDPAddr{IP: localAddr.IP})
	if err != nil {
		return at, offset, weight, err
	}
	defer conn.Close()
	deadline, deadlineIsSet := ctx.Deadline()
	if deadlineIsSet {
		err = conn.SetDeadline(deadline)
		if err != nil {
			return at, offset, weight, err
		}
	}
	err = c.ConnectionProvider.EnableTimestamping(conn, localAddr.Zone)
	if err != nil {
		log.Error("failed to enable timestamping", zap.Error(err))
	}
	err = c.ConnectionProvider.SetDSCP(conn, c.DSCP)
	if err != nil {
		log.Info("failed to set DSCP", zap.Error(err))
	}

	var ntskeData ntske.Data
	if c.Auth.Enabled {
		ntskeData, err = c.Auth.NTSKEFetcher.FetchData()
		if err != nil {
			log.Info("failed to fetch key exchange data", zap.Error(err))
			return at, offset, weight, err
		}
		remoteAddr.IP = net.ParseIP(ntskeData.Server)
		remoteAddr.Port = int(ntskeData.Port)
	}
	ip4 := remoteAddr.IP.To4()
	if ip4 != nil {
		remoteAddr.IP = ip4
	}

	buf := make([]byte, ntp.PacketLen)

	reference := remoteAddr.String()
	cTxTime0 := c.Lclk.Now()
	interleavedReq := false

	ntpreq := ntp.Packet{}
	ntpreq.SetVersion(ntp.VersionMax)
	ntpreq.SetMode(ntp.ModeClient)
	if c.InterleavedMode && reference == c.prev.reference &&
		cTxTime0.Sub(ntp.TimeFromTime64(c.prev.cTxTime)) <= 2*time.Second {
		interleavedReq = true
		ntpreq.OriginTime = c.prev.sRxTime
		ntpreq.ReceiveTime = c.prev.cRxTime
		ntpreq.TransmitTime = c.prev.cTxTime
	} else {
		ntpreq.TransmitTime = ntp.Time64FromTime(cTxTime0)
	}
	ntp.EncodePacket(&buf, &ntpreq)

	var requestID []byte
	var ntsreq nts.Packet
	if c.Auth.Enabled {
		ntsreq, requestID = nts.NewRequestPacket(ntskeData)
		nts.EncodePacket(&buf, &ntsreq)
	}

	n, err := conn.WriteToUDPAddrPort(buf, remoteAddr.AddrPort())
	if err != nil {
		return at, offset, weight, err
	}
	if n != len(buf) {
		return at, offset, weight, errWrite
	}
	cTxTime1, id, err := c.ConnectionProvider.ReadTXTimestamp(conn)
	if err != nil || id != 0 {
		cTxTime1 = c.Lclk.Now()
		log.Error("failed to read packet tx timestamp", zap.Error(err))
	}
	mtrcs.reqsSent.Inc()
	if interleavedReq {
		mtrcs.reqsSentInterleaved.Inc()
	}

	numRetries := 0
	oob := make([]byte, udp.TimestampLen())
	for {
		buf = buf[:cap(buf)]
		oob = oob[:cap(oob)]
		n, oobn, flags, srcAddr, err := conn.ReadMsgUDPAddrPort(buf, oob)
		if err != nil {
			if numRetries != maxNumRetries && deadlineIsSet && c.Lclk.Now().Before(deadline) {
				log.Info("failed to read packet", zap.Error(err))
				numRetries++
				continue
			}
			return at, offset, weight, err
		}
		if flags != 0 {
			err = errUnexpectedPacketFlags
			if numRetries != maxNumRetries && deadlineIsSet && c.Lclk.Now().Before(deadline) {
				log.Info("failed to read packet", zap.Int("flags", flags))
				numRetries++
				continue
			}
			return at, offset, weight, err
		}
		oob = oob[:oobn]
		cRxTime, err := udp.TimestampFromOOBData(oob)
		if err != nil {
			cRxTime = c.Lclk.Now()
			log.Error("failed to read packet rx timestamp", zap.Error(err))
		}
		buf = buf[:n]
		mtrcs.pktsReceived.Inc()

		if compareAddrs(srcAddr.Addr(), remoteAddr.AddrPort().Addr()) != 0 {
			err = errUnexpectedPacketSource
			if numRetries != maxNumRetries && deadlineIsSet && c.Lclk.Now().Before(deadline) {
				log.Info("received packet from unexpected source")
				numRetries++
				continue
			}
			return at, offset, weight, err
		}

		var ntpresp ntp.Packet
		err = ntp.DecodePacket(&ntpresp, buf)
		if err != nil {
			if numRetries != maxNumRetries && deadlineIsSet && c.Lclk.Now().Before(deadline) {
				log.Info("failed to decode packet payload", zap.Error(err))
				numRetries++
				continue
			}
			return at, offset, weight, err
		}

		authenticated := false
		var ntsresp nts.Packet
		if c.Auth.Enabled {
			err = nts.DecodePacket(&ntsresp, buf)
			if err != nil {
				if numRetries != maxNumRetries && deadlineIsSet && c.Lclk.Now().Before(deadline) {
					log.Info("failed to decode NTS packet", zap.Error(err))
					numRetries++
					continue
				}
				return at, offset, weight, err
			}

			err = nts.ProcessResponse(buf, ntskeData.S2cKey, &c.Auth.NTSKEFetcher, &ntsresp, requestID)
			if err != nil {
				if numRetries != maxNumRetries && deadlineIsSet && c.Lclk.Now().Before(deadline) {
					log.Info("failed to process NTS packet", zap.Error(err))
					numRetries++
					continue
				}
				return at, offset, weight, err
			}

			authenticated = true
			mtrcs.pktsAuthenticated.Inc()
		}

		interleavedResp := false
		if interleavedReq && ntpresp.OriginTime == ntpreq.ReceiveTime {
			interleavedResp = true
		} else if ntpresp.OriginTime != ntpreq.TransmitTime {
			err = errUnexpectedPacket
			if numRetries != maxNumRetries && deadlineIsSet && c.Lclk.Now().Before(deadline) {
				log.Info("received packet with unexpected type or structure")
				numRetries++
				continue
			}
			return at, offset, weight, err
		}

		err = ntp.ValidateResponseMetadata(&ntpresp)
		if err != nil {
			return at, offset, weight, err
		}

		log.Debug("received response",
			zap.Time("at", cRxTime),
			zap.String("from", reference),
			zap.Bool("auth", authenticated),
			zap.Object("data", ntp.PacketMarshaler{Pkt: &ntpresp}),
		)

		sRxTime := ntp.TimeFromTime64(ntpresp.ReceiveTime)
		sTxTime := ntp.TimeFromTime64(ntpresp.TransmitTime)

		var t0, t1, t2, t3 time.Time
		if interleavedResp {
			t0 = ntp.TimeFromTime64(c.prev.cTxTime)
			t1 = ntp.TimeFromTime64(c.prev.sRxTime)
			t2 = sTxTime
			t3 = ntp.TimeFromTime64(c.prev.cRxTime)
		} else {
			t0 = cTxTime1
			t1 = sRxTime
			t2 = sTxTime
			t3 = cRxTime
		}

		err = ntp.ValidateResponseTimestamps(t0, t1, t1, t3)
		if err != nil {
			return at, offset, weight, err
		}

		off := ntp.ClockOffset(t0, t1, t2, t3)
		rtd := ntp.RoundTripDelay(t0, t1, t2, t3)

		mtrcs.respsAccepted.Inc()
		if interleavedResp {
			mtrcs.respsAcceptedInterleaved.Inc()
		}
		log.Debug("evaluated response",
			zap.Time("at", cRxTime),
			zap.String("from", reference),
			zap.Bool("interleaved", interleavedResp),
			zap.Duration("clock offset", off),
			zap.Duration("round trip delay", rtd),
		)

		if c.InterleavedMode {
			c.prev.reference = reference
			c.prev.interleaved = interleavedResp
			c.prev.cTxTime = ntp.Time64FromTime(cTxTime1)
			c.prev.cRxTime = ntp.Time64FromTime(cRxTime)
			c.prev.sRxTime = ntpresp.ReceiveTime
		}

		at = cRxTime
		if c.Raw {
			offset, weight = off, 1000.0
		} else {
			offset, weight = filter(log, reference, c.Lclk, t0, t1, t2, t3)
		}

		if c.Histo != nil {
			c.Histo.RecordValue(rtd.Microseconds())
		}

		break
	}

	return at, offset, weight, nil
}
