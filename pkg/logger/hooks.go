package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// LevelCounter is a hook that calls inc each time a log entry at or above
// minLevel is written. The intended use is to increment a Prometheus counter
// per level so that unexpected spikes in error or warning rates can trigger
// alerts without requiring log parsing.
//
//	counter := prometheus.NewCounterVec(...)
//	hook := logger.LevelCounter(zapcore.ErrorLevel, func() { counter.WithLabelValues("error").Inc() })
type LevelCounter struct {
	minLevel zapcore.Level
	inc      func()
}

// NewLevelCounter constructs a LevelCounter hook.
func NewLevelCounter(minLevel zapcore.Level, inc func()) *LevelCounter {
	return &LevelCounter{minLevel: minLevel, inc: inc}
}

// WithHook wraps logger with a zap hook that calls fn for every log entry at
// or above level. This keeps the enrichment logic invisible to call sites.
func WithHook(logger *zap.Logger, level zapcore.Level, fn func()) *zap.Logger {
	hook := zap.Hooks(func(entry zapcore.Entry) error {
		if entry.Level >= level {
			fn()
		}
		return nil
	})
	return logger.WithOptions(hook)
}
