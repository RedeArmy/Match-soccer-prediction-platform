// Package apperrors defines the structured error types returned by the
// application's service and handler layers.
//
// The package is named apperrors rather than errors to prevent shadowing
// the standard library's errors package. Any file that imports both
// this package and stdlib errors would otherwise require an alias, creating
// ambiguity and friction for every engineer reading the code.
//
// Structured errors serve two purposes: they carry a machine-readable code
// that the HTTP handler layer maps to an appropriate status code, and they
// carry a human-readable message suitable for inclusion in API responses.
// Internal error details (database errors, stack traces) must never be
// forwarded to clients — wrap them here and log them at the service layer.
package apperrors

// TODO: define AppError type, constructor functions (NotFound, Unauthorised,
// Conflict, Internal, etc.), and Is/As helpers for use with errors.Is/As.
