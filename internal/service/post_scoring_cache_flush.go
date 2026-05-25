package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
	"github.com/rede/world-cup-quiniela/pkg/tracing"
)

// PostScoringInvalidator is the contract for any cache layer that holds data
// derived from prediction scores. The scoring worker calls InvalidateAfterScoring
// once per MatchFinished event, after ScoreMatch succeeds, so subsequent reads
// within the same Redis cluster see fresh point totals without waiting for
// natural TTL expiry.
//
// Implementations must be non-fatal: cache errors must be logged and swallowed,
// never returned, because scoring has already committed and a brief period of
// stale cache is preferable to blocking the event pipeline.
type PostScoringInvalidator interface {
	InvalidateAfterScoring(ctx context.Context, quinielaIDs []int)
}

// PostScoringCacheFlush implements PostScoringInvalidator using a cache.Store.
// It is intended for the scoring worker, which runs in a separate process from
// the API server and therefore cannot call the API server's in-process cache
// methods directly.
//
// A single call to InvalidateAfterScoring flushes:
//   - per-quiniela leaderboard keys (overall + all phases) via a batched DEL
//   - the entire "global_leaderboard:" namespace via FlushByPrefix
//
// Both operations are best-effort: errors are logged at Warn level and
// swallowed so that a Redis blip does not fail the event handler.
type PostScoringCacheFlush struct {
	store cache.Store
	log   *zap.Logger
}

// NewPostScoringCacheFlush constructs a PostScoringCacheFlush backed by store.
func NewPostScoringCacheFlush(store cache.Store, log *zap.Logger) *PostScoringCacheFlush {
	return &PostScoringCacheFlush{store: store, log: log}
}

// InvalidateAfterScoring flushes all leaderboard and global leaderboard cache
// entries that may have been populated with pre-scoring point totals.
func (f *PostScoringCacheFlush) InvalidateAfterScoring(ctx context.Context, quinielaIDs []int) {
	if len(quinielaIDs) == 0 {
		return
	}

	// Build the full set of per-quiniela keys (1 overall + N phase keys each).
	keys := make([]string, 0, len(quinielaIDs)*(1+len(domain.AllMatchPhases)))
	for _, id := range quinielaIDs {
		keys = append(keys, cacheKeyLeaderboard(id))
		for _, phase := range domain.AllMatchPhases {
			keys = append(keys, cacheKeyPhaseLeaderboard(id, phase))
		}
	}
	if err := f.store.Delete(ctx, keys...); err != nil {
		f.log.Warn("post-scoring leaderboard cache invalidation failed",
			append([]zap.Field{zap.Error(err)}, tracing.LogFields(ctx)...)...)
	}

	// Flush all global_leaderboard:{limit} variants in one SCAN+DEL pass.
	pf, ok := f.store.(cache.PrefixFlusher)
	if !ok {
		return
	}
	if err := pf.FlushByPrefix(ctx, "global_leaderboard:"); err != nil {
		f.log.Warn("post-scoring global leaderboard cache invalidation failed",
			append([]zap.Field{zap.Error(err)}, tracing.LogFields(ctx)...)...)
	}
}

var _ PostScoringInvalidator = (*PostScoringCacheFlush)(nil)
