package apperrors_test

import (
	"testing"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// TestCodes_WireValuesAreLocked asserts the exact string value that each Code
// constant produces on the wire. Any rename of a constant value silently
// breaks API clients that branch on error codes; this test makes that break
// visible in CI before it reaches production.
//
// If you need to change a wire value: bump ErrorAPIVersion, update the
// stability contract in codes.go, and update the expected string here.
func TestCodes_WireValuesAreLocked(t *testing.T) {
	cases := []struct {
		code apperrors.Code
		want string
	}{
		{apperrors.CodeNotFound, "NOT_FOUND"},
		{apperrors.CodeUnauthorised, "UNAUTHORISED"},
		{apperrors.CodeForbidden, "FORBIDDEN"},
		{apperrors.CodeConflict, "CONFLICT"},
		{apperrors.CodeValidation, "VALIDATION"},
		{apperrors.CodeRequestBodyTooLarge, "REQUEST_BODY_TOO_LARGE"},
		{apperrors.CodeBadRequest, "BAD_REQUEST"},
		{apperrors.CodeInternal, "INTERNAL"},
	}

	for _, tc := range cases {
		if string(tc.code) != tc.want {
			t.Errorf("wire value for %q changed: expected %q, got %q — this is a breaking change; bump ErrorAPIVersion", tc.want, tc.want, string(tc.code))
		}
	}
}

// TestErrorAPIVersion_IsPositive guards against accidental zero-value commits.
func TestErrorAPIVersion_IsPositive(t *testing.T) {
	if apperrors.ErrorAPIVersion <= 0 {
		t.Errorf("ErrorAPIVersion must be a positive integer, got %d", apperrors.ErrorAPIVersion)
	}
}
