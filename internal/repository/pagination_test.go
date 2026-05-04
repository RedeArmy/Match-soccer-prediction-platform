package repository_test

import (
	"testing"

	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ── Unbounded ─────────────────────────────────────────────────────────────────

func TestUnbounded_ReturnsNegativeLimit(t *testing.T) {
	p := repository.Unbounded()
	if !p.IsUnbounded() {
		t.Error("expected Unbounded() to return unbounded pagination")
	}
	if p.Offset != 0 {
		t.Errorf("expected Offset=0, got %d", p.Offset)
	}
}

func TestUnbounded_IsUnbounded(t *testing.T) {
	p := repository.Unbounded()
	if !p.IsUnbounded() {
		t.Error("expected IsUnbounded() to return true for Unbounded() pagination")
	}
}

// ── IsUnbounded ───────────────────────────────────────────────────────────────

func TestIsUnbounded_PositiveLimit_ReturnsFalse(t *testing.T) {
	p := repository.Pagination{Limit: 50, Offset: 0}
	if p.IsUnbounded() {
		t.Error("expected IsUnbounded() to return false for positive limit")
	}
}

func TestIsUnbounded_ZeroLimit_ReturnsFalse(t *testing.T) {
	// Zero-value Pagination{} is NOT unbounded - it's invalid and will be
	// rejected by repository methods. Only explicit Unbounded() is unbounded.
	p := repository.Pagination{Limit: 0, Offset: 0}
	if p.IsUnbounded() {
		t.Error("expected IsUnbounded() to return false for zero limit")
	}
}
