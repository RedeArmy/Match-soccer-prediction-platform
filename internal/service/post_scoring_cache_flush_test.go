package service

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// ── NewPostScoringCacheFlush ──────────────────────────────────────────────────

func TestNewPostScoringCacheFlush_ReturnsNonNil(t *testing.T) {
	if NewPostScoringCacheFlush(newStubCache(), zap.NewNop()) == nil {
		t.Fatal("expected non-nil PostScoringCacheFlush")
	}
}

// ── InvalidateAfterScoring ────────────────────────────────────────────────────

func TestPostScoringCacheFlush_EmptyIDs_IsNoop(t *testing.T) {
	st := newStubCache()
	f := NewPostScoringCacheFlush(st, zap.NewNop())

	f.InvalidateAfterScoring(context.Background(), nil)
	f.InvalidateAfterScoring(context.Background(), []int{})

	if len(st.deleted) != 0 {
		t.Errorf("expected no Delete calls for empty IDs, got %d deleted keys", len(st.deleted))
	}
}

func TestPostScoringCacheFlush_DeletesExpectedKeys(t *testing.T) {
	st := newStubCache()
	f := NewPostScoringCacheFlush(st, zap.NewNop())
	ids := []int{5, 9}
	f.InvalidateAfterScoring(context.Background(), ids)

	wantCount := len(ids) * (1 + len(domain.AllMatchPhases))
	if len(st.deleted) != wantCount {
		t.Fatalf("expected %d keys deleted, got %d: %v", wantCount, len(st.deleted), st.deleted)
	}

	deleted := make(map[string]bool, len(st.deleted))
	for _, k := range st.deleted {
		deleted[k] = true
	}
	for _, id := range ids {
		if !deleted[cacheKeyLeaderboard(id)] {
			t.Errorf("expected overall key for ID %d to be deleted", id)
		}
		for _, phase := range domain.AllMatchPhases {
			k := cacheKeyPhaseLeaderboard(id, phase)
			if !deleted[k] {
				t.Errorf("expected phase key %q to be deleted", k)
			}
		}
	}
}

func TestPostScoringCacheFlush_DeleteError_IsNonFatal(t *testing.T) {
	st := newStubCache()
	st.delErr = errors.New("redis down")
	f := NewPostScoringCacheFlush(st, zap.NewNop())
	f.InvalidateAfterScoring(context.Background(), []int{1}) // must not panic
}

func TestPostScoringCacheFlush_WithPrefixFlusher_CallsFlushByPrefix(t *testing.T) {
	pf := &spyPrefixFlusher{}
	f := NewPostScoringCacheFlush(pf, zap.NewNop())
	f.InvalidateAfterScoring(context.Background(), []int{3})

	if len(pf.prefixes) != 1 || pf.prefixes[0] != "global_leaderboard:" {
		t.Errorf("expected FlushByPrefix(%q), got %v", "global_leaderboard:", pf.prefixes)
	}
}

func TestPostScoringCacheFlush_NonPrefixFlusherStore_SkipsGlobalFlush(t *testing.T) {
	// stubCacheStore does not implement PrefixFlusher; function must not panic.
	f := NewPostScoringCacheFlush(newStubCache(), zap.NewNop())
	f.InvalidateAfterScoring(context.Background(), []int{7})
}

func TestPostScoringCacheFlush_PrefixFlushError_IsNonFatal(t *testing.T) {
	pf := &spyPrefixFlusher{err: errors.New("redis error")}
	f := NewPostScoringCacheFlush(pf, zap.NewNop())
	f.InvalidateAfterScoring(context.Background(), []int{2}) // must not panic
}
