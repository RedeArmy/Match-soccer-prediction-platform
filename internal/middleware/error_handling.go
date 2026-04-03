package middleware

// TODO: implement a centralised error-handling middleware or chi NotFound/
// MethodNotAllowed handlers.
//
// Rather than having each handler construct its own error response, a
// centralised error handler reads a typed error (pkg/apperrors.AppError)
// from the request context and translates it into a consistent JSON response
// shape. This ensures all error responses share the same structure regardless
// of which handler produced the error, making the API surface predictable
// for clients and easier to document.
