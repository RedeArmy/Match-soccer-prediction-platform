package repository_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// Ensure apperrors is used — prevents unused-import error in files
// that only reference ErrConflict via errors.Is.
var _ = apperrors.ErrConflict

// ── helpers ───────────────────────────────────────────────────────────────────

// kycDocSeq generates unique file hashes so the uq_kyc_documents_profile_hash
// partial unique index (migration 000141) does not reject calls from the same
// profile that intentionally need multiple distinct documents (e.g. ListByProfile).
var kycDocSeq atomic.Int64

func seedKYCDocument(t *testing.T, profileID int) *domain.KYCDocument {
	t.Helper()
	seq := kycDocSeq.Add(1)
	repo := repository.NewPostgresKYCDocumentRepository(testDB)
	doc := &domain.KYCDocument{
		ProfileID:    profileID,
		ProfileType:  domain.KYCProfileTypeUser,
		DocumentType: domain.KYCDocGovID,
		StorageKey:   fmt.Sprintf("kyc/test/doc-%d.jpg", seq),
		ContentType:  "image/jpeg",
		FileSize:     1024,
		FileHash:     fmt.Sprintf("sha256-%d", seq),
	}
	if err := repo.Create(context.Background(), doc); err != nil {
		t.Fatalf("seedKYCDocument: %v", err)
	}
	return doc
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestKYCDocumentRepository_Create_DuplicateHash_ReturnsConflict(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	p := seedKYCProfile(t, u.ID)
	repo := repository.NewPostgresKYCDocumentRepository(testDB)

	const hash = "sha256-deadbeef"
	first := &domain.KYCDocument{
		ProfileID: p.ID, ProfileType: domain.KYCProfileTypeUser,
		DocumentType: domain.KYCDocGovID,
		StorageKey:   "kyc/orig.jpg", ContentType: "image/jpeg",
		FileSize: 512, FileHash: hash,
	}
	if err := repo.Create(context.Background(), first); err != nil {
		t.Fatalf("first create: %v", err)
	}

	// Second document with identical hash for the same profile — must be rejected.
	dup := &domain.KYCDocument{
		ProfileID: p.ID, ProfileType: domain.KYCProfileTypeUser,
		DocumentType: domain.KYCDocSelfie, // different type, same bytes
		StorageKey:   "kyc/dup.jpg", ContentType: "image/jpeg",
		FileSize: 512, FileHash: hash,
	}
	err := repo.Create(context.Background(), dup)
	if err == nil {
		t.Fatal("expected Conflict error for duplicate file_hash, got nil")
	}
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected apperrors.ErrConflict, got %T: %v", err, err)
	}
}

func TestKYCDocumentRepository_Create_PopulatesID(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	p := seedKYCProfile(t, u.ID)
	doc := seedKYCDocument(t, p.ID)

	if doc.ID == 0 {
		t.Error(msgNonZeroID)
	}
	if doc.UploadedAt.IsZero() {
		t.Error("expected non-zero UploadedAt")
	}
}

// ── GetByID ───────────────────────────────────────────────────────────────────

func TestKYCDocumentRepository_GetByID_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	p := seedKYCProfile(t, u.ID)
	created := seedKYCDocument(t, p.ID)
	repo := repository.NewPostgresKYCDocumentRepository(testDB)

	got, err := repo.GetByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil || got.ID != created.ID {
		t.Errorf("GetByID: got %v, want id=%d", got, created.ID)
	}
	if got.DocumentType != domain.KYCDocGovID {
		t.Errorf("DocumentType: got %q, want gov_id", got.DocumentType)
	}
}

func TestKYCDocumentRepository_GetByID_NotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresKYCDocumentRepository(testDB)

	got, err := repo.GetByID(context.Background(), 99999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf(fmtExpectNilGot, got)
	}
}

// ── ListByProfile ─────────────────────────────────────────────────────────────

func TestKYCDocumentRepository_ListByProfile_ReturnsAll(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	p := seedKYCProfile(t, u.ID)
	seedKYCDocument(t, p.ID)
	seedKYCDocument(t, p.ID)
	repo := repository.NewPostgresKYCDocumentRepository(testDB)

	docs, err := repo.ListByProfile(context.Background(), p.ID, domain.KYCProfileTypeUser)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(docs) != 2 {
		t.Errorf("expected 2 documents, got %d", len(docs))
	}
}

func TestKYCDocumentRepository_ListByProfile_Empty(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresKYCDocumentRepository(testDB)

	docs, err := repo.ListByProfile(context.Background(), 99999, domain.KYCProfileTypeUser)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(docs) != 0 {
		t.Errorf("expected empty slice, got %d", len(docs))
	}
}

// ── MarkVerified ──────────────────────────────────────────────────────────────

func TestKYCDocumentRepository_MarkVerified_SetsVerified(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	p := seedKYCProfile(t, u.ID)
	doc := seedKYCDocument(t, p.ID)
	admin := seedUser(t)
	repo := repository.NewPostgresKYCDocumentRepository(testDB)

	if err := repo.MarkVerified(context.Background(), doc.ID, admin.ID); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	got, _ := repo.GetByID(context.Background(), doc.ID)
	if !got.Verified {
		t.Error("expected verified=true after MarkVerified")
	}
	if got.VerifiedBy == nil || *got.VerifiedBy != admin.ID {
		t.Errorf("VerifiedBy: got %v, want %d", got.VerifiedBy, admin.ID)
	}
	if got.VerifiedAt == nil {
		t.Error("expected non-nil VerifiedAt after MarkVerified")
	}
}

func TestKYCDocumentRepository_MarkVerified_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresKYCDocumentRepository(testDB)

	err := repo.MarkVerified(context.Background(), 99999, 1)
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}
