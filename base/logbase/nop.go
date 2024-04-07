package logbase

import (
	"context"
	"log/slog"
)

type nopHandler struct{}

func (h *nopHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return false
}

func (h *nopHandler) Handle(_ context.Context, _ slog.Record) error {
	return nil
}

func (h *nopHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return h
}

func (h *nopHandler) WithGroup(_ string) slog.Handler {
	return h
}

func NewNopHandler() slog.Handler {
	return &nopHandler{}
}
