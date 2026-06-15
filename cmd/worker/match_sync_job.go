package main

import (
	"context"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
)

// matchSyncState carries the mutable bookkeeping shared across scheduler ticks.
// Using atomic operations avoids a mutex on a hot path that runs every 30 s.
type matchSyncState struct {
	lastLiveCount        atomic.Int32 // live count observed on the most recent poll
	lastPollUnixSec      atomic.Int64 // Unix time of the most recent successful provider call
	consecutiveZeroCount atomic.Int32 // consecutive ticks with zero live matches
	pollingPaused        atomic.Bool  // true when polling is suspended between matches
	resumeAtUnix         atomic.Int64 // Unix time at which polling should resume
}

// globalMatchSyncState is the process-wide instance. Declared at package level
// so it survives across scheduler ticks without threading through context.
var globalMatchSyncState matchSyncState

// makeMatchSyncJob returns the scheduler job function for automated match-result
// ingestion. The job is registered at the fast poll interval (default 30 s) and
// applies the following adaptive logic on each tick:
//
//  1. If match.sync.enabled is false, return immediately (no-op).
//  2. If polling is paused (all matches finished), skip until now >= resumeAtUnix.
//     When that threshold is reached, resume and reset the pause state.
//  3. If no live matches were observed on the last tick AND fewer than
//     match.sync.slow_poll_interval_sec have elapsed since the last poll,
//     skip the provider call to stay within rate limits.
//  4. Otherwise call PollAndApply, record the live count and timestamp.
//  5. After match.sync.stop_after_zero_live_count consecutive zero-live ticks,
//     suspend polling. Query the DB for the next scheduled match kickoff and
//     set resumeAtUnix to (nextKickoff - prematchWindowMin minutes).
func makeMatchSyncJob(
	params service.SystemParamService,
	syncSvc service.MatchSyncer,
	matchRepo repository.MatchRepository,
	log *zap.Logger,
) func(context.Context) error {
	return func(ctx context.Context) error {
		if !params.GetBool(ctx, domain.ParamKeyMatchSyncEnabled, domain.DefaultMatchSyncEnabled) {
			return nil
		}

		prematchWindow := params.GetInt(ctx, domain.ParamKeyMatchSyncPrematchWindowMin, domain.DefaultMatchSyncPrematchWindowMin)

		if handlePauseResume(ctx, matchRepo, log) {
			return nil
		}

		// Adaptive interval: skip the API call when no matches were live on the
		// last tick and we have not yet reached the slow poll threshold.
		slowSec := params.GetInt(ctx, domain.ParamKeyMatchSyncSlowPollIntervalSec, domain.DefaultMatchSyncSlowPollIntervalSec)
		lastLive := globalMatchSyncState.lastLiveCount.Load()
		lastPoll := globalMatchSyncState.lastPollUnixSec.Load()
		elapsed := time.Now().Unix() - lastPoll

		if lastLive == 0 && lastPoll > 0 && elapsed < int64(slowSec) {
			return nil
		}

		liveCount, err := syncSvc.PollAndApply(ctx, prematchWindow)
		if err != nil {
			log.Warn("match sync: PollAndApply failed",
				zap.Error(err),
				zap.Int64("elapsed_since_last_poll_sec", elapsed),
			)
			return nil
		}

		globalMatchSyncState.lastLiveCount.Store(int32(liveCount))
		globalMatchSyncState.lastPollUnixSec.Store(time.Now().Unix())

		if liveCount > 0 {
			globalMatchSyncState.consecutiveZeroCount.Store(0)
			return nil
		}

		suspendPollingIfThresholdReached(ctx, params, matchRepo, prematchWindow, log)
		return nil
	}
}

// resetPauseState clears the three pause-related atomics. Called on normal
// timer expiry and on early-resume when an overdue linked match is detected.
func resetPauseState() {
	globalMatchSyncState.pollingPaused.Store(false)
	globalMatchSyncState.consecutiveZeroCount.Store(0)
	globalMatchSyncState.resumeAtUnix.Store(0)
}

// handlePauseResume checks the global pause state on each tick.
// Returns true when the tick should be skipped entirely.
//
// When paused but the resume timer has not yet expired, a lightweight DB check
// is performed for overdue linked matches — i.e. matches whose kickoff has
// already passed but whose status is still 'scheduled'. This handles the race
// condition where external_match_id is set (via a manual admin sync) after the
// pause began: the overdue check detects the newly-linked match and resumes
// immediately rather than waiting for the full pause window to elapse.
func handlePauseResume(ctx context.Context, matchRepo repository.MatchRepository, log *zap.Logger) bool {
	if !globalMatchSyncState.pollingPaused.Load() {
		return false
	}
	resumeAt := globalMatchSyncState.resumeAtUnix.Load()
	if time.Now().Unix() >= resumeAt {
		resetPauseState()
		log.Info("match sync: polling resumed — approaching next scheduled match")
		return false
	}
	// Timer has not yet expired. Check for matches linked since the pause began
	// that are already past their kickoff time.
	_, _, hasOverdue := nextScheduledMatchKickoff(ctx, matchRepo)
	if hasOverdue {
		resetPauseState()
		log.Info("match sync: polling resumed early — overdue linked match detected")
		return false
	}
	return true
}

// suspendPollingIfThresholdReached accumulates consecutive zero-live ticks and,
// once the configured threshold is reached, pauses polling until the pre-match
// window of the next scheduled match.
//
// If any linked scheduled match already has a kickoff in the past (overdue),
// the counter is reset without pausing: PollAndApply should be retried on the
// next tick to catch the status transition rather than sleeping through it.
func suspendPollingIfThresholdReached(
	ctx context.Context,
	params service.SystemParamService,
	matchRepo repository.MatchRepository,
	prematchWindow int,
	log *zap.Logger,
) {
	stopThreshold := int32(params.GetInt(ctx, domain.ParamKeyMatchSyncStopAfterZeroLiveCount, domain.DefaultMatchSyncStopAfterZeroLiveCount))
	if globalMatchSyncState.consecutiveZeroCount.Add(1) < stopThreshold {
		return
	}

	nextKickoff, found, hasOverdue := nextScheduledMatchKickoff(ctx, matchRepo)
	if hasOverdue {
		// A linked match whose kickoff has already passed is waiting to be
		// transitioned to 'live'. Reset the counter so polling continues.
		globalMatchSyncState.consecutiveZeroCount.Store(0)
		log.Info("match sync: overdue linked match detected — not pausing")
		return
	}
	if !found {
		globalMatchSyncState.pollingPaused.Store(true)
		// Re-check in 5 minutes instead of pausing indefinitely. This ensures
		// that a match linked manually after the pause (setting external_match_id
		// in the DB) is picked up within one re-check cycle rather than waiting
		// for the daily sync or an operator restart.
		globalMatchSyncState.resumeAtUnix.Store(time.Now().Add(5 * time.Minute).Unix())
		log.Info("match sync: no upcoming matches found — polling suspended for 5 min")
		return
	}

	resumeAt := nextKickoff.Unix() - int64(prematchWindow*60)
	if resumeAt <= time.Now().Unix() {
		// Already within the pre-match window — do not pause.
		return
	}
	globalMatchSyncState.pollingPaused.Store(true)
	globalMatchSyncState.resumeAtUnix.Store(resumeAt)
	log.Info("match sync: all matches finished — polling suspended until pre-match window",
		zap.Time("next_kickoff", nextKickoff),
		zap.Time("resume_at", time.Unix(resumeAt, 0).UTC()),
	)
}

// nextScheduledMatchKickoff scans linked sync candidates and returns:
//   - next: the earliest future kickoff among scheduled matches (zero if none)
//   - found: whether any future scheduled match exists
//   - hasOverdue: whether any scheduled match has a kickoff already in the past
//
// hasOverdue is the key signal for both suspendPollingIfThresholdReached (do
// not pause) and handlePauseResume (resume early): it indicates that a linked
// match is waiting to be transitioned but has not yet been seen as live.
func nextScheduledMatchKickoff(ctx context.Context, matchRepo repository.MatchRepository) (next time.Time, found bool, hasOverdue bool) {
	candidates, err := matchRepo.ListSyncCandidates(ctx, 0)
	if err != nil {
		return time.Time{}, false, false
	}
	now := time.Now()
	for _, m := range candidates {
		if m.Status != domain.MatchStatusScheduled {
			continue
		}
		if !m.KickoffAt.After(now) {
			hasOverdue = true
			continue
		}
		if next.IsZero() || m.KickoffAt.Before(next) {
			next = m.KickoffAt
		}
	}
	return next, !next.IsZero(), hasOverdue
}
