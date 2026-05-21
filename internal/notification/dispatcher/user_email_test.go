package dispatcher_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	infraemail "github.com/rede/world-cup-quiniela/internal/infrastructure/email"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/notification/dispatcher"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// ── Email resolver stubs ──────────────────────────────────────────────────────

type stubEmailResolver struct {
	email string
	name  string
	err   error
}

func (s *stubEmailResolver) ResolveEmailByID(_ context.Context, _ int) (string, string, error) {
	return s.email, s.name, s.err
}

// ── Mailer stubs ──────────────────────────────────────────────────────────────

type captureMailer struct {
	messages []infraemail.Message
	err      error
}

func (m *captureMailer) Send(_ context.Context, msg infraemail.Message) (string, error) {
	m.messages = append(m.messages, msg)
	return "captured-id", m.err
}

// ── Test helpers ──────────────────────────────────────────────────────────────

// buildPaymentConfirmedEntry returns an outbox entry for EventPaymentConfirmed
// targeting userID 42.
func buildPaymentConfirmedEntry() *notification.OutboxEntry {
	p := notification.PaymentPayload{
		UserID:      42,
		PaymentID:   99,
		AmountCents: 5000,
		Currency:    "GTQ",
	}
	raw, _ := json.Marshal(p)
	return &notification.OutboxEntry{
		ID:          1,
		EventType:   notification.EventPaymentConfirmed,
		AggregateID: "99",
		Payload:     raw,
	}
}

// buildWithdrawalCompletedEntry returns an outbox entry for EventWithdrawalCompleted
// targeting userID 42.
func buildWithdrawalCompletedEntry() *notification.OutboxEntry {
	p := notification.WithdrawalPayload{
		UserID:      42,
		RequestID:   7,
		AmountCents: 20000,
		Currency:    "GTQ",
	}
	raw, _ := json.Marshal(p)
	return &notification.OutboxEntry{
		ID:          2,
		EventType:   notification.EventWithdrawalCompleted,
		AggregateID: "7",
		Payload:     raw,
	}
}

// newMinimalUserDispatcher constructs a UserDispatcher with only the fields
// needed to exercise the email delivery path.
func newMinimalUserDispatcher(
	notifRepo stubNotifRepo,
	prefRepo stubPrefRepo,
	mailer infraemail.Sender,
	resolver dispatcher.UserEmailResolver,
	dlq *recordingDLQRepo,
) *dispatcher.UserDispatcher {
	nr := notifRepo
	return dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo:     &nr,
		PrefRepo:      &prefRepo,
		DLQRepo:       dlq,
		Mailer:        mailer,
		EmailResolver: resolver,
		FromAddr:      "noreply@test.com",
		Log:           zap.NewNop(),
	})
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestUserDispatcher_PaymentConfirmed_DeliversEmail(t *testing.T) {
	t.Parallel()

	mailer := &captureMailer{}
	resolver := &stubEmailResolver{email: "player@test.com", name: "Alice"}
	notifRepo := stubNotifRepo{inserted: true}
	prefRepo := stubPrefRepo{
		pref: &domain.NotificationPreference{
			ChannelEmail: true, ChannelPush: false, ChannelInApp: true,
		},
	}

	d := newMinimalUserDispatcher(notifRepo, prefRepo, mailer, resolver, &recordingDLQRepo{})
	entry := buildPaymentConfirmedEntry()

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if len(mailer.messages) != 1 {
		t.Fatalf("emails sent: got %d; want 1", len(mailer.messages))
	}
	msg := mailer.messages[0]
	if msg.From != "noreply@test.com" {
		t.Errorf("From: got %q", msg.From)
	}
	if len(msg.To) == 0 || msg.To[0] != "player@test.com" {
		t.Errorf("To: got %v; want [player@test.com]", msg.To)
	}
	if msg.Subject == "" {
		t.Error("Subject must not be empty")
	}
	if msg.HTML == "" {
		t.Error("HTML must not be empty")
	}
}

func TestUserDispatcher_WithdrawalCompleted_DeliversEmail(t *testing.T) {
	t.Parallel()

	mailer := &captureMailer{}
	resolver := &stubEmailResolver{email: "user@test.com", name: "Bob"}
	notifRepo := stubNotifRepo{inserted: true}
	prefRepo := stubPrefRepo{
		pref: &domain.NotificationPreference{
			ChannelEmail: true, ChannelPush: false, ChannelInApp: true,
		},
	}

	d := newMinimalUserDispatcher(notifRepo, prefRepo, mailer, resolver, &recordingDLQRepo{})
	entry := buildWithdrawalCompletedEntry()

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if len(mailer.messages) != 1 {
		t.Fatalf("emails sent: got %d; want 1", len(mailer.messages))
	}
}

func TestUserDispatcher_EmailDisabled_SkipsEmail(t *testing.T) {
	t.Parallel()

	mailer := &captureMailer{}
	resolver := &stubEmailResolver{email: "u@test.com", name: "Carol"}
	notifRepo := stubNotifRepo{inserted: true}
	prefRepo := stubPrefRepo{
		pref: &domain.NotificationPreference{
			ChannelEmail: false, ChannelPush: false, ChannelInApp: true,
		},
	}

	d := newMinimalUserDispatcher(notifRepo, prefRepo, mailer, resolver, &recordingDLQRepo{})
	if err := d.Dispatch(context.Background(), buildPaymentConfirmedEntry()); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if len(mailer.messages) != 0 {
		t.Errorf("emails sent: got %d; want 0 (channel_email=false)", len(mailer.messages))
	}
}

func TestUserDispatcher_ResolverError_NoEmailNoDLQ(t *testing.T) {
	t.Parallel()

	mailer := &captureMailer{}
	resolver := &stubEmailResolver{err: errors.New("user not found")}
	notifRepo := stubNotifRepo{inserted: true}
	prefRepo := stubPrefRepo{
		pref: &domain.NotificationPreference{ChannelEmail: true},
	}
	dlq := &recordingDLQRepo{}

	d := newMinimalUserDispatcher(notifRepo, prefRepo, mailer, resolver, dlq)
	if err := d.Dispatch(context.Background(), buildPaymentConfirmedEntry()); err != nil {
		t.Fatalf("Dispatch must not propagate resolver error: %v", err)
	}
	if len(mailer.messages) != 0 {
		t.Errorf("emails sent: got %d; want 0 (resolver failed)", len(mailer.messages))
	}
}

func TestUserDispatcher_MailerError_WritesDLQ(t *testing.T) {
	t.Parallel()

	mailer := &captureMailer{err: errors.New("send failed")}
	resolver := &stubEmailResolver{email: "u@test.com", name: "Dan"}
	notifRepo := stubNotifRepo{inserted: true}
	prefRepo := stubPrefRepo{
		pref: &domain.NotificationPreference{ChannelEmail: true},
	}
	dlq := &recordingDLQRepo{}

	d := newMinimalUserDispatcher(notifRepo, prefRepo, mailer, resolver, dlq)
	if err := d.Dispatch(context.Background(), buildPaymentConfirmedEntry()); err != nil {
		t.Fatalf("Dispatch must not propagate mailer error: %v", err)
	}
	if len(dlq.entries) == 0 {
		t.Error("expected a DLQ entry for mailer failure; none recorded")
	}
}

func TestUserDispatcher_NoEmailResolver_SkipsEmail(t *testing.T) {
	t.Parallel()

	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	mailer := infraemail.NewResendClientWithBaseURL("key", srv.URL)
	notifRepo := stubNotifRepo{inserted: true}
	prefRepo := stubPrefRepo{
		pref: &domain.NotificationPreference{ChannelEmail: true},
	}

	// EmailResolver is nil → email skipped silently.
	d := dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo:     &notifRepo,
		PrefRepo:      &prefRepo,
		DLQRepo:       &recordingDLQRepo{},
		Mailer:        mailer,
		EmailResolver: nil,
		FromAddr:      "noreply@test.com",
		Log:           zap.NewNop(),
	})

	if err := d.Dispatch(context.Background(), buildPaymentConfirmedEntry()); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if called {
		t.Error("HTTP server was called when EmailResolver is nil — expected no email attempt")
	}
}

func TestUserDispatcher_GlobalOptOut_SkipsEmail(t *testing.T) {
	t.Parallel()

	mailer := &captureMailer{}
	resolver := &stubEmailResolver{email: "u@test.com", name: "Eve"}
	notifRepo := stubNotifRepo{inserted: true}
	// channel_email=true but the user has globally opted out of all emails.
	prefRepo := stubPrefRepo{
		pref:         &domain.NotificationPreference{ChannelEmail: true},
		globalOptOut: true,
	}

	d := newMinimalUserDispatcher(notifRepo, prefRepo, mailer, resolver, &recordingDLQRepo{})
	if err := d.Dispatch(context.Background(), buildPaymentConfirmedEntry()); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if len(mailer.messages) != 0 {
		t.Errorf("emails sent: got %d; want 0 (global email opt-out)", len(mailer.messages))
	}
}

func TestUserDispatcher_AppBaseURL_MakesActionURLAbsolute(t *testing.T) {
	t.Parallel()

	mailer := &captureMailer{}
	resolver := &stubEmailResolver{email: "u@test.com", name: "Grace"}
	notifRepo := stubNotifRepo{inserted: true}
	prefRepo := stubPrefRepo{
		pref: &domain.NotificationPreference{ChannelEmail: true},
	}

	d := dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo:     &notifRepo,
		PrefRepo:      &prefRepo,
		DLQRepo:       &recordingDLQRepo{},
		Mailer:        mailer,
		EmailResolver: resolver,
		FromAddr:      "noreply@test.com",
		AppBaseURL:    "https://quiniela.example.com",
		Log:           zap.NewNop(),
	})

	// EventPaymentConfirmed → actionURL = "/api/v1/users/me/balance" (relative).
	// With AppBaseURL set, the rendered email must contain the absolute href.
	if err := d.Dispatch(context.Background(), buildPaymentConfirmedEntry()); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if len(mailer.messages) != 1 {
		t.Fatalf("emails sent: got %d; want 1", len(mailer.messages))
	}
	html := mailer.messages[0].HTML
	if !strings.Contains(html, "https://quiniela.example.com/api/v1/users/me/balance") {
		t.Errorf("expected absolute action URL in email HTML; got:\n%s", html)
	}
}

func TestUserDispatcher_NoAppBaseURL_ActionURLUnchanged(t *testing.T) {
	t.Parallel()

	mailer := &captureMailer{}
	resolver := &stubEmailResolver{email: "u@test.com", name: "Henry"}
	notifRepo := stubNotifRepo{inserted: true}
	prefRepo := stubPrefRepo{
		pref: &domain.NotificationPreference{ChannelEmail: true},
	}

	// AppBaseURL deliberately empty — relative actionURL must pass through as-is.
	d := dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo:     &notifRepo,
		PrefRepo:      &prefRepo,
		DLQRepo:       &recordingDLQRepo{},
		Mailer:        mailer,
		EmailResolver: resolver,
		FromAddr:      "noreply@test.com",
		AppBaseURL:    "",
		Log:           zap.NewNop(),
	})

	if err := d.Dispatch(context.Background(), buildPaymentConfirmedEntry()); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if len(mailer.messages) != 1 {
		t.Fatalf("emails sent: got %d; want 1", len(mailer.messages))
	}
	html := mailer.messages[0].HTML
	// Relative path must appear as-is when no base URL is configured.
	if !strings.Contains(html, "/api/v1/users/me/balance") {
		t.Errorf("expected relative action URL path in email HTML; got:\n%s", html)
	}
}

func TestUserDispatcher_UnsubscribeURL_AppearsInEmail(t *testing.T) {
	t.Parallel()

	mailer := &captureMailer{}
	resolver := &stubEmailResolver{email: "u@test.com", name: "Frank"}
	notifRepo := stubNotifRepo{inserted: true}
	prefRepo := stubPrefRepo{
		pref: &domain.NotificationPreference{ChannelEmail: true},
	}

	d := dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo:         &notifRepo,
		PrefRepo:          &prefRepo,
		DLQRepo:           &recordingDLQRepo{},
		Mailer:            mailer,
		EmailResolver:     resolver,
		FromAddr:          "noreply@test.com",
		UnsubscribeSecret: "test-secret",
		AppBaseURL:        "https://quiniela.example.com",
		Log:               zap.NewNop(),
	})

	if err := d.Dispatch(context.Background(), buildPaymentConfirmedEntry()); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if len(mailer.messages) != 1 {
		t.Fatalf("emails sent: got %d; want 1", len(mailer.messages))
	}
	html := mailer.messages[0].HTML
	if !strings.Contains(html, "/api/v1/notifications/unsubscribe?token=") {
		t.Error("rendered HTML should contain an unsubscribe link; none found")
	}
	if !strings.Contains(html, "Unsubscribe from emails") {
		t.Error("rendered HTML should contain unsubscribe anchor text")
	}
}

func TestUserDispatcher_EmailSubjectTmpl_OverridesTitle(t *testing.T) {
	t.Parallel()

	mailer := &captureMailer{}
	resolver := &stubEmailResolver{email: "u@test.com", name: "Diana"}
	notifRepo := stubNotifRepo{inserted: true}
	prefRepo := stubPrefRepo{
		pref: &domain.NotificationPreference{ChannelEmail: true, ChannelInApp: true},
	}

	tmplRepo := &stubTemplateRepo{
		tmpl: &domain.NotificationTemplate{
			EventType:        string(notification.EventPaymentConfirmed),
			Locale:           "en",
			TitleTmpl:        "Payment confirmed",
			BodyTmpl:         "Your payment is confirmed.",
			EmailSubjectTmpl: "Your payment of {{formatCents .amount_cents .currency}} is confirmed",
		},
	}

	d := dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo:     &notifRepo,
		PrefRepo:      &prefRepo,
		DLQRepo:       &recordingDLQRepo{},
		Mailer:        mailer,
		EmailResolver: resolver,
		FromAddr:      "noreply@test.com",
		TemplateRepo:  tmplRepo,
		Log:           zap.NewNop(),
	})

	if err := d.Dispatch(context.Background(), buildPaymentConfirmedEntry()); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if len(mailer.messages) != 1 {
		t.Fatalf("emails sent: got %d; want 1", len(mailer.messages))
	}
	subject := mailer.messages[0].Subject
	if !strings.Contains(subject, "GTQ") {
		t.Errorf("Subject = %q; want DB email_subject_tmpl rendered value containing GTQ", subject)
	}
	if subject == "Payment confirmed" {
		t.Errorf("Subject = %q; email_subject_tmpl should override the notification title", subject)
	}
}

func TestUserDispatcher_EmptyEmailSubjectTmpl_FallsBackToTitle(t *testing.T) {
	t.Parallel()

	mailer := &captureMailer{}
	resolver := &stubEmailResolver{email: "u@test.com", name: "Eve"}
	notifRepo := stubNotifRepo{inserted: true}
	prefRepo := stubPrefRepo{
		pref: &domain.NotificationPreference{ChannelEmail: true, ChannelInApp: true},
	}

	tmplRepo := &stubTemplateRepo{
		tmpl: &domain.NotificationTemplate{
			EventType:        string(notification.EventPaymentConfirmed),
			Locale:           "en",
			TitleTmpl:        "Payment confirmed",
			BodyTmpl:         "Your payment is confirmed.",
			EmailSubjectTmpl: "", // empty — should fall back to title
		},
	}

	d := dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo:     &notifRepo,
		PrefRepo:      &prefRepo,
		DLQRepo:       &recordingDLQRepo{},
		Mailer:        mailer,
		EmailResolver: resolver,
		FromAddr:      "noreply@test.com",
		TemplateRepo:  tmplRepo,
		Log:           zap.NewNop(),
	})

	if err := d.Dispatch(context.Background(), buildPaymentConfirmedEntry()); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if len(mailer.messages) != 1 {
		t.Fatalf("emails sent: got %d; want 1", len(mailer.messages))
	}
	if mailer.messages[0].Subject != "Payment confirmed" {
		t.Errorf("Subject = %q; want 'Payment confirmed' (fallback to title when email_subject_tmpl empty)", mailer.messages[0].Subject)
	}
}

func TestUserDispatcher_EmailHTMLTmpl_RendersCustomTemplate(t *testing.T) {
	t.Parallel()

	mailer := &captureMailer{}
	resolver := &stubEmailResolver{email: "u@test.com", name: "Alice"}
	notifRepo := stubNotifRepo{inserted: true}
	prefRepo := stubPrefRepo{
		pref: &domain.NotificationPreference{ChannelEmail: true, ChannelInApp: true},
	}

	tmplRepo := &stubTemplateRepo{
		tmpl: &domain.NotificationTemplate{
			EventType:     string(notification.EventPaymentConfirmed),
			Locale:        "en",
			TitleTmpl:     "Payment confirmed",
			BodyTmpl:      "Your payment is confirmed.",
			EmailHTMLTmpl: `<html><body><h1>{{.Headline}}</h1><p>{{.Body}}</p></body></html>`,
		},
	}

	d := dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo:     &notifRepo,
		PrefRepo:      &prefRepo,
		DLQRepo:       &recordingDLQRepo{},
		Mailer:        mailer,
		EmailResolver: resolver,
		FromAddr:      "noreply@test.com",
		TemplateRepo:  tmplRepo,
		Log:           zap.NewNop(),
	})

	if err := d.Dispatch(context.Background(), buildPaymentConfirmedEntry()); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if len(mailer.messages) != 1 {
		t.Fatalf("emails sent: got %d; want 1", len(mailer.messages))
	}
	html := mailer.messages[0].HTML
	if !strings.Contains(html, "<h1>") {
		t.Errorf("HTML should contain <h1> from custom email_html_tmpl; got: %s", html)
	}
}

func TestUserDispatcher_EmailHTMLTmpl_ParseError_SkipsEmail(t *testing.T) {
	t.Parallel()

	mailer := &captureMailer{}
	resolver := &stubEmailResolver{email: "u@test.com", name: "Bob"}
	notifRepo := stubNotifRepo{inserted: true}
	prefRepo := stubPrefRepo{
		pref: &domain.NotificationPreference{ChannelEmail: true, ChannelInApp: true},
	}

	tmplRepo := &stubTemplateRepo{
		tmpl: &domain.NotificationTemplate{
			EventType:     string(notification.EventPaymentConfirmed),
			Locale:        "en",
			TitleTmpl:     "Payment confirmed",
			BodyTmpl:      "Your payment is confirmed.",
			EmailHTMLTmpl: `{{.unclosed`,
		},
	}

	d := dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo:     &notifRepo,
		PrefRepo:      &prefRepo,
		DLQRepo:       &recordingDLQRepo{},
		Mailer:        mailer,
		EmailResolver: resolver,
		FromAddr:      "noreply@test.com",
		TemplateRepo:  tmplRepo,
		Log:           zap.NewNop(),
	})

	if err := d.Dispatch(context.Background(), buildPaymentConfirmedEntry()); err != nil {
		t.Fatalf("Dispatch must not propagate render error: %v", err)
	}
	if len(mailer.messages) != 0 {
		t.Errorf("emails sent: got %d; want 0 (email_html_tmpl parse error should silently skip)", len(mailer.messages))
	}
}
