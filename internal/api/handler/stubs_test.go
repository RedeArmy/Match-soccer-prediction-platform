package handler_test

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/storage"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
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
func (r *stubUserRepo) GetByExternalSubject(_ context.Context, _ string) (*domain.User, error) {
	return r.user, r.err
}
func (r *stubUserRepo) GetByEmail(_ context.Context, _ string) (*domain.User, error) {
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
func (r *stubUserRepo) ListFiltered(_ context.Context, _ repository.UserFilters, _ repository.CursorPage) ([]*domain.User, string, error) {
	return nil, "", r.err
}
func (r *stubUserRepo) GetStatusCounts(_ context.Context) (repository.UserStatusCounts, error) {
	return repository.UserStatusCounts{}, r.err
}
func (r *stubUserRepo) GetBalance(_ context.Context, _ int) (int, int, error) {
	return 0, 0, r.err
}
func (r *stubUserRepo) GetBalanceCurrency(_ context.Context, _ int) (string, error) {
	return "GTQ", r.err
}
func (r *stubUserRepo) UpdateLocale(_ context.Context, _ int, _ string) error   { return r.err }
func (r *stubUserRepo) UpdateTimezone(_ context.Context, _ int, _ string) error { return r.err }
func (r *stubUserRepo) SetRole(_ context.Context, _ int, _ domain.UserRole) (*domain.User, error) {
	return r.user, r.err
}

const (
	fmtExpect200        = "expected 200, got %d"
	fmtExpect204        = "expected 204, got %d"
	fmtExpect400        = "expected 400, got %d"
	fmtExpect401        = "expected 401, got %d"
	fmtExpect422        = "expected 422, got %d"
	fmtExpect500        = "expected 500, got %d"
	urlGetMyPredictions = "/me"

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
func (s *stubMatchSvc) UpdateResult(_ context.Context, _ int, _, _ int, _ *domain.WinMethod, _ *string) (*domain.Match, error) {
	return s.match, s.err
}
func (s *stubMatchSvc) StartMatch(_ context.Context, _ int) (*domain.Match, error) {
	return s.match, s.err
}
func (s *stubMatchSvc) CorrectResult(_ context.Context, _ int, _, _ int, _ *domain.WinMethod, _ *string) (*domain.Match, error) {
	return s.match, s.err
}
func (s *stubMatchSvc) CancelMatch(_ context.Context, _ int) (*domain.Match, error) {
	return s.match, s.err
}
func (s *stubMatchSvc) UpdateSlots(_ context.Context, _ int, _, _ *int) (*domain.Match, error) {
	return s.match, s.err
}

// stubPredSvc implements service.PredictionService with configurable returns.
type stubPredSvc struct {
	pred           *domain.Prediction
	preds          []*domain.Prediction
	err            error
	created        bool // returned by Submit; set true to simulate first-time creation
	updateCallerID int
	updateID       int
}

func (s *stubPredSvc) Submit(_ context.Context, _ *domain.Prediction) (bool, error) {
	return s.created, s.err
}
func (s *stubPredSvc) Update(_ context.Context, callerUserID, id, _, _ int, _ *domain.WinMethod, _ *string) (*domain.Prediction, error) {
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
	quiniela      *domain.Quiniela
	quinielas     []*domain.Quiniela
	err           error
	nameAvailable bool // used by IsNameAvailable; default false → override to true in tests
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
func (s *stubQuinielaSvc) SetTournamentMode(_ context.Context, _, _ int, _, _ bool) (*domain.Quiniela, error) {
	return s.quiniela, s.err
}
func (s *stubQuinielaSvc) UpdateRequireApproval(_ context.Context, _, _ int, _ bool) (*domain.Quiniela, error) {
	return s.quiniela, s.err
}
func (s *stubQuinielaSvc) UpdateScoreFromZero(_ context.Context, _, _ int, _ bool) (*domain.Quiniela, error) {
	return s.quiniela, s.err
}
func (s *stubQuinielaSvc) IsNameAvailable(_ context.Context, _ string, _ int) (bool, error) {
	return s.nameAvailable, s.err
}

// stubRanker implements service.Ranker with configurable returns.
type stubRanker struct {
	entries []*domain.LeaderboardEntry
	err     error
}

func (s *stubRanker) GetLeaderboard(_ context.Context, _ int) (*service.LeaderboardResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &service.LeaderboardResult{Entries: s.entries}, nil
}

func (s *stubRanker) GetPhaseLeaderboard(_ context.Context, _ int, _ domain.MatchPhase) (*service.LeaderboardResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &service.LeaderboardResult{Entries: s.entries}, nil
}
func (s *stubRanker) GetLeaderboardWithRoundBreakdown(ctx context.Context, id int) (*service.LeaderboardResult, error) {
	return s.GetLeaderboard(ctx, id)
}

// stubGroupAuthz implements service.GroupAuthz with configurable returns.
// memberErr controls RequireActiveMember; ownerErr controls RequireOwner.
// Both default to nil (allow), so tests that do not care about authz can
// use &stubGroupAuthz{} without extra setup.
type stubGroupAuthz struct {
	memberErr error
	ownerErr  error
}

func (s *stubGroupAuthz) RequireActiveMember(_ context.Context, _, _ int) error {
	return s.memberErr
}
func (s *stubGroupAuthz) RequireOwner(_ context.Context, _, _ int) error {
	return s.ownerErr
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
func (s *stubTiebreakerSvc) SetQuestionForPhase(_ context.Context, _ domain.MatchPhase, _ string) (*domain.TiebreakerConfig, error) {
	return s.config, s.err
}
func (s *stubTiebreakerSvc) SetQuestionForQuiniela(_ context.Context, _ int, _ string) (*domain.TiebreakerConfig, error) {
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
func (s *stubTiebreakerSvc) ConfirmResultByID(_ context.Context, _, _ int) error {
	return s.err
}

// stubTournamentSvc implements service.TournamentService with configurable returns.
type stubTournamentSvc struct {
	standings map[string][]*domain.GroupStanding
	entries   []*domain.GroupStanding
	slot      *domain.TournamentSlot
	slots     []*domain.TournamentSlot
	teamNames []string
	err       error
}

func (s *stubTournamentSvc) GetAllStandings(_ context.Context) (map[string][]*domain.GroupStanding, error) {
	return s.standings, s.err
}
func (s *stubTournamentSvc) GetGroupStanding(_ context.Context, _ string) ([]*domain.GroupStanding, error) {
	return s.entries, s.err
}
func (s *stubTournamentSvc) CreateSlot(_ context.Context, _, _ string) (*domain.TournamentSlot, error) {
	return s.slot, s.err
}
func (s *stubTournamentSvc) ConfirmSlot(_ context.Context, _, _ int, _ string) (*domain.TournamentSlot, error) {
	return s.slot, s.err
}
func (s *stubTournamentSvc) ListSlots(_ context.Context) ([]*domain.TournamentSlot, error) {
	return s.slots, s.err
}
func (s *stubTournamentSvc) AutoConfirmGroupSlots(_ context.Context, _ string) error {
	return s.err
}
func (s *stubTournamentSvc) AutoConfirmMatchResultSlots(_ context.Context, _, _, _ string) error {
	return s.err
}
func (s *stubTournamentSvc) BackfillSlots(_ context.Context) error { return s.err }
func (s *stubTournamentSvc) ListTeamNames(_ context.Context) ([]string, error) {
	return s.teamNames, s.err
}
func (s *stubTournamentSvc) AutoConfirmBestThirdSlots(_ context.Context, _ int) ([]service.BestThirdAssignment, error) {
	return nil, s.err
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
func (s *stubMemberSvc) RejectJoin(_ context.Context, _, _, _ int) error { return s.err }
func (s *stubMemberSvc) Leave(_ context.Context, _, _ int) error         { return s.err }
func (s *stubMemberSvc) MarkPaid(_ context.Context, _, _ int) (*domain.GroupMembership, error) {
	return s.membership, s.err
}
func (s *stubMemberSvc) ListByQuiniela(_ context.Context, _ int) ([]*domain.GroupMembership, error) {
	return s.memberships, s.err
}
func (s *stubMemberSvc) ListByUser(_ context.Context, _ int) ([]*domain.GroupMembership, error) {
	return s.memberships, s.err
}
func (s *stubMemberSvc) JoinWithBalance(_ context.Context, _ string, _ int) (*domain.GroupMembership, error) {
	return s.membership, s.err
}

// ── stubBalanceSvc ────────────────────────────────────────────────────────────

type stubBalanceSvc struct {
	balanceCents  int
	reservedCents int
	entries       []*domain.BalanceLedger
	err           error
}

func (s *stubBalanceSvc) GetBalance(_ context.Context, _ int) (int, int, error) {
	return s.balanceCents, s.reservedCents, s.err
}
func (s *stubBalanceSvc) GetBalanceWithCurrency(_ context.Context, _ int) (int, int, string, error) {
	return s.balanceCents, s.reservedCents, "GTQ", s.err
}
func (s *stubBalanceSvc) GetLedger(_ context.Context, _ int, _ repository.Pagination) ([]*domain.BalanceLedger, error) {
	return s.entries, s.err
}

// ── stubBankTransferSvc ───────────────────────────────────────────────────────

type stubBankTransferSvc struct {
	proof  *domain.BankTransferProof
	proofs []*domain.BankTransferProof
	err    error
}

func (s *stubBankTransferSvc) Upload(_ context.Context, _, _ int, _, _, _ string, _ int) (*domain.BankTransferProof, error) {
	return s.proof, s.err
}
func (s *stubBankTransferSvc) GetByID(_ context.Context, _ int) (*domain.BankTransferProof, error) {
	return s.proof, s.err
}
func (s *stubBankTransferSvc) ListByUser(_ context.Context, _ int) ([]*domain.BankTransferProof, error) {
	return s.proofs, s.err
}
func (s *stubBankTransferSvc) ListPending(_ context.Context) ([]*domain.BankTransferProof, error) {
	return s.proofs, s.err
}
func (s *stubBankTransferSvc) ListAll(_ context.Context) ([]*domain.BankTransferProof, error) {
	return s.proofs, s.err
}
func (s *stubBankTransferSvc) ApproveTransfer(_ context.Context, _, _ int, _ *int, _ string) (*domain.BankTransferProof, error) {
	return s.proof, s.err
}
func (s *stubBankTransferSvc) RejectTransfer(_ context.Context, _, _ int, _ string) (*domain.BankTransferProof, error) {
	return s.proof, s.err
}

// ── stubWithdrawalSvc ─────────────────────────────────────────────────────────

type stubWithdrawalSvc struct {
	req  *domain.WithdrawalRequest
	reqs []*domain.WithdrawalRequest
	err  error
}

func (s *stubWithdrawalSvc) Create(_ context.Context, _, _ int, _ string, _ domain.WithdrawalMethod, _ map[string]string) (*domain.WithdrawalRequest, error) {
	return s.req, s.err
}
func (s *stubWithdrawalSvc) AdminGetByID(_ context.Context, _ int) (*domain.WithdrawalRequest, error) {
	return s.req, s.err
}
func (s *stubWithdrawalSvc) ListByUser(_ context.Context, _ int) ([]*domain.WithdrawalRequest, error) {
	return s.reqs, s.err
}
func (s *stubWithdrawalSvc) ListPending(_ context.Context) ([]*domain.WithdrawalRequest, error) {
	return s.reqs, s.err
}
func (s *stubWithdrawalSvc) ListAll(_ context.Context, _ string, _ repository.Pagination) ([]*domain.WithdrawalRequest, error) {
	return s.reqs, s.err
}
func (s *stubWithdrawalSvc) ApproveRequest(_ context.Context, _, _ int, _ string) (*domain.WithdrawalRequest, error) {
	return s.req, s.err
}
func (s *stubWithdrawalSvc) RejectRequest(_ context.Context, _, _ int, _ string) (*domain.WithdrawalRequest, error) {
	return s.req, s.err
}
func (s *stubWithdrawalSvc) ProcessWithdrawal(_ context.Context, _, _ int) (*domain.WithdrawalRequest, error) {
	return s.req, s.err
}
func (s *stubWithdrawalSvc) GetLimits(_ context.Context) (int, int, int, error) {
	if s.err != nil {
		return 0, 0, 0, s.err
	}
	return 3000, 500000, 400, nil
}

// ── stubWebhookPaymentSvc ─────────────────────────────────────────────────────

type stubWebhookPaymentSvc struct {
	err error
}

func (s *stubWebhookPaymentSvc) CreditFromRecurrente(_ context.Context, _ int, _ int, _, _ string) error {
	return s.err
}
func (s *stubWebhookPaymentSvc) ResolveAndCreditPayPalIntent(_ context.Context, _, _ string, _ int) error {
	return s.err
}

// ── stubFileStore ─────────────────────────────────────────────────────────────

type stubFileStore struct {
	putErr     error
	getContent []byte // returned by Get when non-nil
	getType    string // Content-Type returned by Get
	getErr     error  // returned by Get when non-nil
}

func (s *stubFileStore) Put(_ context.Context, _, _ string, _ io.Reader, _ int64) error {
	return s.putErr
}
func (s *stubFileStore) Get(_ context.Context, _ string) (io.ReadCloser, string, error) {
	if s.getErr != nil {
		return nil, "", s.getErr
	}
	if s.getContent != nil {
		return io.NopCloser(bytes.NewReader(s.getContent)), s.getType, nil
	}
	return nil, "", storage.ErrNotFound
}

// stubSystemParamSvc implements service.SystemParamService for handler tests.
// All reads fall back to the supplied default so GroupHandler can resolve
// entry-fee and currency params without a real database.
type stubSystemParamSvc struct{}

func (s *stubSystemParamSvc) Get(_ context.Context, _ string) (*domain.SystemParam, error) {
	return nil, nil
}
func (s *stubSystemParamSvc) GetAll(_ context.Context) ([]*domain.SystemParam, error) {
	return nil, nil
}
func (s *stubSystemParamSvc) GetByCategory(_ context.Context, _ string) ([]*domain.SystemParam, error) {
	return nil, nil
}
func (s *stubSystemParamSvc) Set(_ context.Context, _, _ string, _ int) (*domain.SystemParam, error) {
	return nil, nil
}
func (s *stubSystemParamSvc) GetString(_ context.Context, _ string, defaultVal string) string {
	return defaultVal
}
func (s *stubSystemParamSvc) GetInt(_ context.Context, _ string, defaultVal int) int {
	return defaultVal
}
func (s *stubSystemParamSvc) GetDuration(_ context.Context, _ string, defaultVal time.Duration) time.Duration {
	return defaultVal
}
func (s *stubSystemParamSvc) GetBool(_ context.Context, _ string, defaultVal bool) bool {
	return defaultVal
}
func (s *stubSystemParamSvc) BulkSet(_ context.Context, _ map[string]string, _ int) error {
	return nil
}
func (s *stubSystemParamSvc) BulkPreview(_ context.Context, _ map[string]string) ([]domain.ParamDiff, error) {
	return nil, nil
}
func (s *stubSystemParamSvc) ResetToDefault(_ context.Context, _ string, _ int) (*domain.SystemParam, error) {
	return nil, nil
}
func (s *stubSystemParamSvc) GetHistory(_ context.Context, _ string, _ repository.CursorPage) ([]*domain.SystemParamHistory, string, error) {
	return nil, "", nil
}
func (s *stubFileStore) Delete(_ context.Context, _ string) error { return nil }

// stubPredRepo implements repository.PredictionRepository for handler tests.
// Only ListByGroupAndMatches returns configurable values; all other methods
// return zero values and are not exercised by GroupHandler tests.
type stubPredRepo struct {
	preds []*domain.Prediction
	err   error
}

func (r *stubPredRepo) Upsert(_ context.Context, _ *domain.Prediction) (bool, error) {
	return false, r.err
}
func (r *stubPredRepo) Create(_ context.Context, _ *domain.Prediction) error { return r.err }
func (r *stubPredRepo) GetByID(_ context.Context, _ int) (*domain.Prediction, error) {
	return nil, r.err
}
func (r *stubPredRepo) Update(_ context.Context, _ *domain.Prediction) error { return r.err }
func (r *stubPredRepo) UpdateIfUnchanged(_ context.Context, _ *domain.Prediction, _ time.Time) error {
	return r.err
}
func (r *stubPredRepo) GetByUserAndMatch(_ context.Context, _, _ int) (*domain.Prediction, error) {
	return nil, r.err
}
func (r *stubPredRepo) ListByUser(_ context.Context, _ int) ([]*domain.Prediction, error) {
	return r.preds, r.err
}
func (r *stubPredRepo) ListByMatch(_ context.Context, _ int) ([]*domain.Prediction, error) {
	return r.preds, r.err
}
func (r *stubPredRepo) UpdateManyPoints(_ context.Context, _ map[int]int) error { return r.err }
func (r *stubPredRepo) ScoreMatchBatch(_ context.Context, _ int, _ func([]*domain.Prediction) (map[int]int, error), _ int) error {
	return r.err
}
func (r *stubPredRepo) TotalPointsByQuiniela(_ context.Context, _ int, _ bool, _ time.Time) (map[int]int, error) {
	return nil, r.err
}
func (r *stubPredRepo) TotalPointsByQuinielaAndPhase(_ context.Context, _ int, _ domain.MatchPhase, _ bool, _ time.Time) (map[int]int, error) {
	return nil, r.err
}
func (r *stubPredRepo) PointsByUserAndRound(_ context.Context, _ int, _ bool, _ time.Time) (map[int]map[string]int, error) {
	return nil, r.err
}
func (r *stubPredRepo) ListQuinielaIDsByMatch(_ context.Context, _ int) ([]int, error) {
	return nil, r.err
}
func (r *stubPredRepo) ListByGroupAndMatches(_ context.Context, _ int, _ []int) ([]*domain.Prediction, error) {
	return r.preds, r.err
}
func (r *stubPredRepo) ListByUserAndQuiniela(_ context.Context, _, _ int) ([]*domain.Prediction, error) {
	return r.preds, r.err
}
func (r *stubPredRepo) PredictionStatsByQuiniela(_ context.Context, _ int) (map[int]*domain.UserPredictionStats, error) {
	return nil, r.err
}
func (r *stubPredRepo) GetUserPredictionCounts(_ context.Context, _ int) (*domain.UserPredictionCounts, error) {
	return nil, r.err
}
func (r *stubPredRepo) GetUserPointsByPhase(_ context.Context, _ int) (map[domain.MatchPhase]int, error) {
	return nil, r.err
}
func (r *stubPredRepo) ListUserScoredPointsChronological(_ context.Context, _ int) ([]int, error) {
	return nil, r.err
}
func (r *stubPredRepo) ListAdmin(_ context.Context, _ repository.PredictionAdminFilters, _ repository.Pagination) ([]*domain.Prediction, error) {
	return r.preds, r.err
}
func (r *stubPredRepo) GlobalLeaderboard(_ context.Context, _ int) ([]*domain.GlobalLeaderboardEntry, error) {
	return nil, r.err
}
func (r *stubPredRepo) InsertScoringBatch(_ context.Context, _ []domain.PredictionScoreLog) error {
	return r.err
}
func (r *stubPredRepo) GetScoringCfgSnapshot(_ context.Context, _ int) (*domain.ScoringCfgSnapshot, error) {
	return nil, r.err
}
