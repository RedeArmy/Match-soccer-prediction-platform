# n8n Automation Workflows

Operational alerting and notification workflows for the World Cup Quiniela platform.
The JSON files in this directory are the canonical source of truth for all n8n workflows.

## Required n8n version

**1.49.0** — pinned in `docker-compose.observability.yml` as `n8nio/n8n:1.49.0`.

Each workflow JSON carries `"meta": {"n8n_version": "1.49.0"}` to document the version
the workflow was designed and tested against. Verify all workflows after any version bump.

## Automatic deployment

Workflows are **automatically imported and activated** when the n8n container starts.
The `docker-compose.observability.yml` n8n service overrides the default CMD to:

1. Run `n8n import:workflow --input=<file> --active` for each JSON in this directory
2. Start n8n normally (`exec n8n start`)

On restart, n8n re-imports every workflow (updating them from the JSON source of truth).
No manual UI import is required. All 15 workflows will be active and their webhooks
registered within ~30 seconds of n8n starting.

## Required environment variables

Set these in `/opt/wcq/.env` on the server (see `server/.env.example`):

| Variable | Description |
|----------|-------------|
| `N8N_SMTP_HOST` | SMTP server hostname |
| `N8N_SMTP_PORT` | SMTP port (default: 587) |
| `N8N_SMTP_USER` | SMTP username |
| `N8N_SMTP_PASS` | SMTP password |
| `N8N_SMTP_SENDER` | From address for all notification emails |
| `N8N_ADMIN_EMAIL` | Destination for admin/ops alerts (`$env.ADMIN_EMAIL`) |
| `N8N_COMPLIANCE_EMAIL` | Destination for KYC/compliance alerts (`$env.COMPLIANCE_EMAIL`) |
| `N8N_WEBHOOK_SECRET` | HMAC-SHA256 key matching `WCQ_N8N_WEBHOOKSECRET` on the backend |
| `N8N_ENCRYPTION_KEY` | n8n credential encryption key — `openssl rand -hex 32` |
| `N8N_BASIC_AUTH_USER` | n8n UI username (default: admin) |
| `N8N_BASIC_AUTH_PASS` | n8n UI password |

How workflows access these at runtime:
- `$env.ADMIN_EMAIL` in n8n expressions → reads from container env var `ADMIN_EMAIL`
- `process.env.APP_BASE_URL` in Code nodes → reads from container env var `APP_BASE_URL`
  (`APP_BASE_URL` is set to `FRONTEND_PUBLIC_URL` in the compose service)

## Workflow registry

The application calls n8n via `WCQ_N8N_WEBHOOKURL/{path}`. These paths are hardcoded
in workflow `webhookId` fields and in `internal/observability/notifier.go`.
Changing a `webhookId` requires a matching update in the notifier constants.

| File | Workflow name | `webhookId` (URL path suffix) | Trigger |
|---|---|---|---|
| `bank-transfer-admin-notify.json` | Bank Transfer Admin Notify | `transfer-uploaded` | Admin notified on bank transfer upload |
| `circuit-breaker-alert.json` | Circuit Breaker Alert | `circuit-breaker` | Prometheus → n8n on `WCQCircuitBreakerOpen` |
| `dlq-overflow-alert.json` | DLQ Overflow Alert | `dlq-overflow` | DLQ worker alert |
| `kyc-approved-user-notify.json` | KYC Approved — User Notify | `kyc-approved` | KYC profile approved |
| `kyc-balance-frozen-alert.json` | KYC Balance Frozen — Admin Alert | `kyc-balance-frozen` | Balance frozen on KYC review |
| `kyc-high-risk-escalation.json` | KYC High Risk — Escalation | `kyc-high-risk-escalation` | High-risk profile escalation |
| `kyc-queue-overflow-alert.json` | KYC Queue Overflow — Ops Alert | `kyc-queue-overflow` | KYC queue depth alert |
| `kyc-rejected-user-notify.json` | KYC Rejected — User Notify | `kyc-rejected` | KYC profile rejected |
| `kyc-review-reminder.json` | KYC Re-verification Reminder | *(scheduler-triggered)* | Scheduled reminder for pending reviews |
| `kyc-submission-admin-notify.json` | KYC Submission — Admin Notify | `kyc-submitted` | New KYC submission |
| `kyc-winner-freeze-alert.json` | KYC Winner Freeze — Compliance Alert | `kyc-winner-freeze` | Prize frozen on winner KYC issue |
| `payment-error-escalation.json` | Payment Error Escalation | `payment-error` | Prometheus → n8n on `WCQPaymentErrorRateHigh` |
| `payout-confirmation.json` | Payout Confirmation | `payout-approved` | Withdrawal approved and ready for processing |
| `prometheus-alert-relay.json` | Prometheus Alert Relay | `prometheus-alert-relay` | Generic Alertmanager webhook receiver |
| `sanctions-flag-alert.json` | KYC Sanctions Flag | `sanctions-flag` | Sanctions screening hit |

## CI validation

The test job in `.github/workflows/deploy.yml` runs
`python3 scripts/validate-n8n-workflows.py observability/n8n/workflows`
on every PR and push. The validator checks:
- Valid JSON structure and required fields (`name`, `nodes`, `connections`)
- Unique node IDs within each workflow
- Trigger node presence (webhook or schedule)
- Webhook path uniqueness across all workflows
- Connection graph integrity

## Modifying workflows

1. Make changes in the n8n UI on a staging/dev environment.
2. Export the workflow: **Workflow menu (⋮) → Download**.
3. Replace the relevant JSON file in this directory.
4. Set `"active": true` in the JSON (required for auto-activation on deploy).
5. If the n8n version changed, update `"meta": {"n8n_version": "<new-version>"}`.
6. Run the validator locally: `python3 scripts/validate-n8n-workflows.py`
7. Commit. The next deploy will update the workflow on the server automatically.

## Version upgrade procedure

1. Update the image tag in `docker-compose.observability.yml`.
2. Start the new container. Auto-import will run with the new version.
3. Test each workflow via the n8n **Execute Workflow** button with a synthetic payload.
4. If any node requires a `typeVersion` bump (n8n will warn), update the JSON.
5. Update `"meta": {"n8n_version": "<new-version>"}` in each modified JSON.
6. Commit the updated JSON files.
