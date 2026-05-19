package dispatcher_test

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	infrapush "github.com/rede/world-cup-quiniela/internal/infrastructure/webpush"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/notification/dispatcher"
	"github.com/rede/world-cup-quiniela/internal/notification/hub"
)

func newTestComposite(
	adminEmails string,
	notifRepo *stubNotifRepo,
	prefRepo *stubPrefRepo,
) *dispatcher.CompositeDispatcher {
	nop := zap.NewNop()
	h := hub.New()

	adminD := dispatcher.NewAdminDispatcher(dispatcher.Config{
		Params:   &stubParams{emails: adminEmails},
		LogRepo:  &recordingLogRepo{},
		DLQRepo:  &recordingDLQRepo{},
		Mailer:   &stubMailer{msgID: "ok"},
		FromAddr: "Quiniela <noreply@test.com>",
		Log:      nop,
	})

	userD := dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo: notifRepo,
		PrefRepo:  prefRepo,
		PushRepo:  &stubPushRepo{},
		DLQRepo:   &recordingDLQRepo{},
		Hub:       h,
		Pusher:    infrapush.NoopSender{},
		Log:       nop,
	})

	return dispatcher.NewCompositeDispatcher(adminD, userD)
}

func TestCompositeDispatcher_AdminEvent_RoutesToAdmin(t *testing.T) {
	t.Parallel()
	notifRepo := &stubNotifRepo{inserted: true}
	pref := &stubPrefRepo{err: errors.New("no row")}
	d := newTestComposite("admin@test.com", notifRepo, pref)

	entry := makeEntry(t, notification.EventAdminBankTransferPending,
		notification.AdminBankTransferPayload{ProofID: 1, UserID: 2, AmountCents: 50000, Currency: "GTQ"})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	// Admin event must NOT reach user notification repo.
	if notifRepo.last != nil {
		t.Error("admin event should not be persisted in user notification repo")
	}
}

func TestCompositeDispatcher_UserEvent_RoutesToUser(t *testing.T) {
	t.Parallel()
	notifRepo := &stubNotifRepo{inserted: true}
	pref := &stubPrefRepo{err: errors.New("no row")}
	d := newTestComposite("", notifRepo, pref)

	entry := makeEntry(t, notification.EventPredictionConfirmed,
		notification.PredictionConfirmedPayload{UserID: 5, PredictionID: 1, MatchID: 2, HomeTeam: "MX", AwayTeam: "US"})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	// User event must reach user notification repo.
	if notifRepo.last == nil {
		t.Error("user event should be persisted in user notification repo")
	}
}

func TestCompositeDispatcher_SystemEvent_RoutesToAdmin(t *testing.T) {
	t.Parallel()
	notifRepo := &stubNotifRepo{inserted: true}
	pref := &stubPrefRepo{err: errors.New("no row")}
	d := newTestComposite("ops@test.com", notifRepo, pref)

	entry := makeEntry(t, notification.EventSystemCircuitBreakerOpened,
		notification.SystemAlertPayload{Component: "db", Detail: "timeout", Severity: "critical"})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if notifRepo.last != nil {
		t.Error("system event should not reach user notification repo")
	}
}
