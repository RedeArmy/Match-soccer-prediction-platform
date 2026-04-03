// Package service contains the application's business logic.
//
// Each service orchestrates one domain concern: it reads from and writes to
// repositories, enforces business rules, and emits domain events. Services
// must not be aware of HTTP or database implementation details — they operate
// exclusively on domain entities and depend on repository interfaces defined
// in internal/repository, not on concrete PostgreSQL implementations.
//
// Service interfaces are defined in this file and are the contracts consumed
// by the handler layer. Concrete implementations live in the other files of
// this package and are wired at the composition root (cmd/api/main.go).
// This separation allows handlers to be tested with lightweight mock services
// without touching a real database.
package service

// TODO: define service interfaces once their method signatures stabilise.
// Sketch of the intended surface:
//
//   MatchService       — CreateMatch, UpdateResult, ListMatches, GetMatch
//   PredictionService  — Submit, Update, GetByUser, GetByMatch
//   ScoringService     — ScoreMatch (triggered by MatchFinished event)
//   RankingService     — GetLeaderboard (for a given Quiniela)
//   QuinielaService    — Create, Join, GetByOwner
//   NotificationService — Notify (dispatch push/email on significant events)
