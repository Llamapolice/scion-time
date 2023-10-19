package simulation

import (
	"context"
	"math/rand"

	"example.com/scion-time/base/cryptobase"
)

type SimCrypto struct {
	seededRand rand.Rand
}

func NewSimCrypto(seed int64) *SimCrypto {
	return &SimCrypto{seededRand: *rand.New(rand.NewSource(seed))}
}

func (s *SimCrypto) RandIntn(ctx context.Context, n int) (int, error) {
	if n <= 0 {
		panic("invalid argument to RandIntn: n must be greater than 0")
	}
	return s.seededRand.Intn(n), nil
}

func (s *SimCrypto) Sample(ctx context.Context, k, n int, pick func(dst int, src int)) (int, error) { // basically copied from base/crypto
	if k < 0 {
		panic("invalid argument to Sample: k must be non-negative")
	}
	if n < 0 {
		panic("invalid argument to Sample: n must be non-negative")
	}
	if n < k {
		k = n
	}
	for i := 0; i != k; i++ {
		pick(i, i)
	}
	for i := k; i != n; i++ {
		j := s.seededRand.Intn(i + 1)
		if j < k {
			pick(j, i)
		}
	}
	return k, nil
}

var _ cryptobase.CryptoProvider = (*SimCrypto)(nil)
