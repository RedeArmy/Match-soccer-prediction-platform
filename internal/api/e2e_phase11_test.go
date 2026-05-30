//go:build integration

package api_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/api"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/messaging"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/pkg/config"
)

// newE2EServerWithRecurrente builds a Server wired to e2eDB with the given
// Recurrente webhook secret so that RecurrenteWebhookAuth performs real
// HMAC-SHA256 verification (non-bypass mode). PayPalWebhookID is left empty,
// which activates PayPal bypass mode; use the base newE2EServerUnlimited when
// only PayPal is being tested.
func newE2EServerWithRecurrente(t *testing.T, jwksURL, recurrenteSecret string) *api.Server {
	t.Helper()
	cfg := &config.Config{}
	cfg.Clerk.JWKSURL = jwksURL
	cfg.Payment.RecurrenteWebhookSecret = recurrenteSecret
	srv := api.New(e2eDB, cfg, zaptest.NewLogger(t), messaging.NewInMemoryBus(nil), nil, nil)
	srv.SetLimiterStore(middleware.NewUnlimitedLimiterStore())
	return srv
}

// recurrenteSign computes the HMAC-SHA256 hex digest over body using secret,
// mirroring the algorithm in middleware.RecurrenteWebhookAuth.
func recurrenteSign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// doWebhookRequest fires a POST webhook request through h. headers is a map of
// additional HTTP headers (e.g. the HMAC signature header). No Authorization
// header is set — webhook endpoints sit outside the JWT auth middleware.
func doWebhookRequest(t *testing.T, h http.Handler, path string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// seedE2EPaymentIntent inserts a pending payment_intent into e2eDB and returns
// nothing. The intent expires 30 minutes from now, which is sufficient for any
// test run. token must be globally unique within the test — use a per-test
// constant or a function-scoped prefix.
func seedE2EPaymentIntent(t *testing.T, userID, amountCents int, currency, token string) {
	t.Helper()
	_, err := e2eDB.Exec(context.Background(),
		`INSERT INTO payment_intents (token, user_id, amount_cents, currency, expires_at)
		 VALUES ($1, $2, $3, $4, NOW() + INTERVAL '30 minutes')`,
		token, userID, amountCents, currency,
	)
	if err != nil {
		t.Fatalf("seedE2EPaymentIntent user=%d token=%q: %v", userID, token, err)
	}
}

// queryBalance reads balance_cents for userID directly from e2eDB.
func queryBalance(t *testing.T, userID int) int {
	t.Helper()
	var cents int
	if err := e2eDB.QueryRow(context.Background(),
		`SELECT balance_cents FROM users WHERE id = $1`, userID,
	).Scan(&cents); err != nil {
		t.Fatalf("queryBalance user=%d: %v", userID, err)
	}
	return cents
}

// ── TestE2E_RecurrenteWebhook_CreditsBalance ──────────────────────────────────
//
// Exercises the complete Recurrente payment ingestion path end-to-end:
//
//  1. POST /webhooks/recurrente with a correctly HMAC-SHA256-signed body.
//  2. RecurrenteWebhookAuth middleware verifies the signature (real secret, no bypass).
//  3. PaymentWebhookHandler parses the payload and calls CreditFromRecurrente.
//  4. webhookPaymentService calls CreditIdempotent on BalanceLedgerRepository.
//  5. CreditIdempotent credits users.balance_cents atomically.
//
// This is the gap identified in P1-001: handler-level unit tests stub the
// service layer and never touch the DB, so a bug in CreditIdempotent's
// two-step transaction would be invisible until production.
func TestE2E_RecurrenteWebhook_CreditsBalance(t *testing.T) {
	skipIfNoE2EDB(t)
	cleanE2ETables(t)

	const testSecret = "e2e-recurrente-test-secret"
	const amountCents = 7_500
	const reference = "TXN-E2E-001"

	jwksURL, _ := testJWKSServer(t)
	h := newE2EServerWithRecurrente(t, jwksURL, testSecret).Routes(context.Background())

	userID := seedE2EUser(t, "rw@e2e.test", "e2e-rw-user", domain.RoleUser)

	payload := map[string]any{
		"event_type": "payment.confirmed",
		"data": map[string]any{
			"reference":    reference,
			"amount_cents": amountCents,
			"currency":     "GTQ",
			"user_id":      userID,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	rec := doWebhookRequest(t, h, "/webhooks/recurrente", body, map[string]string{
		"X-Recurrente-Hmac-Sha256": recurrenteSign(testSecret, body),
	})
	assertStatus(t, rec, http.StatusNoContent, "recurrente webhook")

	if got := queryBalance(t, userID); got != amountCents {
		t.Errorf("balance_cents: want %d, got %d", amountCents, got)
	}
}

// ── TestE2E_RecurrenteWebhook_IdempotentOnDuplicate ───────────────────────────
//
// Verifies that a duplicate Recurrente webhook delivery (same reference) credits
// the user exactly once. The idempotency key is enforced by the partial unique
// index on balance_ledger.reference (WHERE reference IS NOT NULL) via
// CreditIdempotent's ON CONFLICT DO NOTHING branch — the second delivery returns
// 204 but the balance is unchanged.
//
// This is the exact failure scenario from P1-001: a refactor of CreditFromRecurrente
// that breaks the idempotency branch would pass all unit tests (stubbed) but
// double-credit users in production.
func TestE2E_RecurrenteWebhook_IdempotentOnDuplicate(t *testing.T) {
	skipIfNoE2EDB(t)
	cleanE2ETables(t)

	const testSecret = "e2e-recurrente-test-secret"
	const amountCents = 5_000
	const reference = "TXN-E2E-IDEM-001"

	jwksURL, _ := testJWKSServer(t)
	h := newE2EServerWithRecurrente(t, jwksURL, testSecret).Routes(context.Background())

	userID := seedE2EUser(t, "idem@e2e.test", "e2e-idem-user", domain.RoleUser)

	payload := map[string]any{
		"event_type": "payment.confirmed",
		"data": map[string]any{
			"reference":    reference,
			"amount_cents": amountCents,
			"currency":     "GTQ",
			"user_id":      userID,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	sig := recurrenteSign(testSecret, body)

	// First delivery — must credit the balance.
	rec := doWebhookRequest(t, h, "/webhooks/recurrente", body, map[string]string{
		"X-Recurrente-Hmac-Sha256": sig,
	})
	assertStatus(t, rec, http.StatusNoContent, "first delivery")

	// Second delivery — same reference, must be a no-op.
	rec = doWebhookRequest(t, h, "/webhooks/recurrente", body, map[string]string{
		"X-Recurrente-Hmac-Sha256": sig,
	})
	assertStatus(t, rec, http.StatusNoContent, "duplicate delivery")

	// Balance must equal exactly one credit.
	if got := queryBalance(t, userID); got != amountCents {
		t.Errorf("balance_cents after duplicate delivery: want %d (one credit), got %d", amountCents, got)
	}
}

// ── TestE2E_PayPalWebhook_CreditsBalance ──────────────────────────────────────
//
// Exercises the PayPal payment ingestion path end-to-end. RSA certificate
// verification is bypassed (PayPalWebhookID is empty, activating bypass mode in
// PayPalWebhookAuth) — the RSA path is already unit-tested separately. This test
// closes the gap for the service → repository → DB leg:
//
//  1. Seed a pending payment_intent.
//  2. POST /webhooks/paypal (bypass mode stamps the verified-body sentinel).
//  3. Handler calls ResolveAndCreditPayPalIntent.
//  4. intentRepo.CaptureAndCredit atomically captures the intent and credits
//     users.balance_cents via creditUserTx.
//  5. Assert balance_cents equals the intent amount.
func TestE2E_PayPalWebhook_CreditsBalance(t *testing.T) {
	skipIfNoE2EDB(t)
	cleanE2ETables(t)

	jwksURL, _ := testJWKSServer(t)
	// Empty PayPalWebhookID → PayPalWebhookAuth bypass mode.
	h := newE2EServerUnlimited(t, jwksURL).Routes(context.Background())

	userID := seedE2EUser(t, "pp@e2e.test", "e2e-pp-user", domain.RoleUser)

	const intentToken = "e2e-paypal-intent-token-001"
	const amountCents = 10_000
	seedE2EPaymentIntent(t, userID, amountCents, "USD", intentToken)

	payload := map[string]any{
		"event_type": "PAYMENT.CAPTURE.COMPLETED",
		"resource": map[string]any{
			"id":        "CAP-E2E-PP-001",
			"custom_id": intentToken,
			"amount":    map[string]any{"value": "100.00", "currency_code": "USD"},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	// No signature headers needed in bypass mode.
	rec := doWebhookRequest(t, h, "/webhooks/paypal", body, nil)
	assertStatus(t, rec, http.StatusNoContent, "paypal webhook")

	if got := queryBalance(t, userID); got != amountCents {
		t.Errorf("balance_cents: want %d, got %d", amountCents, got)
	}
}
