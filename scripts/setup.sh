#!/usr/bin/env bash
# setup.sh — One-time local development environment initialisation.
#
# Run this script once after cloning the repository. It verifies that all
# required tools are installed, creates a local .env file from .env.example,
# starts the infrastructure containers, and waits until Postgres is ready to
# accept connections before returning control to the developer.
#
# Usage: ./scripts/setup.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

echo "==> Checking prerequisites..."

for cmd in docker docker-compose go; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
        echo "ERROR: '$cmd' is required but not installed. Aborting."
        exit 1
    fi
done

echo "==> Creating .env from .env.example (skipped if .env already exists)..."
if [ ! -f "$ROOT_DIR/.env" ]; then
    cp "$ROOT_DIR/.env.example" "$ROOT_DIR/.env"
    echo "    Created .env — set WCQ_JWT_SECRET before running the server."
else
    echo "    .env already exists, skipping."
fi

echo "==> Starting infrastructure services (Postgres, Redis)..."
docker-compose -f "$ROOT_DIR/docker-compose.yml" up -d

echo "==> Waiting for Postgres to become ready..."
until docker-compose -f "$ROOT_DIR/docker-compose.yml" exec -T postgres \
    pg_isready -U quiniela -d quiniela >/dev/null 2>&1; do
    printf "."
    sleep 1
done
echo " ready."

echo ""
echo "==> Setup complete."
echo "    Next steps:"
echo "      1. Set WCQ_JWT_SECRET in .env"
echo "      2. Run 'make migrate' to apply schema migrations"
echo "      3. Run 'make run' to start the API server"
