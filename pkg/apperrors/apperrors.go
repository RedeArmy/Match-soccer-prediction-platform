// Package apperrors defines the structured error types returned by the
// application's service and handler layers.
//
// The package is named apperrors rather than errors to prevent shadowing
// the standard library's errors package. Any file that imports both
// this package and stdlib errors would otherwise require an alias, creating
// ambiguity and friction for every engineer reading the code.
//
// Structured errors serve two purposes: they carry a machine-readable Code
// that the HTTP handler layer maps to an appropriate status code, and they
// carry a human-readable Message suitable for inclusion in API responses.
// Internal error details (database errors, stack traces) must never be
// forwarded to clients — wrap them in the Cause field and log them at the
// service layer before returning the AppError to the handler.
//
// Typical usage in a service:
//
//	row, err := r.db.QueryRow(ctx, query, id)
//	if errors.Is(err, pgx.ErrNoRows) {
//	    return apperrors.NotFound("match not found")
//	}
//	if err != nil {
//	    return apperrors.Internal(err) // cause is logged by the handler
//	}
//
// Typical usage in a handler:
//
//	var appErr *apperrors.AppError
//	if errors.As(err, &appErr) {
//	    respond(w, appErr.HTTPStatus, appErr)
//	    return
//	}
package apperrors

import (
	"errors"
	"net/http"
)

// AppError is the application's structured error type.
//
// It implements the error interface and is compatible with errors.Is and
// errors.As. Two AppErrors are considered equal (via errors.Is) when their
// Code fields match, regardless of their Message or Cause. This allows
// callers to check the category of an error without coupling to a specific
// message string:
//
//	errors.Is(err, apperrors.ErrNotFound) // true for any CodeNotFound error
type AppError struct {
	// Code is the machine-readable error category. Use this in handler logic
	// to select HTTP status codes, and in client code to branch on error type.
	Code Code

	// Message is a human-readable description safe to include in API responses.
	// It must not contain internal details such as SQL errors or file paths.
	Message string

	// HTTPStatus is the HTTP status code the handler should write in the
	// response. It is set by each constructor and corresponds to the Code.
	HTTPStatus int

	// Cause holds the underlying error that triggered this AppError.
	// It is intended for server-side logging only and must never be serialised
	// into the API response. For CodeInternal errors, this is typically the
	// raw database or I/O error. For other codes, Cause is usually nil.
	Cause error
}

// Sentinel errors allow call sites to use errors.Is to check the category
// of an error without depending on a specific message string.
//
// Example:
//
//	if errors.Is(err, apperrors.ErrNotFound) { ... }
var (
	ErrNotFound     = &AppError{Code: CodeNotFound}
	ErrUnauthorised = &AppError{Code: CodeUnauthorised}
	ErrForbidden    = &AppError{Code: CodeForbidden}
	ErrConflict     = &AppError{Code: CodeConflict}
	ErrValidation   = &AppError{Code: CodeValidation}
	ErrInternal     = &AppError{Code: CodeInternal}
)

// Error implements the error interface. It returns the user-facing message.
func (e *AppError) Error() string {
	return e.Message
}

// Unwrap returns the underlying cause, enabling errors.Is and errors.As to
// traverse the error chain beyond this AppError.
func (e *AppError) Unwrap() error {
	return e.Cause
}

// Is reports whether this AppError matches the target error. Two AppErrors
// match when their Code fields are equal. This allows sentinel errors
// (e.g. ErrNotFound) to match any AppError of the same category regardless
// of the specific message or cause.
func (e *AppError) Is(target error) bool {
	var t *AppError
	if !errors.As(target, &t) {
		return false
	}
	return e.Code == t.Code
}

// NotFound returns an AppError indicating that the requested resource does
// not exist. The message should name the resource type to help the client
// understand which lookup failed (e.g. "match not found").
func NotFound(message string) *AppError {
	return &AppError{
		Code:       CodeNotFound,
		Message:    message,
		HTTPStatus: http.StatusNotFound,
	}
}

// Unauthorised returns an AppError indicating that the request lacks valid
// authentication credentials. Use this when the client is not logged in.
// For authenticated users who lack the required permission, use Forbidden.
func Unauthorised(message string) *AppError {
	return &AppError{
		Code:       CodeUnauthorised,
		Message:    message,
		HTTPStatus: http.StatusUnauthorized,
	}
}

// Forbidden returns an AppError indicating that the authenticated user does
// not have permission to perform the requested action. Re-authenticating
// will not resolve this error — the user lacks the required role or resource
// ownership.
func Forbidden(message string) *AppError {
	return &AppError{
		Code:       CodeForbidden,
		Message:    message,
		HTTPStatus: http.StatusForbidden,
	}
}

// Conflict returns an AppError indicating that the request conflicts with
// the current state of a resource — for example, submitting a prediction
// for a match the user has already predicted, or creating a quiniela with
// a name that is already taken.
func Conflict(message string) *AppError {
	return &AppError{
		Code:       CodeConflict,
		Message:    message,
		HTTPStatus: http.StatusConflict,
	}
}

// Validation returns an AppError indicating that the request failed
// domain-level validation. The message should describe which rule was
// violated so that the client can correct the request without guessing.
func Validation(message string) *AppError {
	return &AppError{
		Code:       CodeValidation,
		Message:    message,
		HTTPStatus: http.StatusUnprocessableEntity,
	}
}

// Internal returns an AppError indicating an unexpected server-side failure.
// The cause is stored for server-side logging and must never be forwarded
// to the client. The user-facing message is always the generic MsgInternal
// constant, regardless of the underlying cause.
func Internal(cause error) *AppError {
	return &AppError{
		Code:       CodeInternal,
		Message:    MsgInternal,
		HTTPStatus: http.StatusInternalServerError,
		Cause:      cause,
	}
}
