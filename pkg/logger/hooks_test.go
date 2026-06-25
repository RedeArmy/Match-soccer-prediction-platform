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

func TestWithLevelCounters_WarnLevel_CallsOnWarn(t *testing.T) {
	l, err := logger.New(logger.Config{Level: "debug", Encoding: "json"})
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	var warns, errs int
	hooked := logger.WithLevelCounters(l, func() { warns++ }, func() { errs++ })
	hooked.Warn("warn entry")
	if warns != 1 {
		t.Errorf("onWarn: expected 1 call, got %d", warns)
	}
	if errs != 0 {
		t.Errorf("onError: expected 0 calls on Warn, got %d", errs)
	}
}

func TestWithLevelCounters_ErrorLevel_CallsOnError(t *testing.T) {
	l, err := logger.New(logger.Config{Level: "debug", Encoding: "json"})
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	var warns, errs int
	hooked := logger.WithLevelCounters(l, func() { warns++ }, func() { errs++ })
	hooked.Error("error entry")
	if errs != 1 {
		t.Errorf("onError: expected 1 call, got %d", errs)
	}
	if warns != 0 {
		t.Errorf("onWarn: expected 0 calls on Error, got %d", warns)
	}
}

func TestWithLevelCounters_InfoLevel_CallsNeither(t *testing.T) {
	l, err := logger.New(logger.Config{Level: "debug", Encoding: "json"})
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	var warns, errs int
	hooked := logger.WithLevelCounters(l, func() { warns++ }, func() { errs++ })
	hooked.Info("info entry")
	if warns != 0 || errs != 0 {
		t.Errorf("expected no counters for Info, got warns=%d errs=%d", warns, errs)
	}
}
