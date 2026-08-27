# ============================================================================
# VKAI Panel - Makefile (HiTech Cloud)
# ----------------------------------------------------------------------------
# Thu muc ma nguon:
#   core/   Go API      (module github.com/hitechcloud-vietnam/vkai-panel)
#   panel/  Next.js UI
#   agent/  Go node agent
#
# Duong dan chuan khi cai dat tren may chu: /vkai-panel/...
# ============================================================================

.PHONY: all build build-core build-panel build-agent clean test test-core \
        test-panel test-agent test-coverage lint lint-core lint-panel lint-agent \
        format format-core format-panel dev dev-core dev-panel migrate \
        deps deps-core deps-panel install hooks docker-up docker-down docker-build \
        deploy deploy-release deploy-status rollback status restart \
        install-systemd uninstall-systemd setup check watch-core watch-panel \
        db-backup db-restore ssl-issue ssl-renew logs version help

# --- Thuong hieu -----------------------------------------------------------
BRAND        = VKAI Panel
BRAND_SLUG   = vkai
BRAND_VENDOR = HiTech Cloud
VERSION     ?= 1.0.0

# --- Thu muc ma nguon ------------------------------------------------------
CORE_DIR  = core
PANEL_DIR = panel
AGENT_DIR = agent
BUILD_DIR = build

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

# Go variables
GO = go
GOBUILD = $(GO) build
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
	@echo "  clean         Clean build artifacts"
	@echo "  test          Run all tests"
	@echo "  test-core     Run core/ tests"
	@echo "  test-panel    Run panel/ tests"
	@echo "  test-agent    Run agent/ tests"
	@echo "  lint          Run all linters"
	@echo "  format        Format code"
	@echo "  dev           Start development servers"
	@echo "  dev-core      Start core/ (API) development server"
	@echo "  dev-panel     Start panel/ (UI) development server"
	@echo "  migrate       Run database migrations"
	@echo "  deps          Install dependencies (core/ + panel/)"
	@echo "  hooks         Point git at githooks/ (git config core.hooksPath)"
	@echo "  install       Run the installer (deploy/install.sh) on this host"
	@echo "  docker-up     Start development Docker containers"
	@echo "  docker-down   Stop development Docker containers"
	@echo "  docker-build  Build the production images"
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

## build-panel: Build the Next.js UI
build-panel:
	@echo "Building panel ($(PANEL_DIR)/)..."
	cd $(PANEL_DIR) && $(NPM) install && $(NPM) run build
	@echo "Panel built successfully!"

## build-agent: Build agent binary
build-agent:
	@echo "Building agent ($(AGENT_DIR)/)..."
	@mkdir -p $(BUILD_DIR)
	cd $(AGENT_DIR) && $(GOBUILD) -o ../$(BUILD_DIR)/$(AGENT_BIN) ./cmd
	@echo "Agent built successfully!"

## clean: Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
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

## dev: Start development servers
dev: docker-up dev-core dev-panel

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

## docker-up: Start development Docker containers
docker-up:
	@echo "Starting Docker containers..."
	docker compose -f docker-compose.dev.yml up -d
	@echo "Docker containers started!"

## docker-down: Stop development Docker containers
docker-down:
	@echo "Stopping Docker containers..."
	docker compose -f docker-compose.dev.yml down
	@echo "Docker containers stopped!"

## docker-build: Build the production images
docker-build:
	@echo "Building Docker images..."
	docker compose -f docker-compose.yml build
	@echo "Docker images built!"

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
