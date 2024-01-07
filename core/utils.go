package core

import (
	"bytes"
	"github.com/pelletier/go-toml/v2"
	"go.uber.org/zap"
	"log"
	"os"
)

type SvcConfig struct {
	LocalAddr               string   `toml:"local_address,omitempty"`
	DaemonAddr              string   `toml:"daemon_address,omitempty"`
	RemoteAddr              string   `toml:"remote_address,omitempty"`
	MBGReferenceClocks      []string `toml:"mbg_reference_clocks,omitempty"`
	NTPReferenceClocks      []string `toml:"ntp_reference_clocks,omitempty"`
	SCIONPeers              []string `toml:"scion_peers,omitempty"`
	NTSKECertFile           string   `toml:"ntske_cert_file,omitempty"`
	NTSKEKeyFile            string   `toml:"ntske_key_file,omitempty"`
	NTSKEServerName         string   `toml:"ntske_server_name,omitempty"`
	AuthModes               []string `toml:"auth_modes,omitempty"`
	NTSKEInsecureSkipVerify bool     `toml:"ntske_insecure_skip_verify,omitempty"`
	DSCP                    uint8    `toml:"dscp,omitempty"` // must be in range [0, 63]
}

func LoadConfig[T any](cfgStruct T, configFile string) { // T is pointer to config struct
	raw, err := os.ReadFile(configFile)
	if err != nil {
		log.Fatal("failed to load configuration", zap.Error(err))
	}
	err = toml.NewDecoder(bytes.NewReader(raw)).DisallowUnknownFields().Decode(cfgStruct)
	if err != nil {
		log.Fatal("failed to decode configuration", zap.Error(err))
	}
}
