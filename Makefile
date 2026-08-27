# ============================================================================
# VKAI Panel - Makefile (HiTechCloud)
# ----------------------------------------------------------------------------
# Thu muc ma nguon:
#   core/   Go API      (module github.com/hitechcloud-vietnam/vkai-panel)
#   panel/  Next.js UI
#   agent/  Go node agent
#
# Duong dan chuan khi cai dat tren may chu: /vkai-panel/...
#
# Panel duoc build va chay TRAN tren Linux: binary Go + Next.js standalone +
# systemd. KHONG co target Docker nao o day - Docker chi la TINH NANG cho khach
# hang quan ly container cua ho, khong phai cach dung chinh panel.
# PostgreSQL va Redis cai tran tren may; bo cai deploy/install.sh lo viec do.
# ============================================================================

.PHONY: all build build-core build-panel build-agent package clean test test-core \
        test-panel test-agent test-coverage lint lint-core lint-panel lint-agent \
        format format-core format-panel run-dev dev dev-core dev-panel migrate \
        deps deps-core deps-panel install hooks check-services \
        deploy deploy-release deploy-status rollback status restart \
        install-systemd uninstall-systemd setup check watch-core watch-panel \
        db-backup db-restore ssl-issue ssl-renew logs version help

# --- Thuong hieu -----------------------------------------------------------
BRAND        = VKAI Panel
BRAND_SLUG   = vkai
BRAND_VENDOR = HiTechCloud
VERSION     ?= 1.0.0

# --- Thu muc ma nguon ------------------------------------------------------
CORE_DIR  = core
PANEL_DIR = panel
AGENT_DIR = agent
BUILD_DIR = build
DIST_DIR  = dist

# --- Ten binary ------------------------------------------------------------
API_BIN      = vkai-api
CLI_BIN      = vkai-cli
PANELCTL_BIN = vkai-panelctl
AGENT_BIN    = vkai-agent

# --- Ten systemd service ---------------------------------------------------
SVC_API   = vkai-api
SVC_UI    = vkai-ui
SVC_AGENT = vkai-agent

# --- Duong dan cai dat chuan ----------------------------------------------
PANEL_ROOT ?= /vkai-panel
WEB_ROOT   ?= $(PANEL_ROOT)/www/domains

# --- Goi phat hanh ---------------------------------------------------------
# Bo cuc goi phai trung voi job "Package" trong .github/workflows/ci.yml va voi
# thu tu deploy/scripts/deploy.sh kiem tra, neu khong deploy.sh se tu choi goi.
PKG_NAME ?= vkai-panel-$(VERSION).tar.gz

# Go variables
GO = go
# -trimpath: bo duong dan may build khoi binary. -s -w: bo bang ky hieu/DWARF.
# CGO_ENABLED=0: binary tinh, chay duoc tren may chu khong cung phien ban glibc.
GO_LDFLAGS = -s -w
GOBUILD = CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(GO_LDFLAGS)"
GOTEST = $(GO) test
GOVET = $(GO) vet
GOFMT = gofmt
GOLINT = golangci-lint

# Node variables
NPM = npm
NODE = node

# Default target
all: build

## help: Show this help message
help:
	@echo "$(BRAND) - $(BRAND_VENDOR)"
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all           Build everything (core + panel)"
	@echo "  build         Build core/ and panel/"
	@echo "  build-core    Build core/ binaries only ($(API_BIN), $(CLI_BIN), $(PANELCTL_BIN))"
	@echo "  build-panel   Build panel/ (Next.js) only"
	@echo "  build-agent   Build agent/ binary only ($(AGENT_BIN))"
	@echo "  package       Dong goi ban phat hanh tran ($(PKG_NAME))"
	@echo "  clean         Clean build artifacts"
	@echo "  test          Run all tests"
	@echo "  test-core     Run core/ tests"
	@echo "  test-panel    Run panel/ tests"
	@echo "  test-agent    Run agent/ tests"
	@echo "  lint          Run all linters"
	@echo "  format        Format code"
	@echo "  run-dev       Start API + UI development servers (alias: dev)"
	@echo "  dev-core      Start core/ (API) development server"
	@echo "  dev-panel     Start panel/ (UI) development server"
	@echo "  migrate       Run database migrations"
	@echo "  deps          Install dependencies (core/ + panel/)"
	@echo "  hooks         Point git at githooks/ (git config core.hooksPath)"
	@echo "  install       Run the installer (deploy/install.sh) on this host"
	@echo "  deploy        Build then run the installer"
	@echo "  deploy-status Show deployment status via deploy.sh"
	@echo "  status        Show systemd service status"
	@echo "  version       Show version"
	@echo "  help          Show this help message"

## build: Build core and panel
build: build-core build-panel

## build-core: Build core binaries
build-core:
	@echo "Building core ($(CORE_DIR)/)..."
	@mkdir -p $(BUILD_DIR)
	cd $(CORE_DIR) && $(GOBUILD) -o ../$(BUILD_DIR)/$(API_BIN) ./cmd/api
	cd $(CORE_DIR) && $(GOBUILD) -o ../$(BUILD_DIR)/$(CLI_BIN) ./cmd/cli
	cd $(CORE_DIR) && $(GOBUILD) -o ../$(BUILD_DIR)/$(PANELCTL_BIN) ./cmd/panelctl
	@echo "Core built successfully!"

## build-panel: Build the Next.js UI (standalone)
## `npm run build` chay ca postbuild:standalone, buoc copy .next/static va
## public/ vao .next/standalone. Thieu buoc do thi UI tra ve HTML nhung moi
## /_next/static/*.js deu 404 -> "Application error: a client-side exception".
build-panel:
	@echo "Building panel ($(PANEL_DIR)/)..."
	cd $(PANEL_DIR) && $(NPM) ci && $(NPM) run build
	@test -f $(PANEL_DIR)/.next/standalone/server.js || \
		{ echo "LOI: thieu $(PANEL_DIR)/.next/standalone/server.js"; exit 1; }
	@test -d $(PANEL_DIR)/.next/standalone/.next/static || \
		{ echo "LOI: thieu $(PANEL_DIR)/.next/standalone/.next/static"; exit 1; }
	@echo "Panel built successfully!"

## build-agent: Build agent binary
build-agent:
	@echo "Building agent ($(AGENT_DIR)/)..."
	@mkdir -p $(BUILD_DIR)
	cd $(AGENT_DIR) && $(GOBUILD) -o ../$(BUILD_DIR)/$(AGENT_BIN) ./cmd
	@echo "Agent built successfully!"

## package: Dong goi ban phat hanh tran (tar.gz + SHA256), giong job "Package" cua CI
package: build-core build-agent build-panel
	@echo "Packaging $(PKG_NAME)..."
	rm -rf $(DIST_DIR)
	mkdir -p $(DIST_DIR)/core/bin $(DIST_DIR)/agent/bin $(DIST_DIR)/panel/.next \
		$(DIST_DIR)/deploy/nginx $(DIST_DIR)/deploy/scripts
	cp $(BUILD_DIR)/$(API_BIN) $(BUILD_DIR)/$(CLI_BIN) $(BUILD_DIR)/$(PANELCTL_BIN) $(DIST_DIR)/core/bin/
	cp $(BUILD_DIR)/$(AGENT_BIN) $(DIST_DIR)/agent/bin/
	cp -r $(CORE_DIR)/migrations $(DIST_DIR)/core/migrations
	cp -r $(PANEL_DIR)/.next/standalone $(DIST_DIR)/panel/.next/standalone
	cp $(PANEL_DIR)/package.json $(DIST_DIR)/panel/package.json
	cp -r deploy/systemd $(DIST_DIR)/deploy/systemd
	cp deploy/nginx/vkai-panel.conf $(DIST_DIR)/deploy/nginx/
	cp deploy/scripts/deploy.sh $(DIST_DIR)/deploy/scripts/
	chmod +x $(DIST_DIR)/deploy/scripts/deploy.sh
	tar -czf $(PKG_NAME) -C $(DIST_DIR) .
	sha256sum $(PKG_NAME) > $(PKG_NAME).sha256
	@echo "Package ready: $(PKG_NAME)"
	@echo "Trien khai: sudo bash deploy/scripts/deploy.sh deploy $(PKG_NAME)"

## clean: Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	rm -rf $(DIST_DIR)
	rm -f vkai-panel-*.tar.gz vkai-panel-*.tar.gz.sha256
	rm -rf $(PANEL_DIR)/.next
	rm -rf $(PANEL_DIR)/out
	rm -rf $(PANEL_DIR)/node_modules
	rm -rf $(CORE_DIR)/vendor
	rm -rf $(AGENT_DIR)/bin
	@echo "Cleaned successfully!"

## test: Run all tests
test: test-core test-panel test-agent

## test-core: Run core tests
test-core:
	@echo "Running core tests..."
	cd $(CORE_DIR) && $(GOTEST) -vet=off -v ./...
	@echo "Core tests completed!"

## test-panel: Run panel tests
test-panel:
	@echo "Running panel tests..."
	cd $(PANEL_DIR) && $(NPM) test --if-present
	@echo "Panel tests completed!"

## test-agent: Run agent tests
test-agent:
	@echo "Running agent tests..."
	cd $(AGENT_DIR) && $(GOTEST) -vet=off -v ./...
	@echo "Agent tests completed!"

## test-coverage: Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	cd $(CORE_DIR) && $(GOTEST) -vet=off -coverprofile=coverage.out ./...
	cd $(CORE_DIR) && $(GO) tool cover -html=coverage.out -o coverage.html
	cd $(PANEL_DIR) && $(NPM) run test:coverage --if-present
	@echo "Coverage reports generated!"

## lint: Run linters
lint: lint-core lint-panel lint-agent

## lint-core: Run core linter
lint-core:
	@echo "Running core linter..."
	cd $(CORE_DIR) && $(GOLINT) run
	@echo "Core linting completed!"

## lint-panel: Run panel linter
lint-panel:
	@echo "Running panel linter..."
	cd $(PANEL_DIR) && $(NPM) run lint
	@echo "Panel linting completed!"

## lint-agent: Run agent linter
lint-agent:
	@echo "Running agent linter..."
	cd $(AGENT_DIR) && $(GOLINT) run
	@echo "Agent linting completed!"

## format: Format code
format: format-core format-panel

## format-core: Format core code
format-core:
	@echo "Formatting core code..."
	cd $(CORE_DIR) && $(GOFMT) -w .
	@echo "Core code formatted!"

## format-panel: Format panel code
format-panel:
	@echo "Formatting panel code..."
	cd $(PANEL_DIR) && $(NPM) run format --if-present
	@echo "Panel code formatted!"

## check-services: Kiem tra PostgreSQL/Redis cai tran tren may co dang chay khong
check-services:
	@for svc in postgresql redis-server redis; do \
		if systemctl list-unit-files "$$svc.service" >/dev/null 2>&1 && \
		   systemctl is-active --quiet "$$svc"; then \
			echo "OK: $$svc dang chay."; \
		fi; \
	done
	@systemctl is-active --quiet postgresql || \
		echo "CANH: postgresql khong chay. Cai tran bang: sudo bash deploy/install.sh"
	@systemctl is-active --quiet redis-server || systemctl is-active --quiet redis || \
		echo "CANH: redis khong chay. Cai tran bang: sudo bash deploy/install.sh"

## run-dev: Chay song song API (core/) va UI (panel/) o che do phat trien
## PostgreSQL va Redis phai cai TRAN tren may (deploy/install.sh lo viec do);
## khong con "docker compose up" cho ha tang phat trien nua.
run-dev: check-services
	@echo "Starting $(BRAND) development servers (Ctrl-C de dung ca hai)..."
	@trap 'kill 0' EXIT INT TERM; \
		( cd $(CORE_DIR) && $(GO) run ./cmd/api ) & \
		( cd $(PANEL_DIR) && $(NPM) run dev ) & \
		wait

## dev: Alias cua run-dev (giu lai cho quen tay)
dev: run-dev

## dev-core: Start the API development server
dev-core:
	@echo "Starting core (API) development server..."
	cd $(CORE_DIR) && $(GO) run ./cmd/api

## dev-panel: Start the UI development server
dev-panel:
	@echo "Starting panel (UI) development server..."
	cd $(PANEL_DIR) && $(NPM) run dev

## migrate: Run database migrations
## Set DATABASE_URL, e.g. make migrate DATABASE_URL=postgres://vkai@localhost/vkai_panel
migrate:
	@echo "Running database migrations..."
	@test -n "$(DATABASE_URL)" || { echo "DATABASE_URL is required"; exit 1; }
	@for f in $(CORE_DIR)/migrations/*.sql; do \
		echo "Applying $$(basename $$f)"; \
		psql "$(DATABASE_URL)" -v ON_ERROR_STOP=1 -f "$$f" || exit 1; \
	done
	@echo "Migrations completed!"

## deps: Install build dependencies for core/ and panel/
deps: deps-core deps-panel

## deps-core: Install core dependencies
deps-core:
	@echo "Installing core dependencies..."
	cd $(CORE_DIR) && $(GO) mod tidy
	@echo "Core dependencies installed!"

## deps-panel: Install panel dependencies
deps-panel:
	@echo "Installing panel dependencies..."
	cd $(PANEL_DIR) && $(NPM) install
	@echo "Panel dependencies installed!"

## hooks: Point git at the tracked hooks in githooks/
hooks:
	@echo "Configuring git hooks (githooks/)..."
	git config core.hooksPath githooks
	@echo "Git hooks configured: core.hooksPath=githooks"

## install: Run the installer on this host (cai dat $(BRAND) vao $(PANEL_ROOT))
install:
	@echo "Installing $(BRAND) into $(PANEL_ROOT)..."
	@test -f deploy/install.sh || { echo "deploy/install.sh is missing from this repo"; exit 1; }
	sudo bash deploy/install.sh
	@echo "Installation completed!"

## deploy: Build then run the installer
deploy: build install

## deploy-release: Deploy a specific release
deploy-release:
	@echo "Deploying release..."
	./deploy/scripts/deploy.sh deploy $(release)
	@echo "Deployment completed!"

## rollback: Rollback to previous release
rollback:
	@echo "Rolling back..."
	./deploy/scripts/deploy.sh rollback
	@echo "Rollback completed!"

## deploy-status: Show deployment status via deploy.sh
deploy-status:
	./deploy/scripts/deploy.sh status

## restart: Restart services
restart:
	@echo "Restarting services..."
	./deploy/scripts/deploy.sh restart
	@echo "Services restarted!"

## install-systemd: Install systemd unit files
install-systemd:
	@echo "Installing systemd services..."
	@for u in $(SVC_API) $(SVC_UI) $(SVC_AGENT); do \
		if [ -f "deploy/systemd/$$u.service" ]; then \
			sudo cp "deploy/systemd/$$u.service" /etc/systemd/system/; \
		else \
			echo "skip: deploy/systemd/$$u.service not found"; \
		fi; \
	done
	sudo systemctl daemon-reload
	sudo systemctl enable $(SVC_API) $(SVC_UI)
	@echo "Systemd services installed!"

## uninstall-systemd: Uninstall systemd unit files
uninstall-systemd:
	@echo "Uninstalling systemd services..."
	-sudo systemctl disable $(SVC_API) $(SVC_UI) $(SVC_AGENT)
	-sudo systemctl stop $(SVC_API) $(SVC_UI) $(SVC_AGENT)
	sudo rm -f /etc/systemd/system/$(SVC_API).service
	sudo rm -f /etc/systemd/system/$(SVC_UI).service
	sudo rm -f /etc/systemd/system/$(SVC_AGENT).service
	sudo systemctl daemon-reload
	@echo "Systemd services uninstalled!"

## setup: Setup development environment
setup: hooks
	@echo "Setting up development environment..."
	./setup-dev.sh
	@echo "Development environment setup completed!"

## check: Run all checks
check: lint test
	@echo "All checks passed!"

## watch-core: Watch for changes and rebuild the API
watch-core:
	@echo "Watching for core changes..."
	cd $(CORE_DIR) && air

## watch-panel: Watch for changes and rebuild the UI
watch-panel:
	@echo "Watching for panel changes..."
	cd $(PANEL_DIR) && $(NPM) run dev

## db-backup: Backup database
db-backup:
	@echo "Backing up database..."
	@test -x scripts/backup-db.sh || { echo "scripts/backup-db.sh is missing from this repo"; exit 1; }
	./scripts/backup-db.sh
	@echo "Database backup completed!"

## db-restore: Restore database
db-restore:
	@echo "Restoring database..."
	@test -x scripts/restore-db.sh || { echo "scripts/restore-db.sh is missing from this repo"; exit 1; }
	./scripts/restore-db.sh
	@echo "Database restore completed!"

## ssl-issue: Issue SSL certificate
ssl-issue:
	@echo "Issuing SSL certificate..."
	@test -x scripts/ssl-issue.sh || { echo "scripts/ssl-issue.sh is missing from this repo"; exit 1; }
	./scripts/ssl-issue.sh
	@echo "SSL certificate issued!"

## ssl-renew: Renew SSL certificates
ssl-renew:
	@echo "Renewing SSL certificates..."
	@test -x scripts/ssl-renew.sh || { echo "scripts/ssl-renew.sh is missing from this repo"; exit 1; }
	./scripts/ssl-renew.sh
	@echo "SSL certificates renewed!"

## status: Show service status
status:
	@echo "Service Status:"
	@echo "==============="
	@systemctl status $(SVC_API) --no-pager
	@echo ""
	@systemctl status $(SVC_UI) --no-pager
	@echo ""
	@systemctl status $(SVC_AGENT) --no-pager
	@echo ""
	@systemctl status nginx --no-pager
	@echo ""
	@systemctl status postgresql --no-pager
	@echo ""
	@systemctl status redis-server --no-pager

## logs: Show service logs
logs:
	@echo "Service Logs:"
	@echo "============="
	@journalctl -u $(SVC_API) -f

## version: Show version
version:
	@echo "$(BRAND) v$(VERSION) - $(BRAND_VENDOR)"
	@echo "Core:  $(shell cd $(CORE_DIR) && $(GO) version)"
	@echo "Panel: $(shell cd $(PANEL_DIR) && $(NODE) --version)"
	@echo "Agent: $(shell cd $(AGENT_DIR) && $(GO) version)"
