package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
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
func (r *PostgresKYCDocumentRepository) Create(ctx context.Context, doc *domain.KYCDocument) error {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	const q = `
		INSERT INTO kyc_documents
			(profile_id, profile_type, document_type, storage_key, content_type, file_size, file_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, uploaded_at
	`
	return r.db.QueryRow(ctx, q,
		doc.ProfileID, string(doc.ProfileType), string(doc.DocumentType),
		doc.StorageKey, doc.ContentType, doc.FileSize, doc.FileHash,
	).Scan(&doc.ID, &doc.UploadedAt)
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

var _ KYCDocumentRepository = (*PostgresKYCDocumentRepository)(nil)
