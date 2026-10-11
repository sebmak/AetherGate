# -------------------------------------------------------------
# Stage 1: Build Frontend (React + Vite + TypeScript)
# -------------------------------------------------------------
FROM node:22-alpine AS web-builder

WORKDIR /app/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci

COPY web/ ./
RUN npm run build

# -------------------------------------------------------------
# Stage 2: Build Backend (Go + Embedded Frontend Assets)
# -------------------------------------------------------------
FROM golang:alpine AS go-builder

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOTOOLCHAIN=auto

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy dependency manifests
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy source code and built web assets from Stage 1
COPY . .
COPY --from=web-builder /app/web/dist ./web/dist

# Build statically linked binary
RUN go build -ldflags="-s -w" -o /app/bin/proxyserver ./cmd/proxyserver

# -------------------------------------------------------------
# Stage 3: Minimal Production Image (Alpine)
# -------------------------------------------------------------
FROM alpine:3.21

# Install runtime dependencies:
# - ca-certificates: TLS verification (Let's Encrypt, Cloudflare API, external IP resolvers)
# - tzdata: accurate timezone support for logging
# - su-exec: secure privilege dropping matching Unraid PUID/PGID (nobody:users)
RUN apk add --no-cache ca-certificates tzdata su-exec \
    && mkdir -p /var/app /data/certs

WORKDIR /var/app

# Copy binary from builder
COPY --from=go-builder /app/bin/proxyserver /var/app/proxyserver
RUN chmod +x /var/app/proxyserver

# Copy entrypoint script for dynamic Unraid PUID/PGID support
COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Unraid Default Permissions: PUID 99 (nobody), PGID 100 (users)
ENV PUID=99 \
    PGID=100 \
    DATA_DIR=/data \
    CONFIG_PATH=/data/config.json \
    CERT_DIR=/data/certs \
    ADMIN_PORT=8080 \
    HTTP_PORT=9080 \
    HTTPS_PORT=9443

EXPOSE 8080 9080 9443

VOLUME ["/data"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:8080/api/health || exit 1

ENTRYPOINT ["/entrypoint.sh"]
