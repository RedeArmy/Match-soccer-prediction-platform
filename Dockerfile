# ── Build stage ───────────────────────────────────────────────────────────────
#
# The module files are copied and dependencies downloaded in a separate layer
# before the application source. Docker caches each layer independently, so
# this step is only re-executed when go.mod or go.sum changes — not on every
# source file change. In a typical CI pipeline this saves 30–60 seconds per
# build by reusing the cached dependency layer.
FROM golang:1.26.2-alpine3.22 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO_ENABLED=0 produces a statically linked binary with no libc dependency,
# which is required to run in the minimal runtime image below.
# -w strips DWARF debug tables (reduces binary size by ~20%).
# -s is intentionally omitted: stripping the symbol table breaks crash
# symbolication in Sentry, Datadog APM, and Go runtime stack traces.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w" \
    -o /bin/api \
    ./cmd/api

# ── Runtime stage ─────────────────────────────────────────────────────────────
#
# alpine is chosen over scratch because it provides:
#   - /etc/ssl/certs   Required for TLS connections to PostgreSQL and Redis.
#   - tzdata           Required for time-zone-aware scheduling operations.
#   - A POSIX shell    Useful for exec-ing into the container during incidents.
#
# If image size is a critical constraint, switch to scratch and COPY the
# required certificate bundles and timezone data from the builder stage.
FROM alpine:3.21

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=builder /bin/api .

# Run as a non-root user. Container orchestration platforms (Kubernetes with
# PodSecurityAdmission, ECS task definitions) commonly enforce this policy
# and will reject pods that run as UID 0.
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
USER appuser

EXPOSE 8080

CMD ["./api"]
