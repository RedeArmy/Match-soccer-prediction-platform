// Package apperrors_test exercises the public surface of pkg/apperrors.
//
// Tests are organised into three groups:
//
//  1. Constructor tests — verify that each constructor sets the correct Code,
//     Message, HTTPStatus, and Cause fields.
//
//  2. errors.Is tests — verify that the Is method enables category-level
//     matching via sentinel errors (e.g. errors.Is(err, ErrNotFound)).
//
//  3. errors.As tests — verify that errors.As extracts an *AppError from an
//     error chain that may include wrapping via fmt.Errorf("%w", ...).
//
// No mocks or external dependencies are required; the package is pure Go.
package apperrors_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	msgMatchNotFound    = "match not found"
	msgQuinielaNotFound = "quiniela not found"
	fmtCode             = "Code: expected %q, got %q"
	fmtHTTPStatus       = "HTTPStatus: expected %d, got %d"
)

// ── Constructor tests ─────────────────────────────────────────────────────────

func TestNotFound_SetsFields(t *testing.T) {
	err := apperrors.NotFound(msgMatchNotFound)

	if err.Code != apperrors.CodeNotFound {
		t.Errorf(fmtCode, apperrors.CodeNotFound, err.Code)
	}
	if err.Message != msgMatchNotFound {
		t.Errorf("Message: expected %q, got %q", msgMatchNotFound, err.Message)
	}
	if err.HTTPStatus != http.StatusNotFound {
		t.Errorf(fmtHTTPStatus, http.StatusNotFound, err.HTTPStatus)
	}
	if err.Cause != nil {
		t.Errorf("Cause: expected nil, got %v", err.Cause)
	}
}

func TestUnauthorised_SetsFields(t *testing.T) {
	err := apperrors.Unauthorised("token expired")

	if err.Code != apperrors.CodeUnauthorised {
		t.Errorf(fmtCode, apperrors.CodeUnauthorised, err.Code)
	}
	if err.HTTPStatus != http.StatusUnauthorized {
		t.Errorf(fmtHTTPStatus, http.StatusUnauthorized, err.HTTPStatus)
	}
}

func TestForbidden_SetsFields(t *testing.T) {
	err := apperrors.Forbidden("admin only")

	if err.Code != apperrors.CodeForbidden {
		t.Errorf(fmtCode, apperrors.CodeForbidden, err.Code)
	}
	if err.HTTPStatus != http.StatusForbidden {
		t.Errorf(fmtHTTPStatus, http.StatusForbidden, err.HTTPStatus)
	}
}

func TestConflict_SetsFields(t *testing.T) {
	err := apperrors.Conflict("prediction already submitted")

	if err.Code != apperrors.CodeConflict {
		t.Errorf(fmtCode, apperrors.CodeConflict, err.Code)
	}
	if err.HTTPStatus != http.StatusConflict {
		t.Errorf(fmtHTTPStatus, http.StatusConflict, err.HTTPStatus)
	}
}

func TestValidation_SetsFields(t *testing.T) {
	err := apperrors.Validation("score must not be negative")

	if err.Code != apperrors.CodeValidation {
		t.Errorf(fmtCode, apperrors.CodeValidation, err.Code)
	}
	if err.HTTPStatus != http.StatusUnprocessableEntity {
		t.Errorf(fmtHTTPStatus, http.StatusUnprocessableEntity, err.HTTPStatus)
	}
}

func TestBadRequest_SetsFields(t *testing.T) {
	err := apperrors.BadRequest("invalid webhook signature")

	if err.Code != apperrors.CodeBadRequest {
		t.Errorf(fmtCode, apperrors.CodeBadRequest, err.Code)
	}
	if err.Message != "invalid webhook signature" {
		t.Errorf("Message: expected %q, got %q", "invalid webhook signature", err.Message)
	}
	if err.HTTPStatus != http.StatusBadRequest {
		t.Errorf(fmtHTTPStatus, http.StatusBadRequest, err.HTTPStatus)
	}
	if err.Cause != nil {
		t.Errorf("Cause: expected nil, got %v", err.Cause)
	}
}

func TestInternal_SetsFieldsAndStoredCause(t *testing.T) {
	cause := errors.New("pgx: connection refused")
	err := apperrors.Internal(cause)

	if err.Code != apperrors.CodeInternal {
		t.Errorf(fmtCode, apperrors.CodeInternal, err.Code)
	}
	if err.HTTPStatus != http.StatusInternalServerError {
		t.Errorf(fmtHTTPStatus, http.StatusInternalServerError, err.HTTPStatus)
	}
	if err.Message != apperrors.MsgInternal {
		t.Errorf("Message: expected generic internal message, got %q", err.Message)
	}
	if !errors.Is(err, cause) {
		t.Error("expected errors.Is to find the cause via Unwrap")
	}
}

// TestError_ReturnsMessage verifies that the error interface is satisfied and
// returns the user-facing message, not the internal cause details.
func TestError_ReturnsMessage(t *testing.T) {
	err := apperrors.NotFound(msgQuinielaNotFound)

	if err.Error() != msgQuinielaNotFound {
		t.Errorf("Error(): expected %q, got %q", msgQuinielaNotFound, err.Error())
	}
}

// ── errors.Is tests ───────────────────────────────────────────────────────────

// TestIs_MatchesSameCategorysentinel verifies that each constructor-produced
// error matches its corresponding sentinel via errors.Is.
func TestIs_MatchesSameCategory(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		sentinel error
	}{
		{"NotFound", apperrors.NotFound("x"), apperrors.ErrNotFound},
		{"Unauthorised", apperrors.Unauthorised("x"), apperrors.ErrUnauthorised},
		{"Forbidden", apperrors.Forbidden("x"), apperrors.ErrForbidden},
		{"Conflict", apperrors.Conflict("x"), apperrors.ErrConflict},
		{"Validation", apperrors.Validation("x"), apperrors.ErrValidation},
		{"Internal", apperrors.Internal(errors.New("db")), apperrors.ErrInternal},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !errors.Is(tc.err, tc.sentinel) {
				t.Errorf("errors.Is(%v, %v): expected true, got false", tc.err, tc.sentinel)
			}
		})
	}
}

// TestIs_DoesNotMatchDifferentCategory ensures that errors.Is returns false
// when the codes differ, preventing category confusion at call sites.
func TestIs_DoesNotMatchDifferentCategory(t *testing.T) {
	err := apperrors.NotFound("x")

	if errors.Is(err, apperrors.ErrConflict) {
		t.Error("errors.Is(NotFound, ErrConflict): expected false, got true")
	}
	if errors.Is(err, apperrors.ErrInternal) {
		t.Error("errors.Is(NotFound, ErrInternal): expected false, got true")
	}
}

// TestIs_MatchesThroughWrap verifies that errors.Is traverses a wrapping
// chain created with fmt.Errorf("%w", ...), as occurs when a service wraps
// a repository error before returning it to the handler.
func TestIs_MatchesThroughWrap(t *testing.T) {
	base := apperrors.NotFound("user not found")
	wrapped := fmt.Errorf("get user: %w", base)

	if !errors.Is(wrapped, apperrors.ErrNotFound) {
		t.Error("errors.Is through fmt.Errorf wrap: expected true, got false")
	}
}

// ── errors.As tests ───────────────────────────────────────────────────────────

// TestAs_ExtractsAppError verifies that errors.As extracts an *AppError so
// that handler code can access HTTPStatus and Message without a type assertion.
func TestAs_ExtractsAppError(t *testing.T) {
	original := apperrors.Validation("home score must not be negative")
	wrapped := fmt.Errorf("submit prediction: %w", original)

	var appErr *apperrors.AppError
	if !errors.As(wrapped, &appErr) {
		t.Fatal("errors.As: expected to extract *AppError, got false")
	}
	if appErr.Code != apperrors.CodeValidation {
		t.Errorf(fmtCode, apperrors.CodeValidation, appErr.Code)
	}
	if appErr.HTTPStatus != http.StatusUnprocessableEntity {
		t.Errorf(fmtHTTPStatus, http.StatusUnprocessableEntity, appErr.HTTPStatus)
	}
}

// TestAs_ReturnsFalseForNonAppError verifies that errors.As correctly returns
// false when the error chain contains no *AppError, so that handler fallback
// logic for unexpected errors is exercised properly.
func TestAs_ReturnsFalseForNonAppError(t *testing.T) {
	plain := errors.New("plain error")

	var appErr *apperrors.AppError
	if errors.As(plain, &appErr) {
		t.Error("errors.As on plain error: expected false, got true")
	}
}
