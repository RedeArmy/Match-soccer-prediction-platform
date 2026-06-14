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

		// When paused, check if we've reached the resume threshold.
		// resumeAtUnix == 0 is the sentinel for "no upcoming matches found at
		// pause time" — keep the pause until the operator adds a match or the
		// daily sync updates kickoffs, then the next pause cycle will recalculate.
		if globalMatchSyncState.pollingPaused.Load() {
			resumeAt := globalMatchSyncState.resumeAtUnix.Load()
			if resumeAt == 0 || time.Now().Unix() < resumeAt {
				return nil
			}
			// Resume: reset pause state.
			globalMatchSyncState.pollingPaused.Store(false)
			globalMatchSyncState.consecutiveZeroCount.Store(0)
			globalMatchSyncState.resumeAtUnix.Store(0)
			log.Info("match sync: polling resumed — approaching next scheduled match")
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

		// Accumulate consecutive zero-live observations.
		stopThreshold := int32(params.GetInt(ctx, domain.ParamKeyMatchSyncStopAfterZeroLiveCount, domain.DefaultMatchSyncStopAfterZeroLiveCount))
		newZeroCount := globalMatchSyncState.consecutiveZeroCount.Add(1)
		if newZeroCount < stopThreshold {
			return nil
		}

		// Threshold reached: pause polling until near the next scheduled match.
		nextKickoff, found := nextScheduledMatchKickoff(ctx, matchRepo)
		if !found {
			// No upcoming matches known — pause indefinitely (slow poll will still
			// fire, but this state will keep returning nil until a new match appears).
			globalMatchSyncState.pollingPaused.Store(true)
			globalMatchSyncState.resumeAtUnix.Store(0)
			log.Info("match sync: no upcoming matches found — polling suspended")
			return nil
		}

		resumeAt := nextKickoff.Unix() - int64(prematchWindow*60)
		if resumeAt <= time.Now().Unix() {
			// Next match is already within the prematch window — do not pause.
			return nil
		}
		globalMatchSyncState.pollingPaused.Store(true)
		globalMatchSyncState.resumeAtUnix.Store(resumeAt)
		log.Info("match sync: all matches finished — polling suspended until pre-match window",
			zap.Time("next_kickoff", nextKickoff),
			zap.Time("resume_at", time.Unix(resumeAt, 0).UTC()),
		)
		return nil
	}
}

// nextScheduledMatchKickoff returns the earliest upcoming linked scheduled match
// kickoff (after now). Returns false when no such match exists.
func nextScheduledMatchKickoff(ctx context.Context, matchRepo repository.MatchRepository) (time.Time, bool) {
	candidates, err := matchRepo.ListSyncCandidates(ctx, 0)
	if err != nil {
		return time.Time{}, false
	}
	now := time.Now()
	var earliest time.Time
	for _, m := range candidates {
		if m.Status != domain.MatchStatusScheduled || !m.KickoffAt.After(now) {
			continue
		}
		if earliest.IsZero() || m.KickoffAt.Before(earliest) {
			earliest = m.KickoffAt
		}
	}
	return earliest, !earliest.IsZero()
}
