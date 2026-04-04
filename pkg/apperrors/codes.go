package apperrors

// Code is a machine-readable string that identifies the category of an
// application error. Handlers use this value to select the appropriate
// HTTP status code; clients may use it to display localised error messages
// or to branch on specific failure modes without string-matching the
// human-readable message.
//
// Codes are intentionally broad categories rather than fine-grained
// identifiers. Adding a new code for every possible failure mode creates
// an ever-growing enum that the client must handle exhaustively. The
// current set covers the cases that require distinct client behaviour;
// further differentiation should be expressed in the Message field.
type Code string

const (
	// CodeNotFound indicates that the requested resource does not exist.
	// Maps to HTTP 404 Not Found.
	CodeNotFound Code = "NOT_FOUND"

	// CodeUnauthorised indicates that the request lacks valid authentication
	// credentials. The client should re-authenticate and retry.
	// Maps to HTTP 401 Unauthorised.
	CodeUnauthorised Code = "UNAUTHORISED"

	// CodeForbidden indicates that the authenticated user does not have
	// permission to perform the requested action. Re-authenticating will
	// not resolve this — the user lacks the required role or ownership.
	// Maps to HTTP 403 Forbidden.
	CodeForbidden Code = "FORBIDDEN"

	// CodeConflict indicates that the request conflicts with the current
	// state of the resource — for example, a duplicate prediction or a
	// quiniela name that is already taken.
	// Maps to HTTP 409 Conflict.
	CodeConflict Code = "CONFLICT"

	// CodeValidation indicates that the request body or parameters failed
	// domain-level validation — for example, a negative score, a prediction
	// submitted after kickoff, or a missing required field.
	// Maps to HTTP 422 Unprocessable Entity.
	CodeValidation Code = "VALIDATION"

	// CodeInternal indicates an unexpected server-side failure. The cause is
	// logged internally but never forwarded to the client, as it may contain
	// sensitive infrastructure details (database errors, stack traces, etc.).
	// Maps to HTTP 500 Internal Server Error.
	CodeInternal Code = "INTERNAL"
)
