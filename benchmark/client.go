package benchmark

import (
	"context"
	"log/slog"
	"os"
)

func logFatal(ctx context.Context, log *slog.Logger, msg string, attrs ...slog.Attr) {
	log.LogAttrs(ctx, slog.LevelError, msg, attrs...)
	os.Exit(1)
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
