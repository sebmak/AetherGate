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
# Stage 2: Build Backend (Go 1.27 + Embedded Frontend Assets)
# -------------------------------------------------------------
FROM golang:1.24-alpine AS go-builder

ENV CGO_ENABLED=0
ENV GOOS=linux

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
# Stage 3: Minimal Production Image
# -------------------------------------------------------------
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S appgroup && adduser -S appuser -G appgroup \
    && mkdir -p /var/app /data/certs \
    && chown -R appuser:appgroup /var/app /data

WORKDIR /var/app

# Copy binary from builder
COPY --from=go-builder --chown=appuser:appgroup /app/bin/proxyserver /var/app/proxyserver
RUN chmod +x /var/app/proxyserver

USER appuser

ENV DATA_DIR=/data
ENV CONFIG_PATH=/data/config.json
ENV CERT_DIR=/data/certs
ENV ADMIN_PORT=8080
ENV HTTP_PORT=9080
ENV HTTPS_PORT=9443

EXPOSE 8080 9080 9443

VOLUME ["/data"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:8080/api/health || exit 1

ENTRYPOINT ["/var/app/proxyserver"]
