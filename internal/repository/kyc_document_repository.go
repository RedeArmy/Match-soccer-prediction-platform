package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresKYCDocumentRepository is the PostgreSQL-backed implementation of KYCDocumentRepository.
type PostgresKYCDocumentRepository struct {
	db *pgxpool.Pool
}

// NewPostgresKYCDocumentRepository constructs a PostgresKYCDocumentRepository.
func NewPostgresKYCDocumentRepository(db *pgxpool.Pool) *PostgresKYCDocumentRepository {
	return &PostgresKYCDocumentRepository{db: db}
}

// Create inserts a new KYC document metadata row.
//
// Returns apperrors.Conflict when the same profile has already submitted a
// document with an identical SHA-256 hash (enforced by the
// uq_kyc_documents_profile_hash partial unique index). The caller should treat
// this as a duplicate-upload error and surface an actionable message to the user.
func (r *PostgresKYCDocumentRepository) Create(ctx context.Context, doc *domain.KYCDocument) error {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	const q = `
		INSERT INTO kyc_documents
			(profile_id, profile_type, document_type, storage_key, content_type, file_size, file_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, uploaded_at
	`
	err := r.db.QueryRow(ctx, q,
		doc.ProfileID, string(doc.ProfileType), string(doc.DocumentType),
		doc.StorageKey, doc.ContentType, doc.FileSize, doc.FileHash,
	).Scan(&doc.ID, &doc.UploadedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return apperrors.Conflict("a document with identical content has already been submitted for this profile")
		}
		return apperrors.Internal(err)
	}
	return nil
}

// GetByID returns the KYC document by primary key, or nil when not found.
func (r *PostgresKYCDocumentRepository) GetByID(ctx context.Context, id int64) (*domain.KYCDocument, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	const q = `
		SELECT id, profile_id, profile_type, document_type,
		       storage_key, content_type, file_size, file_hash,
		       verified, verified_at, verified_by, uploaded_at
		  FROM kyc_documents WHERE id = $1
	`
	row := r.db.QueryRow(ctx, q, id)
	doc, err := scanKYCDocument(row)
	if err != nil {
		return nil, singleScanErr(err)
	}
	return doc, nil
}

// ListByProfile returns all documents for a profile ordered by uploaded_at DESC.
func (r *PostgresKYCDocumentRepository) ListByProfile(ctx context.Context, profileID int, profileType domain.KYCProfileType) ([]*domain.KYCDocument, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	const q = `
		SELECT id, profile_id, profile_type, document_type,
		       storage_key, content_type, file_size, file_hash,
		       verified, verified_at, verified_by, uploaded_at
		  FROM kyc_documents
		 WHERE profile_id = $1 AND profile_type = $2
		 ORDER BY uploaded_at DESC
	`
	rows, err := r.db.Query(ctx, q, profileID, string(profileType))
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return collectRows(rows, func(r pgx.Rows) (*domain.KYCDocument, error) {
		return scanKYCDocument(r)
	})
}

// MarkVerified sets verified=true and stamps verified_at/verified_by.
func (r *PostgresKYCDocumentRepository) MarkVerified(ctx context.Context, docID int64, adminID int) error {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	tag, err := r.db.Exec(ctx, `
		UPDATE kyc_documents
		   SET verified    = TRUE,
		       verified_at = NOW(),
		       verified_by = $2
		 WHERE id = $1
	`, docID, adminID)
	if err != nil {
		return apperrors.Internal(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("kyc document not found")
	}
	return nil
}

func scanKYCDocument(s rowScanner) (*domain.KYCDocument, error) {
	doc := &domain.KYCDocument{}
	var profileType, docType string
	err := s.Scan(
		&doc.ID, &doc.ProfileID, &profileType, &docType,
		&doc.StorageKey, &doc.ContentType, &doc.FileSize, &doc.FileHash,
		&doc.Verified, &doc.VerifiedAt, &doc.VerifiedBy, &doc.UploadedAt,
	)
	if err != nil {
		return nil, err
	}
	doc.ProfileType = domain.KYCProfileType(profileType)
	doc.DocumentType = domain.KYCDocumentType(docType)
	return doc, nil
}

// ListExpiredDocuments returns up to limit document metadata rows that belong
// to user profiles whose accounts were deleted before cutoff.  Results are
// ordered by uploaded_at ASC so older documents are processed first.
//
// Only 'user' profile_type is handled here; 'org' profiles are a future
// extension.  The JOIN path is:
//
//	kyc_documents → kyc_profiles (profile_id) → users (user_id, deleted_at).
func (r *PostgresKYCDocumentRepository) ListExpiredDocuments(ctx context.Context, cutoff time.Time, limit int) ([]*domain.KYCDocument, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	const q = `
		SELECT d.id, d.profile_id, d.profile_type, d.document_type,
		       d.storage_key, d.content_type, d.file_size, d.file_hash,
		       d.verified, d.verified_at, d.verified_by, d.uploaded_at
		  FROM kyc_documents d
		  JOIN kyc_profiles p ON p.id = d.profile_id AND d.profile_type = 'user'
		  JOIN users u ON u.id = p.user_id
		 WHERE u.deleted_at IS NOT NULL
		   AND u.deleted_at < $1
		 ORDER BY d.uploaded_at ASC
		 LIMIT $2
	`
	rows, err := r.db.Query(ctx, q, cutoff, limit)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return collectRows(rows, func(r pgx.Rows) (*domain.KYCDocument, error) {
		return scanKYCDocument(r)
	})
}

// DeleteByID permanently removes a document metadata row.
// The caller is responsible for deleting the physical file from the FileStore
// before or after this call — the two operations are intentionally not atomic
// to keep each step simple and retriable.
func (r *PostgresKYCDocumentRepository) DeleteByID(ctx context.Context, id int64) error {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	tag, err := r.db.Exec(ctx, `DELETE FROM kyc_documents WHERE id = $1`, id)
	if err != nil {
		return apperrors.Internal(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("kyc document not found")
	}
	return nil
}

var _ KYCDocumentRepository = (*PostgresKYCDocumentRepository)(nil)
