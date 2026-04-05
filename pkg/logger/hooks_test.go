package logger_test

import (
	"testing"

	"go.uber.org/zap/zapcore"

	"github.com/rede/world-cup-quiniela/pkg/logger"
)

func TestNewLevelCounter_ReturnsNonNil(t *testing.T) {
	if logger.NewLevelCounter(zapcore.ErrorLevel, func() { /* no-op: only the constructor return value is under test */ }) == nil {
		t.Error("expected non-nil LevelCounter")
	}
}

func TestWithHook_AtOrAboveLevel_CallsFn(t *testing.T) {
	l, err := logger.New(logger.Config{Level: "debug", Encoding: "json"})
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}

	var calls int
	hooked := logger.WithHook(l, zapcore.ErrorLevel, func() { calls++ })
	hooked.Error("triggered")

	if calls != 1 {
		t.Errorf("expected fn called once at error level, got %d", calls)
	}
}

func TestWithHook_BelowLevel_DoesNotCallFn(t *testing.T) {
	l, err := logger.New(logger.Config{Level: "debug", Encoding: "json"})
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}

	var calls int
	hooked := logger.WithHook(l, zapcore.ErrorLevel, func() { calls++ })
	hooked.Info("not triggered") // below error level

	if calls != 0 {
		t.Errorf("expected fn not called for info level, got %d", calls)
	}
}
