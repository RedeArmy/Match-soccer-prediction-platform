package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	msgAuthRequired        = "authentication required"
	msgUserNotFound        = "user account not found; please try again shortly"
	msgInvalidWithdrawalID = "invalid withdrawal id"
)

// writeJSON delegates to middleware.WriteJSON, the single canonical implementation
// shared across the entire API surface.
func writeJSON(w http.ResponseWriter, status int, v any) { middleware.WriteJSON(w, status, v) }

// writeError delegates to middleware.WriteError, the single canonical error serialiser.
func writeError(w http.ResponseWriter, r *http.Request, log *zap.Logger, err error) {
	middleware.WriteError(w, r, log, err)
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
	if err != nil || n <= 0 {
		return 0, err
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
// When limit is absent or zero, returns 0 (treated as unbounded by the caller).
// When offset is absent, defaults to 0.
// This is used for endpoints that historically returned unbounded results and
// are being migrated to support optional pagination for consistency.
func parsePaginationParams(r *http.Request) (limit, offset int) {
	limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ = strconv.Atoi(r.URL.Query().Get("offset"))
	// Negative values are treated as 0 by Atoi's error case (already handled).
	// limit=0 is valid and means "return all results".
	return limit, offset
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
