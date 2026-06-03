FROM node:22-alpine AS base
WORKDIR /app
COPY package*.json ./

# ── Dev ─────────────────────────────────────────────────────────────────────
FROM base AS dev
RUN npm ci
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
RUN find /app -type d -exec chmod 555 {} \;

USER nextjs
EXPOSE 3000

HEALTHCHECK --interval=15s --timeout=5s --start-period=15s --retries=3 \
  CMD wget -qO- http://localhost:3000/api/health || exit 1

CMD ["node", "server.js"]
