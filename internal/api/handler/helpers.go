package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	msgAuthRequired = "authentication required"
	msgUserNotFound = "user account not found; please try again shortly"
)

// writeJSON delegates to middleware.WriteJSON, the single canonical implementation
// shared across the entire API surface.
func writeJSON(w http.ResponseWriter, status int, v any) { middleware.WriteJSON(w, status, v) }

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

// parsePagination reads ?limit and ?page from the request and returns a
// Pagination value. Defaults: limit=50, page=1. Max limit is capped at 200.
func parsePagination(r *http.Request) repository.Pagination {
	const defaultLimit = 50
	const maxLimit = 200

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = defaultLimit
	} else if limit > maxLimit {
		limit = maxLimit
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
