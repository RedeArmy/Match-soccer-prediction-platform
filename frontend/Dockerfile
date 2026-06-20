FROM node:22-alpine AS base
WORKDIR /app
COPY package*.json ./

# ── Dev ─────────────────────────────────────────────────────────────────────
FROM base AS dev
RUN npm ci --ignore-scripts
EXPOSE 3000
CMD ["npm", "run", "dev"]

# ── Builder ──────────────────────────────────────────────────────────────────
FROM base AS builder
RUN npm ci --frozen-lockfile --ignore-scripts
COPY . .
ENV NEXT_TELEMETRY_DISABLED=1
# Clerk publishable key must be available at build time for static pages
ARG NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY
ENV NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY=$NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY
# PayPal client ID is read by client components (e.g. the deposit page) and is
# inlined into the JS bundle at build time — setting it at container runtime
# has no effect, so it must be available here too.
ARG NEXT_PUBLIC_PAYPAL_CLIENT_ID
ENV NEXT_PUBLIC_PAYPAL_CLIENT_ID=$NEXT_PUBLIC_PAYPAL_CLIENT_ID
ARG NEXT_PUBLIC_GRAFANA_URL
ENV NEXT_PUBLIC_GRAFANA_URL=$NEXT_PUBLIC_GRAFANA_URL
RUN npm run build

# ── Runner (production) ───────────────────────────────────────────────────────
FROM node:22-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1

RUN addgroup --system --gid 1001 nodejs \
 && adduser  --system --uid 1001 nextjs

COPY --from=builder /app/public ./public
COPY --from=builder --chown=nextjs:nodejs --chmod=444 /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs --chmod=444 /app/.next/static ./.next/static
# Directories need the execute bit for traversal; files stay read-only (444).
RUN find /app -type d -exec chmod 555 {} \; \
 && mkdir -p /app/.next/cache \
 && chown nextjs:nodejs /app/.next/cache \
 && chmod 755 /app/.next/cache

USER nextjs
EXPOSE 3000
# Next.js standalone reads HOSTNAME to determine the bind address.
# Docker injects HOSTNAME=<container-id> by default, causing the server to
# bind to the container's Docker network IP instead of all interfaces.
ENV HOSTNAME=0.0.0.0

HEALTHCHECK --interval=15s --timeout=5s --start-period=15s --retries=3 \
  CMD node -e "require('http').get('http://127.0.0.1:3000/api/health',r=>process.exit(r.statusCode===200?0:1)).on('error',()=>process.exit(1))"

CMD ["node", "server.js"]
