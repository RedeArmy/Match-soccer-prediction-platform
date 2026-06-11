#!/usr/bin/env bash
# server/setup.sh — One-time initialization of a fresh Hetzner CX53 server.
#
# Tested on Ubuntu 24.04 LTS.
#
# What this script does:
#   1. Creates a non-root `deploy` user with Docker access
#   2. Installs Docker CE + Compose v2
#   3. Installs Caddy (system package, auto-managed by systemd)
#   4. Creates /opt/wcq with the repo's compose files and config
#   5. Configures Caddy to load env from /opt/wcq/.env (for {$VAR} substitution)
#   6. Logs in to GHCR so the deploy user can pull private images
#   7. Starts the full stack: app + observability
#
# Prerequisites on your LOCAL machine:
#   - SSH access as root (or sudo user) to the server
#   - A .env file ready at /opt/wcq/.env on the server (see server/.env.example)
#   - DNS A records pointing APP_DOMAIN, API_DOMAIN, GRAFANA_DOMAIN, N8N_DOMAIN
#     to the server IP (needed for Caddy TLS)
#
# Usage (from your local machine):
#   scp server/.env.example root@<server-ip>:/opt/wcq/.env
#   # Edit /opt/wcq/.env on the server to fill in real values, then:
#   ssh root@<server-ip> 'bash -s' < server/setup.sh

set -euo pipefail

WCQ_DIR=/opt/wcq
DEPLOY_USER=deploy

echo "==> [1/7] Creating deploy user..."
id "$DEPLOY_USER" &>/dev/null || useradd -m -s /bin/bash "$DEPLOY_USER"
usermod -aG sudo "$DEPLOY_USER" 2>/dev/null || true

echo "==> [2/7] Installing Docker CE..."
apt-get update -qq
apt-get install -y -qq ca-certificates curl gnupg lsb-release

install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg \
  | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
chmod a+r /etc/apt/keyrings/docker.gpg

echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
  https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" \
  | tee /etc/apt/sources.list.d/docker.list > /dev/null

apt-get update -qq
apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

systemctl enable --now docker
usermod -aG docker "$DEPLOY_USER"
echo "    Docker $(docker --version) installed ✓"

echo "==> [3/7] Installing Caddy..."
apt-get install -y -qq debian-keyring debian-archive-keyring apt-transport-https curl
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' \
  | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' \
  | tee /etc/apt/sources.list.d/caddy-stable.list
apt-get update -qq
apt-get install -y -qq caddy
echo "    Caddy $(caddy version) installed ✓"

echo "==> [4/7] Creating /opt/wcq directory structure..."
mkdir -p "${WCQ_DIR}"/{observability/prometheus/rules,observability/grafana/provisioning,observability/dashboards,observability/tempo,server}
chown -R "${DEPLOY_USER}:${DEPLOY_USER}" "${WCQ_DIR}"

# Copy compose files and observability config from the repo (run from repo root)
# If running via `ssh root@host 'bash -s' < server/setup.sh` you need to scp
# the files separately. This block is a no-op if run directly on the server
# from the cloned repo.
if [[ -f "$(dirname "$0")/../docker-compose.prod.yml" ]]; then
  REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
  cp "${REPO_ROOT}/docker-compose.prod.yml"          "${WCQ_DIR}/"
  cp "${REPO_ROOT}/docker-compose.observability.yml" "${WCQ_DIR}/"
  cp "${REPO_ROOT}/Caddyfile"                        "${WCQ_DIR}/"
  cp "${REPO_ROOT}/server/deploy.sh"                 "${WCQ_DIR}/server/"
  cp -r "${REPO_ROOT}/observability/"                "${WCQ_DIR}/"
  echo "    Compose + config files copied ✓"
fi

echo "==> [5/7] Configuring Caddy to load /opt/wcq/.env..."
mkdir -p /etc/systemd/system/caddy.service.d
cat > /etc/systemd/system/caddy.service.d/env.conf << 'EOF'
[Service]
EnvironmentFile=/opt/wcq/.env
EOF

# Tell Caddy to use the Caddyfile in /opt/wcq
mkdir -p /etc/systemd/system/caddy.service.d
cat > /etc/systemd/system/caddy.service.d/config.conf << 'EOF'
[Service]
ExecStart=
ExecStart=/usr/bin/caddy run --environ --config /opt/wcq/Caddyfile
ExecReload=
ExecReload=/usr/bin/caddy reload --config /opt/wcq/Caddyfile --force
EOF

systemctl daemon-reload
systemctl enable caddy
echo "    Caddy systemd unit configured ✓"

echo "==> [6/7] Logging in to GitHub Container Registry (ghcr.io)..."
echo ""
echo "    You will need a GitHub Personal Access Token (PAT) with"
echo "    'read:packages' scope to pull private images."
echo "    Create one at: https://github.com/settings/tokens"
echo ""
read -rp "    GitHub username: " GHCR_USER
read -rsp "    GitHub PAT (read:packages): " GHCR_TOKEN
echo ""
echo "${GHCR_TOKEN}" | sudo -u "${DEPLOY_USER}" \
  docker login ghcr.io -u "${GHCR_USER}" --password-stdin
echo "    GHCR login saved ✓"

echo "==> [7/7] Starting services..."
if [[ ! -f "${WCQ_DIR}/.env" ]]; then
  echo "ERROR: ${WCQ_DIR}/.env not found." >&2
  echo "       Copy server/.env.example to ${WCQ_DIR}/.env and fill in real values,"
  echo "       then re-run this step manually:"
  echo "         cd ${WCQ_DIR}"
  echo "         docker compose -f docker-compose.prod.yml up -d"
  echo "         docker compose -f docker-compose.observability.yml up -d"
  echo "         systemctl start caddy"
  exit 1
fi

cd "${WCQ_DIR}"
sudo -u "${DEPLOY_USER}" docker compose -f docker-compose.prod.yml up -d
sudo -u "${DEPLOY_USER}" docker compose -f docker-compose.observability.yml up -d
systemctl start caddy

echo ""
echo "================================================================"
echo "  Server setup complete ✓"
echo ""
echo "  Next steps:"
echo "    1. Verify DNS records point to this server's IP"
echo "    2. Check Caddy TLS:  systemctl status caddy"
echo "    3. Check services:   docker ps"
echo "    4. Add GitHub Actions secrets (see table in server/.env.example)"
echo "================================================================"
