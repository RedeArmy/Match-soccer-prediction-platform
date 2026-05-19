package dispatcher_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/domain"
	infraemail "github.com/rede/world-cup-quiniela/internal/infrastructure/email"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/notification/dispatcher"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ── Test doubles ─────────────────────────────────────────────────────────────

type stubParams struct{ emails string }

func (s *stubParams) GetString(_ context.Context, key, defaultVal string) string {
	if key == domain.ParamKeyNotifyAdminEmails {
		return s.emails
	}
	return defaultVal
}

type recordingLogRepo struct {
	entries []*domain.AdminNotificationLog
}

func (r *recordingLogRepo) Create(_ context.Context, e *domain.AdminNotificationLog) error {
	r.entries = append(r.entries, e)
	e.ID = int64(len(r.entries))
	e.CreatedAt = time.Now()
	return nil
}

var _ repository.AdminNotificationLogCreator = (*recordingLogRepo)(nil)

type recordingDLQRepo struct {
	entries []*domain.NotificationDLQEntry
}

func (r *recordingDLQRepo) CreateEntry(_ context.Context, e *domain.NotificationDLQEntry) error {
	r.entries = append(r.entries, e)
	return nil
}

var _ repository.NotificationDLQEntryCreator = (*recordingDLQRepo)(nil)

type stubMailer struct {
	sendErr error
	msgID   string
	calls   int
}

func (s *stubMailer) Send(_ context.Context, _ infraemail.Message) (string, error) {
	s.calls++
	return s.msgID, s.sendErr
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func makeEntry(t *testing.T, et notification.EventType, payload any) *notification.OutboxEntry {
	t.Helper()
	entry, err := notification.NewOutboxEntry(et, "test", "1", payload)
	if err != nil {
		t.Fatalf("NewOutboxEntry: %v", err)
	}
	entry.ID = 99
	return entry
}

func newDispatcher(
	log *zap.Logger,
	emails string,
	mailer infraemail.Sender,
	logRepo repository.AdminNotificationLogCreator,
	dlqRepo repository.NotificationDLQEntryCreator,
	n8nURL string,
) *dispatcher.AdminDispatcher {
	return dispatcher.NewAdminDispatcher(dispatcher.Config{
		Params:   &stubParams{emails: emails},
		LogRepo:  logRepo,
		DLQRepo:  dlqRepo,
		Mailer:   mailer,
		FromAddr: "Quiniela <noreply@test.com>",
		N8nURL:   n8nURL,
		Log:      log,
	})
}

// ── Tests ────────────────────────────────────────────────────────────────────

func TestAdminDispatcher_NonAdminEvent_Noop(t *testing.T) {
	t.Parallel()
	log := zaptest.NewLogger(t)
	mailer := &stubMailer{}
	logRepo := &recordingLogRepo{}
	dlqRepo := &recordingDLQRepo{}

	d := newDispatcher(log, "admin@test.com", mailer, logRepo, dlqRepo, "")

	entry := makeEntry(t, notification.EventPredictionConfirmed,
		notification.PredictionConfirmedPayload{UserID: 1, MatchID: 2})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch returned unexpected error: %v", err)
	}
	if mailer.calls != 0 {
		t.Errorf("mailer.Send called %d times; want 0 for non-admin event", mailer.calls)
	}
	if len(logRepo.entries) != 0 {
		t.Errorf("log entries written: %d; want 0", len(logRepo.entries))
	}
}

func TestAdminDispatcher_NoRecipientsConfigured_Noop(t *testing.T) {
	t.Parallel()
	log := zaptest.NewLogger(t)
	mailer := &stubMailer{}
	logRepo := &recordingLogRepo{}
	dlqRepo := &recordingDLQRepo{}

	d := newDispatcher(log, "", mailer, logRepo, dlqRepo, "")

	entry := makeEntry(t, notification.EventAdminBankTransferPending,
		notification.AdminBankTransferPayload{ProofID: 1, UserID: 2, AmountCents: 50000, Currency: "GTQ"})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch returned unexpected error: %v", err)
	}
	if mailer.calls != 0 {
		t.Errorf("mailer.Send called %d times; want 0 when no recipients", mailer.calls)
	}
}

func TestAdminDispatcher_SuccessfulDelivery(t *testing.T) {
	t.Parallel()
	log := zaptest.NewLogger(t)
	mailer := &stubMailer{msgID: "resend-msg-123"}
	logRepo := &recordingLogRepo{}
	dlqRepo := &recordingDLQRepo{}

	d := newDispatcher(log, "admin@test.com", mailer, logRepo, dlqRepo, "")

	entry := makeEntry(t, notification.EventAdminBankTransferPending,
		notification.AdminBankTransferPayload{ProofID: 7, UserID: 42, AmountCents: 100000, Currency: "GTQ"})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if mailer.calls != 1 {
		t.Errorf("mailer.Send calls: got %d; want 1", mailer.calls)
	}
	if len(logRepo.entries) != 1 {
		t.Fatalf("log entries: got %d; want 1", len(logRepo.entries))
	}
	le := logRepo.entries[0]
	if le.Status != domain.AdminNotifStatusSent {
		t.Errorf("log status: got %q; want %q", le.Status, domain.AdminNotifStatusSent)
	}
	if le.ResendMsgID != "resend-msg-123" {
		t.Errorf("log resend_msg_id: got %q; want resend-msg-123", le.ResendMsgID)
	}
	if len(dlqRepo.entries) != 0 {
		t.Errorf("DLQ entries: got %d; want 0 on success", len(dlqRepo.entries))
	}
}

func TestAdminDispatcher_EmailFailure_WritesDLQ_ReturnsError(t *testing.T) {
	t.Parallel()
	log := zaptest.NewLogger(t)
	sendErr := errors.New("resend: status 503")
	mailer := &stubMailer{sendErr: sendErr}
	logRepo := &recordingLogRepo{}
	dlqRepo := &recordingDLQRepo{}

	d := newDispatcher(log, "admin@test.com", mailer, logRepo, dlqRepo, "")

	entry := makeEntry(t, notification.EventAdminWithdrawalPending,
		notification.AdminWithdrawalPayload{RequestID: 3, UserID: 9, AmountCents: 250000, Currency: "GTQ"})

	err := d.Dispatch(context.Background(), entry)
	if err == nil {
		t.Fatal("expected error on email failure; got nil")
	}

	if len(logRepo.entries) != 1 {
		t.Fatalf("log entries: got %d; want 1", len(logRepo.entries))
	}
	if logRepo.entries[0].Status != domain.AdminNotifStatusFailed {
		t.Errorf("log status: got %q; want failed", logRepo.entries[0].Status)
	}
	if len(dlqRepo.entries) != 1 {
		t.Fatalf("DLQ entries: got %d; want 1 on failure", len(dlqRepo.entries))
	}
	dlq := dlqRepo.entries[0]
	if dlq.Channel != "email" {
		t.Errorf("DLQ channel: got %q; want email", dlq.Channel)
	}
	if *dlq.OutboxID != entry.ID {
		t.Errorf("DLQ outbox_id: got %d; want %d", *dlq.OutboxID, entry.ID)
	}
}

func TestAdminDispatcher_MultipleRecipients(t *testing.T) {
	t.Parallel()
	log := zaptest.NewLogger(t)
	var lastMsg infraemail.Message
	mailer := &struct {
		infraemail.Sender
		stubMailer
	}{}
	captureMailer := &capturingMailer{}
	logRepo := &recordingLogRepo{}
	dlqRepo := &recordingDLQRepo{}

	d := newDispatcher(log, "a@test.com, b@test.com, c@test.com", captureMailer, logRepo, dlqRepo, "")

	entry := makeEntry(t, notification.EventAdminHighValueWithdrawal,
		notification.AdminWithdrawalPayload{RequestID: 1, UserID: 5, AmountCents: 2_000_000, Currency: "GTQ"})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	lastMsg = captureMailer.last
	_ = mailer
	if len(lastMsg.To) != 3 {
		t.Errorf("recipients: got %d; want 3", len(lastMsg.To))
	}
}

func TestAdminDispatcher_SystemEvent_FiresN8nWebhook(t *testing.T) {
	t.Parallel()

	var webhookHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			webhookHits.Add(1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	log := zaptest.NewLogger(t)
	mailer := &stubMailer{msgID: "msg-abc"}
	logRepo := &recordingLogRepo{}
	dlqRepo := &recordingDLQRepo{}

	d := newDispatcher(log, "admin@test.com", mailer, logRepo, dlqRepo, srv.URL)

	entry := makeEntry(t, notification.EventSystemCircuitBreakerOpened,
		notification.SystemAlertPayload{Component: "payment-gateway", Detail: "timeout", Severity: "critical"})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	// Give the best-effort webhook goroutine a moment to complete.
	time.Sleep(50 * time.Millisecond)

	if webhookHits.Load() != 1 {
		t.Errorf("n8n webhook hits: got %d; want 1", webhookHits.Load())
	}
}

func TestAdminDispatcher_BalanceLedgerMismatch_CriticalSubject(t *testing.T) {
	t.Parallel()
	log := zaptest.NewLogger(t)
	captureMailer := &capturingMailer{}
	logRepo := &recordingLogRepo{}
	dlqRepo := &recordingDLQRepo{}

	d := newDispatcher(log, "admin@test.com", captureMailer, logRepo, dlqRepo, "")

	entry := makeEntry(t, notification.EventSystemBalanceLedgerMismatch,
		notification.SystemAlertPayload{Component: "ledger", Detail: "sum mismatch", Severity: "critical"})

	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !containsCI(captureMailer.last.Subject, "CRITICAL") {
		t.Errorf("subject %q does not contain CRITICAL", captureMailer.last.Subject)
	}
}

func TestAdminDispatcher_WriteLog_RepoError_Swallowed(t *testing.T) {
	t.Parallel()
	log := zaptest.NewLogger(t)
	mailer := &stubMailer{msgID: "ok-id"}
	dlqRepo := &recordingDLQRepo{}

	d := newDispatcher(log, "admin@test.com", mailer, &errorLogRepo{}, dlqRepo, "")

	entry := makeEntry(t, notification.EventAdminBankTransferPending,
		notification.AdminBankTransferPayload{ProofID: 1, UserID: 2, AmountCents: 100, Currency: "GTQ"})

	// log-write failure must not propagate — Dispatch should return nil
	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Errorf("Dispatch returned unexpected error when log repo fails: %v", err)
	}
}

func TestAdminDispatcher_WriteDLQ_RepoError_Swallowed(t *testing.T) {
	t.Parallel()
	log := zaptest.NewLogger(t)
	sendErr := errors.New("resend: status 503")
	mailer := &stubMailer{sendErr: sendErr}
	logRepo := &recordingLogRepo{}

	d := newDispatcher(log, "admin@test.com", mailer, logRepo, &errorDLQRepo{}, "")

	entry := makeEntry(t, notification.EventAdminWithdrawalPending,
		notification.AdminWithdrawalPayload{RequestID: 1, UserID: 2, AmountCents: 500, Currency: "GTQ"})

	// DLQ-write failure is swallowed; Dispatch returns the email error
	err := d.Dispatch(context.Background(), entry)
	if err == nil {
		t.Fatal("expected email error to propagate even when DLQ write fails")
	}
}

func TestAdminDispatcher_N8nWebhook_Non2xx_Swallowed(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	log := zaptest.NewLogger(t)
	mailer := &stubMailer{msgID: "msg-ok"}
	logRepo := &recordingLogRepo{}
	dlqRepo := &recordingDLQRepo{}

	d := newDispatcher(log, "admin@test.com", mailer, logRepo, dlqRepo, srv.URL)

	entry := makeEntry(t, notification.EventSystemCircuitBreakerOpened,
		notification.SystemAlertPayload{Component: "db", Detail: "timeout", Severity: "critical"})

	// n8n 503 is best-effort — Dispatch must still return nil
	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Errorf("Dispatch returned unexpected error on n8n 503: %v", err)
	}
}

func TestAdminDispatcher_N8nWebhook_RequestFailed_Swallowed(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.Close() // close immediately so every connection attempt fails

	log := zaptest.NewLogger(t)
	mailer := &stubMailer{msgID: "msg-ok"}
	logRepo := &recordingLogRepo{}
	dlqRepo := &recordingDLQRepo{}

	d := newDispatcher(log, "admin@test.com", mailer, logRepo, dlqRepo, srv.URL)

	entry := makeEntry(t, notification.EventSystemCircuitBreakerOpened,
		notification.SystemAlertPayload{Component: "cache", Detail: "connect failed", Severity: "critical"})

	// HTTP failure on n8n is best-effort — Dispatch must still return nil
	if err := d.Dispatch(context.Background(), entry); err != nil {
		t.Errorf("Dispatch returned unexpected error on n8n request failure: %v", err)
	}
}

// ── Helpers ──────────────────────────────────────────────────────────────────

type errorLogRepo struct{}

func (r *errorLogRepo) Create(_ context.Context, _ *domain.AdminNotificationLog) error {
	return errors.New("log repo: simulated write failure")
}

var _ repository.AdminNotificationLogCreator = (*errorLogRepo)(nil)

type errorDLQRepo struct{}

func (r *errorDLQRepo) CreateEntry(_ context.Context, _ *domain.NotificationDLQEntry) error {
	return errors.New("dlq repo: simulated write failure")
}

var _ repository.NotificationDLQEntryCreator = (*errorDLQRepo)(nil)

type capturingMailer struct{ last infraemail.Message }

func (m *capturingMailer) Send(_ context.Context, msg infraemail.Message) (string, error) {
	m.last = msg
	return "cap-id", nil
}

func containsCI(s, substr string) bool {
	return len(s) >= len(substr) &&
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if equalFold(s[i:i+len(substr)], substr) {
					return true
				}
			}
			return false
		}()
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ca, cb := a[i], b[i]
		if ca >= 'a' && ca <= 'z' {
			ca -= 32
		}
		if cb >= 'a' && cb <= 'z' {
			cb -= 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}
