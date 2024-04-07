package crypto

// Random numbers with a given upper bound and reservoir sampling using a
// cryptographically secure random number generator based on
// Daniel Lemire, Fast Random Integer Generation in an Interval
// ACM Transactions on Modeling and Computer Simulation 29 (1), 2019
// https://lemire.me/en/publication/arxiv1805/

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"log/slog"
	"math"

	"example.com/scion-time/base/cryptobase"
)

// SafeCrypto provides random number generation and utilities based on crypto/rand, safe to be used in prod
type SafeCrypto struct {
	Log *slog.Logger
}

// RandIntn enables the use of the SafeCrypto struct as an "inheritor" of [cryptobase.CryptoProvider]
func (s *SafeCrypto) RandIntn(ctx context.Context, n int) (int, error) {
	return RandIntn(ctx, n)
}

// Sample enables the use of the SafeCrypto struct as an "inheritor" of [cryptobase.CryptoProvider]
func (s *SafeCrypto) Sample(ctx context.Context, k, n int, pick func(dst int, src int)) (int, error) {
	return Sample(ctx, k, n, pick)
}

var _ cryptobase.CryptoProvider = (*SafeCrypto)(nil)

func randInt31(ctx context.Context, n int) (int, error) {
	if n < 2 {
		return 0, nil
	}
	if n > math.MaxInt32 {
		panic("invalid argument to randInt31: n must not be greater than 2147483647")
	}
	t := uint32(-n) % uint32(n)
	b := make([]byte, 4)
	var x uint32
	for {
		n, err := rand.Read(b)
		if err != nil {
			return 0, err
		}
		if n != len(b) {
			panic("unexpected result from random number generator")
		}
		x = binary.LittleEndian.Uint32(b)
		if x > t {
			break
		}
		err = ctx.Err()
		if err != nil {
			return 0, err
		}
	}
	return int(x % uint32(n)), nil
}

func randInt63(ctx context.Context, n int) (int, error) {
	if n < 2 {
		return 0, nil
	}
	t := uint64(-n) % uint64(n)
	b := make([]byte, 8)
	var x uint64
	for {
		n, err := rand.Read(b)
		if err != nil {
			return 0, err
		}
		if n != len(b) {
			panic("unexpected result from random number generator")
		}
		x = binary.LittleEndian.Uint64(b)
		if x > t {
			break
		}
		err = ctx.Err()
		if err != nil {
			return 0, err
		}
	}
	return int(x % uint64(n)), nil
}

func RandIntn(ctx context.Context, n int) (int, error) {
	if n <= 0 {
		panic("invalid argument to RandIntn: n must be greater than 0")
	}
	if n <= math.MaxInt32 {
		return randInt31(ctx, n)
	}
	return randInt63(ctx, n)
}

func Sample(ctx context.Context, k, n int, pick func(dst, src int)) (int, error) {
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
		j, err := RandIntn(ctx, i+1)
		if err != nil {
			return 0, err
		}
		if j < k {
			pick(j, i)
		}
	}
	return k, nil
}
