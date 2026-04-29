package handler

import (
	"encoding/json"
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

// decodeJSON reads the request body and decodes it as JSON into a value of
// type T. DisallowUnknownFields is always set so that unexpected keys in the
// client payload are rejected rather than silently ignored — this catches
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

// paginationDefaultLimit and paginationMaxLimit are the package-wide defaults
// for paginated admin endpoints. They match the seed values in system_params
// (pagination.default_limit / pagination.max_limit) and serve as the fallback
// for call sites that read from system_params at request time.
const (
	paginationDefaultLimit = 50
	paginationMaxLimit     = 200
)

// parsePagination reads ?limit and ?page from the request and returns a
// Pagination value. Defaults: limit=50, page=1. Max limit is capped at 200.
func parsePagination(r *http.Request) repository.Pagination {

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = paginationDefaultLimit
	} else if limit > paginationMaxLimit {
		limit = paginationMaxLimit
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
