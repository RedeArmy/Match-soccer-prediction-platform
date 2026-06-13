#!/usr/bin/env bash
# server/deploy.sh — Rolling deploy script for the Hetzner production server.
#
# Called by GitHub Actions after new images are pushed to GHCR.
# Can also be run manually for hotfixes or rollbacks.
#
# Required env vars (passed by GitHub Actions or set manually):
#   API_IMAGE      — fully qualified image ref, e.g. ghcr.io/owner/repo:abc1234
#   WORKER_IMAGE   — e.g. ghcr.io/owner/repo-worker:abc1234
#   FRONTEND_IMAGE — e.g. ghcr.io/owner/repo-frontend:abc1234
#
# Usage (manual rollback to a previous SHA):
#   API_IMAGE=ghcr.io/owner/repo:abc1234 \
#   WORKER_IMAGE=ghcr.io/owner/repo-worker:abc1234 \
#   FRONTEND_IMAGE=ghcr.io/owner/repo-frontend:abc1234 \
#   bash server/deploy.sh

set -euo pipefail

: "${API_IMAGE:?API_IMAGE is required}"
: "${WORKER_IMAGE:?WORKER_IMAGE is required}"
: "${FRONTEND_IMAGE:?FRONTEND_IMAGE is required}"

WCQ_DIR=/opt/wcq
COMPOSE="docker compose -f ${WCQ_DIR}/docker-compose.prod.yml"
OBS_COMPOSE="docker compose -f ${WCQ_DIR}/docker-compose.observability.yml"

echo "==> deploy: api=${API_IMAGE}"
echo "            worker=${WORKER_IMAGE}"
echo "            frontend=${FRONTEND_IMAGE}"

# ── 1. Update image tags in .env ─────────────────────────────────────────────
# upsert: update existing line or append if missing (handles first deploy)
upsert_env() {
  local key=$1 value=$2
  if grep -q "^${key}=" "${WCQ_DIR}/.env" 2>/dev/null; then
    sed -i "s|^${key}=.*$|${key}=${value}|" "${WCQ_DIR}/.env"
  else
    echo "${key}=${value}" >> "${WCQ_DIR}/.env"
  fi
}

upsert_env "API_IMAGE"      "${API_IMAGE}"
upsert_env "WORKER_IMAGE"   "${WORKER_IMAGE}"
upsert_env "FRONTEND_IMAGE" "${FRONTEND_IMAGE}"

# ── 2. Pull new images (fail fast — before touching running containers) ───────
echo "==> pulling images..."
$COMPOSE pull --quiet api worker frontend

# ── 3. Rolling restart ────────────────────────────────────────────────────────
# wait_healthy: polls Docker health status until the container reports healthy
# or the timeout (seconds) is exceeded.
wait_healthy() {
  local svc=$1 timeout=${2:-90} elapsed=0 cid status
  echo "    waiting for ${svc} to be healthy..."
  while [[ $elapsed -lt "$timeout" ]]; do
    cid=$($COMPOSE ps -q "$svc" 2>/dev/null || true)
    if [[ -n "$cid" ]]; then
      status=$(docker inspect --format='{{.State.Health.Status}}' "$cid" 2>/dev/null || echo "")
      if [[ "$status" = "healthy" ]]; then
        echo "    ${svc}: healthy ✓"
        return 0
      fi
    fi
    sleep 2
    elapsed=$((elapsed + 2))
  done
  echo "ERROR: ${svc} did not become healthy in ${timeout}s" >&2
  $COMPOSE logs --tail=80 "$svc" >&2
  return 1
}

# Redis must be healthy before any app container starts.
# `--no-deps` is intentionally omitted here so that the network and
# volume are created on the first deploy. On subsequent deploys this
# is a no-op if Redis is already healthy.
echo "==> ensuring redis is healthy..."
$COMPOSE up -d --no-deps redis
wait_healthy redis 30

# API first — runs DB migrations on startup (advisory lock, multi-replica safe).
# The worker reads from system_params and other tables seeded by migrations, so
# it must not start until the API has completed all pending migrations.
# Allow 120s: migrations may take up to ~30s on a large database.
echo "==> restarting api..."
$COMPOSE up -d --no-deps --force-recreate api
wait_healthy api 120

# Worker second — all migrations are guaranteed applied by this point.
echo "==> restarting worker..."
$COMPOSE up -d --no-deps --force-recreate worker
wait_healthy worker 90

# Frontend last — BFF proxy routes require API to be healthy first.
echo "==> restarting frontend..."
$COMPOSE up -d --no-deps --force-recreate frontend
wait_healthy frontend 60

# ── 4. Reload Caddy to pick up any Caddyfile changes ────────────────────────
if systemctl is-active --quiet caddy 2>/dev/null; then
  echo "==> reloading caddy..."
  sudo systemctl reload caddy
  echo "    caddy: reloaded ✓"
else
  echo "==> caddy not managed by systemd — skipping reload"
fi

# ── 5. Ensure observability stack is up ──────────────────────────────────────
if [[ -f "${WCQ_DIR}/docker-compose.observability.yml" ]]; then
  # Remove any config files that were accidentally created as directories by a
  # failed scp run. Docker is used here so the cleanup runs as root inside the
  # container and can remove directories owned by container-process UIDs without
  # requiring a NOPASSWD sudoers rule.
  echo "==> fixing observability config type conflicts..."
  docker run --rm -v "${WCQ_DIR}/observability:/data" alpine \
    sh -c 'find /data -maxdepth 5 \( -name "*.yml" -o -name "*.json" \) -type d -exec rm -rf {} + 2>/dev/null; exit 0'

  echo "==> reconciling observability stack..."
  $OBS_COMPOSE up -d
fi

# ── 6. Prune old images (keep last 2 per repo via `latest` + previous SHA) ───
echo "==> pruning dangling images..."
docker image prune -f

echo "==> deploy complete ✓"
