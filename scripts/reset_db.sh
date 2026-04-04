#!/usr/bin/env bash
# reset_db.sh — Drop and recreate the local development database.
#
# Useful when a migration has been applied incorrectly and you need a clean
# slate. This script is intentionally destructive.
#
# Safety: the script refuses to run if WCQ_DATABASE_DSN does not point to
# localhost or 127.0.0.1, preventing accidental execution against a staging
# or production database.
#
# Usage: ./scripts/reset_db.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

# Load .env so that POSTGRES_* and WCQ_DATABASE_DSN are available even when
# the script is run directly (not via `make`). Variables already exported in
# the caller's environment take precedence over the file values.
if [[ -f "$ROOT_DIR/.env" ]]; then
    set -o allexport
    # shellcheck source=/dev/null
    source "$ROOT_DIR/.env"
    set +o allexport
fi

# All credentials are read from environment variables — no defaults are
# hardcoded here. The values must be present in .env or exported by the caller.
if [[ -z "${POSTGRES_USER:-}" || -z "${POSTGRES_DB:-}" || -z "${WCQ_DATABASE_DSN:-}" ]]; then
    echo "ERROR: POSTGRES_USER, POSTGRES_DB, and WCQ_DATABASE_DSN must be set." >&2
    echo "       Copy .env.example to .env and fill in the required values." >&2
    exit 1
fi

DSN="$WCQ_DATABASE_DSN"

# Refuse to run against any host that does not resolve to the local machine.
if [[ "$DSN" != *"localhost"* && "$DSN" != *"127.0.0.1"* ]]; then
    echo "ERROR: WCQ_DATABASE_DSN does not point to localhost." >&2
    echo "       This script must not be run against a non-local database." >&2
    echo "       DSN: $DSN" >&2
    exit 1
fi

echo "==> WARNING: This will destroy all data in the local database."
read -r -p "    Are you sure? [y/N] " confirm
if [[ "${confirm,,}" != "y" ]]; then
    echo "Aborted."
    exit 0
fi

echo "==> Dropping and recreating database..."
docker-compose -f "$ROOT_DIR/docker-compose.yml" exec -T postgres \
    psql -U "$POSTGRES_USER" -c "DROP DATABASE IF EXISTS $POSTGRES_DB;"
docker-compose -f "$ROOT_DIR/docker-compose.yml" exec -T postgres \
    psql -U "$POSTGRES_USER" -c "CREATE DATABASE $POSTGRES_DB;"

echo "==> Running migrations..."
WCQ_DATABASE_DSN="$DSN" go run "$ROOT_DIR/cmd/migrate"

echo "==> Database reset complete."
