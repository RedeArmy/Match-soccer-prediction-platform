// Package logger_test exercises the public surface of pkg/logger.
//
// The logger package is a thin factory over zap. Tests here verify that
// construction succeeds with valid configuration and fails with a descriptive
// error for invalid configuration. zap's internal encoding and level
// behaviour is intentionally not tested — that is the responsibility of the
// zap library's own test suite, not ours.
package logger_test

import (
	"strings"
	"testing"

	"github.com/rede/world-cup-quiniela/pkg/logger"
)

func TestNew_JSONEncoding_ReturnsLogger(t *testing.T) {
	l, err := logger.New(logger.Config{Level: "info", Encoding: "json"})
	if err != nil {
		t.Fatalf("expected no error for json encoding, got: %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil logger, got nil")
	}
}

func TestNew_ConsoleEncoding_ReturnsLogger(t *testing.T) {
	l, err := logger.New(logger.Config{Level: "info", Encoding: "console"})
	if err != nil {
		t.Fatalf("expected no error for console encoding, got: %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil logger, got nil")
	}
}

func TestNew_InvalidLevel_ReturnsError(t *testing.T) {
	_, err := logger.New(logger.Config{Level: "verbose", Encoding: "json"})
	if err == nil {
		t.Fatal("expected error for invalid log level, got nil")
	}
	if !strings.Contains(err.Error(), "verbose") {
		t.Errorf("expected error to reference the invalid level, got: %v", err)
	}
}

// TestNew_AllValidLevels verifies that every log level accepted by zap can
// be used to construct a logger without error. This guards against future
// refactors that inadvertently tighten the level parsing logic.
func TestNew_AllValidLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error", "dpanic", "panic", "fatal"}

	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			l, err := logger.New(logger.Config{Level: level, Encoding: "json"})
			if err != nil {
				t.Fatalf("level %q: expected no error, got: %v", level, err)
			}
			if l == nil {
				t.Fatalf("level %q: expected non-nil logger, got nil", level)
			}
		})
	}
}
