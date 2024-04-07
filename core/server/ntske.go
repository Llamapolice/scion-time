package server

import (
	"context"
	"errors"
	"log/slog"
	"net"

	"example.com/scion-time/net/ntske"
)

var errNoCookie = errors.New("failed to add at least one cookie")

func newNTSKEMsg(ctx context.Context, log *slog.Logger,
	localIP net.IP, localPort int, data *ntske.Data, provider *ntske.Provider) (
	ntske.ExchangeMsg, error) {
	var msg ntske.ExchangeMsg
	msg.AddRecord(ntske.NextProto{
		NextProto: ntske.NTPv4,
	})
	msg.AddRecord(ntske.Algorithm{
		Algo: []uint16{ntske.AES_SIV_CMAC_256},
	})
	msg.AddRecord(ntske.Server{
		Addr: []byte(localIP.String()),
	})
	msg.AddRecord(ntske.Port{
		Port: uint16(localPort),
	})

	var plaintextCookie ntske.ServerCookie
	plaintextCookie.Algo = ntske.AES_SIV_CMAC_256
	plaintextCookie.C2S = data.C2sKey
	plaintextCookie.S2C = data.S2cKey
	key := provider.Current()
	addedCookie := false
	for range 8 {
		encryptedCookie, err := plaintextCookie.EncryptWithNonce(key.Value, key.ID)
		if err != nil {
			log.LogAttrs(ctx, slog.LevelInfo, "failed to encrypt cookie", slog.Any("error", err))
			continue
		}

		b := encryptedCookie.Encode()
		msg.AddRecord(ntske.Cookie{
			Cookie: b,
		})
		addedCookie = true
	}
	if !addedCookie {
		return ntske.ExchangeMsg{}, errNoCookie
	}

	msg.AddRecord(ntske.End{})

	return msg, nil
}
