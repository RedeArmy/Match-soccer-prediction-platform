# i18n Contract — Backend / Frontend Boundary

This document defines the language-responsibility boundary between the Go backend and every frontend consumer (web app, mobile client, third-party API). It is the authoritative reference for any engineer adding a new string or a new API endpoint.

---

## Architecture summary

The platform uses a **hybrid** approach:

| Layer | Responsibility |
|---|---|
| Backend | Translates everything it renders server-side: push notification payloads, in-app notification content, email subjects and bodies, KYC/AML error messages |
| Frontend | Translates everything it renders from structured data: UI labels, navigation, form validation display copy, generic error messages mapped from `code` |
| Audit / operator strings | Always English, never translated |

---

## User locale preference

### Storage

`users.locale VARCHAR(10) NOT NULL DEFAULT 'es' CHECK (locale IN ('en', 'es'))`

Supported values: `"en"` (English), `"es"` (Spanish). Default `"es"` for the Guatemala-primary user base.

### Resolution priority (backend)

1. **User profile** — `users.locale` for the target user (highest priority)
2. **System parameter** — `notify.default_locale` system param
3. **Compile-time default** — `domain.DefaultLocale = "es"`

`Accept-Language` header is not currently read. It may be added in a future iteration for unauthenticated request paths.

### Read endpoint

```
GET /api/v1/users/me
Authorization: Bearer <clerk-jwt>
```

Response `200 OK`:
```json
{
  "id": 42,
  "name": "Carlos",
  "email": "carlos@example.com",
  "role": "user",
  "balance_cents": 10000,
  "reserved_cents": 0,
  "kyc_tier": 1,
  "locale": "es",
  "created_at": "2026-01-01T00:00:00Z"
}
```

### Update endpoint

```
PATCH /api/v1/users/me
Authorization: Bearer <clerk-jwt>
Content-Type: application/json
```

Request body:
```json
{ "locale": "en" }
```

- Accepted values: `"en"`, `"es"`. Any other value returns `422 VALIDATION`.
- Returns `204 No Content` on success.
- The change takes effect on the next notification delivery; previously queued outbox entries are not re-translated.

---

## Error response format

All errors use this envelope (schema version 1, stable):

```json
{
  "error": {
    "schema_version": 1,
    "code": "VALIDATION",
    "message": "quiniela_id must be a positive integer"
  }
}
```

### Stable error codes

These wire values are locked. Renaming or removing a code is a breaking change.

| `code` | HTTP status | Meaning |
|---|---|---|
| `NOT_FOUND` | 404 | Requested resource does not exist |
| `UNAUTHORISED` | 401 | Missing or invalid authentication |
| `FORBIDDEN` | 403 | Authenticated but not permitted; may include a locale-translated explanation for KYC blocks |
| `CONFLICT` | 409 | Request conflicts with current state (duplicate prediction, already a member, etc.) |
| `VALIDATION` | 422 | Domain-level input failure |
| `BAD_REQUEST` | 400 | Protocol/transport-level failure (malformed signature, etc.) |
| `REQUEST_BODY_TOO_LARGE` | 413 | Body exceeds size limit |
| `INTERNAL` | 500 | Unexpected server error |
| `RATE_LIMITED` | 429 | Client exceeded request quota; check `Retry-After` header |

### What the frontend should do with `message`

The `message` field is always safe to display as a fallback. It is:
- English for all non-KYC errors
- Locale-resolved (user's stored language) for `FORBIDDEN` errors produced by KYC/AML checks

The frontend **should** map `code` to its own locale-appropriate copy for all non-KYC errors. The `message` field acts as the safe English fallback while that mapping is being built out.

---

## Notification channels — what the backend pre-translates

### Push notifications

`Title` and `Body` in the FCM-compatible Web Push payload are resolved using the recipient's stored locale before delivery. The Service Worker receives pre-translated strings and should render them directly.

```json
{
  "notification_id": 123,
  "type": "payment.confirmed",
  "title": "Pago confirmado",
  "body": "Tu pago de Q50.00 ha sido confirmado.",
  "action_url": "/api/v1/users/me/balance",
  "icon": "/icons/icon-192.png",
  "badge": "/icons/badge.png"
}
```

Digest push (when burst throttling is active):
```json
{
  "notification_id": 0,
  "type": "digest",
  "title": "Tienes 3 nuevas notificaciones",
  "body": "Toca para ver tus últimas actualizaciones.",
  "action_url": "/notifications"
}
```

The frontend **must not** re-translate these strings. The `action_url` is a relative path; prefix it with the app origin.

### In-app notifications (SSE + inbox)

`GET /api/v1/notifications` returns stored rows. `GET /api/v1/notifications/stream` (SSE) pushes live events. Both channels deliver pre-translated `title` and `body` fields. The frontend renders them directly.

```json
{
  "id": 456,
  "event_type": "withdrawal.completed",
  "title": "Retiro completado",
  "body": "Tu retiro de Q200.00 ha sido procesado.",
  "action_url": "/api/v1/withdrawals",
  "created_at": "2026-06-01T12:00:00Z"
}
```

### Email

Subject lines and body content are resolved server-side using the recipient's stored locale. Operator-editable templates are keyed by `(event_type, locale)`. The frontend has no involvement in email rendering.

---

## What the frontend must translate

| Category | Examples |
|---|---|
| Navigation / chrome | Menu labels, page titles, button text |
| Form labels and placeholders | "Email address", "Amount (GTQ)", "Enter your prediction" |
| Form validation display copy | "This field is required", "Must be a positive number" |
| Generic error messages for non-KYC codes | Map `code` → locale copy; display `message` as fallback |
| Relative dates and timestamps | Format `created_at` ISO strings using the user's locale |
| Currency formatting | Use `balance_cents / 100` with locale-appropriate number formatting |

---

## Supported locales

| Tag | Language | Status |
|---|---|---|
| `es` | Spanish | Supported, platform default |
| `en` | English | Supported |

BCP-47 subtags are normalised to the language subtag: `"es-GT"` → `"es"`, `"en-US"` → `"en"`. Any unrecognised tag is treated as `"es"`.

---

## Adding a new string

**Backend-owned string** (notification content, KYC error): Add both `en` and `es` variants using `domain.LocaleStr(en, es, locale)` in the relevant content builder or service function. Both strings must be non-empty — the `TestUserEventBuilders_BothLocalesNonEmpty` test will catch empty strings.

**Frontend-owned string** (UI copy): Add to the frontend i18n catalog. Do not add it to the backend.

**Error message for a new `apperrors.X()` call**: Use a clear English string. Do not translate it unless it is a KYC/AML `Forbidden` message (user-visible financial block).

---

## Migration path for fine-grained error codes

When the frontend is ready to map specific errors to locale copy without relying on `message`:

1. Add a `sub_code` string field to `middleware.ErrorDetail` (backward-compatible; existing clients ignore unknown fields).
2. Introduce typed constants (e.g. `"ERR_PREDICTION_DEADLINE_PASSED"`, `"ERR_INSUFFICIENT_BALANCE"`).
3. Frontend maps `sub_code` → locale copy; continues using `message` as fallback.
4. `schema_version` does **not** need to be bumped for an additive field.
