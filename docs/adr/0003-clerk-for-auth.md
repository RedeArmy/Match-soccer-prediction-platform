# ADR 0003 — Clerk for Authentication and Identity

**Status:** Accepted  
**Date:** 2026-05-25  
**Deciders:** Engineering team

---

## Context

The application needs user authentication, JWT issuance, session management, and
social login (Google, GitHub). These are solved problems with well-known attack
surfaces that require ongoing maintenance: token rotation, brute-force protection,
leaked-credential detection, MFA, and compliance updates. Building these in-house
would take significant engineering time and leave the project exposed while the
implementation matures.

Alternatives considered:

| Option | Authentication | Identity Store | Maintenance burden |
|---|---|---|---|
| Build in-house (bcrypt + JWT) | Manual | Internal DB | Full — team owns all of it |
| Auth0 | Managed | External | Low — vendor manages it |
| Supabase Auth | Managed | External | Low — vendor manages it |
| **Clerk** | **Managed** | **External** | **Low — vendor manages it** |
| Firebase Authentication | Managed | External | Low — vendor manages it |

All managed options were viable. Clerk was chosen because it offers a generous free
tier, first-class JWKS endpoint for stateless JWT verification, a React SDK that
integrates with the existing frontend stack, and an HTTP webhook for syncing user
lifecycle events (created, updated, deleted) into the application database.

---

## Decision

Use **Clerk** as the external identity provider. The application:

1. Verifies every request by validating the Clerk-issued JWT against the JWKS endpoint
   (`WCQ_CLERK_JWKSURL`). Tokens are verified with `pkg/clerk/verifier.go`. No session
   state is stored in the application database.

2. Maintains a local `users` table as a **shadow copy** of the Clerk identity store,
   synchronized by the Clerk webhook (`POST /webhooks/clerk`). The shadow copy holds
   application-specific columns (role, ban status, balance) that Clerk does not manage.

3. Treats the Clerk `sub` claim as the stable external user identifier. The internal
   `users.clerk_id` column is the join key between the JWT and the local row.

4. Uses `middleware.ResolveUser` to load the local user row from the shadow copy on
   every authenticated request, making role and ban status available to handlers without
   additional database round-trips.

The Clerk webhook secret (`WCQ_CLERK_WEBHOOKSECRET`) is validated using HMAC-SHA256
to prevent spoofed lifecycle events from corrupting the shadow copy.

---

## Consequences

**Positive**

- Zero authentication code to maintain: token rotation, brute-force protection, MFA,
  and social login are all handled by Clerk.
- Stateless JWT verification: no session table, no distributed session store.
- The shadow copy pattern keeps Clerk-agnostic business logic: application code never
  calls the Clerk API directly in request paths — only the webhook handler does.
- Switching identity providers in the future requires only: a new JWKS endpoint, a
  new `verifier.go`, and a new webhook handler. Application handlers and services are
  unaffected.

**Negative / trade-offs**

- Vendor dependency: a Clerk outage or pricing change affects authentication for all
  users. The JWKS endpoint is cached with a TTL to tolerate short outages.
- Shadow-copy drift: if the Clerk webhook delivery fails, the local `users` table can
  fall out of sync. The Clerk webhook dashboard provides replay; the team monitors
  webhook failure alerts.
- The Clerk free tier has monthly active user limits. Pricing must be reviewed before
  any marketing push that significantly grows the user base.

---

## Reference

- Clerk JWT verification: `pkg/clerk/verifier.go`
- Shadow-copy sync: `internal/api/handlers/webhook_clerk_handler.go`
- ResolveUser middleware: `internal/api/middleware/user.go`
- Clerk webhook secret validation: `internal/api/middleware/webhook.go`
