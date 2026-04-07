package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// writeJSON serialises v to JSON and writes it to w with the given status code.
// The Content-Type header is set before writing the body.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// decodeError wraps a JSON decode failure in a Validation AppError so that
// the handler can delegate the HTTP response to WriteError without
// duplicating status-mapping logic.
func decodeError(err error) error {
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
