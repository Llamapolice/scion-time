package cryptocore

import (
	"context"
	"sync/atomic"

	"example.com/scion-time/base/cryptobase"
)

// TODO: structure copied from timecore

var lcrypt atomic.Value

func RegisterCrypto(c cryptobase.CryptoProvider) {
	if c == nil {
		panic("crypto provider must not be nil")
	}
	if swapped := lcrypt.CompareAndSwap(nil, c); !swapped {
		panic("crypto provider already registered, can only register one")
	}
}

func RandIntn(ctx context.Context, n int) (int, error) {
	return getCrypt().RandIntn(ctx, n)
}

func Sample(ctx context.Context, k, n int, pick func(dst, src int)) (int, error) {
	return getCrypt().Sample(ctx, k, n, pick)
}

func getCrypt() cryptobase.CryptoProvider {
	c := lcrypt.Load().(cryptobase.CryptoProvider)
	if c == nil {
		panic("no crypto provider registered")
	}
	return c
}
