# Build API server
FROM golang:1.22-alpine AS api-builder

WORKDIR /build

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o /vkai-api ./cmd/api/main.go

# Build Agent
FROM golang:1.22-alpine AS agent-builder

WORKDIR /build

COPY agent/go.mod agent/go.sum ./
RUN go mod download

COPY agent/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o /vkaid ./cmd/main.go

# API Server runtime
FROM alpine:3.19 AS api

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=api-builder /vkai-api .
COPY backend/config.yaml .

EXPOSE 30110

CMD ["./vkai-api"]

# Agent runtime
FROM alpine:3.19 AS agent

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=agent-builder /vkaid .

EXPOSE 30111

CMD ["./vkaid"]
