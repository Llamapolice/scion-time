package server

import (
	"bufio"
	"context"
	"crypto/tls"
	"log/slog"
	"net"
	"os"
	"strconv"

	"example.com/scion-time/net/ntske"
)

func writeNTSKEErrorMsgTLS(ctx context.Context, log *slog.Logger, conn *tls.Conn, code int) {
	var msg ntske.ExchangeMsg
	msg.AddRecord(ntske.Error{
		Code: uint16(code),
	})

	buf, err := msg.Pack()
	if err != nil {
		log.LogAttrs(ctx, slog.LevelInfo, "failed to build packet", slog.Any("error", err))
		return
	}

	n, err := conn.Write(buf.Bytes())
	if err != nil || n != buf.Len() {
		log.LogAttrs(ctx, slog.LevelInfo, "failed to write error message", slog.Any("error", err))
		return
	}
}

func handleKeyExchangeTLS(ctx context.Context, log *slog.Logger, conn *tls.Conn, localPort int, provider *ntske.Provider) {
	defer conn.Close()

	var err error
	var data ntske.Data
	reader := bufio.NewReader(conn)
	err = ntske.ReadData(ctx, log, reader, &data)
	if err != nil {
		log.LogAttrs(ctx, slog.LevelInfo, "failed to read key exchange", slog.Any("error", err))
		writeNTSKEErrorMsgTLS(ctx, log, conn, ntske.ErrorCodeBadRequest)
		return
	}

	err = ntske.ExportKeys(conn.ConnectionState(), &data)
	if err != nil {
		log.LogAttrs(ctx, slog.LevelInfo, "failed to export keys", slog.Any("error", err))
		writeNTSKEErrorMsgTLS(ctx, log, conn, ntske.ErrorCodeInternalServer)
		return
	}

	localIP := conn.LocalAddr().(*net.TCPAddr).IP

	msg, err := newNTSKEMsg(ctx, log, localIP, localPort, &data, provider)
	if err != nil {
		log.LogAttrs(ctx, slog.LevelInfo, "failed to create packet", slog.Any("error", err))
		writeNTSKEErrorMsgTLS(ctx, log, conn, ntske.ErrorCodeInternalServer)
		return
	}

	buf, err := msg.Pack()
	if err != nil {
		log.LogAttrs(ctx, slog.LevelInfo, "failed to build packet", slog.Any("error", err))
		writeNTSKEErrorMsgTLS(ctx, log, conn, ntske.ErrorCodeInternalServer)
		return
	}

	n, err := conn.Write(buf.Bytes())
	if err != nil || n != buf.Len() {
		log.LogAttrs(ctx, slog.LevelInfo, "failed to write response", slog.Any("error", err))
		return
	}
}

func runNTSKEServerTLS(ctx context.Context, log *slog.Logger,
	listener net.Listener, localPort int, provider *ntske.Provider) {
	defer listener.Close()
	for {
		conn, err := ntske.AcceptTLSConn(listener)
		if err != nil {
			log.LogAttrs(ctx, slog.LevelInfo, "failed to accept client", slog.Any("error", err))
			continue
		}
		go handleKeyExchangeTLS(ctx, log, conn, localPort, provider)
	}
}

func StartNTSKEServerIP(ctx context.Context, log *slog.Logger, localIP net.IP, localPort int, config *tls.Config, provider *ntske.Provider) {
	ntskeAddr := net.JoinHostPort(localIP.String(), strconv.Itoa(ntske.ServerPortIP))
	log.LogAttrs(ctx, slog.LevelInfo,
		"server listening via IP",
		slog.Any("ip", localIP),
		slog.Int64("port", int64(ntske.ServerPortIP)),
	)

	listener, err := tls.Listen("tcp", ntskeAddr, config)
	if err != nil {
		log.LogAttrs(ctx, slog.LevelError, "failed to create TLS listener")
		os.Exit(1)
	}

	go runNTSKEServerTLS(ctx, log, listener, localPort, provider)
}
