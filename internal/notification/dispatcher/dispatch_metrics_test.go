package dispatcher_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/notification/dispatcher"
)

// ── AdminDispatcher: metric-recording paths ───────────────────────────────────

// TestAdminDispatcher_WithMetrics_SuccessfulDispatch exercises the success
// branch of recordAdminDispatch (events counter, duration histogram,
// emailStatus="sent") when OTel instruments are registered.
func TestAdminDispatcher_WithMetrics_SuccessfulDispatch(t *testing.T) {
	t.Parallel()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	mailer := &stubMailer{msgID: "msg-metrics-ok"}
	d := newDispatcher(zap.NewNop(), "admin@test.com", mailer, &recordingLogRepo{}, &recordingDLQRepo{}, "", "")
	if err := d.RegisterMetrics(mp.Meter("test")); err != nil {
		t.Fatalf("RegisterMetrics: %v", err)
	}

	entry := makeEntry(t, notification.EventAdminBankTransferPending,
		notification.AdminBankTransferPayload{ProofID: 1, UserID: 2, AmountCents: 100, Currency: "GTQ"})
	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if mailer.calls != 1 {
		t.Errorf("mailer.Send calls: got %d; want 1", mailer.calls)
	}
}

// TestAdminDispatcher_WithMetrics_FailedDispatch exercises the failure branch
// of recordAdminDispatch (emailStatus="failed") when email delivery fails and
// OTel instruments are registered.
func TestAdminDispatcher_WithMetrics_FailedDispatch(t *testing.T) {
	t.Parallel()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	mailer := &stubMailer{sendErr: errors.New("resend: 503")}
	d := newDispatcher(zap.NewNop(), "admin@test.com", mailer, &recordingLogRepo{}, &recordingDLQRepo{}, "", "")
	if err := d.RegisterMetrics(mp.Meter("test")); err != nil {
		t.Fatalf("RegisterMetrics: %v", err)
	}

	entry := makeEntry(t, notification.EventAdminBankTransferPending,
		notification.AdminBankTransferPayload{ProofID: 1, UserID: 2, AmountCents: 100, Currency: "GTQ"})
	if err := d.Dispatch(context.Background(), entry); err == nil {
		t.Fatal("expected error on email failure; got nil")
	}
}

// ── UserDispatcher: metric-recording paths ────────────────────────────────────

// TestUserDispatcher_WithMetrics_SuccessPath exercises:
//   - recordDispatch with status="success" (events counter + duration histogram)
//   - push instruments with status="sent" (user_push.go)
//   - email instruments with status="sent" (user_email.go)
func TestUserDispatcher_WithMetrics_SuccessPath(t *testing.T) {
	t.Parallel()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	notifRepo := &stubNotifRepo{inserted: true}
	pushRepo := &stubPushRepo{subs: []*domain.PushSubscription{
		{ID: 1, UserID: 7, Endpoint: "https://push.example.com/metrics-ok", P256dhKey: "k", AuthKey: "a", Active: true},
	}}
	pusher := &stubPusher{code: http.StatusCreated}
	mailer := &captureMailer{}
	resolver := &stubEmailResolver{email: "u@test.com", name: "Test"}

	d := dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo:     notifRepo,
		PrefRepo:      &stubPrefRepo{pref: allEnabled()},
		PushRepo:      pushRepo,
		DLQRepo:       &recordingDLQRepo{},
		Pusher:        pusher,
		Mailer:        mailer,
		EmailResolver: resolver,
		FromAddr:      "noreply@test.com",
		Log:           zap.NewNop(),
	})
	if err := d.RegisterMetrics(mp.Meter("test")); err != nil {
		t.Fatalf("RegisterMetrics: %v", err)
	}

	// EventPaymentConfirmed is P1High → email is delivered; push is delivered.
	entry := makeEntry(t, notification.EventPaymentConfirmed,
		notification.PaymentPayload{UserID: 7, PaymentID: 1, AmountCents: 500, Currency: "GTQ"})
	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if pusher.sent != 1 {
		t.Errorf("push sent: got %d; want 1", pusher.sent)
	}
	if len(mailer.messages) != 1 {
		t.Errorf("emails sent: got %d; want 1", len(mailer.messages))
	}
}

// TestUserDispatcher_WithMetrics_PushSendError exercises the push instruments
// with status="failed" when the push service returns a network error.
func TestUserDispatcher_WithMetrics_PushSendError(t *testing.T) {
	t.Parallel()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	notifRepo := &stubNotifRepo{inserted: true}
	pushRepo := &stubPushRepo{subs: []*domain.PushSubscription{
		{ID: 2, UserID: 8, Endpoint: "https://push.example.com/net-err", P256dhKey: "k", AuthKey: "a", Active: true},
	}}
	pusher := &stubPusher{err: errors.New("network error")}

	d := dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo: notifRepo,
		PrefRepo:  &stubPrefRepo{pref: allEnabled()},
		PushRepo:  pushRepo,
		DLQRepo:   &recordingDLQRepo{},
		Pusher:    pusher,
		Log:       zap.NewNop(),
	})
	if err := d.RegisterMetrics(mp.Meter("test")); err != nil {
		t.Fatalf("RegisterMetrics: %v", err)
	}

	entry := makeEntry(t, notification.EventPaymentConfirmed,
		notification.PaymentPayload{UserID: 8, PaymentID: 2, AmountCents: 100, Currency: "GTQ"})
	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch must not propagate push error: %v", err)
	}
}

// TestUserDispatcher_WithMetrics_PushGone exercises the push instruments with
// status="failed" via the HTTP 410 Gone (expired subscription) path.
func TestUserDispatcher_WithMetrics_PushGone(t *testing.T) {
	t.Parallel()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	notifRepo := &stubNotifRepo{inserted: true}
	pushRepo := &stubPushRepo{subs: []*domain.PushSubscription{
		{ID: 3, UserID: 9, Endpoint: "https://push.example.com/gone", P256dhKey: "k", AuthKey: "a", Active: true},
	}}
	pusher := &stubPusher{code: http.StatusGone}

	d := dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo: notifRepo,
		PrefRepo:  &stubPrefRepo{pref: allEnabled()},
		PushRepo:  pushRepo,
		DLQRepo:   &recordingDLQRepo{},
		Pusher:    pusher,
		Log:       zap.NewNop(),
	})
	if err := d.RegisterMetrics(mp.Meter("test")); err != nil {
		t.Fatalf("RegisterMetrics: %v", err)
	}

	entry := makeEntry(t, notification.EventPaymentConfirmed,
		notification.PaymentPayload{UserID: 9, PaymentID: 3, AmountCents: 100, Currency: "GTQ"})
	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch must not propagate 410 Gone: %v", err)
	}
}

// TestUserDispatcher_WithMetrics_EmailSendFailure exercises the email
// instruments with status="failed" when the mailer returns an error.
func TestUserDispatcher_WithMetrics_EmailSendFailure(t *testing.T) {
	t.Parallel()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	notifRepo := &stubNotifRepo{inserted: true}
	mailer := &captureMailer{err: errors.New("send failed")}
	resolver := &stubEmailResolver{email: "u@test.com", name: "Test"}

	d := dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo:     notifRepo,
		PrefRepo:      &stubPrefRepo{pref: allEnabled()},
		DLQRepo:       &recordingDLQRepo{},
		Mailer:        mailer,
		EmailResolver: resolver,
		FromAddr:      "noreply@test.com",
		Log:           zap.NewNop(),
	})
	if err := d.RegisterMetrics(mp.Meter("test")); err != nil {
		t.Fatalf("RegisterMetrics: %v", err)
	}

	entry := makeEntry(t, notification.EventPaymentConfirmed,
		notification.PaymentPayload{UserID: 10, PaymentID: 4, AmountCents: 100, Currency: "GTQ"})
	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch must not propagate email error: %v", err)
	}
}

// TestUserDispatcher_WithMetrics_PersistFailure exercises the recordDispatch
// failure branch (status="failed") when notification persistence fails.
func TestUserDispatcher_WithMetrics_PersistFailure(t *testing.T) {
	t.Parallel()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	notifRepo := &stubNotifRepo{err: errors.New("db error")}

	d := dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo: notifRepo,
		PrefRepo:  &stubPrefRepo{pref: allEnabled()},
		DLQRepo:   &recordingDLQRepo{},
		Log:       zap.NewNop(),
	})
	if err := d.RegisterMetrics(mp.Meter("test")); err != nil {
		t.Fatalf("RegisterMetrics: %v", err)
	}

	entry := makeEntry(t, notification.EventPaymentConfirmed,
		notification.PaymentPayload{UserID: 11, PaymentID: 5, AmountCents: 100, Currency: "GTQ"})
	if err := d.Dispatch(context.Background(), entry); err == nil {
		t.Fatal("expected error on persist failure; got nil")
	}
}

// TestUserDispatcher_WithMetrics_DroppedRecipient exercises the recordDispatch
// "dropped" branch when the recipient cannot be resolved from the payload.
func TestUserDispatcher_WithMetrics_DroppedRecipient(t *testing.T) {
	t.Parallel()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	notifRepo := &stubNotifRepo{}

	d := dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo: notifRepo,
		PrefRepo:  &stubPrefRepo{},
		DLQRepo:   &recordingDLQRepo{},
		Log:       zap.NewNop(),
	})
	if err := d.RegisterMetrics(mp.Meter("test")); err != nil {
		t.Fatalf("RegisterMetrics: %v", err)
	}

	// SystemAlertPayload has no user_id: resolveRecipient returns (0, false).
	entry := makeEntry(t, notification.EventPredictionConfirmed,
		notification.SystemAlertPayload{Component: "test"})
	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch must not error when recipient is unresolvable: %v", err)
	}
	if notifRepo.last != nil {
		t.Error("dropped event must not be persisted")
	}
}
