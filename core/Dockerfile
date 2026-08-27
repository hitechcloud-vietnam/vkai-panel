# VKAI Panel - Core API (Go), thu muc core/
# Build stage
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the API binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o vkai-api ./cmd/api/

# Build the panel access-gate admin CLI (vkai panel port/entrance ...)
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o vkai-panelctl ./cmd/panelctl/

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -S vkai && adduser -S vkai -G vkai

# Set working directory
WORKDIR /app

# Copy binaries from builder
COPY --from=builder /app/vkai-api .
COPY --from=builder /app/vkai-panelctl /usr/local/bin/vkai-panelctl

# Copy migrations
COPY --from=builder /app/migrations ./migrations

# Khong COPY config.yaml: tep nay khong duoc theo doi trong git (xem .gitignore)
# nen ban checkout sach khong co no. Cau hinh lay tu bien moi truong VKAI_*
# hoac tu /etc/vkai/config.yaml duoc mount vao khi chay.

# Duong dan chuan cua panel (/vkai-panel/...) va thu muc cau hinh /etc/vkai
RUN mkdir -p /vkai-panel/www/domains /vkai-panel/www/backup /vkai-panel/www/default \
             /vkai-panel/logs/sites /vkai-panel/etc /vkai-panel/ssl /vkai-panel/tmp \
             /etc/vkai && \
    chown -R vkai:vkai /app /vkai-panel /etc/vkai

# Switch to non-root user
USER vkai

# Expose the internal API port. 80/443 are NEVER exposed by the panel: they
# belong to the customer websites. The panel port itself (VKAI_PANEL_PORT,
# default 8888) is published by the reverse proxy in docker-compose.yml.
EXPOSE 30110

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:30110/health || exit 1

# Run the application
CMD ["./vkai-api"]
