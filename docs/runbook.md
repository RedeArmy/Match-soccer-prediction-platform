# On-call Runbook — World Cup Quiniela

One-page decision guide per alert. Each section maps to a `runbook_url` annotation in
`observability/prometheus/rules/alerting_rules.yml`.

**Escalation path (all alerts):** Platform Slack channel → lead engineer → CTO.
No 24/7 on-call rotation is established yet; all alerts route through n8n and Slack.

---

## WCQDLQDepthWarning / WCQDLQDepthCritical

**What it means:** The `notification_dlq` table holds dispatch entries that exhausted
all retries. Warning = > 10 unresolved entries for 5 min. Critical = > 50 for 5 min.
Users are not receiving notifications (email, push, in-app) for the affected events.

**First actions:**
1. `GET /api/v1/admin/notification-dlq` — review the event types and error messages.
   Common causes: Resend API key expired, VAPID keys rotated, database write spike.
2. Check the worker logs (`fly logs -a wcq-worker`) for the error that triggered the DLQ.
3. If the provider is recovered, use `POST /api/v1/admin/notification-dlq/{id}/resolve`
   to re-queue individual entries, or `POST .../replay` for bulk replay.

**Resolution verification:** `outbox_dlq_depth` drops to zero within 2 minutes of replay.

---

## WCQOutboxLagHigh / WCQOutboxLagCritical

**What it means:** The oldest due-but-unprocessed outbox row is older than 30 s (High)
or 5 min (Critical). Notifications are delayed; the worker may be overloaded or stopped.

**First actions:**
1. Confirm the worker process is running: `fly status -a wcq-worker`.
   If not running, restart: `fly machine restart -a wcq-worker`.
2. Check worker logs for repeated errors (DB connection failure, Redis unavailable).
3. If the worker is running but lagging, check `outbox_pending_events` — a large spike
   may have overwhelmed the batch processor. The batch size is `notify.outbox_batch_size`
   in system_params; temporarily increase it if safe to do so.

**Resolution verification:** `outbox_oldest_pending_age_seconds` drops below 30 s.

---

## WCQCircuitBreakerOpen

**What it means:** A circuit breaker (`{{ $labels.backend }}`) has been open for > 60 s.
Calls to that backend are failing immediately without reaching the dependency.

**Backends and their effect:**
- `paypal-cert`: PayPal webhook verification disabled → PayPal webhooks return 500.
- `file-store`: File uploads (bank transfer proofs, KYC documents) return 500.
- `cache`: Leaderboard and match-list cache bypassed → all requests hit the DB directly.

**First actions:**
1. Check the dependency status page (PayPal, Cloudflare R2, etc.).
2. Review recent error logs for the backend name: `fly logs -a wcq-api | grep backend`.
3. Circuit breakers are self-healing — if the dependency recovers, the breaker
   transitions to half-open automatically after the configured cooldown.

**Resolution verification:** `circuit_breaker_state{backend="..."}` returns to 0 (closed).

---

## WCQPaymentErrorRateHigh

**What it means:** More than 1 % of payment webhook calls failed in the last 5 minutes.
Users' deposits may not be confirming; money is not being credited to balances.

**First actions:**
1. Check `/webhooks/recurrente` and `/webhooks/paypal` error rates separately in
   the Grafana HTTP dashboard (filter by `route`).
2. **Recurrente errors:** Verify `WCQ_PAYMENT_RECURRENTEWEBHOOKSECRET` matches the
   Recurrente dashboard. If the secret was rotated, update the Fly secret:
   `fly secrets set WCQ_PAYMENT_RECURRENTEWEBHOOKSECRET=<new>`.
3. **PayPal errors:** Check if `WCQ_PAYMENT_PAYPALWEBHOOKID` is current. Check
   `WCQCircuitBreakerOpen{backend="paypal-cert"}` — if the cert fetcher is open,
   PayPal verification is failing because the cert endpoint is unreachable.

**Resolution verification:** Payment error rate drops below 0.1 % over a 5-min window.

---

## WCQHTTP5xxRateHigh

**What it means:** Global HTTP 5xx rate exceeds 0.5 % for 5 minutes.
This is a catch-all for errors not covered by more specific alerts.

**First actions:**
1. Check the Grafana HTTP dashboard — identify which routes are producing 5xx.
2. If a single route is responsible, look at service logs for that handler.
3. If the database is implicated (pgx errors in logs), verify DB health:
   `fly postgres connect -a wcq-db -c "SELECT 1"`.

**Resolution verification:** 5xx rate drops below 0.1 % over a 5-min window.

---

## WCQAMLThresholdHit

**What it means:** One or more transactions in the last 5 minutes met or exceeded the
`kyc.aml_threshold_cents` system parameter (default Q25,000). The transaction was **not
blocked** — it committed normally. A corresponding `kyc.aml_flagged` entry was written to
`audit_log`. This alert fires immediately so the compliance team can review and file a UAF
report if required by the applicable Guatemalan AML regulations.

**First actions:**
1. Identify the transactions requiring review:
   ```sql
   SELECT al.id, al.actor_id, al.resource_type, al.resource_id,
          al.data->>'amount_cents' AS amount_cents,
          al.data->>'currency' AS currency,
          al.data->>'operation' AS operation,
          al.created_at
   FROM audit_log al
   WHERE al.action = 'kyc.aml_flagged'
     AND al.created_at >= NOW() - INTERVAL '1 hour'
   ORDER BY al.created_at DESC;
   ```
2. For each flagged transaction, obtain the user profile from `kyc_profiles` and check
   their KYC tier and verification history.
3. Determine whether a UAF report is required under Article 23 of the Guatemalan
   Ley Contra el Lavado de Dinero u Otros Activos.
4. If a report is required, complete the UAF electronic form within the mandatory window.
5. Log the review decision (report filed / not applicable) in the incident log.

**Tuning the threshold:** If the alert fires too frequently (legitimate users regularly
exceed Q25,000), raise `kyc.aml_threshold_cents` via the admin system params API. The
default Q25,000 matches the UAF mandatory reporting threshold; do not lower it below this
value.

---

## WCQKYCAuditEventDropped

**What it means:** A KYC operation (submission, approval, freeze) committed without its
corresponding `kyc_events` row. This is a compliance gap — the audit trail is incomplete.

**First actions:**
1. Identify the affected user from the service logs (search for `kyc_audit_event_drop`).
2. Reconstruct the missing event from the `kyc_profiles` table and insert it manually
   into `kyc_events` with the correct `event_type`, `actor_id`, and `created_at`.
3. Document the gap and the manual fix in the incident log.

**This alert requires a post-incident write-up regardless of severity.**

---

## WCQIPRateLimitGlobalBlocked

**What it means:** The L1 global IP rate limiter has blocked more than 100 requests per minute
from one or more source IPs for at least 2 consecutive minutes. This indicates a concentrated
DoS attempt, aggressive bot scan, or misconfigured client.

**First actions:**
1. Identify the offending IP(s): filter `wcq_ip_rate_limit_blocked_total{layer="global"}` by
   source IP label in Grafana or query directly in Prometheus.
2. Cross-reference the IP against the request logs: `fly logs -a wcq-api | grep <ip>`.
   - If the IP is a known bot or crawler: no immediate action; the rate limiter is doing its job.
   - If the IP belongs to a legitimate user (e.g., shared NAT): consider raising the global
     bucket limit via `api.ip_rate_limit_global_rps` in system_params.
3. If the blocking pattern suggests a coordinated attack from multiple IPs, consider adding
   Fly.io firewall rules to block the offending CIDR ranges.

**Resolution verification:** `increase(wcq_ip_rate_limit_blocked_total{layer="global"}[1m])`
drops below 100.

---

## WCQIPRateLimitWebhookBlocked

**What it means:** The L2 webhook IP rate limiter has blocked more than 20 payment webhook
requests per minute. Legitimate webhook senders (Recurrente, PayPal) are well-behaved and
will never trigger this threshold under normal operation. Any non-zero rate here almost
certainly indicates a replay attack cycling through fake source IPs.

**First actions:**
1. Confirm the attack source: `fly logs -a wcq-api | grep "webhooks/"` — look for repeated
   POST requests from unusual IPs.
2. Verify PayPal and Recurrente delivery status on their respective dashboards to confirm
   legitimate webhooks are not being blocked (they should not be, given the low baseline rate).
3. If the attack is ongoing, consider temporarily raising the L2 bucket threshold via
   `api.ip_rate_limit_webhook_rps` in system_params, or applying Fly firewall rules.
4. Check `WCQPaymentErrorRateHigh` — if the attack exhausted PayPal's retry budget, real
   webhook deliveries may have been dropped. Trigger manual reconciliation if needed.

**Resolution verification:** `increase(wcq_ip_rate_limit_blocked_total{layer="webhook"}[1m])`
drops to zero.

---

## WCQRateLimitDegraded

**What it means:** The Redis-backed rate limiter fell back to in-process mode because
Redis was unavailable. Per-replica limits apply but cross-replica enforcement is disabled.

**First actions:**
1. Check Redis health: `fly redis status`.
2. If Redis is down, check the Fly Redis plan limits — the free tier has connection caps.
3. The application continues to function; this is a degraded-but-safe state.

**Resolution verification:** `wcq_rate_limit_fail_open_total` stops increasing.

---

## WCQIdempotencyDegraded

**What it means:** The Redis idempotency store was unavailable for payment write endpoints.
`POST /withdrawals` and `POST /bank-transfers` may have executed more than once on
concurrent requests across replicas during this window.

**First actions:**
1. Restore Redis (same steps as WCQRateLimitDegraded).
2. Immediately review recent withdrawal and bank-transfer activity for duplicates:
   `SELECT * FROM withdrawal_requests WHERE created_at > NOW() - INTERVAL '10 minutes'`.
   If duplicates exist, cancel the redundant ones via the admin API.

**This is a critical state for money movement — act within 15 minutes.**

---

## WCQSSEBroadcastDrops

**What it means:** SSE events were dropped because one or more client connection buffers
were full. Users with a slow or stalled browser connection are missing real-time updates.

**First actions:**
1. This is usually self-healing — the hub evicts connections after 5 consecutive drops.
2. Monitor `notification_sse_evicted_total` to confirm eviction is working.
3. If drops continue without eviction, check `notify.sse_chan_buf_size` in system_params
   and consider increasing it (default 64; requires API server restart to take effect).

---

## WCQPrizeDistributionFailed

**What it means:** A prize distribution attempt failed with a non-idempotency error after
`prizes_distributed_at` may have been stamped on the quiniela. Some winners may be
missing their credits. This requires manual reconciliation.

**First actions:**
1. Find the affected quiniela in the audit log:
   `GET /api/v1/admin/audit-log` filtered by `action=admin_group.prizes_distributed`.
2. Query `balance_ledger` for the quiniela's winners to verify which credits were applied:
   `SELECT user_id, delta_cents, ref_id, ref_type FROM balance_ledger WHERE ref_type='quiniela' AND ref_id=<id>`.
3. For any winner missing their credit, use the admin balance endpoint to credit
   the correct amount manually and document the action in the audit log.

**Do not re-trigger `POST /admin/groups/{id}/distribute-prizes`** — if
`prizes_distributed_at` was stamped, a second call returns 409. Manual credit is required.

---

## WCQLedgerRowCountWarning

**What it means:** The `balance_ledger` table has grown beyond 2M rows (warning) or 10M
rows (critical). Partitioning is recommended at ~50M rows per ADR-0008. No immediate
operational action is required, but the zero-downtime migration must be planned.

**First actions:**
1. Confirm the row count: `SELECT COUNT(*) FROM balance_ledger;` (exact) or
   `SELECT reltuples::bigint FROM pg_class WHERE relname = 'balance_ledger';` (fast estimate).
2. Review the partitioning plan in [`docs/adr/0008-balance-ledger-partitioning.md`](adr/0008-balance-ledger-partitioning.md).
3. Read the DB pool sizing guidance and scaling triggers in [`docs/capacity.md`](capacity.md)
   before scheduling the migration — the partitioning window also affects pool configuration.
4. Schedule the migration during a low-traffic window (after the FIFA 2026 tournament,
   if applicable). The migration is zero-downtime but requires ~1h for large tables.

**Resolution verification:** Alert clears automatically once the row count drops below
the threshold (e.g., after a successful migration and old-table cleanup).

---

## WCQRateLimitDegradedPerReplica

> **Note:** This alert is merged with `WCQRateLimitDegraded` in alerting_rules.yml.
> This section documents the per-replica fallback behaviour for reference.

**Per-replica RPS ceiling when Redis is unavailable:**

When Redis is unreachable, the per-user token bucket falls back to an in-process
`LimiterStore`. Rate limits apply per replica — a user can send N requests to each
Fly.io machine independently.

| Config | Default | Per-replica ceiling (1 replica) | Effective ceiling (2 replicas) |
|--------|---------|----------------------------------|--------------------------------|
| `api.rate_limit_rps` | 10 req/s | 10 req/s | 20 req/s |
| `api.rate_limit_burst` | 20 req | 20 req burst | 40 req burst |

Under normal conditions with Redis available, the Redis-backed store enforces limits
cluster-wide. The degraded state is visible via `wcq_rate_limit_fail_open_total`.

---

## Capacity Planning

Full capacity model, measured baselines, and scaling triggers are documented in
[`docs/capacity.md`](capacity.md). Quick reference for on-call decisions:

| Trigger | Action | Detail in capacity.md |
|---|---|---|
| p99 latency > 1 s sustained | Raise `WCQ_DATABASE_MAXOPENCONNS`; consider read replica | DB Pool Capacity section |
| `WCQLedgerRowCountWarning` fires | Plan partitioning migration | Scaling Recommendations table |
| Multiple Fly.io machines deployed | Verify Redis rate-limiting is active | Runtime Configuration table |
| Persistent DB pool wait > 50 ms | Scale connections or add caching | Theoretical DB Pool Capacity |

**Connection pool sizing** (from `docs/capacity.md`):

With `MaxOpenConns = 25` and Fly.io `hard_limit = 250`:
- A 10:1 request-to-connection ratio is expected under normal load.
- Simple reads hold a connection for ~3 ms → pool capacity ~8,000 ops/s.
- Transactional writes hold ~10 ms → pool capacity ~2,500 ops/s.
- Prize distribution holds 50–200 ms but is admin-triggered and infrequent.

**Connection pool E2E test**: `TestConcurrentReadinessProbes` in
`internal/api/e2e_concurrency_test.go` fires 50 concurrent `/health/ready` requests
against a 5-connection pool (10× oversubscription, mirroring production's 25/250 ratio).
Run with `make test-integration`.
