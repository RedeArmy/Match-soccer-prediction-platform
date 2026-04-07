---
name: Phase 0 Implementation State
description: Clerk user-sync + RBAC + WC2026 stadiums — what was implemented and what remains
type: project
---

Phase 0 (Clerk user-sync + RBAC + Stadiums) is fully implemented and all tests pass.

**Why:** Frontend needs real user identities (Clerk subjects mapped to internal users), admin-only route protection, and stadium data for match display.

**How to apply:** Use this as context when planning the next phase (Quiniela/Tiebreaker endpoints, stadium CRUD endpoints for admins).

## What was implemented

### Migrations
- `000006_add_clerk_subject_to_users` — nullable TEXT `clerk_subject` UNIQUE with partial index
- `000007_create_stadiums` — 16 FIFA WC2026 venues seed table
- `000008_add_stadium_to_matches` — nullable FK `stadium_id` → stadiums (ON DELETE SET NULL)

### Domain (`internal/domain/entities.go`)
- `User.ClerkSubject string` added
- `Stadium` struct added (ID, Name, City, Country, Capacity, timestamps)
- `Match.StadiumID *int` and `Match.Stadium *Stadium` added

### Repository layer
- `UserRepository` interface: added `GetByClerkSubject(ctx, subject) (*User, error)`
- `StadiumRepository` interface: `GetByID`, `List`
- `user_repository.go`: `userColumns` now includes `clerk_subject`; scanUser/collectUsers use `*string` to handle NULL; `GetByClerkSubject` added; `Update` sets `clerk_subject`
- `match_repository.go`: `matchColumns` includes `stadium_id`; all queries updated
- `stadium_repository.go`: new file, full Postgres implementation

### Seed (`internal/infrastructure/database/seed.go`)
- `seedStadiums`: all 16 WC2026 venues seeded with ON CONFLICT DO NOTHING

### Middleware (`internal/middleware/auth.go`)
- `RequireRole(userRepo, log, roles...)` middleware — looks up Clerk subject via `GetByClerkSubject`, checks role, returns 401/403 on failure

### Handlers
- `responses.go`: `StadiumResponse` added; `MatchResponse` gets `StadiumID *int` + `Stadium *StadiumResponse`
- `helpers.go`: removed `clerkSubjectToUserID` stub
- `prediction_handler.go`: added `userRepo` field + `resolveUserID` method; uses repo lookup instead of parseInt
- `webhook_handler.go`: new file — Clerk webhook with Svix HMAC-SHA256 signature verification; handles `user.created` and `user.updated` events

### Server (`internal/api/server.go`)
- Routes restructured: webhook endpoint at `/webhooks/clerk` (no JWT auth)
- Admin routes guarded with `RequireRole(userRepo, RoleAdmin)`: POST /matches, PATCH /matches/{id}, POST /matches/{id}/start
- All repos constructed once and shared

### Tests updated
- `handler/stubs_test.go`: added `stubUserRepo`
- `prediction_handler_test.go`: `NewPredictionHandler` takes `userRepo`; `TestSubmit_InvalidUserID_Returns401` → `TestSubmit_UserNotFound_Returns401`
- `service/ranking_service_test.go`: `stubUserRepo` gains `GetByClerkSubject`
- `repository/repository_test.go`: `cleanTables` now includes `stadiums`

## Pending / Next steps
1. Stadium CRUD endpoints (`GET /api/v1/stadiums`, `GET /api/v1/stadiums/{id}`) — admin write operations if needed
2. Assign stadiums to matches via `PATCH /api/v1/matches/{id}` body extension
3. Quiniela and Tiebreaker HTTP endpoints (currently no handlers exist for them)
4. Swagger docs: regenerate after server.go changes (`make swagger-gen`)
5. `GET /api/v1/matches/{id}` could optionally JOIN stadium for the embedded `stadium` object
