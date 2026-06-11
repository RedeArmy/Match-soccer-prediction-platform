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
  while [ $elapsed -lt "$timeout" ]; do
    cid=$($COMPOSE ps -q "$svc" 2>/dev/null || true)
    if [ -n "$cid" ]; then
      status=$(docker inspect --format='{{.State.Health.Status}}' "$cid" 2>/dev/null || echo "")
      if [ "$status" = "healthy" ]; then
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

# Worker first — no user traffic; can tolerate a brief restart gap.
echo "==> restarting worker..."
$COMPOSE up -d --no-deps --force-recreate worker
wait_healthy worker 90

# API second — runs DB migrations on startup (advisory lock, multi-replica safe).
# Allow 120s: migrations may take up to ~30s on a large database.
echo "==> restarting api..."
$COMPOSE up -d --no-deps --force-recreate api
wait_healthy api 120

# Frontend last — BFF proxy routes require API to be healthy first.
echo "==> restarting frontend..."
$COMPOSE up -d --no-deps --force-recreate frontend
wait_healthy frontend 60

# ── 4. Ensure observability stack is up ──────────────────────────────────────
if [ -f "${WCQ_DIR}/docker-compose.observability.yml" ]; then
  echo "==> reconciling observability stack..."
  $OBS_COMPOSE up -d --remove-orphans
fi

# ── 5. Prune old images (keep last 2 per repo via `latest` + previous SHA) ───
echo "==> pruning dangling images..."
docker image prune -f

echo "==> deploy complete ✓"
