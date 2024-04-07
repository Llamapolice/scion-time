package zaplog

import (
	"sync/atomic"

	"go.uber.org/zap"
)

var logger atomic.Pointer[zap.Logger]

func Logger() *zap.Logger { return logger.Load() }

func SetLogger(l *zap.Logger) { logger.Store(l) }
