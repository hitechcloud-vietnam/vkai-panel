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

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o vkai-api ./cmd/api/

# Build migrate binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o vkai-migrate ./cmd/migrate/

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
COPY --from=builder /app/vkai-migrate .

# Copy migrations
COPY --from=builder /app/migrations ./migrations

# Copy configuration
COPY --from=builder /app/config ./config

# Create necessary directories
RUN mkdir -p /var/log/vkai /etc/vkai && \
    chown -R vkai:vkai /app /var/log/vkai /etc/vkai

# Switch to non-root user
USER vkai

# Expose port
EXPOSE 30110

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:30110/health || exit 1

# Run the application
CMD ["./vkai-api"]
