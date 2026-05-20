package dispatcher_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"go.uber.org/zap"

	infrapush "github.com/rede/world-cup-quiniela/internal/infrastructure/webpush"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/notification/dispatcher"
	"github.com/rede/world-cup-quiniela/internal/notification/hub"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// ── Test doubles ─────────────────────────────────────────────────────────────

type stubNotifRepo struct {
	inserted    bool
	err         error
	last        *domain.UserNotification
	createCount int
}

func (s *stubNotifRepo) Create(_ context.Context, n *domain.UserNotification) (bool, error) {
	s.createCount++
	s.last = n
	return s.inserted, s.err
}

// stubMemberLister satisfies dispatcher.GroupMemberLister for fan-out tests.
type stubMemberLister struct {
	memberIDs []int
	err       error
}

func (s *stubMemberLister) ListActiveMemberIDsByGroup(_ context.Context, _ int) ([]int, error) {
	return s.memberIDs, s.err
}
func (s *stubNotifRepo) List(_ context.Context, _, _, _ int, _ bool) ([]*domain.UserNotification, error) {
	return nil, nil
}
func (s *stubNotifRepo) CountUnread(_ context.Context, _ int) (int, error) { return 0, nil }
func (s *stubNotifRepo) MarkRead(_ context.Context, _ int64, _ int) error  { return nil }
func (s *stubNotifRepo) MarkAllRead(_ context.Context, _ int) error        { return nil }

type stubPrefRepo struct {
	pref *domain.NotificationPreference
	err  error
}

func (s *stubPrefRepo) Get(_ context.Context, _ int, _ string) (*domain.NotificationPreference, error) {
	return s.pref, s.err
}
func (s *stubPrefRepo) ListByUser(_ context.Context, _ int) ([]*domain.NotificationPreference, error) {
	return nil, nil
}
func (s *stubPrefRepo) Upsert(_ context.Context, _ *domain.NotificationPreference) error { return nil }

type stubPushRepo struct {
	subs       []*domain.PushSubscription
	markCalled int64 // last sub ID marked inactive
}

func (s *stubPushRepo) Create(_ context.Context, _ *domain.PushSubscription) error { return nil }
func (s *stubPushRepo) ListActiveByUser(_ context.Context, _ int) ([]*domain.PushSubscription, error) {
	return s.subs, nil
}
func (s *stubPushRepo) DeleteByEndpoint(_ context.Context, _ string) error { return nil }
func (s *stubPushRepo) MarkInactive(_ context.Context, id int64) error {
	s.markCalled = id
	return nil
}

type stubPusher struct {
	code int
	err  error
	sent int
}

func (s *stubPusher) Send(_ context.Context, _ infrapush.Message) (int, error) {
	s.sent++
	return s.code, s.err
}

type stubPgNotifier struct {
	calls   int
	channel string
	payload string
}

func (s *stubPgNotifier) Notify(_ context.Context, channel, payload string) error {
	s.calls++
	s.channel = channel
	s.payload = payload
	return nil
}

// stubParamService implements service.SystemParamService with configurable
// string and int lookups; all other methods return zero values.
type stubParamService struct {
	strings map[string]string
	ints    map[string]int
}

func (s *stubParamService) GetString(_ context.Context, key, def string) string {
	if v, ok := s.strings[key]; ok {
		return v
	}
	return def
}

func (s *stubParamService) GetInt(_ context.Context, key string, def int) int {
	if v, ok := s.ints[key]; ok {
		return v
	}
	return def
}

func (s *stubParamService) GetDuration(_ context.Context, _ string, def time.Duration) time.Duration {
	return def
}
func (s *stubParamService) GetBool(_ context.Context, _ string, def bool) bool { return def }
func (s *stubParamService) Get(_ context.Context, _ string) (*domain.SystemParam, error) {
	return nil, nil
}
func (s *stubParamService) GetAll(_ context.Context) ([]*domain.SystemParam, error) { return nil, nil }
func (s *stubParamService) GetByCategory(_ context.Context, _ string) ([]*domain.SystemParam, error) {
	return nil, nil
}
func (s *stubParamService) Set(_ context.Context, _, _ string, _ int) (*domain.SystemParam, error) {
	return nil, nil
}
func (s *stubParamService) BulkSet(_ context.Context, _ map[string]string, _ int) error { return nil }
func (s *stubParamService) ResetToDefault(_ context.Context, _ string, _ int) (*domain.SystemParam, error) {
	return nil, nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func allEnabled() *domain.NotificationPreference {
	return &domain.NotificationPreference{
		ChannelEmail: true,
		ChannelPush:  true,
		ChannelInApp: true,
	}
}

func newUserDispatcher(
	notifRepo *stubNotifRepo,
	prefRepo *stubPrefRepo,
	pushRepo *stubPushRepo,
	pusher infrapush.Sender,
	pgNotifier dispatcher.PgNotifier,
	dlqRepo *recordingDLQRepo,
) *dispatcher.UserDispatcher {
	h := hub.New()
	return dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo:  notifRepo,
		PrefRepo:   prefRepo,
		PushRepo:   pushRepo,
		DLQRepo:    dlqRepo,
		Hub:        h,
		Pusher:     pusher,
		PgNotifier: pgNotifier,
		Log:        zap.NewNop(),
	})
}

// ── Tests ────────────────────────────────────────────────────────────────────

func TestUserDispatcher_AdminEvent_Skipped(t *testing.T) {
	t.Parallel()
	notifRepo := &stubNotifRepo{inserted: false}
	d := newUserDispatcher(notifRepo, &stubPrefRepo{err: errors.New("not found")}, &stubPushRepo{}, infrapush.NoopSender{}, nil, &recordingDLQRepo{})

	entry := makeEntry(t, notification.EventAdminBankTransferPending,
		notification.AdminBankTransferPayload{ProofID: 1, UserID: 2, AmountCents: 100, Currency: "GTQ"})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	if notifRepo.last != nil {
		t.Error("admin event should not reach user notification repo")
	}
}

func TestUserDispatcher_MissingUserID_Skipped(t *testing.T) {
	t.Parallel()
	notifRepo := &stubNotifRepo{}
	d := newUserDispatcher(notifRepo, &stubPrefRepo{err: errors.New("not found")}, &stubPushRepo{}, infrapush.NoopSender{}, nil, &recordingDLQRepo{})

	// SystemAlertPayload has no user_id field.
	entry := makeEntry(t, notification.EventPredictionConfirmed,
		notification.SystemAlertPayload{Component: "test"})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	if notifRepo.last != nil {
		t.Error("payload without user_id should be skipped")
	}
}

func TestUserDispatcher_SuccessfulDispatch_PersistsAndNotifies(t *testing.T) {
	t.Parallel()
	notifRepo := &stubNotifRepo{inserted: true}
	pgNotif := &stubPgNotifier{}
	dlq := &recordingDLQRepo{}
	d := newUserDispatcher(notifRepo, &stubPrefRepo{pref: allEnabled()}, &stubPushRepo{}, infrapush.NoopSender{}, pgNotif, dlq)

	entry := makeEntry(t, notification.EventPredictionConfirmed,
		notification.PredictionConfirmedPayload{UserID: 5, PredictionID: 1, MatchID: 2, HomeTeam: "MX", AwayTeam: "US"})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}

	if notifRepo.last == nil {
		t.Fatal("notification should have been persisted")
	}
	if notifRepo.last.UserID != 5 {
		t.Errorf("notification user_id: got %d; want 5", notifRepo.last.UserID)
	}
	if pgNotif.calls != 1 {
		t.Errorf("pg_notify calls: got %d; want 1", pgNotif.calls)
	}
	if pgNotif.channel != "user_notifications" {
		t.Errorf("pg_notify channel: got %q; want user_notifications", pgNotif.channel)
	}
	if len(dlq.entries) != 0 {
		t.Errorf("DLQ entries: got %d; want 0 on success", len(dlq.entries))
	}
}

func TestUserDispatcher_PersistFailure_WritesDLQ_ReturnsError(t *testing.T) {
	t.Parallel()
	persistErr := errors.New("db error")
	notifRepo := &stubNotifRepo{err: persistErr}
	dlq := &recordingDLQRepo{}
	d := newUserDispatcher(notifRepo, &stubPrefRepo{pref: allEnabled()}, &stubPushRepo{}, infrapush.NoopSender{}, nil, dlq)

	entry := makeEntry(t, notification.EventPaymentConfirmed,
		notification.PaymentPayload{UserID: 3, PaymentID: 1, AmountCents: 500, Currency: "GTQ"})

	err := d.Dispatch(context.Background(), entry)
	if err == nil {
		t.Fatal("expected error on persist failure")
	}
	if len(dlq.entries) != 1 {
		t.Errorf("DLQ entries: got %d; want 1 on persist failure", len(dlq.entries))
	}
}

func TestUserDispatcher_IdempotencyConflict_NoError(t *testing.T) {
	t.Parallel()
	// inserted=false, err=nil means the idempotency key already exists.
	notifRepo := &stubNotifRepo{inserted: false, err: nil}
	pgNotif := &stubPgNotifier{}
	d := newUserDispatcher(notifRepo, &stubPrefRepo{pref: allEnabled()}, &stubPushRepo{}, infrapush.NoopSender{}, pgNotif, &recordingDLQRepo{})

	entry := makeEntry(t, notification.EventPredictionConfirmed,
		notification.PredictionConfirmedPayload{UserID: 7, PredictionID: 2, MatchID: 3, HomeTeam: "BR", AwayTeam: "AR"})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch returned error on idempotency conflict: %v", err)
	}
	if pgNotif.calls != 0 {
		t.Errorf("pg_notify should not fire when notification was not inserted (duplicate); got %d calls", pgNotif.calls)
	}
}

func TestUserDispatcher_PushGone_MarksInactive(t *testing.T) {
	t.Parallel()
	notifRepo := &stubNotifRepo{inserted: true}
	pushRepo := &stubPushRepo{subs: []*domain.PushSubscription{
		{ID: 42, UserID: 1, Endpoint: "https://push.example.com/1", P256dhKey: "key1", AuthKey: "auth1", Active: true},
	}}
	pusher := &stubPusher{code: http.StatusGone}
	d := newUserDispatcher(notifRepo, &stubPrefRepo{pref: allEnabled()}, pushRepo, pusher, nil, &recordingDLQRepo{})

	entry := makeEntry(t, notification.EventPredictionConfirmed,
		notification.PredictionConfirmedPayload{UserID: 1, PredictionID: 1, MatchID: 1, HomeTeam: "A", AwayTeam: "B"})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	if pushRepo.markCalled != 42 {
		t.Errorf("MarkInactive sub_id: got %d; want 42", pushRepo.markCalled)
	}
}

func TestUserDispatcher_PushSendError_Swallowed(t *testing.T) {
	t.Parallel()
	notifRepo := &stubNotifRepo{inserted: true}
	pushRepo := &stubPushRepo{subs: []*domain.PushSubscription{
		{ID: 1, UserID: 2, Endpoint: "https://push.example.com/x", P256dhKey: "k", AuthKey: "a", Active: true},
	}}
	pusher := &stubPusher{err: errors.New("network error")}
	d := newUserDispatcher(notifRepo, &stubPrefRepo{pref: allEnabled()}, pushRepo, pusher, nil, &recordingDLQRepo{})

	entry := makeEntry(t, notification.EventWithdrawalApproved,
		notification.WithdrawalPayload{UserID: 2, RequestID: 5, AmountCents: 1000, Currency: "GTQ"})

	// Push send error must not surface as a Dispatch error.
	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch returned error on push send failure: %v", err)
	}
}

func TestUserDispatcher_NoPushChannel_NoPushSent(t *testing.T) {
	t.Parallel()
	notifRepo := &stubNotifRepo{inserted: true}
	pusher := &stubPusher{}
	pref := &domain.NotificationPreference{ChannelEmail: true, ChannelPush: false, ChannelInApp: true}
	d := newUserDispatcher(notifRepo, &stubPrefRepo{pref: pref}, &stubPushRepo{subs: []*domain.PushSubscription{
		{ID: 1, UserID: 3, Endpoint: "https://e", P256dhKey: "k", AuthKey: "a", Active: true},
	}}, pusher, nil, &recordingDLQRepo{})

	entry := makeEntry(t, notification.EventAccountWelcome,
		notification.AccountWelcomePayload{UserID: 3, UserName: "Alice", Email: "a@a.com"})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	if pusher.sent != 0 {
		t.Errorf("push send calls: got %d; want 0 (channel disabled)", pusher.sent)
	}
}

func TestUserDispatcher_PushWithParams_SendsWithOverriddenAssets(t *testing.T) {
	t.Parallel()
	notifRepo := &stubNotifRepo{inserted: true}
	pushRepo := &stubPushRepo{subs: []*domain.PushSubscription{
		{ID: 7, UserID: 9, Endpoint: "https://push.example.com/p", P256dhKey: "k", AuthKey: "a", Active: true},
	}}
	pusher := &stubPusher{code: http.StatusCreated}
	params := &stubParamService{
		strings: map[string]string{
			domain.ParamKeyNotifyPushIconURL:  "/cdn/icon-192.png",
			domain.ParamKeyNotifyPushBadgeURL: "/cdn/badge-72.png",
		},
		ints: map[string]int{
			domain.ParamKeyNotifyWebPushTTLSec: 3600,
		},
	}
	d := dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo: notifRepo,
		PrefRepo:  &stubPrefRepo{pref: allEnabled()},
		PushRepo:  pushRepo,
		DLQRepo:   &recordingDLQRepo{},
		Hub:       hub.New(),
		Pusher:    pusher,
		Log:       zap.NewNop(),
		Params:    params,
	})

	entry := makeEntry(t, notification.EventPredictionConfirmed,
		notification.PredictionConfirmedPayload{UserID: 9, PredictionID: 1, MatchID: 1, HomeTeam: "A", AwayTeam: "B"})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	if pusher.sent != 1 {
		t.Errorf("push send calls: got %d; want 1", pusher.sent)
	}
}

func TestUserDispatcher_BroadcastEvent_Skipped(t *testing.T) {
	t.Parallel()
	notifRepo := &stubNotifRepo{inserted: true}
	d := newUserDispatcher(notifRepo, &stubPrefRepo{err: errors.New("no pref")}, &stubPushRepo{}, infrapush.NoopSender{}, nil, &recordingDLQRepo{})

	// MatchEventPayload has no user_id field — dispatcher must skip without error.
	entry := makeEntry(t, notification.EventMatchResultEntered,
		notification.MatchEventPayload{HomeTeam: "A", AwayTeam: "B"})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch returned error for broadcast event: %v", err)
	}
	if notifRepo.last != nil {
		t.Error("broadcast event with no user_id must be skipped (not persisted)")
	}
}

// ── Content rendering coverage ────────────────────────────────────────────────

func TestUserDispatcher_AllUserEvents_Rendered(t *testing.T) {
	t.Parallel()

	events := []struct {
		et      notification.EventType
		payload any
	}{
		{notification.EventPredictionDeadlineApproach, notification.PredictionDeadlinePayload{UserID: 1, HomeTeam: "A", AwayTeam: "B", MinutesLeft: 15}},
		{notification.EventPredictionMissingReminder, notification.PredictionDeadlinePayload{UserID: 1, HomeTeam: "C", AwayTeam: "D", MinutesLeft: 5}},
		{notification.EventPredictionLocked, notification.PredictionLockedPayload{UserID: 1, HomeTeam: "E", AwayTeam: "F"}},
		{notification.EventPredictionScored, notification.PredictionScoredPayload{UserID: 1, HomeTeam: "G", AwayTeam: "H", HomeScore: 2, AwayScore: 1, PointsEarned: 10}},
		// MatchEventPayload and some group payloads have no user_id (broadcast events)
		// and are intentionally skipped by the dispatcher — tested separately.
		{notification.EventGroupJoinApproved, notification.GroupJoinPayload{UserID: 1, QuinielaName: "Liga A"}},
		{notification.EventGroupJoinRejected, notification.GroupJoinPayload{UserID: 1, QuinielaName: "Liga B"}},
		{notification.EventGroupLeaderboardMilestone, notification.GroupLeaderboardMilestonePayload{UserID: 1, NewRank: 1, TotalPoints: 50, QuinielaName: "Liga E"}},
		{notification.EventPaymentConfirmed, notification.PaymentPayload{UserID: 1, AmountCents: 10000, Currency: "GTQ"}},
		{notification.EventPaymentFailed, notification.PaymentPayload{UserID: 1, AmountCents: 5000, Currency: "GTQ", Reason: "declined"}},
		{notification.EventPaymentBankTransferSubmitted, notification.BankTransferPayload{UserID: 1, AmountCents: 20000, Currency: "GTQ"}},
		{notification.EventPaymentBankTransferApproved, notification.BankTransferPayload{UserID: 1, AmountCents: 20000, Currency: "GTQ"}},
		{notification.EventPaymentBankTransferRejected, notification.BankTransferPayload{UserID: 1, AmountCents: 20000, Currency: "GTQ"}},
		{notification.EventWithdrawalRequested, notification.WithdrawalPayload{UserID: 1, AmountCents: 30000, Currency: "GTQ"}},
		{notification.EventWithdrawalApproved, notification.WithdrawalPayload{UserID: 1, AmountCents: 30000, Currency: "GTQ"}},
		{notification.EventWithdrawalRejected, notification.WithdrawalPayload{UserID: 1, AmountCents: 30000, Currency: "GTQ"}},
		{notification.EventWithdrawalCompleted, notification.WithdrawalPayload{UserID: 1, AmountCents: 30000, Currency: "GTQ"}},
		{notification.EventWithdrawalFailed, notification.WithdrawalPayload{UserID: 1, AmountCents: 30000, Currency: "GTQ"}},
		{notification.EventAccountWelcome, notification.AccountWelcomePayload{UserID: 1, UserName: "Bob"}},
		{notification.EventAccountBalanceCredited, notification.AccountBalancePayload{UserID: 1, AmountCents: 500, BalanceAfter: 5000, Currency: "GTQ"}},
		{notification.EventAccountLowBalance, notification.AccountBalancePayload{UserID: 1, BalanceAfter: 100, Currency: "GTQ"}},
		{notification.EventAccountBalanceDebited, notification.AccountBalancePayload{UserID: 1, AmountCents: 200, BalanceAfter: 4800, Currency: "GTQ"}},
		// EventGroupMemberJoined / EventGroupMemberLeft are broadcast (fan-out) events
		// tested separately in TestUserDispatcher_BroadcastFanOut_*.
		{notification.EventPaymentPendingTimeout, notification.PaymentPayload{UserID: 1, PaymentID: 9, AmountCents: 8000, Currency: "GTQ"}},
		{notification.EventWithdrawalProcessing, notification.WithdrawalPayload{UserID: 1, RequestID: 11, AmountCents: 25000, Currency: "GTQ"}},
		{notification.EventWithdrawalPendingTimeout, notification.WithdrawalPayload{UserID: 1, RequestID: 12, AmountCents: 25000, Currency: "GTQ"}},
		// Unknown event type exercises the default branch.
		{notification.EventType("custom.unknown"), notification.PredictionConfirmedPayload{UserID: 1}},
	}

	for _, tc := range events {
		tc := tc
		t.Run(string(tc.et), func(t *testing.T) {
			t.Parallel()
			notifRepo := &stubNotifRepo{inserted: true}
			d := newUserDispatcher(notifRepo, &stubPrefRepo{err: errors.New("no pref")}, &stubPushRepo{}, infrapush.NoopSender{}, nil, &recordingDLQRepo{})

			entry := makeEntry(t, tc.et, tc.payload)
			if err := d.Dispatch(context.Background(), entry); err != nil {
				t.Fatalf("Dispatch returned error for %s: %v", tc.et, err)
			}
			if notifRepo.last == nil {
				t.Fatalf("notification not persisted for %s", tc.et)
			}
			if notifRepo.last.Title == "" {
				t.Errorf("empty title for %s", tc.et)
			}
			if notifRepo.last.Body == "" {
				t.Errorf("empty body for %s", tc.et)
			}
		})
	}
}

// ── Bug 1: EventGroupJoinRequested must notify the owner, not the requester ──

func TestUserDispatcher_GroupJoinRequested_NotifiesOwner(t *testing.T) {
	t.Parallel()

	notifRepo := &stubNotifRepo{inserted: true}
	d := newUserDispatcher(notifRepo, &stubPrefRepo{err: errors.New("no pref")}, &stubPushRepo{}, infrapush.NoopSender{}, nil, &recordingDLQRepo{})

	const requesterID = 10
	const ownerID = 99
	entry := makeEntry(t, notification.EventGroupJoinRequested, notification.GroupJoinPayload{
		QuinielaID:   5,
		QuinielaName: "Liga Test",
		UserID:       requesterID,
		OwnerID:      ownerID,
	})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	if notifRepo.last == nil {
		t.Fatal("notification not persisted")
	}
	if notifRepo.last.UserID != ownerID {
		t.Errorf("notification delivered to user %d; want owner %d", notifRepo.last.UserID, ownerID)
	}
}

// ── Bug 2: EventGroupMemberJoined/Left must fan-out to all active members ────

func TestUserDispatcher_BroadcastFanOut_DeliveredToAllMembers(t *testing.T) {
	t.Parallel()

	notifRepo := &stubNotifRepo{inserted: true}
	memberIDs := []int{2, 3, 4}                                            // actor is UserID=1; all three are other members
	lister := &stubMemberLister{memberIDs: append([]int{1}, memberIDs...)} // include actor to verify exclusion

	d := dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo:    notifRepo,
		PrefRepo:     &stubPrefRepo{err: errors.New("no pref")},
		PushRepo:     &stubPushRepo{},
		DLQRepo:      &recordingDLQRepo{},
		Hub:          hub.New(),
		Pusher:       infrapush.NoopSender{},
		MemberLister: lister,
		Log:          zap.NewNop(),
	})

	entry := makeEntry(t, notification.EventGroupMemberJoined, notification.GroupJoinPayload{
		QuinielaID:   7,
		QuinielaName: "Liga X",
		UserID:       1, // actor — must be excluded from fan-out
		OwnerID:      2,
	})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	// Actor (UserID=1) excluded → 3 deliveries to IDs 2, 3, 4
	if notifRepo.createCount != len(memberIDs) {
		t.Errorf("notifications persisted: got %d; want %d", notifRepo.createCount, len(memberIDs))
	}
}

func TestUserDispatcher_BroadcastFanOut_NilLister_Skips(t *testing.T) {
	t.Parallel()

	notifRepo := &stubNotifRepo{inserted: true}
	// newUserDispatcher does NOT set MemberLister — broadcast must be silently skipped.
	d := newUserDispatcher(notifRepo, &stubPrefRepo{err: errors.New("no pref")}, &stubPushRepo{}, infrapush.NoopSender{}, nil, &recordingDLQRepo{})

	entry := makeEntry(t, notification.EventGroupMemberLeft, notification.GroupJoinPayload{
		QuinielaID: 3,
		UserID:     1,
	})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch returned unexpected error: %v", err)
	}
	if notifRepo.createCount != 0 {
		t.Errorf("expected 0 notifications when MemberLister is nil; got %d", notifRepo.createCount)
	}
}

func TestUserDispatcher_BroadcastFanOut_ListerError_Propagates(t *testing.T) {
	t.Parallel()

	notifRepo := &stubNotifRepo{inserted: true}
	lister := &stubMemberLister{err: errors.New("db timeout")}

	d := dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo:    notifRepo,
		PrefRepo:     &stubPrefRepo{err: errors.New("no pref")},
		PushRepo:     &stubPushRepo{},
		DLQRepo:      &recordingDLQRepo{},
		Hub:          hub.New(),
		Pusher:       infrapush.NoopSender{},
		MemberLister: lister,
		Log:          zap.NewNop(),
	})

	entry := makeEntry(t, notification.EventGroupMemberJoined, notification.GroupJoinPayload{
		QuinielaID: 9,
		UserID:     1,
	})

	if err := d.Dispatch(context.Background(), entry); err == nil {
		t.Fatal("expected error when MemberLister fails; got nil")
	}
}
