# syntax=docker/dockerfile:1.6

###############################################################################
# Build stage
###############################################################################
FROM golang:1.25.5-alpine AS builder

ARG TARGETARCH
ARG COMMIT_SHA=dev

RUN apk add --no-cache build-base git curl

WORKDIR /src

# Cache modules
COPY go.mod go.sum ./
RUN go mod download

# Download Tailwind CLI (pinned to v3.4.17)
RUN TAILWIND_VERSION="v3.4.17" && \
  case "${TARGETARCH}" in \
    amd64) TAILWIND_ASSET="tailwindcss-linux-x64" ;; \
    arm64) TAILWIND_ASSET="tailwindcss-linux-arm64" ;; \
    *) echo "Unsupported arch: ${TARGETARCH}"; exit 1 ;; \
  esac && \
  curl -sL "https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND_VERSION}/${TAILWIND_ASSET}" -o /usr/local/bin/tailwindcss && \
  chmod +x /usr/local/bin/tailwindcss

# Copy Tailwind config and source CSS
COPY tailwind.config.js ./
COPY web ./web

# Build Tailwind CSS
RUN tailwindcss -i web/static/app.css -o web/static/app.built.css --minify

# Copy remaining application source
COPY cmd ./cmd
COPY internal ./internal
COPY pkg ./pkg

# Build binary with commit SHA for cache busting
RUN CGO_ENABLED=1 GOOS=linux go build \
  -trimpath \
  -ldflags="-X formlander/internal/server.buildCommit=${COMMIT_SHA}" \
  -o /src/formlander \
  ./cmd/formlander
# Runtime stage
###############################################################################
FROM alpine:3.20

WORKDIR /app

# Install minimal runtime dependencies
RUN apk add --no-cache ca-certificates tzdata curl && \
  addgroup -g 1000 formlander && \
  adduser -D -u 1000 -G formlander formlander && \
  mkdir -p /app/storage /app/storage/logs && \
  chown -R formlander:formlander /app

COPY --from=builder /src/formlander /usr/local/bin/formlander

# Run as non-root user
USER formlander

ENV FORMLANDER_ENV=production \
  FORMLANDER_PORT=8080 \
  FORMLANDER_DATA_DIR=/app/storage \
  FORMLANDER_LOGS_DIR=/app/storage/logs \
  FORMLANDER_SESSION_TIMEOUT_SECONDS=1800

EXPOSE 8080

# Health check using curl with proper error handling
# -f flag makes curl fail on HTTP errors (4xx, 5xx)
# Using shell form to ensure environment variable expansion
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD /bin/sh -c "curl -f http://localhost:${FORMLANDER_PORT}/_health || exit 1"

VOLUME ["/app/storage"]

ENTRYPOINT ["/usr/local/bin/formlander"]
