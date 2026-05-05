# Technical Debt Analysis — Three Critical Issues

**Standard:** MAANG SDE III  
**Date:** 2026-05-04  
**Severity:** High (Production Impact)

---

## Executive Summary

Three technical debt items identified via static analysis and production monitoring require immediate remediation:

1. **Inconsistent List Response Shapes** — Two endpoints return bare arrays whilst the rest return `Paged[T]`, creating client-side fragility and future breaking changes
2. **Overly Broad Error Suppression** — Transaction rollback failures are silently swallowed, masking connection failures and timeouts
3. **Silent Event Bus Misconfiguration** — Missing validation allows in-memory driver in production, causing complete scoring system failure with no alerts

All three violate defensive programming principles and have caused or will cause production incidents.

---

## Issue 1: Inconsistent List Response Shapes

### Problem Statement

**Affected Endpoints:**
- `GET /api/v1/predictions/me` — returns `[]PredictionResponse`
- `GET /api/v1/groups/{id}/members` — returns `[]MemberResponse`

**Expected Pattern:**
- All other list endpoints return `Paged[T]` with `data` array and `page` metadata

**Impact:**

| Consequence | Severity | Explanation |
|-------------|----------|-------------|
| **Client Fragility** | High | Clients cannot assume uniform response shape; must implement two parsing paths |
| **Breaking Change Risk** | High | Adding pagination to these endpoints later requires versioning or coordinated deployment |
| **API Inconsistency** | Medium | Violates Principle of Least Astonishment; developers expect consistent patterns |
| **Type Safety Loss** | Medium | TypeScript/OpenAPI clients lose generic pagination handling |

### Solution: Wrap in Paged[T] with Optional Pagination

See `TECHNICAL_DEBT_RESOLUTION.md` for detailed implementation.

---

## Issue 2: Overly Broad Error Suppression on Transaction Rollback

### Problem Statement

**Current Pattern:**
```go
defer tx.Rollback(ctx) //nolint:errcheck
```

**Why Dangerous:**

After successful `tx.Commit()`, `tx.Rollback()` returns `pgx.ErrTxClosed` (harmless).  
**But** if rollback fails due to connection loss, timeout, or context cancellation, the error is silently lost.

### Solution: Defensive Error Checking

Check specifically for `pgx.ErrTxClosed` and log all other errors.

See `TECHNICAL_DEBT_RESOLUTION.md` for detailed implementation.

---

## Issue 3: Silent Event Bus Misconfiguration

### Problem Statement

**Current Behaviour:**

If `WCQ_EVENTBUS_DRIVER` is unset or misconfigured:
- Defaults to `in_memory` driver
- API server starts normally ✅
- Worker process starts normally ✅
- **But:** Worker never receives events (different process boundary)
- **Result:** Match scoring never runs, zero alerts

### Solution: Three-Layered Defence

1. **Startup Validation** — Reject `in_memory` in production
2. **Startup Banner** — Prominent configuration logging
3. **Structured Logging** — Machine-parseable for alerting

See `TECHNICAL_DEBT_RESOLUTION.md` for detailed implementation.

---

## Implementation Priority

1. **Issue 3** (Event Bus) — **CRITICAL** — Prevents production outages
2. **Issue 2** (Rollback) — **HIGH** — Improves observability
3. **Issue 1** (List Responses) — **MEDIUM** — Prevents future breaking changes

All three implemented in single PR for atomic deployment.
