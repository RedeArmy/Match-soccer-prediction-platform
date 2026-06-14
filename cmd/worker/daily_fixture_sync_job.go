package main

import (
	"context"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/observability"
	"github.com/rede/world-cup-quiniela/internal/service"
)

// dailyFixtureSyncState tracks whether today's sync has already succeeded.
// Using an atomic avoids a mutex on a path that fires once per calendar day.
type dailyFixtureSyncState struct {
	lastSuccessDate atomic.Value // string "YYYY-MM-DD" in scheduler timezone
}

// globalDailyFixtureSyncState is the process-wide singleton. Package-level so
// it survives across scheduler ticks without threading through context.
var globalDailyFixtureSyncState dailyFixtureSyncState

// makeDailyFixtureSyncJob returns a scheduler job that runs ONCE per calendar
// day (fired by RegisterDaily at match.dailysync.hour in Guatemala time).
// On failure the job logs, fires an n8n alert, and returns without retry.
func makeDailyFixtureSyncJob(
	params service.SystemParamService,
	syncSvc service.MatchSyncer,
	notifier *observability.Notifier,
	log *zap.Logger,
) func(context.Context) error {
	return func(ctx context.Context) error {
		tzName := params.GetString(ctx, domain.ParamKeyNotifySchedulerTimezone, domain.DefaultNotifySchedulerTimezone)
		loc, err := time.LoadLocation(tzName)
		if err != nil {
			loc = time.UTC
		}

		today := time.Now().In(loc).Format("2006-01-02")

		// Idempotency guard: RegisterDaily should fire once per day, but guard
		// in case the scheduler fires twice (e.g. after a clock correction).
		if v, _ := globalDailyFixtureSyncState.lastSuccessDate.Load().(string); v == today {
			return nil
		}

		result, err := syncSvc.DailyFixtureSync(ctx, 1, 2026, nil, nil)
		if err != nil {
			log.Error("match daily sync: failed",
				zap.String("date", today),
				zap.Error(err),
			)
			notifier.NotifyDailyFixtureSyncFailed(ctx, today, err.Error())
			return nil
		}

		globalDailyFixtureSyncState.lastSuccessDate.Store(today)
		log.Info("match daily sync: complete",
			zap.String("date", today),
			zap.Int("total", result.Total),
			zap.Int("kickoffs_updated", result.Updated),
		)
		return nil
	}
}
