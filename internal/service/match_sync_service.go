package service

import (
	"context"
	"errors"
	"fmt"
	"time"

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
//   - 1H/HT/2H/ET/PEN_LIVE : already live — write current score to DB
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

	// PollAndApply fetches linked candidates from the provider and applies any
	// status/result transitions. prematchWindowMin restricts scheduled matches to
	// those within that many minutes of kick-off (0 = no restriction). Returns the
	// count of matches observed as live so the caller can tune poll frequency.
	PollAndApply(ctx context.Context, prematchWindowMin int) (liveCount int, err error)

	// ReconcileDate compares internal match state against the provider for all
	// matches scheduled on a given UTC date and returns a diff report. No writes
	// are performed; the caller decides whether to apply corrections.
	ReconcileDate(ctx context.Context, leagueID, season int) ([]SyncDiff, error)

	// DailyFixtureSync first auto-links any unlinked matches for the given date
	// range by fetching all fixtures from the provider (leagueID, season) and
	// matching them against internal matches by team name. It then validates every
	// linked scheduled/live match: correcting kickoff times and transitioning to
	// finished when the provider reports a terminal status. startDate and endDate,
	// when non-nil, restrict both the auto-link scan and the candidate processing
	// to matches whose kickoff falls within [startDate, endDate].
	DailyFixtureSync(ctx context.Context, leagueID, season int, startDate, endDate *time.Time) (*DailySyncResult, error)
}

// DailySyncResult summarises the outcome of a DailyFixtureSync call.
type DailySyncResult struct {
	Total     int // total fixtures returned by the provider
	Linked    int // newly linked matches (external_match_id was nil)
	Updated   int // kickoff times corrected
	Corrected int // finished-match scores corrected
}

// SyncDiff describes a discrepancy between the internal match state and
// the external provider's observation. Returned by ReconcileDate.
type SyncDiff struct {
	MatchID        int
	InternalStatus domain.MatchStatus
	ExternalStatus footballprovider.FixtureStatus
	InternalHome   *int
	InternalAway   *int
	ExternalHome   int
	ExternalAway   int
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
func (s *matchSyncService) PollAndApply(ctx context.Context, prematchWindowMin int) (int, error) {
	ctx, span := otel.Tracer("match_sync").Start(ctx, "match_sync.poll_and_apply")
	defer span.End()

	if s.provider == nil {
		return 0, fmt.Errorf("match sync: no provider configured (missing API key)")
	}

	candidates, err := s.matchRepo.ListSyncCandidates(ctx, prematchWindowMin)
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
		return false, false, nil
	case fix.Status.IsLive() && m.Status == domain.MatchStatusScheduled:
		return s.applyKickoff(ctx, m.ID, isLive)
	case fix.Status.IsLive() && m.Status == domain.MatchStatusLive:
		if err := s.applyLiveScore(ctx, m, fix); err != nil {
			s.log.Warn("match sync: live score update failed",
				zap.Int("match_id", m.ID), zap.Error(err))
		}
		return true, false, nil
	case fix.Status.IsFinished() && m.Status == domain.MatchStatusScheduled:
		// The live phase was missed entirely. Start the match first so that
		// predictions are locked before the result is written.
		if _, err := s.matchSvc.StartMatch(ctx, m.ID); err != nil && !errors.Is(err, apperrors.ErrValidation) {
			return false, false, fmt.Errorf("StartMatch (missed live phase): %w", err)
		}
		return s.applyResult(ctx, m.ID, fix)
	case fix.Status.IsFinished() && m.Status == domain.MatchStatusLive:
		return s.applyResult(ctx, m.ID, fix)
	case fix.Status.IsCancelled():
		s.log.Warn("match sync: fixture cancelled/postponed — manual action required",
			zap.Int("match_id", m.ID),
			zap.Int64("external_id", *m.ExternalMatchID),
			zap.String("provider_status", string(fix.Status)),
		)
		return false, false, nil
	default:
		return false, false, nil
	}
}

func (s *matchSyncService) applyKickoff(ctx context.Context, matchID int, isLive bool) (bool, bool, error) {
	if _, err := s.matchSvc.StartMatch(ctx, matchID); err != nil {
		if errors.Is(err, apperrors.ErrValidation) {
			s.log.Info("match sync: StartMatch already done (idempotent)", zap.Int("match_id", matchID))
			return true, false, nil
		}
		return isLive, false, fmt.Errorf("StartMatch: %w", err)
	}
	s.log.Info("match sync: started match", zap.Int("match_id", matchID))
	return true, true, nil
}

func (s *matchSyncService) applyLiveScore(ctx context.Context, m *domain.Match, fix *footballprovider.Fixture) error {
	home, away := fix.HomeScore, fix.AwayScore
	m.HomeScore = &home
	m.AwayScore = &away
	period := string(fix.Status)
	m.Period = &period
	m.PenaltyHomeScore = fix.PenaltyHomeScore
	m.PenaltyAwayScore = fix.PenaltyAwayScore
	return s.matchRepo.Update(ctx, m)
}

func (s *matchSyncService) applyResult(ctx context.Context, matchID int, fix *footballprovider.Fixture) (bool, bool, error) {
	winMethod := winMethodFromStatus(fix.Status)
	if _, err := s.matchSvc.UpdateResult(ctx, matchID, fix.HomeScore, fix.AwayScore, winMethod, nil); err != nil {
		if errors.Is(err, apperrors.ErrValidation) {
			s.log.Info("match sync: UpdateResult already done (idempotent)", zap.Int("match_id", matchID))
			return false, false, nil
		}
		return false, false, fmt.Errorf("UpdateResult: %w", err)
	}
	// Clear live period and persist final penalty score without touching the result.
	if err := s.matchRepo.UpdateLiveProgress(ctx, matchID, nil, fix.PenaltyHomeScore, fix.PenaltyAwayScore); err != nil {
		s.log.Warn("match sync: UpdateLiveProgress after result failed",
			zap.Int("match_id", matchID), zap.Error(err))
	}
	s.log.Info("match sync: result recorded",
		zap.Int("match_id", matchID),
		zap.Int("home", fix.HomeScore),
		zap.Int("away", fix.AwayScore),
		zap.String("status", string(fix.Status)),
	)
	return false, true, nil
}

// ReconcileDate lists all sync candidates and compares their local state
// against the provider to surface divergences. No mutations are performed.
func (s *matchSyncService) ReconcileDate(ctx context.Context, leagueID, season int) ([]SyncDiff, error) {
	if s.provider == nil {
		return nil, fmt.Errorf("match sync: no provider configured")
	}
	candidates, err := s.matchRepo.ListSyncCandidates(ctx, 0)
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

// DailyFixtureSync auto-links unlinked matches and validates all linked
// candidates against the provider for the given date range.
func (s *matchSyncService) DailyFixtureSync(ctx context.Context, leagueID, season int, startDate, endDate *time.Time) (*DailySyncResult, error) {
	if s.provider == nil {
		return nil, fmt.Errorf("match sync: no provider configured (missing API key)")
	}

	result := &DailySyncResult{}

	// Phase 1: auto-link unlinked matches by fetching provider fixtures for each
	// date in the range and matching by team name.
	s.autoLinkByDateRange(ctx, leagueID, season, startDate, endDate, result)

	// Phase 2: validate all linked candidates.
	candidates, err := s.matchRepo.ListSyncCandidates(ctx, 0)
	if err != nil {
		return nil, fmt.Errorf("daily fixture sync: list candidates: %w", err)
	}

	candidates = filterByDateRange(candidates, startDate, endDate)
	result.Total = len(candidates)

	for _, m := range candidates {
		s.dailySyncOneMatch(ctx, m, result)
	}

	return result, nil
}

// autoLinkByDateRange fetches provider fixtures for every UTC date in
// [startDate, endDate] and links any internal match not yet associated with
// an external fixture ID. When both pointers are nil the range is
// [today UTC − 30 days, tomorrow UTC] so that any matches the daily job
// missed (e.g. during a deployment gap) are retroactively linked on the next
// manual trigger without the operator having to supply explicit dates. Evening
// fixtures for UTC-6 users — which api-football lists under the next UTC
// calendar day — are captured in the same sync run because the range extends
// one day past today.
func (s *matchSyncService) autoLinkByDateRange(ctx context.Context, leagueID, season int, startDate, endDate *time.Time, result *DailySyncResult) {
	now := time.Now().UTC().Truncate(24 * time.Hour)
	start := now.AddDate(0, 0, -30) // default look-back: 30 days
	if startDate != nil {
		start = startDate.UTC().Truncate(24 * time.Hour)
	}
	end := now.AddDate(0, 0, 1) // default: today + tomorrow UTC
	if endDate != nil {
		end = endDate.UTC().Truncate(24 * time.Hour)
	}

	// alreadyLinked prevents the same internal match from being linked twice
	// when the same fixture appears in both the today and tomorrow scans.
	alreadyLinked := make(map[int]struct{})
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		fixtures, err := s.provider.GetFixturesByDate(ctx, leagueID, season, dateStr)
		if err != nil {
			s.log.Warn("match daily sync: GetFixturesByDate failed",
				zap.String("date", dateStr), zap.Error(err))
			continue
		}
		for _, fix := range fixtures {
			s.tryAutoLink(ctx, fix, result, alreadyLinked)
		}
	}
}

// tryAutoLink attempts to link a single provider fixture to an internal match
// by team name. alreadyLinked tracks matches linked earlier in the same sync
// run so that a fixture appearing on both the today and tomorrow UTC scans
// does not trigger a double-link attempt.
//
// Team-name resolution goes through the team_name_aliases table (see FindByTeams).
// If the primary lookup fails, the home/away pair is retried in reverse order:
// for neutral-venue tournaments the provider may designate the "home" team
// differently from the local DB, and reversing reliably recovers those cases.
func (s *matchSyncService) tryAutoLink(ctx context.Context, fix *footballprovider.Fixture, result *DailySyncResult, alreadyLinked map[int]struct{}) {
	m, err := s.matchRepo.FindByTeams(ctx, fix.HomeTeam, fix.AwayTeam)
	if err != nil {
		s.log.Warn("match daily sync: FindByTeams error (check team_name_aliases migration)",
			zap.Int64("external_id", fix.ExternalID),
			zap.String("provider_home", fix.HomeTeam),
			zap.String("provider_away", fix.AwayTeam),
			zap.Error(err))
		return
	}
	if m == nil {
		// Retry with teams swapped: the provider may assign home/away differently
		// from the local DB for neutral-venue World Cup fixtures.
		m, err = s.matchRepo.FindByTeams(ctx, fix.AwayTeam, fix.HomeTeam)
		if err != nil {
			s.log.Warn("match daily sync: FindByTeams error on swapped retry",
				zap.Int64("external_id", fix.ExternalID),
				zap.String("provider_home", fix.HomeTeam),
				zap.String("provider_away", fix.AwayTeam),
				zap.Error(err))
			return
		}
		if m == nil {
			s.log.Warn("match daily sync: no internal match found for provider fixture — add alias if team name differs",
				zap.Int64("external_id", fix.ExternalID),
				zap.String("provider_home", fix.HomeTeam),
				zap.String("provider_away", fix.AwayTeam))
			return
		}
		s.log.Info("match daily sync: resolved match via swapped home/away",
			zap.String("provider_home", fix.HomeTeam), zap.String("provider_away", fix.AwayTeam))
	}
	if m.ExternalMatchID != nil {
		return // already linked in DB
	}
	if _, seen := alreadyLinked[m.ID]; seen {
		return // linked earlier in this sync run
	}
	if err := s.matchRepo.LinkExternal(ctx, m.ID, "api-football", fix.ExternalID); err != nil {
		s.log.Warn("match daily sync: auto-link failed",
			zap.Int("match_id", m.ID), zap.Int64("external_id", fix.ExternalID), zap.Error(err))
		return
	}
	alreadyLinked[m.ID] = struct{}{}
	result.Linked++
	s.log.Info("match daily sync: linked match",
		zap.Int("match_id", m.ID), zap.Int64("external_id", fix.ExternalID),
		zap.String("home", fix.HomeTeam), zap.String("away", fix.AwayTeam))
}

// filterByDateRange returns only the matches whose kickoff falls within
// [startDate, endDate] (inclusive, day granularity). Returns candidates
// unchanged when both pointers are nil.
func filterByDateRange(candidates []*domain.Match, startDate, endDate *time.Time) []*domain.Match {
	if startDate == nil && endDate == nil {
		return candidates
	}
	kept := candidates[:0]
	for _, m := range candidates {
		if startDate != nil && m.KickoffAt.Before(*startDate) {
			continue
		}
		if endDate != nil && m.KickoffAt.After(*endDate) {
			continue
		}
		kept = append(kept, m)
	}
	return kept
}

func (s *matchSyncService) dailySyncOneMatch(ctx context.Context, m *domain.Match, result *DailySyncResult) {
	fix, err := s.provider.GetFixture(ctx, *m.ExternalMatchID)
	if err != nil {
		s.log.Warn("match daily sync: GetFixture failed",
			zap.Int("match_id", m.ID),
			zap.Int64("external_id", *m.ExternalMatchID),
			zap.Error(err),
		)
		return
	}
	s.maybeCorrectKickoff(ctx, m, fix, result)
	s.maybeTransitionStatus(ctx, m, fix)
}

func (s *matchSyncService) maybeCorrectKickoff(ctx context.Context, m *domain.Match, fix *footballprovider.Fixture, result *DailySyncResult) {
	if m.Status != domain.MatchStatusScheduled || fix.KickoffUTC.IsZero() {
		return
	}
	diff := fix.KickoffUTC.Unix() - m.KickoffAt.Unix()
	if diff < 0 {
		diff = -diff
	}
	if diff <= 60 {
		return
	}
	if err := s.matchRepo.UpdateKickoff(ctx, m.ID, fix.KickoffUTC); err != nil {
		s.log.Warn("match daily sync: UpdateKickoff failed",
			zap.Int("match_id", m.ID), zap.Error(err))
		return
	}
	result.Updated++
}

func (s *matchSyncService) maybeTransitionStatus(ctx context.Context, m *domain.Match, fix *footballprovider.Fixture) {
	if fix.Status.IsFinished() {
		s.applyFinishedTransition(ctx, m, fix)
		return
	}
	if fix.Status.IsLive() && m.Status == domain.MatchStatusScheduled {
		s.applyStartTransition(ctx, m)
	}
}

func (s *matchSyncService) applyFinishedTransition(ctx context.Context, m *domain.Match, fix *footballprovider.Fixture) {
	switch m.Status {
	case domain.MatchStatusLive, domain.MatchStatusScheduled:
		if m.Status == domain.MatchStatusScheduled {
			// Live phase was missed; start the match to lock predictions before
			// writing the result.
			if _, err := s.matchSvc.StartMatch(ctx, m.ID); err != nil && !errors.Is(err, apperrors.ErrValidation) {
				s.log.Warn("match daily sync: StartMatch (missed live phase) failed",
					zap.Int("match_id", m.ID), zap.Error(err))
				return
			}
		}
		winMethod := winMethodFromStatus(fix.Status)
		if _, err := s.matchSvc.UpdateResult(ctx, m.ID, fix.HomeScore, fix.AwayScore, winMethod, nil); err != nil {
			if !errors.Is(err, apperrors.ErrValidation) {
				s.log.Warn("match daily sync: UpdateResult failed",
					zap.Int("match_id", m.ID), zap.Error(err))
			}
		}
		if err := s.matchRepo.UpdateLiveProgress(ctx, m.ID, nil, fix.PenaltyHomeScore, fix.PenaltyAwayScore); err != nil {
			s.log.Warn("match daily sync: UpdateLiveProgress after result failed",
				zap.Int("match_id", m.ID), zap.Error(err))
		}
	case domain.MatchStatusFinished, domain.MatchStatusCancelled:
		// already in a terminal state; no transition needed
	}
}

func (s *matchSyncService) applyStartTransition(ctx context.Context, m *domain.Match) {
	if _, err := s.matchSvc.StartMatch(ctx, m.ID); err != nil {
		if !errors.Is(err, apperrors.ErrValidation) {
			s.log.Warn("match daily sync: StartMatch failed",
				zap.Int("match_id", m.ID), zap.Error(err))
		}
	}
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
