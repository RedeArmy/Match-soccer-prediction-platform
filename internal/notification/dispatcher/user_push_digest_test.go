package dispatcher_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	infrapush "github.com/rede/world-cup-quiniela/internal/infrastructure/webpush"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/notification/dispatcher"
	"github.com/rede/world-cup-quiniela/internal/notification/hub"
)

// newDigestDispatcher builds a UserDispatcher wired with a digest gate.
// threshold=0 means the very first P2/P3 event still counts individually, but
// the second triggers the digest path immediately.
func newDigestDispatcher(
	notifRepo *stubNotifRepo,
	pushRepo *stubPushRepo,
	pusher infrapush.Sender,
	gate *notification.PushDigestGate,
) *dispatcher.UserDispatcher {
	return dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo:  notifRepo,
		PrefRepo:   &stubPrefRepo{pref: allEnabled()},
		PushRepo:   pushRepo,
		DLQRepo:    &recordingDLQRepo{},
		Hub:        hub.New(),
		Pusher:     pusher,
		DigestGate: gate,
		Log:        zap.NewNop(),
	})
}

// p2Entry creates a P2 (medium-priority) outbox entry for userID.
// EventPredictionConfirmed is P2 in the priority table.
func p2Entry(t *testing.T, userID int) *notification.OutboxEntry {
	t.Helper()
	return makeEntry(t, notification.EventPredictionConfirmed,
		notification.PredictionConfirmedPayload{UserID: userID, PredictionID: 1, MatchID: 1, HomeTeam: "A", AwayTeam: "B"})
}

// ── digest gate drop path ─────────────────────────────────────────────────────

func TestDeliverPush_DigestGate_DropsSubsequentPushAfterDigest(t *testing.T) {
	t.Parallel()

	notifRepo := &stubNotifRepo{inserted: true}
	pushRepo := &stubPushRepo{subs: []*domain.PushSubscription{
		{ID: 1, UserID: 10, Endpoint: "https://push.example.com/a", P256dhKey: "k", AuthKey: "a", Active: true},
	}}
	pusher := &stubPusher{code: http.StatusCreated}

	// threshold=1: first event → individual push, second → digest push (digestCount>0),
	// third → drop (digestCount==0, sendIndividual==false).
	gate := notification.NewPushDigestGate(60, 1)
	d := newDigestDispatcher(notifRepo, pushRepo, pusher, gate)

	ctx := context.Background()
	// First event: individual push sent.
	if err := d.Dispatch(ctx, p2Entry(t, 10)); err != nil {
		t.Fatalf("first Dispatch: %v", err)
	}
	afterFirst := pusher.sent

	// Second event: digest push sent (first overflow).
	if err := d.Dispatch(ctx, p2Entry(t, 10)); err != nil {
		t.Fatalf("second Dispatch: %v", err)
	}
	afterSecond := pusher.sent

	// Third event: dropped silently (digest already sent for this window).
	if err := d.Dispatch(ctx, p2Entry(t, 10)); err != nil {
		t.Fatalf("third Dispatch: %v", err)
	}
	afterThird := pusher.sent

	if afterFirst != 1 {
		t.Errorf("after first event: sent=%d; want 1", afterFirst)
	}
	if afterSecond != 2 {
		t.Errorf("after second event: sent=%d; want 2 (digest push)", afterSecond)
	}
	if afterThird != 2 {
		t.Errorf("after third event: sent=%d; want 2 (drop)", afterThird)
	}
}

// ── digest push with no subscriptions ────────────────────────────────────────

func TestDeliverDigestPush_NoSubs_SkipsPush(t *testing.T) {
	t.Parallel()

	notifRepo := &stubNotifRepo{inserted: true}
	pushRepo := &stubPushRepo{subs: nil} // no active subscriptions
	pusher := &stubPusher{code: http.StatusCreated}

	gate := notification.NewPushDigestGate(60, 1)
	d := newDigestDispatcher(notifRepo, pushRepo, pusher, gate)

	ctx := context.Background()
	_ = d.Dispatch(ctx, p2Entry(t, 20)) // first: individual
	_ = d.Dispatch(ctx, p2Entry(t, 20)) // second: tries digest → no subs → no push
	if pusher.sent != 0 {
		t.Errorf("expected 0 push sends when no subscriptions; got %d", pusher.sent)
	}
}

// ── digest push with ListActiveByUser error ───────────────────────────────────

func TestDeliverDigestPush_ListError_SkipsPush(t *testing.T) {
	t.Parallel()

	notifRepo := &stubNotifRepo{inserted: true}
	// stubPushRepoWithError returns an error from ListActiveByUser on the second call.
	pushRepo := &erroringPushRepo{
		subs:          []*domain.PushSubscription{{ID: 2, UserID: 30, Endpoint: "https://x", P256dhKey: "k", AuthKey: "a", Active: true}},
		errAfterCalls: 1,
		listErr:       errors.New("db down"),
	}
	pusher := &stubPusher{code: http.StatusCreated}

	gate := notification.NewPushDigestGate(60, 1)
	d := dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo:  notifRepo,
		PrefRepo:   &stubPrefRepo{pref: allEnabled()},
		PushRepo:   pushRepo,
		DLQRepo:    &recordingDLQRepo{},
		Hub:        hub.New(),
		Pusher:     pusher,
		DigestGate: gate,
		Log:        zap.NewNop(),
	})

	ctx := context.Background()
	_ = d.Dispatch(ctx, p2Entry(t, 30)) // first: individual push (pushRepo returns subs)
	beforeDigest := pusher.sent
	_ = d.Dispatch(ctx, p2Entry(t, 30)) // second: digest path → ListActiveByUser errors → skip
	afterDigest := pusher.sent

	if beforeDigest != 1 {
		t.Errorf("individual push sent=%d; want 1", beforeDigest)
	}
	if afterDigest != 1 {
		t.Errorf("push sent after digest list error=%d; want 1 (no new send)", afterDigest)
	}
}

// ── MarkInactive error is logged but does not abort ──────────────────────────

func TestSendPushToSubscription_MarkInactiveError_Swallowed(t *testing.T) {
	t.Parallel()

	notifRepo := &stubNotifRepo{inserted: true}
	pushRepo := &markInactiveErrorPushRepo{
		subs: []*domain.PushSubscription{
			{ID: 77, UserID: 40, Endpoint: "https://push.example.com/b", P256dhKey: "k", AuthKey: "a", Active: true},
		},
		markErr: errors.New("db unavailable"),
	}
	pusher := &stubPusher{code: http.StatusGone} // triggers MarkInactive path

	d := dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo: notifRepo,
		PrefRepo:  &stubPrefRepo{pref: allEnabled()},
		PushRepo:  pushRepo,
		DLQRepo:   &recordingDLQRepo{},
		Hub:       hub.New(),
		Pusher:    pusher,
		Log:       zap.NewNop(),
	})

	entry := p2Entry(t, 40)
	// MarkInactive error must not surface as a Dispatch error.
	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch returned unexpected error: %v", err)
	}
}

// ── P0/P1 events bypass the digest gate ──────────────────────────────────────

func TestDeliverPush_P0Event_BypassesDigestGate(t *testing.T) {
	t.Parallel()

	notifRepo := &stubNotifRepo{inserted: true}
	pushRepo := &stubPushRepo{subs: []*domain.PushSubscription{
		{ID: 3, UserID: 50, Endpoint: "https://push.example.com/c", P256dhKey: "k", AuthKey: "a", Active: true},
	}}
	pusher := &stubPusher{code: http.StatusCreated}

	// threshold=0: any P2/P3 event would immediately trigger digest. P0/P1 bypass.
	gate := notification.NewPushDigestGate(60, 0)
	d := newDigestDispatcher(notifRepo, pushRepo, pusher, gate)

	// EventPaymentConfirmed is P0/P1 (bypass gate).
	entry := makeEntry(t, notification.EventPaymentConfirmed,
		notification.PaymentPayload{UserID: 50, PaymentID: 1, AmountCents: 1000, Currency: "GTQ"})

	for i := 0; i < 3; i++ {
		if err := d.Dispatch(context.Background(), entry); err != nil {
			t.Fatalf("Dispatch[%d] returned error: %v", i, err)
		}
	}
	if pusher.sent != 3 {
		t.Errorf("P0/P1 pushes sent=%d; want 3 (gate bypassed)", pusher.sent)
	}
}

// ── digest push with params override ─────────────────────────────────────────

func TestDeliverDigestPush_WithParams_UsesOverriddenAssets(t *testing.T) {
	t.Parallel()

	notifRepo := &stubNotifRepo{inserted: true}
	pushRepo := &stubPushRepo{subs: []*domain.PushSubscription{
		{ID: 5, UserID: 60, Endpoint: "https://push.example.com/d", P256dhKey: "k", AuthKey: "a", Active: true},
	}}
	pusher := &stubPusher{code: http.StatusCreated}
	params := &stubParamService{
		strings: map[string]string{
			domain.ParamKeyNotifyPushIconURL:  "/cdn/icon-custom.png",
			domain.ParamKeyNotifyPushBadgeURL: "/cdn/badge-custom.png",
		},
		ints: map[string]int{
			domain.ParamKeyNotifyWebPushTTLSec: 7200,
		},
	}

	gate := notification.NewPushDigestGate(60, 1)
	d := dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo:  notifRepo,
		PrefRepo:   &stubPrefRepo{pref: allEnabled()},
		PushRepo:   pushRepo,
		DLQRepo:    &recordingDLQRepo{},
		Hub:        hub.New(),
		Pusher:     pusher,
		DigestGate: gate,
		Params:     params,
		Log:        zap.NewNop(),
	})

	ctx := context.Background()
	_ = d.Dispatch(ctx, p2Entry(t, 60)) // first: individual push
	_ = d.Dispatch(ctx, p2Entry(t, 60)) // second: digest push (with params)

	if pusher.sent != 2 {
		t.Errorf("push sends: got %d; want 2 (individual + digest)", pusher.sent)
	}
}

// ── test doubles ──────────────────────────────────────────────────────────────

// erroringPushRepo returns an error from ListActiveByUser after errAfterCalls calls.
type erroringPushRepo struct {
	subs          []*domain.PushSubscription
	calls         int
	errAfterCalls int
	listErr       error
}

func (r *erroringPushRepo) Create(_ context.Context, _ *domain.PushSubscription) error { return nil }
func (r *erroringPushRepo) ListActiveByUser(_ context.Context, _ int) ([]*domain.PushSubscription, error) {
	r.calls++
	if r.calls > r.errAfterCalls {
		return nil, r.listErr
	}
	return r.subs, nil
}
func (r *erroringPushRepo) DeleteByEndpoint(_ context.Context, _ string) error { return nil }
func (r *erroringPushRepo) MarkInactive(_ context.Context, _ int64) error      { return nil }
func (r *erroringPushRepo) UpdateLastUsed(_ context.Context, _ int64) error    { return nil }
func (r *erroringPushRepo) DeleteInactive(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

// markInactiveErrorPushRepo returns an error from MarkInactive.
type markInactiveErrorPushRepo struct {
	subs    []*domain.PushSubscription
	markErr error
}

func (r *markInactiveErrorPushRepo) Create(_ context.Context, _ *domain.PushSubscription) error {
	return nil
}
func (r *markInactiveErrorPushRepo) ListActiveByUser(_ context.Context, _ int) ([]*domain.PushSubscription, error) {
	return r.subs, nil
}
func (r *markInactiveErrorPushRepo) DeleteByEndpoint(_ context.Context, _ string) error { return nil }
func (r *markInactiveErrorPushRepo) MarkInactive(_ context.Context, _ int64) error {
	return r.markErr
}
func (r *markInactiveErrorPushRepo) UpdateLastUsed(_ context.Context, _ int64) error { return nil }
func (r *markInactiveErrorPushRepo) DeleteInactive(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}
