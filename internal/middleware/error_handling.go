package middleware

import (
	"encoding/json"
	"errors"
	"net/http"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// errorResponse is the JSON envelope written for all error responses.
//
// Using a consistent envelope across every error — regardless of which
// handler produced it — makes the API surface predictable for clients and
// simplifies frontend error handling. Clients can always expect to find the
// machine-readable code and the human-readable message at the same path in
// the response body.
type errorResponse struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WriteError translates err into a JSON HTTP response.
//
// If err is (or wraps) an *apperrors.AppError, WriteError uses its HTTPStatus
// code and Message. If the AppError carries a Cause, that internal error is
// logged at error level — it must never appear in the response body.
//
// If err is not an AppError, WriteError logs it and responds with 500 and the
// generic internal error message. This fallback ensures that unexpected errors
// (e.g. a nil pointer dereference caught by the Recover middleware) still
// produce a well-formed JSON response instead of an empty or text/plain body.
func WriteError(w http.ResponseWriter, r *http.Request, log *zap.Logger, err error) {
	var appErr *apperrors.AppError
	if errors.As(err, &appErr) {
		if appErr.Cause != nil {
			log.Error("application error",
				zap.String("request_id", GetRequestID(r.Context())),
				zap.String("code", string(appErr.Code)),
				zap.Error(appErr.Cause),
			)
		}
		writeJSON(w, appErr.HTTPStatus, errorResponse{
			Error: errorDetail{
				Code:    string(appErr.Code),
				Message: appErr.Message,
			},
		})
		return
	}

	log.Error("unexpected error",
		zap.String("request_id", GetRequestID(r.Context())),
		zap.Error(err),
	)
	writeJSON(w, http.StatusInternalServerError, errorResponse{
		Error: errorDetail{
			Code:    string(apperrors.CodeInternal),
			Message: apperrors.MsgInternal,
		},
	})
}

// writeJSON serialises v as JSON and writes it to w with the given status code.
// If serialisation fails it falls back to a plain-text 500 response rather
// than panicking, since a panic inside an error handler would be unrecoverable.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}
