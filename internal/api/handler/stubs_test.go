package handler_test

import (
	"context"

	"github.com/rede/world-cup-quiniela/internal/domain"
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
	pred  *domain.Prediction
	preds []*domain.Prediction
	err   error
}

func (s *stubPredSvc) Submit(_ context.Context, _ *domain.Prediction) error { return s.err }
func (s *stubPredSvc) Update(_ context.Context, _ int, _, _ int) (*domain.Prediction, error) {
	return s.pred, s.err
}
func (s *stubPredSvc) GetByUser(_ context.Context, _ int) ([]*domain.Prediction, error) {
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
func (s *stubQuinielaSvc) RotateInviteCode(_ context.Context, _, _ int) (*domain.Quiniela, error) {
	return s.quiniela, s.err
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
func (s *stubMemberSvc) MarkPaid(_ context.Context, _, _ int) (*domain.GroupMembership, error) {
	return s.membership, s.err
}
func (s *stubMemberSvc) ListByQuiniela(_ context.Context, _ int) ([]*domain.GroupMembership, error) {
	return s.memberships, s.err
}
func (s *stubMemberSvc) ListByUser(_ context.Context, _ int) ([]*domain.GroupMembership, error) {
	return s.memberships, s.err
}
