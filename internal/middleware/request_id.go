package middleware

// TODO: implement a custom RequestID middleware if chi's built-in version
// does not meet requirements — for example, if the application must accept
// a caller-supplied trace ID from an upstream gateway (X-Trace-Id) rather
// than always generating a new UUID.
//
// If chi's chimiddleware.RequestID is sufficient (it generates a UUID and
// sets X-Request-Id on both the request context and the response header),
// this file can be removed and the chi middleware used directly in server.go.
