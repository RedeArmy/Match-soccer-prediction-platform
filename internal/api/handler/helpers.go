package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	msgAuthRequired = "authentication required"
	msgUserNotFound = "user account not found; please try again shortly"
)

// writeJSON serialises v to JSON and writes it to w with the given status code.
// The Content-Type header is set before writing the body.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
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
