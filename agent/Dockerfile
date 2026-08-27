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
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o vkaid ./cmd/agent/

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

# Create necessary directories
RUN mkdir -p /var/log/vkai /etc/vkai

# Expose port
EXPOSE 30111

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:30111/health || exit 1

# Run the application
CMD ["./vkaid"]
