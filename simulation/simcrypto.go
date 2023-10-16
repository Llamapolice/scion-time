package simulation

import (
	"context"

	"example.com/scion-time/base/cryptobase"
)

type SimulationCrypto struct {
}

func (s *SimulationCrypto) RandIntn(ctx context.Context, n int) (int, error) {
	//TODO implement me
	panic("implement me")
}

func (s *SimulationCrypto) Sample(ctx context.Context, k, n int, pick func(dst int, src int)) (int, error) {
	//TODO implement me
	panic("implement me")
}

var _ cryptobase.CryptoProvider = (*SimulationCrypto)(nil)
