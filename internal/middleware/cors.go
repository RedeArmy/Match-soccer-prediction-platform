package middleware

// TODO: implement CORS middleware.
//
// Configure allowed origins, methods, and headers based on the deployment
// environment. In production, restrict AllowedOrigins to the specific
// frontend domain(s) rather than using a wildcard, which would allow any
// website to make credentialed requests to the API on behalf of a logged-in
// user (a CSRF-style vulnerability).
//
// Consider using github.com/rs/cors rather than implementing CORS manually,
// as the specification has several edge cases (preflight caching, vary headers)
// that are easy to get wrong.
