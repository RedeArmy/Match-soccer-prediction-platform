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

DSN="${WCQ_DATABASE_DSN:-postgres://quiniela:quiniela@localhost:5432/quiniela?sslmode=disable}"

# Refuse to run against any host that does not resolve to the local machine.
if [[ "$DSN" != *"localhost"* && "$DSN" != *"127.0.0.1"* ]]; then
    echo "ERROR: WCQ_DATABASE_DSN does not point to localhost."
    echo "       This script must not be run against a non-local database."
    echo "       DSN: $DSN"
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
    psql -U quiniela -c "DROP DATABASE IF EXISTS quiniela;"
docker-compose -f "$ROOT_DIR/docker-compose.yml" exec -T postgres \
    psql -U quiniela -c "CREATE DATABASE quiniela;"

echo "==> Running migrations..."
WCQ_DATABASE_DSN="$DSN" go run "$ROOT_DIR/cmd/migrate"

echo "==> Database reset complete."
