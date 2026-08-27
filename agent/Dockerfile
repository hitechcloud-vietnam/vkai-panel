# VKAI Panel - Node agent (Go), thu muc agent/
# Build stage
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files. The agent has no third-party dependencies, so there is no
# go.sum; the glob keeps the COPY valid either way.
COPY go.mod go.sum* ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary. The main package lives at ./cmd, not ./cmd/agent.
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o vkaid ./cmd

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user (but agent needs root for system operations)
# We'll use root for now as agent needs system access

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/vkaid .

# Duong dan chuan cua panel
RUN mkdir -p /vkai-panel/logs /vkai-panel/etc /vkai-panel/www/domains /etc/vkai

# Expose port
EXPOSE 30111

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:30111/health || exit 1

# Run the application
CMD ["./vkaid"]
