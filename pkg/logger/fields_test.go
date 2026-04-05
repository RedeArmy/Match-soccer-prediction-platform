package logger_test

import (
	"testing"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/pkg/logger"
)

// Each field constructor is verified against the equivalent zap primitive so
// that renames of canonical key names are caught immediately by the test suite.

func TestUserID_ReturnsCorrectField(t *testing.T) {
	if got := logger.UserID("u_123"); got != zap.String("user_id", "u_123") {
		t.Errorf("UserID: got %v", got)
	}
}

func TestMatchID_ReturnsCorrectField(t *testing.T) {
	if got := logger.MatchID(42); got != zap.Int("match_id", 42) {
		t.Errorf("MatchID: got %v", got)
	}
}

func TestQuinielaID_ReturnsCorrectField(t *testing.T) {
	if got := logger.QuinielaID(7); got != zap.Int("quiniela_id", 7) {
		t.Errorf("QuinielaID: got %v", got)
	}
}

func TestPredictionID_ReturnsCorrectField(t *testing.T) {
	if got := logger.PredictionID(99); got != zap.Int("prediction_id", 99) {
		t.Errorf("PredictionID: got %v", got)
	}
}

func TestRequestID_ReturnsCorrectField(t *testing.T) {
	if got := logger.RequestID("req-abc"); got != zap.String("request_id", "req-abc") {
		t.Errorf("RequestID: got %v", got)
	}
}

func TestEventType_ReturnsCorrectField(t *testing.T) {
	if got := logger.EventType("match.finished"); got != zap.String("event_type", "match.finished") {
		t.Errorf("EventType: got %v", got)
	}
}

func TestLatencyMS_ReturnsCorrectField(t *testing.T) {
	if got := logger.LatencyMS(150); got != zap.Int64("latency_ms", 150) {
		t.Errorf("LatencyMS: got %v", got)
	}
}
