package domain

// TODO: implement validation functions for domain entities.
//
// Validation at the domain layer enforces business invariants independently
// of transport or persistence concerns. For example, ValidatePrediction should
// check that HomeScore and AwayScore are non-negative and within a plausible
// range, not that the HTTP request body was well-formed JSON (that is the
// handler's responsibility).
//
// Functions here should return descriptive errors from pkg/apperrors so that
// the service layer can propagate them to the handler without additional wrapping.
