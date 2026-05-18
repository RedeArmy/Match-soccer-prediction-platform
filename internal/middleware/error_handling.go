package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// jsonPool recycles bytes.Buffer instances used by WriteJSON so that the GC
// does not need to collect a new buffer on every API response. Buffers grow to
// the size of the largest payload they have encoded and are then reused,
// amortising the heap allocation cost under concurrent traffic.
var jsonPool = sync.Pool{New: func() any {
	b := new(bytes.Buffer)
	b.Grow(512)
	return b
}}

// ErrorResponse is the JSON envelope written for all error responses.
//
// Using a consistent envelope across every error - regardless of which
// handler produced it - makes the API surface predictable for clients and
// simplifies frontend error handling. Clients can always expect to find the
// machine-readable code and the human-readable message at the same path in
// the response body.
//
// This is the single canonical definition; handler/responses.go re-exports it
// as type aliases so Swagger annotations continue to reference handler.ErrorResponse.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail carries the machine-readable error code and human-readable message.
//
// SchemaVersion matches apperrors.ErrorAPIVersion. Clients can gate on this
// field to detect breaking changes in the error envelope without re-parsing
// other fields. It is always present and positive.
type ErrorDetail struct {
	SchemaVersion int    `json:"schema_version"`
	Code          string `json:"code"`
	Message       string `json:"message"`
}

// unexported aliases keep the internal helper callsites unchanged.
type errorResponse = ErrorResponse
type errorDetail = ErrorDetail

// WriteError translates err into a JSON HTTP response.
//
// If err is (or wraps) an *apperrors.AppError, WriteError uses its HTTPStatus
// code and Message. If the AppError carries a Cause, that internal error is
// logged at error level - it must never appear in the response body.
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
				SchemaVersion: apperrors.ErrorAPIVersion,
				Code:          string(appErr.Code),
				Message:       appErr.Message,
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
			SchemaVersion: apperrors.ErrorAPIVersion,
			Code:          string(apperrors.CodeInternal),
			Message:       apperrors.MsgInternal,
		},
	})
}

// WriteJSON serialises v as JSON and writes it to w with the given status code.
// Encode errors are silently discarded: WriteHeader has already been called,
// so the status code cannot change and appending an error message would corrupt
// the response body. Callers that need to detect encode failures should
// pre-validate before calling this function.
//
// A bytes.Buffer is borrowed from jsonPool to avoid allocating a new
// intermediate buffer on every call. Under concurrent load the pool keeps
// previously-grown buffers alive, reducing GC pressure on the heap.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	buf := jsonPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer jsonPool.Put(buf)

	_ = json.NewEncoder(buf).Encode(v)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// writeJSON is the package-internal alias so existing call sites within the
// middleware package do not need to be updated.
func writeJSON(w http.ResponseWriter, status int, v any) { WriteJSON(w, status, v) }
