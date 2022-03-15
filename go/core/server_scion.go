package core

import (
	"log"
	"net"

	"github.com/scionproto/scion/go/lib/addr"
	"github.com/scionproto/scion/go/lib/snet"
	"github.com/scionproto/scion/go/lib/topology/underlay"

	"example.com/scion-time/go/core/timebase"
	"example.com/scion-time/go/net/ntp"
	"example.com/scion-time/go/net/udp"
)

const scionServerLogPrefix = "[core/server_scion]"

func runSCIONServer(conn *net.UDPConn, localHostPort int) {
	defer conn.Close()
	udp.EnableTimestamping(conn)

	var pkt snet.Packet
	var udppkt snet.UDPPayload
	oob := make([]byte, udp.TimestampLen())
	for {
		pkt.Prepare()
		oob = oob[:cap(oob)]

		n, oobn, flags, lastHop, err := conn.ReadMsgUDP(pkt.Bytes, oob)
		if err != nil {
			log.Printf("%s Failed to read packet: %v", scionServerLogPrefix, err)
			continue
		}
		if flags != 0 {
			log.Printf("%s Failed to read packet, flags: %v", scionServerLogPrefix, flags)
			continue
		}

		oob = oob[:oobn]
		rxt, err := udp.TimestampFromOOBData(oob)
		if err != nil {
			log.Printf("%s Failed to read packet timestamp: %v", scionServerLogPrefix, err)
			rxt = timebase.Now()
		}
		pkt.Bytes = pkt.Bytes[:n]

		err = pkt.Decode()
		if err != nil {
			log.Printf("%s Failed to decode packet: %v", err, scionServerLogPrefix)
			continue
		}

		var ok bool
		udppkt, ok = pkt.Payload.(snet.UDPPayload)
		if !ok {
			log.Printf("%s Packet payload is not a UDP packet", scionServerLogPrefix)
			continue
		}

		if int(udppkt.DstPort) != localHostPort {
			m, err := conn.WriteTo(pkt.Bytes, &net.UDPAddr{IP: pkt.Destination.Host.IP(), Port: int(udppkt.DstPort)})
			if err != nil || m != n {
				log.Printf("%s Failed to forward packet: %v, %v\n", scionServerLogPrefix, err, m)
				continue
			}
		} else {
			var ntpreq ntp.Packet
			err = ntp.DecodePacket(&ntpreq, udppkt.Payload)
			if err != nil {
				log.Printf("%s Failed to decode packet payload: %v", scionServerLogPrefix, err)
				continue
			}

			log.Printf("%s Received request at %v: %+v", scionServerLogPrefix, rxt, ntpreq)

			err = validateRequest(&ntpreq, int(udppkt.SrcPort))
			if err != nil {
				log.Printf("%s Unexpected request packet: %v", scionServerLogPrefix, err)
				continue
			}

			var ntpresp ntp.Packet
			handleRequest(&ntpreq, rxt, &ntpresp)

			ntp.EncodePacket(&udppkt.Payload, &ntpresp)
			udppkt.DstPort, udppkt.SrcPort = udppkt.SrcPort, udppkt.DstPort

			pkt.Destination, pkt.Source = pkt.Source, pkt.Destination
			rpath, ok := pkt.Path.(snet.RawPath)
			if !ok {
				log.Printf("%s Failed to reverse path, unecpected path type: %v", scionServerLogPrefix, pkt.Path)
				continue
			}
			replypather := snet.DefaultReplyPather{}
			replyPath, err := replypather.ReplyPath(rpath)
			if err != nil {
				log.Printf("%s Failed to reverse path: %v", scionServerLogPrefix, err)
				continue
			}
			pkt.Path = replyPath
			pkt.Payload = &udppkt
			err = pkt.Serialize()
			if err != nil {
				log.Printf("%s Failed to serialize packet: %v", scionServerLogPrefix, err)
				continue
			}

			n, err = conn.WriteTo(pkt.Bytes, lastHop)
			if err != nil {
				log.Printf("%s Failed to write packet: %v", scionServerLogPrefix, err)
				continue
			}
			if n != len(pkt.Bytes) {
				log.Printf("%s Failed to write entire packet: %v/%v", scionServerLogPrefix, n, len(pkt.Bytes))
				continue
			}
		}
	}
}

func StartSCIONServer(localIA addr.IA, localHost *net.UDPAddr) error {
	log.Printf("%s Listening in %v on %v:%d via SCION", scionServerLogPrefix, localIA, localHost.IP, localHost.Port)

	localHostPort := localHost.Port
	localHost.Port = underlay.EndhostPort

	conn, err := net.ListenUDP("udp", localHost)
	if err != nil {
		log.Fatalf("%s Failed to listen for packets: %v", scionServerLogPrefix, err)
	}

	go runSCIONServer(conn, localHostPort)

	return nil
}
