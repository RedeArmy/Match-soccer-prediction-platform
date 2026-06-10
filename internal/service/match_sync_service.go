package service

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
	"github.com/rede/world-cup-quiniela/pkg/footballprovider"
	"github.com/rede/world-cup-quiniela/pkg/tracing"
)

// MatchSyncer orchestrates automated ingestion of match results from an
// external football data provider.
//
// PollAndApply fetches the current state of every linked, non-finished match
// from the external API and applies the appropriate local transition:
//
//   - NS → live  : StartMatch (emits MatchStarted, closes prediction window)
//   - 1H/HT/2H/ET/PEN_LIVE : already live — no action
//   - FT  → finished (normal time)    : UpdateResult with no win method
//   - AET → finished (after extra time): UpdateResult with extra_time
//   - PEN → finished (after penalties) : UpdateResult with penalties
//   - PST/CANC/ABD → logged + skipped (manual handling required)
//
// The caller — the match-sync scheduler job — is responsible for deciding
// which poll interval to use. PollAndApply returns the number of live matches
// observed so the caller can switch between fast (live) and slow (idle)
// polling intervals without a separate GetLiveFixtures call.
type MatchSyncer interface {
	// LinkExternal associates a match with an external provider fixture ID.
	// matchID is the internal primary key; externalID is the provider's fixture ID.
	// provider must be "api-football" for the only supported implementation.
	LinkExternal(ctx context.Context, matchID int, provider string, externalID int64) error

	// UnlinkExternal removes the external association, reverting to manual management.
	UnlinkExternal(ctx context.Context, matchID int) error

	// PollAndApply fetches all linked candidates from the provider and applies
	// any status/result transitions. Returns the count of matches that were
	// observed as live (includes HT) so the caller can tune poll frequency.
	PollAndApply(ctx context.Context) (liveCount int, err error)

	// ReconcileDate compares internal match state against the provider for all
	// matches scheduled on a given UTC date and returns a diff report. No writes
	// are performed; the caller decides whether to apply corrections.
	ReconcileDate(ctx context.Context, leagueID, season int) ([]SyncDiff, error)
}

// SyncDiff describes a discrepancy between the internal match state and
// the external provider's observation. Returned by ReconcileDate.
type SyncDiff struct {
	MatchID         int
	InternalStatus  domain.MatchStatus
	ExternalStatus  footballprovider.FixtureStatus
	InternalHome    *int
	InternalAway    *int
	ExternalHome    int
	ExternalAway    int
}

// matchSyncService is the concrete implementation of MatchSyncer.
type matchSyncService struct {
	matchRepo repository.MatchRepository
	matchSvc  MatchService
	provider  footballprovider.Client
	log       *zap.Logger
}

// NewMatchSyncService constructs a matchSyncService.
// provider may be nil when the API key is absent; PollAndApply returns an
// error immediately in that case so the caller can log and skip the tick.
func NewMatchSyncService(
	matchRepo repository.MatchRepository,
	matchSvc MatchService,
	provider footballprovider.Client,
	log *zap.Logger,
) MatchSyncer {
	return &matchSyncService{
		matchRepo: matchRepo,
		matchSvc:  matchSvc,
		provider:  provider,
		log:       log,
	}
}

func (s *matchSyncService) LinkExternal(ctx context.Context, matchID int, provider string, externalID int64) error {
	if provider != "api-football" {
		return apperrors.Validation(fmt.Sprintf("unsupported provider %q: only 'api-football' is accepted", provider))
	}
	if externalID <= 0 {
		return apperrors.Validation("externalID must be a positive integer")
	}
	return s.matchRepo.LinkExternal(ctx, matchID, provider, externalID)
}

func (s *matchSyncService) UnlinkExternal(ctx context.Context, matchID int) error {
	return s.matchRepo.UnlinkExternal(ctx, matchID)
}

// PollAndApply fetches the current state of every linked, non-finished match
// from the external provider and transitions local state accordingly.
// It is designed to be called on a tight interval (30 s) during live matches
// and returns quickly when no candidates exist.
func (s *matchSyncService) PollAndApply(ctx context.Context) (int, error) {
	ctx, span := otel.Tracer("match_sync").Start(ctx, "match_sync.poll_and_apply")
	defer span.End()

	if s.provider == nil {
		return 0, fmt.Errorf("match sync: no provider configured (missing API key)")
	}

	candidates, err := s.matchRepo.ListSyncCandidates(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list candidates failed")
		return 0, fmt.Errorf("match sync: list candidates: %w", err)
	}
	if len(candidates) == 0 {
		span.SetAttributes(attribute.Int("candidates", 0))
		return 0, nil
	}

	span.SetAttributes(attribute.Int("candidates", len(candidates)))

	liveCount := 0
	applied := 0
	for _, m := range candidates {
		live, didApply, err := s.processOne(ctx, m)
		if err != nil {
			s.log.Warn("match sync: processOne failed",
				append([]zap.Field{
					zap.Int("match_id", m.ID),
					zap.Int64p("external_match_id", m.ExternalMatchID),
					zap.Error(err),
				}, tracing.LogFields(ctx)...)...,
			)
			continue
		}
		if live {
			liveCount++
		}
		if didApply {
			applied++
		}
		if err := s.matchRepo.UpdateSyncState(ctx, m.ID); err != nil {
			s.log.Warn("match sync: UpdateSyncState failed",
				append([]zap.Field{zap.Int("match_id", m.ID), zap.Error(err)},
					tracing.LogFields(ctx)...)...,
			)
		}
	}

	span.SetAttributes(
		attribute.Int("live_count", liveCount),
		attribute.Int("applied", applied),
	)
	s.log.Info("match sync: poll cycle complete",
		zap.Int("candidates", len(candidates)),
		zap.Int("live", liveCount),
		zap.Int("applied", applied),
	)
	return liveCount, nil
}

// processOne fetches the provider state for a single match and applies any
// required transition. Returns (isLive, transitionApplied, error).
func (s *matchSyncService) processOne(ctx context.Context, m *domain.Match) (bool, bool, error) {
	if m.ExternalMatchID == nil {
		return false, false, nil
	}

	fix, err := s.provider.GetFixture(ctx, *m.ExternalMatchID)
	if err != nil {
		return false, false, fmt.Errorf("GetFixture: %w", err)
	}

	isLive := fix.Status.IsLive()

	switch {
	case fix.Status == footballprovider.StatusNotStarted && m.Status == domain.MatchStatusScheduled:
		// Still waiting for kickoff — nothing to do.
		return false, false, nil

	case fix.Status.IsLive() && m.Status == domain.MatchStatusScheduled:
		// Kickoff detected: transition to live.
		if _, err := s.matchSvc.StartMatch(ctx, m.ID); err != nil {
			// If the match was already started concurrently (e.g. manual admin
			// action), the error is Validation("only started from scheduled").
			// Treat this as a non-fatal idempotency case.
			if errors.Is(err, apperrors.ErrValidation) {
				s.log.Info("match sync: StartMatch already done (idempotent)",
					zap.Int("match_id", m.ID))
				return true, false, nil
			}
			return isLive, false, fmt.Errorf("StartMatch: %w", err)
		}
		s.log.Info("match sync: started match", zap.Int("match_id", m.ID))
		return true, true, nil

	case fix.Status.IsLive() && m.Status == domain.MatchStatusLive:
		// Already live — no state change required.
		return true, false, nil

	case fix.Status.IsFinished() && m.Status != domain.MatchStatusFinished:
		// Match ended: record the result.
		winMethod := winMethodFromStatus(fix.Status)
		if _, err := s.matchSvc.UpdateResult(ctx, m.ID, fix.HomeScore, fix.AwayScore, winMethod); err != nil {
			if errors.Is(err, apperrors.ErrValidation) {
				// Match may have been finished manually already.
				s.log.Info("match sync: UpdateResult already done (idempotent)",
					zap.Int("match_id", m.ID))
				return false, false, nil
			}
			return false, false, fmt.Errorf("UpdateResult: %w", err)
		}
		s.log.Info("match sync: result recorded",
			zap.Int("match_id", m.ID),
			zap.Int("home", fix.HomeScore),
			zap.Int("away", fix.AwayScore),
			zap.String("status", string(fix.Status)),
		)
		return false, true, nil

	case fix.Status.IsCancelled():
		s.log.Warn("match sync: fixture cancelled/postponed — manual action required",
			zap.Int("match_id", m.ID),
			zap.Int64("external_id", *m.ExternalMatchID),
			zap.String("provider_status", string(fix.Status)),
		)
		return false, false, nil

	default:
		// No applicable transition (e.g. already finished, unknown status).
		return false, false, nil
	}
}

// ReconcileDate lists all sync candidates and compares their local state
// against the provider to surface divergences. No mutations are performed.
func (s *matchSyncService) ReconcileDate(ctx context.Context, leagueID, season int) ([]SyncDiff, error) {
	if s.provider == nil {
		return nil, fmt.Errorf("match sync: no provider configured")
	}
	candidates, err := s.matchRepo.ListSyncCandidates(ctx)
	if err != nil {
		return nil, fmt.Errorf("match sync: list candidates: %w", err)
	}

	var diffs []SyncDiff
	for _, m := range candidates {
		if m.ExternalMatchID == nil {
			continue
		}
		fix, err := s.provider.GetFixture(ctx, *m.ExternalMatchID)
		if err != nil {
			s.log.Warn("match sync: reconcile GetFixture failed",
				zap.Int("match_id", m.ID), zap.Error(err))
			continue
		}
		// A divergence is any mismatch in status or score.
		localFinished := m.Status == domain.MatchStatusFinished
		providerFinished := fix.Status.IsFinished()
		scoreDiverges := m.HomeScore != nil && m.AwayScore != nil &&
			(*m.HomeScore != fix.HomeScore || *m.AwayScore != fix.AwayScore)
		if (localFinished != providerFinished) || scoreDiverges {
			diffs = append(diffs, SyncDiff{
				MatchID:        m.ID,
				InternalStatus: m.Status,
				ExternalStatus: fix.Status,
				InternalHome:   m.HomeScore,
				InternalAway:   m.AwayScore,
				ExternalHome:   fix.HomeScore,
				ExternalAway:   fix.AwayScore,
			})
		}
	}
	return diffs, nil
}

// winMethodFromStatus maps the terminal provider status to the domain WinMethod.
// Returns nil for FT (normal time result requires no win method annotation).
func winMethodFromStatus(s footballprovider.FixtureStatus) *domain.WinMethod {
	switch s {
	case footballprovider.StatusAfterET:
		wm := domain.WinMethodExtraTime
		return &wm
	case footballprovider.StatusAfterPEN:
		wm := domain.WinMethodPenalties
		return &wm
	default:
		return nil
	}
}

var _ MatchSyncer = (*matchSyncService)(nil)
