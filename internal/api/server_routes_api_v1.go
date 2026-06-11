package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
)

// apiV1Deps bundles the shared dependencies passed to every /api/v1 route
// registration helper. Grouping them here follows the same pattern as
// coreRepos: future additions require a single field change, not updates
// to every helper's call site.
type apiV1Deps struct {
	h               appHandlers
	repos           coreRepos
	bodySizeLimit   int64
	uploadSizeLimit int64
	idem            func(http.Handler) http.Handler // idempotency middleware for payment writes
}

// registerMatchRoutes wires the /matches subrouter.
//
// Admin-only mutations (create, update, start) are gated per-route via
// RequireRole rather than at the subrouter level so read endpoints (List, Get)
// remain accessible to all authenticated users. ResolveUser is applied at the
// subrouter level so banned users are rejected before any handler runs.
func (s *Server) registerMatchRoutes(r chi.Router, d apiV1Deps) {
	r.Route("/matches", func(r chi.Router) {
		r.Use(middleware.RequestBodyLimit(d.bodySizeLimit))
		r.Use(middleware.ResolveUser(d.repos.user, s.log))
		r.Get("/", d.h.match.ListMatches)
		r.Get("/{id}", d.h.match.GetMatch)
		r.With(middleware.RequireRole(d.repos.user, s.log, domain.RoleAdmin)).Post("/", d.h.match.CreateMatch)
		r.With(middleware.RequireRole(d.repos.user, s.log, domain.RoleAdmin)).Patch("/{id}", d.h.match.UpdateResult)
		r.With(middleware.RequireRole(d.repos.user, s.log, domain.RoleAdmin)).Post("/{id}/start", d.h.match.StartMatch)
		// External provider linking — admin only.
		r.With(middleware.RequireRole(d.repos.user, s.log, domain.RoleAdmin)).Post("/{id}/external-link", d.h.adminMatchSync.LinkExternal)
		r.With(middleware.RequireRole(d.repos.user, s.log, domain.RoleAdmin)).Delete("/{id}/external-link", d.h.adminMatchSync.UnlinkExternal)
	})
}

// registerPredictionRoutes wires the /predictions subrouter.
//
// ResolveUser is applied at the subrouter level so all prediction endpoints
// can read the caller's domain.User from context without each handler querying
// the database independently.
func (s *Server) registerPredictionRoutes(r chi.Router, d apiV1Deps) {
	r.Route(routePredictions, func(r chi.Router) {
		r.Use(middleware.RequestBodyLimit(d.bodySizeLimit))
		r.Use(middleware.ResolveUser(d.repos.user, s.log))
		r.Post("/", d.h.prediction.Submit)
		r.Get("/me", d.h.prediction.GetMine)
		r.Patch("/{id}", d.h.prediction.Update)
	})
}

// registerGroupRoutes wires the /groups subrouter.
//
// ResolveUser is applied at the subrouter level. Ownership-scoped operations
// (rename, approve join, leave) are enforced by the service layer — not via
// RequireRole — because they are resource-scoped, not system-role-scoped.
// User-facing tiebreaker endpoints live here because they are group-scoped.
func (s *Server) registerGroupRoutes(r chi.Router, d apiV1Deps) {
	r.Route("/groups", func(r chi.Router) {
		r.Use(middleware.RequestBodyLimit(d.bodySizeLimit))
		r.Use(middleware.ResolveUser(d.repos.user, s.log))
		r.Post("/", d.h.group.Create)
		r.Post("/join", d.h.group.Join)
		r.Post("/join-with-balance", d.h.group.JoinWithBalance)
		r.Get("/me", d.h.group.ListMyGroups)
		r.Get("/{id}", d.h.group.GetByID)
		// Only the group CreateOwner may rename; ownership enforced in the service layer.
		r.Patch("/{id}", d.h.group.RenameGroup)
		// Only the group CreateOwner may change the tournament payment mode.
		r.Patch("/{id}/tournament-mode", d.h.group.SetTournamentMode)
		r.Get("/{id}/members", d.h.group.ListMembers)
		r.Get("/{id}/leaderboard", d.h.leaderboard.GetLeaderboard)
		// Any active member may approve or reject a pending join request.
		r.Post("/{id}/members/{membershipID}/approve", d.h.group.ApproveJoin)
		r.Delete("/{id}/members/{membershipID}", d.h.group.RejectJoin)
		// Self-removal only — a user removes themselves from the group.
		r.Delete("/{id}/members/me", d.h.group.Leave)
		// Tiebreaker: active members submit and view their own prediction.
		r.Post("/{id}/tiebreaker", d.h.tiebreaker.Submit)
		r.Get("/{id}/tiebreaker", d.h.tiebreaker.GetMine)
	})
}

// registerTiebreakerAdminRoutes wires the /tiebreaker subrouter.
//
// These two endpoints are exposed directly under /api/v1 (not under /admin)
// because they act on a tournament-wide singleton, not on a specific group.
// Only the system administrator may set the global question and confirm the
// result. RequireRole stores the resolved user in context after the role check,
// so no separate ResolveUser middleware is needed.
func (s *Server) registerTiebreakerAdminRoutes(r chi.Router, d apiV1Deps) {
	r.Route("/tiebreaker", func(r chi.Router) {
		r.Use(middleware.RequestBodyLimit(d.bodySizeLimit))
		r.With(middleware.RequireRole(d.repos.user, s.log, domain.RoleAdmin)).Patch("/question", d.h.tiebreaker.SetQuestion)
		r.With(middleware.RequireRole(d.repos.user, s.log, domain.RoleAdmin)).Patch("/result", d.h.tiebreaker.ConfirmResult)
	})
}

// registerTournamentRoutes wires the /tournament subrouter.
//
// ResolveUser is applied at the subrouter level so banned users are rejected
// on read endpoints as well as admin mutations. resolveRequestUser caches the
// user in context, so admin mutations that use RequireRole do not pay a second
// database round-trip.
func (s *Server) registerTournamentRoutes(r chi.Router, d apiV1Deps) {
	r.Route("/tournament", func(r chi.Router) {
		r.Use(middleware.RequestBodyLimit(d.bodySizeLimit))
		r.Use(middleware.ResolveUser(d.repos.user, s.log))
		r.Get("/standings", d.h.tournament.GetAllStandings)
		r.Get("/standings/{group}", d.h.tournament.GetGroupStanding)
		r.Get("/slots", d.h.tournament.ListSlots)
		r.With(middleware.RequireRole(d.repos.user, s.log, domain.RoleAdmin)).Post("/slots", d.h.tournament.CreateSlot)
		r.With(middleware.RequireRole(d.repos.user, s.log, domain.RoleAdmin)).Patch("/slots/{id}", d.h.tournament.ConfirmSlot)
	})
}

// registerUserRoutes wires the /users subrouter.
//
// ResolveUser is applied at the subrouter level so every endpoint can read
// the authenticated caller's domain.User from context (e.g. for stats).
func (s *Server) registerUserRoutes(r chi.Router, d apiV1Deps) {
	r.Route(routeUsers, func(r chi.Router) {
		r.Use(middleware.RequestBodyLimit(d.bodySizeLimit))
		r.Use(middleware.ResolveUser(d.repos.user, s.log))
		r.Get("/me", d.h.user.GetMe)
		r.Patch("/me", d.h.user.UpdateMe)
		r.Get("/me/stats", d.h.userStats.GetMyStats)
		r.Get("/me/balance", d.h.balance.GetBalance)
		r.Get("/me/balance/ledger", d.h.balance.GetLedger)
	})
}

// registerPaymentRoutes wires the payment-related subrouters:
// /payment-intents, /bank-transfers, /withdrawals.
//
// Payment writes use the idempotency middleware (idem) to prevent duplicate
// submissions across retries. POST /bank-transfers receives a multipart upload
// up to uploadSizeLimit; all other endpoints use bodySizeLimit. Per-route
// limits avoid the MaxBytesReader stacking problem.
func (s *Server) registerPaymentRoutes(r chi.Router, d apiV1Deps) {
	// POST /api/v1/paypal/create-order — server-side PayPal order creation.
	// ResolveUser is required so the handler can mint the payment intent for the
	// correct user. Credentials stay in the backend; the browser receives only the order ID.
	r.With(middleware.RequestBodyLimit(d.bodySizeLimit), middleware.ResolveUser(d.repos.user, s.log)).
		Post("/paypal/create-order", d.h.paypalOrder.Create)

	r.Route("/payment-intents", func(r chi.Router) {
		r.Use(middleware.RequestBodyLimit(d.bodySizeLimit))
		r.Use(middleware.ResolveUser(d.repos.user, s.log))
		r.With(d.idem).Post("/", d.h.paymentIntent.Create)
	})

	r.Route("/bank-transfers", func(r chi.Router) {
		r.Use(middleware.ResolveUser(d.repos.user, s.log))
		// POST receives a multipart upload; GET has no body.
		r.With(middleware.RequestBodyLimit(d.uploadSizeLimit), d.idem).Post("/", d.h.bankTransfer.Upload)
		r.With(middleware.RequestBodyLimit(d.bodySizeLimit)).Get("/", d.h.bankTransfer.ListMine)
		// Download proxies the file through the server; no body size limit needed.
		r.Get("/{id}/download", d.h.bankTransfer.DownloadProof)
	})

	r.Route("/withdrawals", func(r chi.Router) {
		r.Use(middleware.RequestBodyLimit(d.bodySizeLimit))
		r.Use(middleware.ResolveUser(d.repos.user, s.log))
		r.With(d.idem).Post("/", d.h.withdrawal.Create)
		r.Get("/", d.h.withdrawal.ListMine)
		r.Get("/limits", d.h.withdrawal.Limits)
	})

	// GET /api/v1/banks — active Guatemalan banks for the withdrawal dropdown.
	r.With(middleware.ResolveUser(d.repos.user, s.log)).Get(routeBanks, d.h.bank.List)

	// GET /api/v1/bank-account-types — active account types for the withdrawal dropdown.
	r.With(middleware.ResolveUser(d.repos.user, s.log)).Get(routeBankAccountTypes, d.h.bank.ListAccountTypes)
}

// registerKYCRoutes wires the /kyc subrouter.
// Per-route body limits avoid the MaxBytesReader stacking problem:
//   - GET endpoints have no body — no limit applied.
//   - POST /submit accepts a small JSON body → bodySizeLimit (64 KB).
//   - POST /documents receives a multipart upload → the handler enforces the
//     KYC-specific 10 MB limit via http.MaxBytesReader; no middleware limit
//     is stacked on top to avoid the 64 KB group limit shadowing the handler.
func (s *Server) registerKYCRoutes(r chi.Router, d apiV1Deps) {
	r.Route("/kyc", func(r chi.Router) {
		r.Use(middleware.ResolveUser(d.repos.user, s.log))
		r.Get("/status", d.h.kyc.GetStatus)
		r.Get("/requirements", d.h.kyc.GetRequirements)
		r.Get("/documents", d.h.kyc.ListDocuments)
		r.Get("/events", d.h.kyc.ListEvents)
		r.With(middleware.RequestBodyLimit(d.bodySizeLimit)).Post("/submit", d.h.kyc.Submit)
		r.Post("/documents", d.h.kyc.UploadDocument)
		r.Delete("/documents/{docID}", d.h.kyc.DeleteDocument)
	})
}

// registerNotificationRoutes wires the /notifications and /push subrouters.
func (s *Server) registerNotificationRoutes(r chi.Router, d apiV1Deps) {
	r.Route("/notifications", func(r chi.Router) {
		r.Use(middleware.RequestBodyLimit(d.bodySizeLimit))
		r.Use(middleware.ResolveUser(d.repos.user, s.log))
		r.Get("/", d.h.notification.GetInbox)
		r.Get("/stream", d.h.notification.GetStream)
		r.Post("/mark-read", d.h.notification.MarkRead)
		r.Get("/preferences", d.h.notification.GetPreferences)
		r.Patch("/preferences", d.h.notification.UpdatePreferences)
	})

	r.Route("/push", func(r chi.Router) {
		r.Use(middleware.RequestBodyLimit(d.bodySizeLimit))
		r.Use(middleware.ResolveUser(d.repos.user, s.log))
		r.Get("/vapid-public-key", d.h.notification.GetVAPIDPublicKey)
		r.Post("/subscribe", d.h.notification.SubscribePush)
		r.Delete("/subscribe", d.h.notification.UnsubscribePush)
	})
}

// registerAdminRoutes wires the /admin subrouter.
//
// All routes require RoleAdmin. RequireRole stores the resolved domain.User in
// context after the role check, so handlers can call UserFromContext (for audit
// trail adminID) without a second database round-trip. No separate ResolveUser
// middleware is needed.
//
// adminRateStore is a separate, tighter in-process token bucket for the /admin
// prefix so a single admin cannot hammer expensive bulk endpoints without
// consuming their general user quota. Applied after RequireRole so non-admins
// are rejected before consuming a token from the admin bucket.
func (s *Server) registerAdminRoutes(r chi.Router, d apiV1Deps, adminRateStore middleware.Allower) {
	r.Route("/admin", func(r chi.Router) {
		r.Use(middleware.RequestBodyLimit(d.bodySizeLimit))
		r.Use(middleware.RequireRole(d.repos.user, s.log, domain.RoleAdmin))
		r.Use(middleware.RateLimitByUserID(adminRateStore, s.log))

		// Users
		r.Get(routeUsers, d.h.adminUser.ListUsers)
		r.Get("/users/{id}", d.h.adminUser.GetUserProfile)
		r.Post("/users/{id}/ban", d.h.adminUser.BanUser)
		r.Delete("/users/{id}/ban", d.h.adminUser.UnbanUser)
		r.Patch("/users/{id}/role", d.h.adminUser.SetRole)
		r.Post("/users/bulk-ban", d.h.adminUser.BulkBan)

		// Groups
		r.Delete("/groups/{id}", d.h.adminGroup.DeleteGroup)
		r.Delete("/groups/{id}/members/{membershipID}", d.h.adminGroup.RemoveMember)
		r.Patch("/groups/{id}/settings", d.h.adminGroup.UpdateGroupSettings)
		r.Post("/groups/{id}/transfer-ownership", d.h.adminGroup.TransferOwnership)
		r.Get("/groups/{id}/payments", d.h.adminPayment.ListByGroup)
		r.Get("/groups/{id}/leaderboard/history", d.h.adminLeaderboard.SnapshotHistory)

		// Payments
		r.Get("/payments/pending", d.h.adminPayment.ListPending)
		r.Get("/payments", d.h.adminPayment.List)
		r.Post("/payments/{id}/validate", d.h.adminPayment.ValidateDeposit)
		r.Post("/payments/{id}/reject", d.h.adminPayment.RejectDeposit)

		// Bank transfers
		r.Get("/bank-transfers/pending", d.h.bankTransfer.AdminListPending)
		r.Get("/bank-transfers/{id}/download", d.h.bankTransfer.AdminDownloadProof)
		r.Post("/bank-transfers/{id}/approve", d.h.bankTransfer.AdminApprove)
		r.Post("/bank-transfers/{id}/reject", d.h.bankTransfer.AdminReject)

		// Withdrawals
		r.Get("/withdrawals", d.h.withdrawal.AdminListAll)
		r.Get("/withdrawals/pending", d.h.withdrawal.AdminListPending)
		r.Post("/withdrawals/{id}/approve", d.h.withdrawal.AdminApprove)
		r.Post("/withdrawals/{id}/reject", d.h.withdrawal.AdminReject)
		r.Post("/withdrawals/{id}/process", d.h.withdrawal.AdminProcess)

		// Banks & account types
		r.Get(routeBanks, d.h.adminBank.ListBanks)
		r.Post(routeBanks, d.h.adminBank.CreateBank)
		r.Patch("/banks/{id}/active", d.h.adminBank.SetBankActive)
		r.Get(routeBankAccountTypes, d.h.adminBank.ListAccountTypes)
		r.Post(routeBankAccountTypes, d.h.adminBank.CreateAccountType)
		r.Patch("/bank-account-types/{id}/active", d.h.adminBank.SetAccountTypeActive)

		// KYC review
		r.Get("/kyc/risk-dashboard", d.h.adminKYC.RiskDashboard)
		r.Get("/kyc/queue", d.h.adminKYC.ListQueue)
		r.Get("/kyc/profiles/{profileID}", d.h.adminKYC.GetProfile)
		r.Get("/kyc/profiles/{profileID}/documents", d.h.adminKYC.ListDocumentsForProfile)
		r.Get("/kyc/profiles/{profileID}/events", d.h.adminKYC.ListProfileEvents)
		r.Post("/kyc/profiles/{profileID}/approve", d.h.adminKYC.Approve)
		r.Post("/kyc/profiles/{profileID}/reject", d.h.adminKYC.Reject)
		r.Post("/kyc/profiles/{profileID}/escalate", d.h.adminKYC.Escalate)
		r.Post("/kyc/profiles/{profileID}/request-doc", d.h.adminKYC.RequestDocument)
		r.Post("/kyc/documents/{docID}/verify", d.h.adminKYC.VerifyDocument)
		r.Get("/kyc/frozen-balances", d.h.adminKYC.ListFrozenBalances)
		r.Post("/kyc/users/{userID}/release-freeze", d.h.adminKYC.ReleaseFrozenBalance)

		// Leaderboard & Predictions
		r.Get("/leaderboard", d.h.adminLeaderboard.GlobalLeaderboard)
		r.Get(routePredictions, d.h.adminLeaderboard.ListPredictions)
		r.Get("/predictions/match/{matchID}", d.h.adminLeaderboard.ListPredictionsByMatch)

		// DLQ — Redis Streams (event processing failures)
		r.Get("/dlq", d.h.adminDLQ.Stats)
		r.Post("/dlq/replay", d.h.adminDLQ.Replay)
		r.Delete("/dlq", d.h.adminDLQ.Purge)

		// Notification DLQ — PostgreSQL (delivery failures: email, push, in-app)
		r.Get("/notification-dlq", d.h.adminNotifDLQ.Stats)
		r.Post("/notification-dlq/{id}/resolve", d.h.adminNotifDLQ.Resolve)

		// Audit log
		r.Get("/audit-log", d.h.adminAudit.List)
		r.Get("/audit-log/entity/{type}/{id}", d.h.adminAudit.ListByEntity)

		// System parameters
		r.Get("/system-params", d.h.adminParam.ListAll)
		r.Get("/system-params/{key}", d.h.adminParam.Get)
		r.Patch("/system-params/{key}", d.h.adminParam.Set)
		r.Post("/system-params/{key}/reset", d.h.adminParam.Reset)
		r.Get("/system-params/{key}/history", d.h.adminParam.History)
		r.Post("/system-params/bulk", d.h.adminParam.BulkSet)

		// Tiebreakers
		r.Get("/tiebreaker/submissions", d.h.adminTiebreaker.ListSubmissions)

		// Conflicts
		r.Get("/conflicts", d.h.adminConflict.ListConflicts)
		r.Post("/conflicts/{type}/{id}/resolve", d.h.adminConflict.ResolveConflict)

		// Stats / observability
		r.Get("/stats", d.h.adminStats.GetDashboardStats)
		r.Get("/stats/conflicts/summary", d.h.adminConflict.ConflictSummary)

		// Bulk group operations
		r.Post("/groups/bulk-delete", d.h.adminGroup.BulkDeleteGroups)
		r.Post("/groups/{id}/members/bulk-remove", d.h.adminGroup.BulkRemoveMembers)
		r.Post("/groups/{id}/leaderboard/recalculate", d.h.adminGroup.RecalculateLeaderboard)
		r.Post("/groups/{id}/distribute-prizes", d.h.adminGroup.DistributePrizes)

		// Scoring rules
		r.Get("/scoring-rules", d.h.adminScoringRules.List)
		r.Get("/scoring-rules/{phase}", d.h.adminScoringRules.GetByPhase)
		r.Patch("/scoring-rules/{phase}", d.h.adminScoringRules.Update)

		// SSE hub observability — per-replica counters; aggregate in Prometheus for cluster totals
		r.Get("/notifications/sse/stats", d.h.adminSSEStats.Stats)

		// Observability dashboard endpoints
		r.Route("/observability", func(r chi.Router) {
			r.Get("/metrics/summary", d.h.adminObservability.MetricsSummary)
			r.Get("/tracing/recent-errors", d.h.adminObservability.TracingRecentErrors)
			r.Get("/active-connections", d.h.adminObservability.ActiveConnections)
			r.Get("/dlq", d.h.adminObsDLQ.Stats)
			r.Get("/audit-log", d.h.adminAudit.List)
			r.Get("/circuit-breakers", d.h.adminObsCircuit.List)
			r.Get("/n8n/workflows", d.h.adminN8n.Workflows)
			r.Get("/n8n/executions/recent", d.h.adminN8n.RecentExecutions)
		})

		// Exchange rate management
		r.Get("/exchange-rate/current", d.h.adminExchangeRate.GetCurrent)
		r.Get("/exchange-rate/history", d.h.adminExchangeRate.GetHistory)
		r.Post("/exchange-rate/override", d.h.adminExchangeRate.Override)
		r.Post("/exchange-rate/refresh", d.h.adminExchangeRate.Refresh)

		// Match-sync — automated result ingestion from external provider.
		r.Post("/match-sync/poll", d.h.adminMatchSync.TriggerPoll)
		r.Get("/match-sync/reconcile", d.h.adminMatchSync.Reconcile)

		// Notification content templates (DB-backed, operator-editable)
		const tmplByKey = "/notification-templates/{event_type}/{locale}"
		r.Get("/notification-templates", d.h.adminNotifTemplate.List)
		r.Get(tmplByKey, d.h.adminNotifTemplate.Get)
		r.Put(tmplByKey, d.h.adminNotifTemplate.Upsert)
		r.Delete(tmplByKey, d.h.adminNotifTemplate.Delete)
		r.Post(tmplByKey+"/preview", d.h.adminNotifTemplate.Preview)
		r.Get(tmplByKey+"/history", d.h.adminNotifTemplate.History)
		r.Post(tmplByKey+"/rollback", d.h.adminNotifTemplate.Rollback)
	})
}
