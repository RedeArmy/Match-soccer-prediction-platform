package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	msgAuthRequired          = "authentication required"
	msgUserNotFound          = "user account not found; please try again shortly"
	msgInvalidWithdrawalID   = "invalid withdrawal id"
	msgInvalidBankTransferID = "invalid bank transfer id"
	msgInvalidIntentID       = "invalid intent id"
	msgTokenRequired         = "token is required"
)

// moneyJSONBodyLimit is the per-handler body cap applied to all JSON money
// endpoints (withdrawals, bank-transfer reviews). 4 KB is far larger than any
// legitimate payload for these endpoints; the limit provides defense-in-depth
// on top of the global RequestBodyLimit middleware.
const moneyJSONBodyLimit int64 = 4 * 1024

// writeJSON delegates to middleware.WriteJSON, the single canonical implementation
// shared across the entire API surface.
func writeJSON(w http.ResponseWriter, status int, v any) { middleware.WriteJSON(w, status, v) }

// writeError delegates to middleware.WriteError, the single canonical error serialiser.
func writeError(w http.ResponseWriter, r *http.Request, log *zap.Logger, err error) {
	middleware.WriteError(w, r, log, err)
}

// requireCaller resolves the authenticated caller from the request context.
// It writes a 401 response and returns (nil, false) when no user is present,
// which happens only if the request bypassed RequireAuth middleware (should
// never occur in production but guards the handler defensively).
func requireCaller(w http.ResponseWriter, r *http.Request, log *zap.Logger) (*domain.User, bool) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, log, apperrors.Unauthorised(msgAuthRequired))
	}
	return caller, ok
}

// groupMemberChecker is the narrow slice of service.GroupAuthz consumed by
// requireGroupMember. Declared here so helpers.go stays import-free of the
// full service package while still being testable with a simple stub.
type groupMemberChecker interface {
	RequireActiveMember(ctx context.Context, quinielaID, callerID int) error
}

// requireGroupMember resolves the caller and verifies active membership in
// quinielaID. It writes the appropriate error response and returns (nil, false)
// when either check fails. Handlers that need the caller for other work after
// the membership check should call requireCaller and authz.RequireActiveMember
// separately; this helper is for the common case where both are needed together.
func requireGroupMember(w http.ResponseWriter, r *http.Request, log *zap.Logger, authz groupMemberChecker, quinielaID int) (*domain.User, bool) {
	caller, ok := requireCaller(w, r, log)
	if !ok {
		return nil, false
	}
	if err := authz.RequireActiveMember(r.Context(), quinielaID, caller.ID); err != nil {
		writeError(w, r, log, err)
		return nil, false
	}
	return caller, true
}

// decodeJSON reads the request body and decodes it as JSON into a value of
// type T. DisallowUnknownFields is always set so that unexpected keys in the
// client payload are rejected rather than silently ignored - this catches
// field-name typos that would otherwise produce confusing zero-value behaviour.
//
// On failure the returned error is already wrapped by decodeError and is safe
// to pass directly to middleware.WriteError.
func decodeJSON[T any](r *http.Request) (T, error) {
	var v T
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return v, decodeError(err)
	}
	return v, nil
}

// decodeError maps a JSON decode failure to the appropriate AppError.
//
// Three cases:
//   - err is *http.MaxBytesError: the body exceeded the RequestBodyLimit;
//     return 413 so the client knows to reduce payload size.
//   - err is non-nil: JSON was malformed or of the wrong type; return 422.
//   - err is nil: decode succeeded but required fields are absent; return 422.
func decodeError(err error) error {
	if errors.As(err, new(*http.MaxBytesError)) {
		return apperrors.RequestBodyTooLarge()
	}
	msg := "request body is missing required fields"
	if err != nil {
		msg = "request body could not be parsed as JSON"
	}
	return apperrors.Validation(msg)
}

// parseIntParam converts a query-string value to a positive integer.
func parseIntParam(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("parse int %q: %w", s, err)
	}
	if n <= 0 {
		return 0, nil
	}
	return n, nil
}

// parseCursorPage reads ?limit and ?cursor from the request and returns a
// CursorPage value. Defaults: limit=50, max 200. An absent cursor means "first
// page". Clients receive the cursor from the previous response's next_cursor
// field and pass it back verbatim; they must not interpret its contents.
func parseCursorPage(r *http.Request) repository.CursorPage {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = domain.DefaultPaginationDefaultLimit
	} else if limit > domain.DefaultPaginationMaxLimit {
		limit = domain.DefaultPaginationMaxLimit
	}
	return repository.CursorPage{
		Limit:  limit,
		Cursor: r.URL.Query().Get("cursor"),
	}
}

// parsePagination reads ?limit and ?page from the request and returns a
// Pagination value. Defaults: limit=50, page=1. Max limit is capped at 200.
func parsePagination(r *http.Request) repository.Pagination {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = domain.DefaultPaginationDefaultLimit
	} else if limit > domain.DefaultPaginationMaxLimit {
		limit = domain.DefaultPaginationMaxLimit
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	return repository.Pagination{
		Limit:  limit,
		Offset: (page - 1) * limit,
	}
}

// parseOptionalInt reads a named query parameter as an *int. Returns nil when
// the parameter is absent or not a valid positive integer.
func parseOptionalInt(r *http.Request, name string) *int {
	s := r.URL.Query().Get(name)
	if s == "" {
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return nil
	}
	return &n
}

// parsePaginationParams reads optional ?limit and ?offset query parameters.
// When limit is absent or non-positive, it defaults to DefaultPaginationDefaultLimit.
// When limit exceeds DefaultPaginationMaxLimit it is capped to that value.
// When offset is absent or negative, it defaults to 0.
func parsePaginationParams(r *http.Request) (limit, offset int) {
	limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ = strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = domain.DefaultPaginationDefaultLimit
	} else if limit > domain.DefaultPaginationMaxLimit {
		limit = domain.DefaultPaginationMaxLimit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// formatUploadStorageKey builds a human-readable storage key for user-uploaded files.
//
// Format: {prefix}_{userID}_{fileType}_{YYYY}_{MM}_{DD}_{HH}:{MM}:{SS}{ext}
//
// Examples:
//
//	kyc_9_selfie_2026_06_11_11:18:59.jpg
//	bank_transfer_42_voucher_2026_06_11_10:34:45.pdf
func formatUploadStorageKey(userID int, prefix, fileType string, t time.Time, ext string) string {
	ts := t.UTC().Format("2006_01_02_15:04:05")
	return fmt.Sprintf("%s_%d_%s_%s%s", prefix, userID, fileType, ts, ext)
}

// applySlicePagination returns a subslice of items bounded by limit and offset.
// When limit is 0, all items from offset onwards are returned (unbounded).
// When offset >= len(items), an empty slice is returned (not an error).
// This is used to apply in-memory pagination to results that were fetched
// without pagination support at the repository layer.
func applySlicePagination[T any](items []T, limit, offset int) []T {
	if offset >= len(items) {
		return []T{} // offset past end of results
	}
	start := offset
	end := len(items)
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	return items[start:end]
}
