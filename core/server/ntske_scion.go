package server

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"os"

	"github.com/quic-go/quic-go"

	"example.com/scion-time/net/ntske"
	"example.com/scion-time/net/scion"
	"example.com/scion-time/net/udp"
)

func writeNTSKEErrorMsgQUIC(ctx context.Context, log *slog.Logger, stream quic.Stream, code int) {
	var msg ntske.ExchangeMsg
	msg.AddRecord(ntske.Error{
		Code: uint16(code),
	})

	buf, err := msg.Pack()
	if err != nil {
		log.LogAttrs(ctx, slog.LevelInfo, "failed to build packet", slog.Any("error", err))
		return
	}

	n, err := stream.Write(buf.Bytes())
	if err != nil || n != buf.Len() {
		log.LogAttrs(ctx, slog.LevelInfo, "failed to write error message", slog.Any("error", err))
		return
	}
}

func handleKeyExchangeQUIC(ctx context.Context, log *slog.Logger,
	conn quic.Connection, localPort int, provider *ntske.Provider) error {
	stream, err := conn.AcceptStream(context.Background())
	if err != nil {
		return err
	}
	defer stream.Close()

	var data ntske.Data
	reader := bufio.NewReader(stream)
	err = ntske.ReadData(ctx, log, reader, &data)
	if err != nil {
		log.LogAttrs(ctx, slog.LevelInfo, "failed to read key exchange", slog.Any("error", err))
		writeNTSKEErrorMsgQUIC(ctx, log, stream, ntske.ErrorCodeBadRequest)
		return err
	}

	err = ntske.ExportKeys(conn.ConnectionState().TLS, &data)
	if err != nil {
		log.LogAttrs(ctx, slog.LevelInfo, "failed to export keys", slog.Any("error", err))
		writeNTSKEErrorMsgQUIC(ctx, log, stream, ntske.ErrorCodeInternalServer)
		return err
	}

	localIP := conn.LocalAddr().(udp.UDPAddr).Host.IP

	msg, err := newNTSKEMsg(ctx, log, localIP, localPort, &data, provider)
	if err != nil {
		log.LogAttrs(ctx, slog.LevelInfo, "failed to create packet", slog.Any("error", err))
		writeNTSKEErrorMsgQUIC(ctx, log, stream, ntske.ErrorCodeInternalServer)
		return err
	}

	buf, err := msg.Pack()
	if err != nil {
		log.LogAttrs(ctx, slog.LevelInfo, "failed to build packet", slog.Any("error", err))
		writeNTSKEErrorMsgQUIC(ctx, log, stream, ntske.ErrorCodeInternalServer)
		return err
	}

	_, err = stream.Write(buf.Bytes())
	if err != nil {
		log.LogAttrs(ctx, slog.LevelInfo, "failed to write response", slog.Any("error", err))
		return err
	}

	return nil
}

func runNTSKEServerQUIC(ctx context.Context, log *slog.Logger,
	listener *scion.QUICListener, localPort int, provider *ntske.Provider) {
	defer listener.Close()
	for {
		conn, err := listener.Accept(ctx)
		if err != nil {
			log.LogAttrs(ctx, slog.LevelInfo, "failed to accept connection", slog.Any("error", err))
			continue
		}

		go func() {
			err := handleKeyExchangeQUIC(ctx, log, conn, localPort, provider)
			var errApplication *quic.ApplicationError
			if err != nil && !(errors.As(err, &errApplication) && errApplication.ErrorCode == 0) {
				log.Info("failed to handle connection",
					slog.Any("remote", conn.RemoteAddr()),
					slog.Any("error", err),
				)
			}
		}()
	}
}

func StartNTSKEServerSCION(ctx context.Context, log *slog.Logger, localAddr udp.UDPAddr, config *tls.Config, provider *ntske.Provider) {
	log.LogAttrs(ctx, slog.LevelInfo,
		"server listening via SCION",
		slog.Any("ip", localAddr.Host.IP),
		slog.Int64("port", int64(ntske.ServerPortSCION)),
	)

	localPort := localAddr.Host.Port
	localAddr.Host.Port = ntske.ServerPortSCION

	listener, err := scion.ListenQUIC(ctx, localAddr, config, nil /* quicCfg */)
	if err != nil {
		log.LogAttrs(ctx, slog.LevelError, "failed to create QUIC listener")
		os.Exit(1)
	}

	go runNTSKEServerQUIC(ctx, log, listener, localPort, provider)
}
