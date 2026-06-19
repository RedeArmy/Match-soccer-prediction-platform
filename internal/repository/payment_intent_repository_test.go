package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// seedPaymentIntent inserts a pending payment intent for the given user.
func seedPaymentIntent(t *testing.T, userID, amountCents int) *domain.PaymentIntent {
	t.Helper()
	repo := repository.NewPostgresPaymentIntentRepository(testDB)
	intent := &domain.PaymentIntent{
		Token:       "tok_" + nextCode(),
		UserID:      userID,
		AmountCents: amountCents,
		Currency:    "GTQ",
		Status:      domain.PaymentIntentPending,
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	if err := repo.Create(context.Background(), intent); err != nil {
		t.Fatalf("seedPaymentIntent: %v", err)
	}
	return intent
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestPaymentIntentRepository_Create_PopulatesID(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	intent := seedPaymentIntent(t, u.ID, 5000)

	if intent.ID == 0 {
		t.Error(msgNonZeroID)
	}
}

func TestPaymentIntentRepository_Create_PopulatesTimestamps(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	intent := seedPaymentIntent(t, u.ID, 2000)

	if intent.CreatedAt.IsZero() {
		t.Error("created_at must not be zero")
	}
	if intent.UpdatedAt.IsZero() {
		t.Error("updated_at must not be zero")
	}
}

func TestPaymentIntentRepository_Create_DuplicateTokenReturnsError(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	intent := &domain.PaymentIntent{
		Token:       "duplicate-token",
		UserID:      u.ID,
		AmountCents: 1000,
		Currency:    "GTQ",
		Status:      domain.PaymentIntentPending,
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	if err := repo.Create(context.Background(), intent); err != nil {
		t.Fatalf("first create: %v", err)
	}

	intent2 := &domain.PaymentIntent{
		Token:       "duplicate-token",
		UserID:      u.ID,
		AmountCents: 2000,
		Currency:    "GTQ",
		Status:      domain.PaymentIntentPending,
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	if err := repo.Create(context.Background(), intent2); err == nil {
		t.Error("expected error for duplicate token, got nil")
	}
}

// ── CaptureAndCredit ──────────────────────────────────────────────────────────

func TestPaymentIntentRepository_CaptureAndCredit_HappyPath(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	intent := seedPaymentIntent(t, u.ID, 3000)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	captured, err := repo.CaptureAndCredit(context.Background(), intent.Token, "CAP-001", intent.AmountCents, "", 0)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if captured == nil {
		t.Fatal("expected captured intent, got nil")
	}
	if captured.Status != domain.PaymentIntentCaptured {
		t.Errorf("status: got %q, want captured", captured.Status)
	}
	if captured.CaptureID == nil || *captured.CaptureID != "CAP-001" {
		t.Errorf("capture_id: got %v, want CAP-001", captured.CaptureID)
	}
}

func TestPaymentIntentRepository_CaptureAndCredit_CreditsUserBalance(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	intent := seedPaymentIntent(t, u.ID, 4000)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	if _, err := repo.CaptureAndCredit(context.Background(), intent.Token, "CAP-002", intent.AmountCents, "", 0); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	userRepo := repository.NewPostgresUserRepository(testDB)
	bal, _, err := userRepo.GetBalance(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if bal != 4000 {
		t.Errorf("balance: got %d, want 4000", bal)
	}
}

func TestPaymentIntentRepository_CaptureAndCredit_IdempotentReplaySameCaptureID(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	intent := seedPaymentIntent(t, u.ID, 2500)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	if _, err := repo.CaptureAndCredit(context.Background(), intent.Token, "CAP-DUP", intent.AmountCents, "", 0); err != nil {
		t.Fatalf("first capture: %v", err)
	}

	_, err := repo.CaptureAndCredit(context.Background(), intent.Token, "CAP-DUP", intent.AmountCents, "", 0)
	if !errors.Is(err, repository.ErrPaymentIntentAlreadyCaptured) {
		t.Errorf("expected ErrPaymentIntentAlreadyCaptured, got %v", err)
	}
}

func TestPaymentIntentRepository_CaptureAndCredit_DifferentCaptureIDReturnsConflict(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	intent := seedPaymentIntent(t, u.ID, 2500)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	if _, err := repo.CaptureAndCredit(context.Background(), intent.Token, "CAP-FIRST", intent.AmountCents, "", 0); err != nil {
		t.Fatalf("first capture: %v", err)
	}

	_, err := repo.CaptureAndCredit(context.Background(), intent.Token, "CAP-SECOND", intent.AmountCents, "", 0)
	if !errors.As(err, new(*apperrors.AppError)) {
		t.Errorf("expected AppError (conflict), got %T: %v", err, err)
	}
}

func TestPaymentIntentRepository_CaptureAndCredit_TokenNotFoundReturnsNotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	_, err := repo.CaptureAndCredit(context.Background(), "nonexistent-token", "CAP-999", 0, "", 0)
	if !errors.As(err, new(*apperrors.AppError)) {
		t.Errorf("expected AppError (not found), got %T: %v", err, err)
	}
}

func TestPaymentIntentRepository_CaptureAndCredit_ExpiredIntentReturnsNotFound(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	// Insert an already-expired intent directly.
	intent := &domain.PaymentIntent{
		Token:       "tok_expired_" + nextCode(),
		UserID:      u.ID,
		AmountCents: 1000,
		Currency:    "GTQ",
		Status:      domain.PaymentIntentPending,
		ExpiresAt:   time.Now().Add(-time.Hour), // expired 1 hour ago
	}
	if err := repo.Create(context.Background(), intent); err != nil {
		t.Fatalf("create expired intent: %v", err)
	}

	_, err := repo.CaptureAndCredit(context.Background(), intent.Token, "CAP-EXP", intent.AmountCents, "", 0)
	if !errors.As(err, new(*apperrors.AppError)) {
		t.Errorf("expected AppError (not found/expired), got %T: %v", err, err)
	}
}

func TestPaymentIntentRepository_CaptureAndCredit_SoftDeletedUserReturnsNotFound(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	intent := seedPaymentIntent(t, u.ID, 2000)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	// Soft-delete the user so that the UPDATE users … WHERE deleted_at IS NULL
	// inside creditUserTx matches 0 rows and returns ErrNoRows → NotFound.
	if _, err := testDB.Exec(context.Background(),
		`UPDATE users SET deleted_at = NOW() WHERE id = $1`, u.ID,
	); err != nil {
		t.Fatalf("soft-delete user: %v", err)
	}

	_, err := repo.CaptureAndCredit(context.Background(), intent.Token, "CAP-SOFTDEL", intent.AmountCents, "", 0)
	if !errors.As(err, new(*apperrors.AppError)) {
		t.Errorf("expected AppError (user not found), got %T: %v", err, err)
	}
}

// ── GetByToken ────────────────────────────────────────────────────────────────

func TestPaymentIntentRepository_GetByToken_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	pi := seedPaymentIntent(t, u.ID, 5000)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	got, err := repo.GetByToken(context.Background(), pi.Token)
	if err != nil {
		t.Fatalf("GetByToken: %v", err)
	}
	if got == nil || got.Token != pi.Token {
		t.Errorf("token: got %v, want %q", got, pi.Token)
	}
}

func TestPaymentIntentRepository_GetByToken_NotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	got, err := repo.GetByToken(context.Background(), "no-such-token")
	if err != nil {
		t.Fatalf("expected nil error for missing token, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

// ── GetByID ───────────────────────────────────────────────────────────────────

func TestPaymentIntentRepository_GetByID_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	pi := seedPaymentIntent(t, u.ID, 3000)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	got, err := repo.GetByID(context.Background(), pi.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil || got.ID != pi.ID {
		t.Errorf("id: got %v, want %d", got, pi.ID)
	}
}

func TestPaymentIntentRepository_GetByID_NotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	got, err := repo.GetByID(context.Background(), 999999)
	if err != nil {
		t.Fatalf("expected nil error for missing id, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

// ── MarkCapturedByToken ───────────────────────────────────────────────────────

func TestPaymentIntentRepository_MarkCapturedByToken_UpdatesStatus(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	pi := seedPaymentIntent(t, u.ID, 5000)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	if err := repo.MarkCapturedByToken(context.Background(), pi.Token); err != nil {
		t.Fatalf("MarkCapturedByToken: %v", err)
	}

	got, _ := repo.GetByToken(context.Background(), pi.Token)
	if got.Status != domain.PaymentIntentCaptured {
		t.Errorf("status: got %q, want captured", got.Status)
	}
}

// ── SetComprobante ────────────────────────────────────────────────────────────

func TestPaymentIntentRepository_SetComprobante_UpdatesFields(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	pi := seedPaymentIntent(t, u.ID, 4000)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	if err := repo.SetComprobante(context.Background(), pi.ID, "comprobantes/r.jpg", "image/jpeg", 12345); err != nil {
		t.Fatalf("SetComprobante: %v", err)
	}

	got, _ := repo.GetByID(context.Background(), pi.ID)
	if got.ComprobanteKey == nil || *got.ComprobanteKey != "comprobantes/r.jpg" {
		t.Errorf("comprobante_key: got %v", got.ComprobanteKey)
	}
}

func TestPaymentIntentRepository_SetComprobante_NotEditable_ReturnsNotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	err := repo.SetComprobante(context.Background(), 999999, "key", "image/jpeg", 100)
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

// ── AdminReject ───────────────────────────────────────────────────────────────

func TestPaymentIntentRepository_AdminReject_TransitionsToRejected(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	pi := seedPaymentIntent(t, u.ID, 5000)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	got, err := repo.AdminReject(context.Background(), pi.ID, admin.ID, "invalid")
	if err != nil {
		t.Fatalf("AdminReject: %v", err)
	}
	if got.Status != domain.PaymentIntentRejected {
		t.Errorf("status: got %q, want rejected", got.Status)
	}
}

func TestPaymentIntentRepository_AdminReject_NotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	_, err := repo.AdminReject(context.Background(), 999999, 1, "notes")
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

// ── RequestComprobante ────────────────────────────────────────────────────────

func TestPaymentIntentRepository_RequestComprobante_SetsFlag(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	pi := seedPaymentIntent(t, u.ID, 5000)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	got, err := repo.RequestComprobante(context.Background(), pi.ID, admin.ID)
	if err != nil {
		t.Fatalf("RequestComprobante: %v", err)
	}
	if !got.ComprobanteRequired {
		t.Error("expected comprobante_required=true")
	}
}

func TestPaymentIntentRepository_RequestComprobante_NotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	_, err := repo.RequestComprobante(context.Background(), 999999, 1)
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

// ── ListAllByUser ─────────────────────────────────────────────────────────────

func TestPaymentIntentRepository_ListAllByUser_ReturnsAll(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	seedPaymentIntent(t, u.ID, 1000)
	seedPaymentIntent(t, u.ID, 2000)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	list, err := repo.ListAllByUser(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("ListAllByUser: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("count: got %d, want 2", len(list))
	}
}

func TestPaymentIntentRepository_ListAllByUser_Empty(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	list, err := repo.ListAllByUser(context.Background(), 999999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d", len(list))
	}
}

// ── ListByUserPending ─────────────────────────────────────────────────────────

func TestPaymentIntentRepository_ListByUserPending_ExcludesCaptured(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	pending := seedPaymentIntent(t, u.ID, 1000)
	captured := seedPaymentIntent(t, u.ID, 2000)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	if err := repo.MarkCapturedByToken(context.Background(), captured.Token); err != nil {
		t.Fatalf("MarkCapturedByToken: %v", err)
	}

	list, err := repo.ListByUserPending(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("ListByUserPending: %v", err)
	}
	if len(list) != 1 || list[0].Token != pending.Token {
		t.Errorf("expected only pending intent, got %d intents", len(list))
	}
}

// ── SubmitForReview ───────────────────────────────────────────────────────────

func TestPaymentIntentRepository_SubmitForReview_TransitionsToUnderReview(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	pi := seedPaymentIntent(t, u.ID, 5000)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	if _, err := repo.AdminReject(context.Background(), pi.ID, admin.ID, "wrong"); err != nil {
		t.Fatalf("AdminReject: %v", err)
	}

	got, err := repo.SubmitForReview(context.Background(), pi.ID, u.ID, nil, nil, nil, "I fixed it")
	if err != nil {
		t.Fatalf("SubmitForReview: %v", err)
	}
	if got.Status != domain.PaymentIntentUnderReview {
		t.Errorf("status: got %q, want under_review", got.Status)
	}
}

func TestPaymentIntentRepository_SubmitForReview_WithComprobante(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	pi := seedPaymentIntent(t, u.ID, 5000)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	if _, err := repo.AdminReject(context.Background(), pi.ID, admin.ID, "missing receipt"); err != nil {
		t.Fatalf("AdminReject: %v", err)
	}

	key := "comprobantes/new-receipt.jpg"
	ct := "image/jpeg"
	sz := 9000
	got, err := repo.SubmitForReview(context.Background(), pi.ID, u.ID, &key, &ct, &sz, "Uploaded new receipt")
	if err != nil {
		t.Fatalf("SubmitForReview with comprobante: %v", err)
	}
	if got.ComprobanteKey == nil || *got.ComprobanteKey != key {
		t.Errorf("comprobante_key: got %v, want %q", got.ComprobanteKey, key)
	}
}

func TestPaymentIntentRepository_SubmitForReview_NotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	_, err := repo.SubmitForReview(context.Background(), 999999, 1, nil, nil, nil, "notes")
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

// ── CancelByToken ─────────────────────────────────────────────────────────────

func TestPaymentIntentRepository_CancelByToken_SetsCancelled(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	pi := seedPaymentIntent(t, u.ID, 3000)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	if err := repo.CancelByToken(context.Background(), pi.Token, u.ID); err != nil {
		t.Fatalf("CancelByToken: %v", err)
	}
	got, err := repo.GetByToken(context.Background(), pi.Token)
	if err != nil {
		t.Fatalf("GetByToken after cancel: %v", err)
	}
	if got.Status != domain.PaymentIntentCancelled {
		t.Errorf("status: got %q, want cancelled", got.Status)
	}
}

func TestPaymentIntentRepository_CancelByToken_WrongUser_ReturnsNotFound(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	other := seedUser(t)
	pi := seedPaymentIntent(t, u.ID, 1000)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	err := repo.CancelByToken(context.Background(), pi.Token, other.ID)
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestPaymentIntentRepository_CancelByToken_NonPending_ReturnsNotFound(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	pi := seedPaymentIntent(t, u.ID, 2000)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	// Capture the intent first so it's no longer pending.
	if _, err := repo.CaptureAndCredit(context.Background(), pi.Token, "cap-x", u.ID, "", 0); err != nil {
		t.Fatalf("CaptureAndCredit: %v", err)
	}
	_ = admin // suppress unused-variable warning

	err := repo.CancelByToken(context.Background(), pi.Token, u.ID)
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

// ── AdminCreditExpired ────────────────────────────────────────────────────────

func TestPaymentIntentRepository_AdminCreditExpired_Credits(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	pi := seedPaymentIntent(t, u.ID, 5000)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	got, err := repo.AdminCreditExpired(context.Background(), pi.ID, admin.ID, 5000, "manual credit")
	if err != nil {
		t.Fatalf("AdminCreditExpired: %v", err)
	}
	if got.Status != domain.PaymentIntentCaptured {
		t.Errorf("status: got %q, want captured", got.Status)
	}
}

func TestPaymentIntentRepository_AdminCreditExpired_AlreadyCaptured_ReturnsConflict(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	pi := seedPaymentIntent(t, u.ID, 5000)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	if _, err := repo.AdminCreditExpired(context.Background(), pi.ID, admin.ID, 5000, "first"); err != nil {
		t.Fatalf("first credit: %v", err)
	}

	_, err := repo.AdminCreditExpired(context.Background(), pi.ID, admin.ID, 5000, "second")
	if err == nil {
		t.Fatal("expected conflict error for double credit, got nil")
	}
}

func TestPaymentIntentRepository_AdminCreditExpired_NotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	_, err := repo.AdminCreditExpired(context.Background(), 999999, 1, 5000, "notes")
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

// ── ListForAdmin ──────────────────────────────────────────────────────────────

func TestPaymentIntentRepository_ListForAdmin_NoFilter(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	seedPaymentIntent(t, u.ID, 1000)
	seedPaymentIntent(t, u.ID, 2000)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	list, total, err := repo.ListForAdmin(context.Background(),
		repository.PaymentIntentFilters{},
		repository.Pagination{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("ListForAdmin: %v", err)
	}
	if total != 2 || len(list) != 2 {
		t.Errorf("expected 2 intents, got total=%d list=%d", total, len(list))
	}
}

func TestPaymentIntentRepository_ListForAdmin_FilterByProvider(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	seedPaymentIntent(t, u.ID, 1000) // provider defaults to "paypal"
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	provider := "paypal"
	list, total, err := repo.ListForAdmin(context.Background(),
		repository.PaymentIntentFilters{Provider: &provider},
		repository.Pagination{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("ListForAdmin with provider filter: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Errorf("expected 1 paypal intent, got total=%d list=%d", total, len(list))
	}
}

func TestPaymentIntentRepository_ListForAdmin_FilterByStatus(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	pi := seedPaymentIntent(t, u.ID, 5000)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	if _, err := repo.AdminReject(context.Background(), pi.ID, admin.ID, "test"); err != nil {
		t.Fatalf("AdminReject: %v", err)
	}

	status := domain.PaymentIntentRejected
	list, total, err := repo.ListForAdmin(context.Background(),
		repository.PaymentIntentFilters{Status: &status},
		repository.Pagination{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("ListForAdmin with status filter: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Errorf("expected 1 rejected intent, got total=%d list=%d", total, len(list))
	}
}

// ── creditLedgerKind (recurrente branch) ─────────────────────────────────────

func TestPaymentIntentRepository_AdminCreditExpired_RecurrenteProvider_Credits(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	admin := seedUser(t)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	intent := &domain.PaymentIntent{
		Token:       "tok_" + nextCode(),
		UserID:      u.ID,
		AmountCents: 7500,
		Currency:    "GTQ",
		Provider:    "recurrente",
		Status:      domain.PaymentIntentPending,
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	if err := repo.Create(context.Background(), intent); err != nil {
		t.Fatalf("Create recurrente intent: %v", err)
	}

	entry, err := repo.AdminCreditExpired(context.Background(), intent.ID, admin.ID, 7500, "recurrente credit")
	if err != nil {
		t.Fatalf("AdminCreditExpired (recurrente): %v", err)
	}
	if entry == nil {
		t.Fatal("expected non-nil balance entry for recurrente credit")
	}
}
