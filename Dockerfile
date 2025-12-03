# syntax=docker/dockerfile:1.6

###############################################################################
# Build stage
###############################################################################
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache build-base git

WORKDIR /src

# Cache modules
COPY go.mod go.sum ./
RUN go mod download

# Copy application source (excluding storage/tests via .dockerignore)
COPY cmd ./cmd
COPY internal ./internal
COPY web ./web

# Build binary (dynamic linking like Fusionaly)
RUN CGO_ENABLED=1 GOOS=linux go build \
  -trimpath \
  -o /src/formlander \
  ./cmd/formlander
# Runtime stage
###############################################################################
FROM alpine:3.22

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
