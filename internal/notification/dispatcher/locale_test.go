package dispatcher_test

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/notification/dispatcher"
	"github.com/rede/world-cup-quiniela/internal/notification/hub"
)

// newLocaleDispatcher builds a UserDispatcher wired only for in-app persistence
// (no push, email, or pg_notify).  params controls locale resolution.
func newLocaleDispatcher(notifRepo *stubNotifRepo, params *stubParamService) *dispatcher.UserDispatcher {
	h := hub.New()
	cfg := dispatcher.UserDispatcherConfig{
		NotifRepo: notifRepo,
		PrefRepo:  &stubPrefRepo{pref: &domain.NotificationPreference{ChannelInApp: true}},
		DLQRepo:   &recordingDLQRepo{},
		Hub:       h,
		Log:       zap.NewNop(),
	}
	if params != nil {
		cfg.Params = params
	}
	return dispatcher.NewUserDispatcher(cfg)
}

func TestUserDispatcher_NilParams_DefaultsEnglish(t *testing.T) {
	t.Parallel()

	notifRepo := &stubNotifRepo{inserted: true}
	d := newLocaleDispatcher(notifRepo, nil)

	entry := makeEntry(t, notification.EventPaymentConfirmed,
		notification.PaymentPayload{UserID: 1, PaymentID: 1, AmountCents: 5000, Currency: "GTQ"})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if notifRepo.last == nil {
		t.Fatal("notification not persisted")
	}
	if notifRepo.last.Title != "Payment confirmed" {
		t.Errorf("Title: got %q; want 'Payment confirmed'", notifRepo.last.Title)
	}
}

func TestUserDispatcher_LocaleEN_UsesEnglishText(t *testing.T) {
	t.Parallel()

	notifRepo := &stubNotifRepo{inserted: true}
	params := &stubParamService{
		strings: map[string]string{domain.ParamKeyNotifyDefaultLocale: "en"},
	}
	d := newLocaleDispatcher(notifRepo, params)

	entry := makeEntry(t, notification.EventWithdrawalCompleted,
		notification.WithdrawalPayload{UserID: 1, RequestID: 5, AmountCents: 20000, Currency: "GTQ"})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if notifRepo.last == nil {
		t.Fatal("notification not persisted")
	}
	if notifRepo.last.Title != "Withdrawal completed" {
		t.Errorf("Title: got %q; want 'Withdrawal completed'", notifRepo.last.Title)
	}
}

func TestUserDispatcher_LocaleES_UsesSpanishText(t *testing.T) {
	t.Parallel()

	notifRepo := &stubNotifRepo{inserted: true}
	params := &stubParamService{
		strings: map[string]string{domain.ParamKeyNotifyDefaultLocale: "es"},
	}
	d := newLocaleDispatcher(notifRepo, params)

	entry := makeEntry(t, notification.EventPaymentConfirmed,
		notification.PaymentPayload{UserID: 1, PaymentID: 1, AmountCents: 5000, Currency: "GTQ"})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if notifRepo.last == nil {
		t.Fatal("notification not persisted")
	}
	if notifRepo.last.Title != "Pago confirmado" {
		t.Errorf("Title: got %q; want 'Pago confirmado'", notifRepo.last.Title)
	}
}

func TestUserDispatcher_LocaleES_WithdrawalSpanish(t *testing.T) {
	t.Parallel()

	notifRepo := &stubNotifRepo{inserted: true}
	params := &stubParamService{
		strings: map[string]string{domain.ParamKeyNotifyDefaultLocale: "es"},
	}
	d := newLocaleDispatcher(notifRepo, params)

	entry := makeEntry(t, notification.EventWithdrawalCompleted,
		notification.WithdrawalPayload{UserID: 1, RequestID: 5, AmountCents: 20000, Currency: "GTQ"})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if notifRepo.last == nil {
		t.Fatal("notification not persisted")
	}
	if notifRepo.last.Title != "Retiro completado" {
		t.Errorf("Title: got %q; want 'Retiro completado'", notifRepo.last.Title)
	}
}

func TestUserDispatcher_LocaleES_EmailGreetingSpanish(t *testing.T) {
	t.Parallel()

	mailer := &captureMailer{}
	resolver := &stubEmailResolver{email: "u@test.com", name: "Carlos"}
	notifRepo := &stubNotifRepo{inserted: true}
	prefRepo := &stubPrefRepo{
		pref: &domain.NotificationPreference{ChannelEmail: true},
	}
	params := &stubParamService{
		strings: map[string]string{domain.ParamKeyNotifyDefaultLocale: "es"},
	}

	d := dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo:     notifRepo,
		PrefRepo:      prefRepo,
		DLQRepo:       &recordingDLQRepo{},
		Mailer:        mailer,
		EmailResolver: resolver,
		FromAddr:      "noreply@test.com",
		Params:        params,
		Log:           zap.NewNop(),
	})

	entry := makeEntry(t, notification.EventPaymentConfirmed,
		notification.PaymentPayload{UserID: 1, PaymentID: 1, AmountCents: 5000, Currency: "GTQ"})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if len(mailer.messages) != 1 {
		t.Fatalf("emails sent: got %d; want 1", len(mailer.messages))
	}
	html := mailer.messages[0].HTML
	if !strings.Contains(html, "Hola Carlos") {
		t.Errorf("expected Spanish greeting 'Hola Carlos' in email HTML; got:\n%s", html)
	}
	if !strings.Contains(html, "Abrir aplicación") {
		t.Errorf("expected Spanish CTA 'Abrir aplicación' in email HTML; got:\n%s", html)
	}
}

func TestUserDispatcher_UnknownLocale_FallsBackToEnglish(t *testing.T) {
	t.Parallel()

	notifRepo := &stubNotifRepo{inserted: true}
	params := &stubParamService{
		strings: map[string]string{domain.ParamKeyNotifyDefaultLocale: "fr"}, // unsupported
	}
	d := newLocaleDispatcher(notifRepo, params)

	entry := makeEntry(t, notification.EventPaymentConfirmed,
		notification.PaymentPayload{UserID: 1, PaymentID: 1, AmountCents: 5000, Currency: "GTQ"})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if notifRepo.last == nil {
		t.Fatal("notification not persisted")
	}
	if notifRepo.last.Title != "Payment confirmed" {
		t.Errorf("Title: got %q; want 'Payment confirmed' (fallback for unknown locale)", notifRepo.last.Title)
	}
}
