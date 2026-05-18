package apperrors

// Default user-facing messages for each error code.
//
// These constants are safe to include in API responses - they describe the
// error category without exposing internal implementation details such as
// database error messages, stack traces, or infrastructure topology.
//
// Constructors use these as fallback messages. Callers that need a more
// specific message (e.g. "match not found" instead of the generic "not found")
// should pass a custom message to the constructor rather than overriding
// these constants.
const (
	MsgUnauthorised        = "authentication is required to access this resource"
	MsgForbidden           = "you do not have permission to perform this action"
	MsgRequestBodyTooLarge = "request body exceeds the maximum allowed size"
	MsgInternal            = "an unexpected error occurred; please try again later"
	MsgRateLimited         = "too many requests; please slow down and try again later"
)
