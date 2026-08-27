# VKAI Panel - anh gop: API (core/) va agent (agent/)
# Build context la goc kho ma nguon.
#
#   docker build --target api   -t vkai-core  .
#   docker build --target agent -t vkai-agent .

# Build API server
FROM golang:1.22-alpine AS api-builder

WORKDIR /build

COPY core/go.mod core/go.sum ./
RUN go mod download

COPY core/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o /vkai-api ./cmd/api
# Access gate admin CLI: reads and changes the panel port / security entrance
# from inside the container ("docker compose exec vkai-core vkai-panelctl panel info").
RUN CGO_ENABLED=0 GOOS=linux go build -o /vkai-panelctl ./cmd/panelctl

# Build Agent
FROM golang:1.22-alpine AS agent-builder

WORKDIR /build

# agent/ khong co phu thuoc ben ngoai nen khong co go.sum -> dung glob.
COPY agent/go.mod agent/go.sum* ./
RUN go mod download

COPY agent/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o /vkaid ./cmd

# API Server runtime
FROM alpine:3.19 AS api

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=api-builder /vkai-api .
COPY --from=api-builder /vkai-panelctl /usr/local/bin/vkai-panelctl
# Khong COPY core/config.yaml: tep do khong duoc theo doi trong git nen ban
# checkout sach khong co no. Cau hinh lay tu bien moi truong VKAI_* hoac tu
# /etc/vkai/config.yaml duoc mount vao (viper doc ., ./configs, /etc/vkai).

# Duong dan chuan cua panel ben trong container.
RUN mkdir -p /vkai-panel/www/domains /vkai-panel/www/backup /vkai-panel/logs \
             /vkai-panel/etc /vkai-panel/ssl /vkai-panel/tmp /etc/vkai

# The generated panel port / security entrance is persisted here. Mount
# /vkai-panel/etc from the host on /etc/vkai so it survives a container rebuild.
VOLUME ["/etc/vkai", "/vkai-panel/www/domains", "/vkai-panel/logs", "/vkai-panel/ssl"]

# Panel port only. 80/443 are deliberately NOT exposed: those belong to the
# customer websites, never to the panel. Override with VKAI_PANEL_PORT and the
# matching published port. See docs/PANEL_ACCESS.md.
EXPOSE 8888

CMD ["./vkai-api"]

# Agent runtime
FROM alpine:3.19 AS agent

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=agent-builder /vkaid .

RUN mkdir -p /vkai-panel/logs /vkai-panel/etc /etc/vkai

EXPOSE 30111

CMD ["./vkaid"]
