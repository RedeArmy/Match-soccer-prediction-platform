# n8n Automation Workflows

Operational alerting and notification workflows for the World Cup Quiniela platform.
These JSON files are the canonical source for all n8n workflows — the files in this
directory, not the n8n UI, are the source of truth.

## Required n8n version

**1.49.0** — pinned in `docker-compose.observability.yml` as `n8nio/n8n:1.49.0`.

Each workflow JSON carries `"meta": {"n8n_version": "1.49.0"}` to document which
version the workflow was designed and tested against. Upgrading n8n may change node
`typeVersion` requirements or execution semantics; verify all workflows after any
version bump.

## Importing workflows

1. Start the observability stack:
   ```
   docker compose -f docker-compose.yml -f docker-compose.observability.yml up -d n8n
   ```
2. Open the n8n UI at `http://localhost:5678` and log in.
3. For each `.json` file in `workflows/`:
   - Click **New Workflow → Import from file** and select the file.
   - Activate the workflow with the toggle in the top-right.
4. Set the required environment variables in n8n (Settings → Variables or `.env`):
   - `ADMIN_EMAIL` — destination address for admin-facing alerts.
   - `COMPLIANCE_EMAIL` — destination address for KYC/compliance alerts.

Do **not** export workflows from the n8n UI and commit the result without also updating
the version tag in `meta.n8n_version` if the n8n version changed.

## Webhook path registry

The application calls n8n via `WCQ_N8N_WEBHOOKURL/{path}` (configured via env var).
These paths are hardcoded in the workflow `webhookId` fields and in
`internal/observability/notifier.go`. Changing a `webhookId` in a workflow requires
a matching update in the notifier constants.

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

## Version upgrade procedure

1. Pull the new n8n image in `docker-compose.observability.yml`.
2. Start the new container and import each workflow JSON.
3. Test each workflow with a synthetic payload via the n8n **Execute Workflow** button.
4. If any node requires a `typeVersion` bump (n8n will warn on import), update the JSON.
5. Update `"meta": {"n8n_version": "<new-version>"}` in each modified workflow JSON.
6. Commit the updated JSON files with the version bump.
