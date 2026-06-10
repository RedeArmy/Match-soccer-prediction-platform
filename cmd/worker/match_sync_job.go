package main

import (
	"context"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/service"
)

// matchSyncState carries the mutable bookkeeping shared across scheduler ticks.
// Using atomic operations avoids a mutex on a hot path that runs every 30 s.
type matchSyncState struct {
	lastLiveCount   atomic.Int32 // live count observed on the most recent poll
	lastPollUnixSec atomic.Int64 // Unix time of the most recent successful provider call
}

// globalMatchSyncState is the process-wide instance. Declared at package level
// so it survives across scheduler ticks without threading through context.
var globalMatchSyncState matchSyncState

// makeMatchSyncJob returns the scheduler job function for automated match-result
// ingestion. The job is registered at the fast poll interval (default 30 s) and
// applies the following adaptive logic on each tick:
//
//  1. If match.sync.enabled is false, return immediately (no-op).
//  2. If no live matches were observed on the last tick AND fewer than
//     match.sync.slow_poll_interval_sec have elapsed since the last poll,
//     skip the provider call to stay within rate limits.
//  3. Otherwise call PollAndApply, record the live count and timestamp.
func makeMatchSyncJob(
	params service.SystemParamService,
	syncSvc service.MatchSyncer,
	log *zap.Logger,
) func(context.Context) error {
	return func(ctx context.Context) error {
		if !params.GetBool(ctx, domain.ParamKeyMatchSyncEnabled, domain.DefaultMatchSyncEnabled) {
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

		liveCount, err := syncSvc.PollAndApply(ctx)
		if err != nil {
			log.Warn("match sync: PollAndApply failed",
				zap.Error(err),
				zap.Int64("elapsed_since_last_poll_sec", elapsed),
			)
			// Return nil so the scheduler does not treat this as a fatal error
			// and continues scheduling the job.
			return nil
		}

		globalMatchSyncState.lastLiveCount.Store(int32(liveCount))
		globalMatchSyncState.lastPollUnixSec.Store(time.Now().Unix())
		return nil
	}
}
