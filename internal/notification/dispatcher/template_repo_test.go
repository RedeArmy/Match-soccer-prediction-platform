package dispatcher_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/notification/dispatcher"
	"github.com/rede/world-cup-quiniela/internal/notification/hub"
)

// stubTemplateRepo implements repository.NotificationTemplateRepository for tests.
type stubTemplateRepo struct {
	tmpl *domain.NotificationTemplate
	err  error
}

func (s *stubTemplateRepo) Get(_ context.Context, _, _ string) (*domain.NotificationTemplate, error) {
	return s.tmpl, s.err
}
func (s *stubTemplateRepo) List(_ context.Context) ([]*domain.NotificationTemplate, error) {
	return nil, nil
}
func (s *stubTemplateRepo) Upsert(_ context.Context, _ *domain.NotificationTemplate) error {
	return nil
}
func (s *stubTemplateRepo) Delete(_ context.Context, _, _ string) error { return nil }
func (s *stubTemplateRepo) ListHistory(_ context.Context, _, _ string, _ int) ([]*domain.NotificationTemplateHistory, error) {
	return nil, nil
}
func (s *stubTemplateRepo) GetHistoryEntry(_ context.Context, _ int64, _, _ string) (*domain.NotificationTemplateHistory, error) {
	return nil, nil
}

func newTemplateDispatcher(notifRepo *stubNotifRepo, tmplRepo *stubTemplateRepo) *dispatcher.UserDispatcher {
	h := hub.New()
	return dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo:    notifRepo,
		PrefRepo:     &stubPrefRepo{pref: &domain.NotificationPreference{ChannelInApp: true}},
		DLQRepo:      &recordingDLQRepo{},
		Hub:          h,
		TemplateRepo: tmplRepo,
		Log:          zap.NewNop(),
	})
}

func TestResolveContent_TemplateRepo_HappyPath(t *testing.T) {
	t.Parallel()

	tmplRepo := &stubTemplateRepo{
		tmpl: &domain.NotificationTemplate{
			EventType:     string(notification.EventPaymentConfirmed),
			Locale:        "en",
			TitleTmpl:     "DB: Payment of {{formatCents .amount_cents .currency}} confirmed",
			BodyTmpl:      "DB: Your payment is confirmed.",
			ActionURLTmpl: "",
			UpdatedAt:     time.Now(),
		},
	}
	notifRepo := &stubNotifRepo{inserted: true}
	d := newTemplateDispatcher(notifRepo, tmplRepo)

	entry := makeEntry(t, notification.EventPaymentConfirmed,
		notification.PaymentPayload{UserID: 1, PaymentID: 1, AmountCents: 5000, Currency: "GTQ"})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if notifRepo.last == nil {
		t.Fatal("notification not persisted")
	}
	if notifRepo.last.Title != "DB: Payment of 50.00 GTQ confirmed" {
		t.Errorf("Title: got %q; want DB template title", notifRepo.last.Title)
	}
	if notifRepo.last.Body != "DB: Your payment is confirmed." {
		t.Errorf("Body: got %q; want DB template body", notifRepo.last.Body)
	}
}

func TestResolveContent_TemplateRepo_StoreError_FallsBackToCompiled(t *testing.T) {
	t.Parallel()

	tmplRepo := &stubTemplateRepo{err: errors.New("db unavailable")}
	notifRepo := &stubNotifRepo{inserted: true}
	d := newTemplateDispatcher(notifRepo, tmplRepo)

	entry := makeEntry(t, notification.EventPaymentConfirmed,
		notification.PaymentPayload{UserID: 1, PaymentID: 1, AmountCents: 5000, Currency: "GTQ"})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch must not fail on store error: %v", err)
	}
	if notifRepo.last == nil {
		t.Fatal("notification not persisted")
	}
	// Compiled default title for EventPaymentConfirmed in English.
	if notifRepo.last.Title != "Payment confirmed" {
		t.Errorf("Title: got %q; want compiled default 'Payment confirmed'", notifRepo.last.Title)
	}
}

func TestResolveContent_TemplateRepo_NilTemplate_FallsBackToCompiled(t *testing.T) {
	t.Parallel()

	tmplRepo := &stubTemplateRepo{tmpl: nil, err: nil} // miss: no row for this event
	notifRepo := &stubNotifRepo{inserted: true}
	d := newTemplateDispatcher(notifRepo, tmplRepo)

	entry := makeEntry(t, notification.EventPaymentConfirmed,
		notification.PaymentPayload{UserID: 1, PaymentID: 1, AmountCents: 5000, Currency: "GTQ"})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if notifRepo.last == nil {
		t.Fatal("notification not persisted")
	}
	if notifRepo.last.Title != "Payment confirmed" {
		t.Errorf("Title: got %q; want compiled default 'Payment confirmed'", notifRepo.last.Title)
	}
}

func TestResolveContent_TemplateRepo_BadTemplateSyntax_FallsBackToCompiled(t *testing.T) {
	t.Parallel()

	tmplRepo := &stubTemplateRepo{
		tmpl: &domain.NotificationTemplate{
			TitleTmpl: "{{.unclosed", // invalid — render will fail
			BodyTmpl:  "body",
			UpdatedAt: time.Now(),
		},
	}
	notifRepo := &stubNotifRepo{inserted: true}
	d := newTemplateDispatcher(notifRepo, tmplRepo)

	entry := makeEntry(t, notification.EventPaymentConfirmed,
		notification.PaymentPayload{UserID: 1, PaymentID: 1, AmountCents: 5000, Currency: "GTQ"})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch must not fail on bad template: %v", err)
	}
	if notifRepo.last == nil {
		t.Fatal("notification not persisted")
	}
	if notifRepo.last.Title != "Payment confirmed" {
		t.Errorf("Title: got %q; want compiled default 'Payment confirmed'", notifRepo.last.Title)
	}
}
