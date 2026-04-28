package handler_test

import (
	"context"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// stubUserRepo implements repository.UserRepository for handler tests.
// By default all methods return nil/nil; set the user or err fields to
// control behaviour per test.
type stubUserRepo struct {
	user *domain.User
	err  error
}

func (r *stubUserRepo) Create(_ context.Context, _ *domain.User) error { return r.err }
func (r *stubUserRepo) GetByID(_ context.Context, _ int) (*domain.User, error) {
	return r.user, r.err
}
func (r *stubUserRepo) GetByClerkSubject(_ context.Context, _ string) (*domain.User, error) {
	return r.user, r.err
}
func (r *stubUserRepo) Update(_ context.Context, _ *domain.User) error { return r.err }
func (r *stubUserRepo) Delete(_ context.Context, _ int) error          { return r.err }
func (r *stubUserRepo) List(_ context.Context) ([]*domain.User, error) { return nil, r.err }
func (r *stubUserRepo) ListByIDs(_ context.Context, _ []int) ([]*domain.User, error) {
	return nil, r.err
}
func (r *stubUserRepo) Ban(_ context.Context, _, _ int, _ string) (*domain.User, error) {
	return r.user, r.err
}
func (r *stubUserRepo) Unban(_ context.Context, _ int) error                 { return r.err }
func (r *stubUserRepo) ListBanned(_ context.Context) ([]*domain.User, error) { return nil, r.err }
func (r *stubUserRepo) ListFiltered(_ context.Context, _ repository.UserFilters, _ repository.Pagination) ([]*domain.User, error) {
	return nil, r.err
}
func (r *stubUserRepo) GetStatusCounts(_ context.Context) (repository.UserStatusCounts, error) {
	return repository.UserStatusCounts{}, r.err
}

const (
	fmtExpect200     = "expected 200, got %d"
	fmtExpect204     = "expected 204, got %d"
	fmtExpect400     = "expected 400, got %d"
	fmtExpect401     = "expected 401, got %d"
	fmtExpect422     = "expected 422, got %d"
	fmtExpect500     = "expected 500, got %d"
	urlListByUserID1 = "/?user_id=1"

	homeTeam = "Brazil"
	awayTeam = "Argentina"

	headerContentType = "Content-Type"
	contentTypeJSON   = "application/json"

	pathWebhookClerk    = "/webhooks/clerk"
	headerSvixID        = "svix-id"
	headerSvixTimestamp = "svix-timestamp"
	headerSvixSignature = "svix-signature"

	bodySubmitPrediction = `{"match_id":1,"home_score":2,"away_score":1}`
	bodyUpdatePrediction = `{"home_score":2,"away_score":1}`
	pathPredictionID1    = "/1"
)

// stubMatchSvc implements service.MatchService with configurable returns.
type stubMatchSvc struct {
	match   *domain.Match
	matches []*domain.Match
	err     error
}

func (s *stubMatchSvc) CreateMatch(_ context.Context, _ *domain.Match) error {
	return s.err
}
func (s *stubMatchSvc) GetMatch(_ context.Context, _ int) (*domain.Match, error) {
	return s.match, s.err
}
func (s *stubMatchSvc) ListMatches(_ context.Context) ([]*domain.Match, error) {
	return s.matches, s.err
}
func (s *stubMatchSvc) ListMatchesByPhase(_ context.Context, _ domain.MatchPhase) ([]*domain.Match, error) {
	return s.matches, s.err
}
func (s *stubMatchSvc) ListMatchesByStatus(_ context.Context, _ domain.MatchStatus) ([]*domain.Match, error) {
	return s.matches, s.err
}
func (s *stubMatchSvc) UpdateResult(_ context.Context, _ int, _, _ int) (*domain.Match, error) {
	return s.match, s.err
}
func (s *stubMatchSvc) StartMatch(_ context.Context, _ int) (*domain.Match, error) {
	return s.match, s.err
}

// stubPredSvc implements service.PredictionService with configurable returns.
type stubPredSvc struct {
	pred           *domain.Prediction
	preds          []*domain.Prediction
	err            error
	updateCallerID int
	updateID       int
}

func (s *stubPredSvc) Submit(_ context.Context, _ *domain.Prediction) error { return s.err }
func (s *stubPredSvc) Update(_ context.Context, callerUserID, id, _, _ int) (*domain.Prediction, error) {
	s.updateCallerID = callerUserID
	s.updateID = id
	return s.pred, s.err
}
func (s *stubPredSvc) GetByUser(_ context.Context, _ int) ([]*domain.Prediction, error) {
	return s.preds, s.err
}
func (s *stubPredSvc) GetByUserAndQuiniela(_ context.Context, _, _ int) ([]*domain.Prediction, error) {
	return s.preds, s.err
}
func (s *stubPredSvc) GetByMatch(_ context.Context, _ int) ([]*domain.Prediction, error) {
	return s.preds, s.err
}

// stubQuinielaSvc implements service.QuinielaService with configurable returns.
type stubQuinielaSvc struct {
	quiniela  *domain.Quiniela
	quinielas []*domain.Quiniela
	err       error
}

func (s *stubQuinielaSvc) Create(_ context.Context, q *domain.Quiniela) error {
	if s.err != nil {
		return s.err
	}
	if s.quiniela != nil {
		*q = *s.quiniela
	}
	return nil
}
func (s *stubQuinielaSvc) GetByID(_ context.Context, _ int) (*domain.Quiniela, error) {
	return s.quiniela, s.err
}
func (s *stubQuinielaSvc) GetByInviteCode(_ context.Context, _ string) (*domain.Quiniela, error) {
	return s.quiniela, s.err
}
func (s *stubQuinielaSvc) GetByOwner(_ context.Context, _ int) ([]*domain.Quiniela, error) {
	return s.quinielas, s.err
}
func (s *stubQuinielaSvc) RenameGroup(_ context.Context, _, _ int, _ string) (*domain.Quiniela, error) {
	return s.quiniela, s.err
}

// stubRanker implements service.Ranker with configurable returns.
type stubRanker struct {
	entries []*domain.LeaderboardEntry
	err     error
}

func (s *stubRanker) GetLeaderboard(_ context.Context, _ int) ([]*domain.LeaderboardEntry, error) {
	return s.entries, s.err
}

func (s *stubRanker) GetPhaseLeaderboard(_ context.Context, _ int, _ domain.MatchPhase) ([]*domain.LeaderboardEntry, error) {
	return s.entries, s.err
}

// stubUserStatsSvc implements service.MyStatsGetter with configurable returns.
type stubUserStatsSvc struct {
	stats *domain.UserStats
	err   error
}

func (s *stubUserStatsSvc) GetMyStats(_ context.Context, _ int) (*domain.UserStats, error) {
	return s.stats, s.err
}

// stubTiebreakerSvc implements service.TiebreakerService with configurable returns.
type stubTiebreakerSvc struct {
	config *domain.TiebreakerConfig
	tb     *domain.Tiebreaker
	view   *domain.TiebreakerView
	err    error
}

func (s *stubTiebreakerSvc) SetQuestion(_ context.Context, _ string) (*domain.TiebreakerConfig, error) {
	return s.config, s.err
}
func (s *stubTiebreakerSvc) Submit(_ context.Context, _, _, _ int) (*domain.Tiebreaker, error) {
	return s.tb, s.err
}
func (s *stubTiebreakerSvc) GetMine(_ context.Context, _, _ int) (*domain.TiebreakerView, error) {
	return s.view, s.err
}
func (s *stubTiebreakerSvc) ConfirmResult(_ context.Context, _ int) error {
	return s.err
}

// stubTournamentSvc implements service.TournamentService with configurable returns.
type stubTournamentSvc struct {
	standings map[string][]*domain.GroupStanding
	entries   []*domain.GroupStanding
	slot      *domain.TournamentSlot
	slots     []*domain.TournamentSlot
	err       error
}

func (s *stubTournamentSvc) GetAllStandings(_ context.Context) (map[string][]*domain.GroupStanding, error) {
	return s.standings, s.err
}
func (s *stubTournamentSvc) GetGroupStanding(_ context.Context, _ string) ([]*domain.GroupStanding, error) {
	return s.entries, s.err
}
func (s *stubTournamentSvc) CreateSlot(_ context.Context, _ string) (*domain.TournamentSlot, error) {
	return s.slot, s.err
}
func (s *stubTournamentSvc) ConfirmSlot(_ context.Context, _, _ int, _ string) (*domain.TournamentSlot, error) {
	return s.slot, s.err
}
func (s *stubTournamentSvc) ListSlots(_ context.Context) ([]*domain.TournamentSlot, error) {
	return s.slots, s.err
}

// stubMemberSvc implements service.GroupMembershipService with configurable returns.
type stubMemberSvc struct {
	membership  *domain.GroupMembership
	memberships []*domain.GroupMembership
	err         error
}

func (s *stubMemberSvc) Join(_ context.Context, _ string, _ int) (*domain.GroupMembership, error) {
	return s.membership, s.err
}
func (s *stubMemberSvc) ApproveJoin(_ context.Context, _, _, _ int) (*domain.GroupMembership, error) {
	return s.membership, s.err
}
func (s *stubMemberSvc) Leave(_ context.Context, _, _ int) error { return s.err }
func (s *stubMemberSvc) MarkPaid(_ context.Context, _, _ int) (*domain.GroupMembership, error) {
	return s.membership, s.err
}
func (s *stubMemberSvc) ListByQuiniela(_ context.Context, _ int) ([]*domain.GroupMembership, error) {
	return s.memberships, s.err
}
func (s *stubMemberSvc) ListByUser(_ context.Context, _ int) ([]*domain.GroupMembership, error) {
	return s.memberships, s.err
}
