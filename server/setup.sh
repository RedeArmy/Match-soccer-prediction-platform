#!/usr/bin/env bash
# server/setup.sh — One-time initialisation of a fresh Hetzner CX53 server.
#
# Tested on Ubuntu 24.04 LTS.
# Safe to re-run: every step is idempotent.
#
# What this script does:
#   1. Creates the deploy user with Docker + passwordless-caddy-reload access
#   2. Installs Docker CE + Compose v2
#   3. Installs Caddy (system package, auto-managed by systemd)
#   4. Creates /opt/wcq directory structure (compose files, certs dir, config)
#   5. Configures the Caddy systemd unit to load /opt/wcq/.env
#   6. Logs in to GHCR so the deploy user can pull private images
#   7. Starts the full stack: app + observability
#
# Prerequisites on your LOCAL machine before running:
#   - SSH access as root to the server
#   - /opt/wcq/.env already present on the server (see server/.env.example)
#   - /opt/wcq/certs/supabase-ca.crt uploaded (Supabase → Settings → SSL)
#   - DNS A records for APP_DOMAIN, API_DOMAIN, GRAFANA_DOMAIN, N8N_DOMAIN
#     pointing to this server's IP (required for Caddy Let's Encrypt)
#
# Usage (from your local machine):
#   # 1. Upload the .env and Supabase CA cert:
#   scp server/.env.example root@<ip>:/opt/wcq/.env   # then edit on server
#   scp supabase-ca.crt     root@<ip>:/opt/wcq/certs/supabase-ca.crt
#
#   # 2. Run setup (streams the script; no repo clone needed on the server):
#   ssh root@<ip> 'bash -s' < server/setup.sh

set -euo pipefail

WCQ_DIR=/opt/wcq
DEPLOY_USER=deploy

# ── 1. deploy user ───────────────────────────────────────────────────────────
echo "==> [1/7] Configuring deploy user..."
if ! id "$DEPLOY_USER" &>/dev/null; then
  useradd -m -s /bin/bash "$DEPLOY_USER"
  echo "    user '${DEPLOY_USER}' created ✓"
else
  echo "    user '${DEPLOY_USER}' already exists — skipping"
fi

usermod -aG sudo "$DEPLOY_USER" 2>/dev/null || true

# Passwordless rules required by deploy.sh:
#   - systemctl reload/restart caddy  — picks up Caddyfile changes on every deploy
#   - chown /opt/wcq/observability    — reclaims ownership before scp -r when
#     container processes (n8n, tempo) have written files as root or other UIDs
cat > /etc/sudoers.d/deploy-wcq << 'EOF'
deploy ALL=(ALL) NOPASSWD: /usr/bin/systemctl reload caddy, /usr/bin/systemctl restart caddy
deploy ALL=(ALL) NOPASSWD: /usr/bin/chown -R deploy\:deploy /opt/wcq/observability
EOF
chmod 440 /etc/sudoers.d/deploy-wcq
echo "    sudoers rules written ✓"

# ── 2. Docker CE ─────────────────────────────────────────────────────────────
echo "==> [2/7] Installing Docker CE..."
apt-get update -qq
apt-get install -y -qq ca-certificates curl gnupg lsb-release

install -m 0755 -d /etc/apt/keyrings

if [[ ! -f /etc/apt/keyrings/docker.gpg ]]; then
  curl --proto '=https' --tlsv1.2 -fsSL https://download.docker.com/linux/ubuntu/gpg \
    | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
  chmod a+r /etc/apt/keyrings/docker.gpg
fi

if [[ ! -f /etc/apt/sources.list.d/docker.list ]]; then
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" \
    | tee /etc/apt/sources.list.d/docker.list > /dev/null
  apt-get update -qq
fi

apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
systemctl enable --now docker
usermod -aG docker "$DEPLOY_USER"
echo "    Docker $(docker --version) installed ✓"

# ── 3. Caddy ─────────────────────────────────────────────────────────────────
echo "==> [3/7] Installing Caddy..."
apt-get install -y -qq debian-keyring debian-archive-keyring apt-transport-https

if [[ ! -f /usr/share/keyrings/caddy-stable-archive-keyring.gpg ]]; then
  curl --proto '=https' --tlsv1.2 -fsSL 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' \
    | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
fi

if [[ ! -f /etc/apt/sources.list.d/caddy-stable.list ]]; then
  curl --proto '=https' --tlsv1.2 -fsSL 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' \
    | tee /etc/apt/sources.list.d/caddy-stable.list > /dev/null
  apt-get update -qq
fi

apt-get install -y -qq caddy
echo "    Caddy $(caddy version) installed ✓"

# ── 4. /opt/wcq directory structure ─────────────────────────────────────────
echo "==> [4/7] Creating /opt/wcq directory structure..."
mkdir -p "${WCQ_DIR}"/{certs,server,observability/prometheus/rules,observability/grafana/provisioning,observability/dashboards,observability/tempo}
chown -R "${DEPLOY_USER}:${DEPLOY_USER}" "${WCQ_DIR}"
echo "    directories created ✓"

# Verify the Supabase CA cert is present — the API and worker containers
# mount it at /certs/supabase-ca.crt for sslmode=verify-full connections.
if [[ ! -f "${WCQ_DIR}/certs/supabase-ca.crt" ]]; then
  echo ""
  echo "  WARNING: ${WCQ_DIR}/certs/supabase-ca.crt is missing."
  echo "  Download it from Supabase → Project → Settings → Database → SSL certificate"
  echo "  and upload it before starting the stack:"
  echo "    scp supabase-ca.crt root@<ip>:${WCQ_DIR}/certs/supabase-ca.crt"
  echo ""
fi

# Copy compose and config files from the repo when running from a local clone.
# When streamed via SSH ('bash -s' < setup.sh) these files must be SCP'd
# separately — the block below is a no-op in that case.
if [[ -f "$(dirname "$0")/../docker-compose.prod.yml" ]]; then
  REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
  cp "${REPO_ROOT}/docker-compose.prod.yml"          "${WCQ_DIR}/"
  cp "${REPO_ROOT}/Caddyfile"                        "${WCQ_DIR}/"
  cp "${REPO_ROOT}/server/deploy.sh"                 "${WCQ_DIR}/server/"
  [[ -f "${REPO_ROOT}/docker-compose.observability.yml" ]] && \
    cp "${REPO_ROOT}/docker-compose.observability.yml" "${WCQ_DIR}/"
  [[ -d "${REPO_ROOT}/observability" ]] && \
    cp -r "${REPO_ROOT}/observability/" "${WCQ_DIR}/"
  echo "    compose + config files copied ✓"
fi

# ── 5. Caddy systemd configuration ───────────────────────────────────────────
echo "==> [5/7] Configuring Caddy systemd unit..."
mkdir -p /etc/systemd/system/caddy.service.d

cat > /etc/systemd/system/caddy.service.d/env.conf << 'EOF'
[Service]
EnvironmentFile=/opt/wcq/.env
EOF

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

# ── 6. GHCR login ────────────────────────────────────────────────────────────
echo "==> [6/7] Logging in to GitHub Container Registry (ghcr.io)..."
if sudo -u "${DEPLOY_USER}" docker info 2>/dev/null | grep -q "ghcr.io"; then
  echo "    already logged in to ghcr.io — skipping"
else
  if [[ -t 0 ]]; then
    echo ""
    echo "  You need a GitHub Personal Access Token with 'read:packages' scope."
    echo "  Create one at: https://github.com/settings/tokens"
    echo ""
    read -rp  "  GitHub username: " GHCR_USER
    read -rsp "  GitHub PAT (read:packages): " GHCR_TOKEN
    echo ""
    echo "${GHCR_TOKEN}" | sudo -u "${DEPLOY_USER}" \
      docker login ghcr.io -u "${GHCR_USER}" --password-stdin
    echo "    GHCR login saved ✓"
  else
    echo "  WARNING: not running in a TTY — skipping interactive GHCR login."
    echo "  Log in manually as the deploy user before starting the stack:"
    echo "    sudo -u ${DEPLOY_USER} docker login ghcr.io"
  fi
fi

# ── 7. Start the stack ───────────────────────────────────────────────────────
echo "==> [7/7] Starting services..."

if [[ ! -f "${WCQ_DIR}/.env" ]]; then
  echo "ERROR: ${WCQ_DIR}/.env not found." >&2
  echo "       Copy server/.env.example to ${WCQ_DIR}/.env and fill in real values," >&2
  echo "       then re-run or start manually:" >&2
  echo "         cd ${WCQ_DIR}" >&2
  echo "         docker compose -f docker-compose.prod.yml up -d" >&2
  echo "         systemctl start caddy" >&2
  exit 1
fi

if [[ ! -f "${WCQ_DIR}/docker-compose.prod.yml" ]]; then
  echo "ERROR: ${WCQ_DIR}/docker-compose.prod.yml not found." >&2
  echo "       SCP it from the repo before starting:" >&2
  echo "         scp docker-compose.prod.yml root@<ip>:${WCQ_DIR}/" >&2
  exit 1
fi

cd "${WCQ_DIR}"
sudo -u "${DEPLOY_USER}" docker compose -f docker-compose.prod.yml up -d

if [[ -f "${WCQ_DIR}/docker-compose.observability.yml" ]]; then
  sudo -u "${DEPLOY_USER}" docker compose -f docker-compose.observability.yml up -d
fi

systemctl start caddy

echo ""
echo "================================================================"
echo "  Server setup complete ✓"
echo ""
echo "  Next steps:"
echo "    1. Verify DNS:   dig +short \${APP_DOMAIN}"
echo "    2. Check Caddy:  systemctl status caddy"
echo "    3. Check stack:  docker ps"
echo "    4. Add GitHub Actions secrets (see server/.env.example)"
echo "    5. Upload Supabase CA cert if not already done:"
echo "       scp supabase-ca.crt root@<ip>:${WCQ_DIR}/certs/supabase-ca.crt"
echo "================================================================"
