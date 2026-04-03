package service

// TODO: implement MatchService.
//
// Responsibilities:
//   - Validate match data before persisting (teams not empty, kick-off in future).
//   - Enforce that results can only be set when the match is Live or Finished.
//   - Emit a MatchFinished domain event after a result is confirmed, so that
//     ScoringService and NotificationService react without being called directly.
