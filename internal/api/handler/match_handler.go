// Package handler contains the HTTP request handlers for the World Cup
// quiniela REST API.
//
// Each handler file in this package is responsible for a single resource:
// parsing the HTTP request, delegating to the appropriate service, and
// translating the result (or error) into an HTTP response. Handlers must
// contain no business logic; they are a thin translation layer between
// the HTTP protocol and the service layer.
//
// Dependencies (services, logger) are injected via the constructor of the
// enclosing struct rather than through package-level globals, which keeps
// handlers testable in isolation using httptest.
package handler

// TODO: implement MatchHandler with routes:
//   GET    /api/v1/matches          — list all scheduled matches
//   GET    /api/v1/matches/{id}     — get a single match by ID
//   POST   /api/v1/matches          — create a match (admin only)
//   PATCH  /api/v1/matches/{id}     — update match result (admin only)
