package cryptobase

import "context"

// CryptoProvider defines the functions needed to get random numbers or random samples of a slice
type CryptoProvider interface {
	RandIntn(ctx context.Context, n int) (int, error)
	Sample(ctx context.Context, k, n int, pick func(dst, src int)) (int, error)
}
