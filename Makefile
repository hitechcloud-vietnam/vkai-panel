# vKAI Panel Makefile

.PHONY: all build clean test lint format help

# Variables
BACKEND_DIR = backend
FRONTEND_DIR = frontend
AGENT_DIR = agent
BUILD_DIR = build
BINARY_NAME = vkai-api
AGENT_BINARY = vkaid

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
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all          Build everything"
	@echo "  build        Build backend and frontend"
	@echo "  build-backend Build backend only"
	@echo "  build-frontend Build frontend only"
	@echo "  build-agent  Build agent only"
	@echo "  clean        Clean build artifacts"
	@echo "  test         Run all tests"
	@echo "  test-backend Run backend tests"
	@echo "  test-frontend Run frontend tests"
	@echo "  lint         Run linters"
	@echo "  format       Format code"
	@echo "  dev          Start development servers"
	@echo "  dev-backend  Start backend development server"
	@echo "  dev-frontend Start frontend development server"
	@echo "  migrate      Run database migrations"
	@echo "  install      Install dependencies"
	@echo "  docker-up    Start Docker containers"
	@echo "  docker-down  Stop Docker containers"
	@echo "  deploy       Deploy to production"
	@echo "  help         Show this help message"

## build: Build backend and frontend
build: build-backend build-frontend

## build-backend: Build backend binary
build-backend:
	@echo "Building backend..."
	cd $(BACKEND_DIR) && $(GOBUILD) -o ../$(BUILD_DIR)/$(BINARY_NAME) ./cmd/api/
	@echo "Backend built successfully!"

## build-frontend: Build frontend
build-frontend:
	@echo "Building frontend..."
	cd $(FRONTEND_DIR) && $(NPM) install && $(NPM) run build
	@echo "Frontend built successfully!"

## build-agent: Build agent binary
build-agent:
	@echo "Building agent..."
	cd $(AGENT_DIR) && $(GOBUILD) -o ../$(BUILD_DIR)/$(AGENT_BINARY) ./cmd/
	@echo "Agent built successfully!"

## clean: Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	rm -rf $(FRONTEND_DIR)/.next
	rm -rf $(FRONTEND_DIR)/out
	rm -rf $(FRONTEND_DIR)/node_modules
	rm -rf $(BACKEND_DIR)/vendor
	@echo "Cleaned successfully!"

## test: Run all tests
test: test-backend test-frontend

## test-backend: Run backend tests
test-backend:
	@echo "Running backend tests..."
	cd $(BACKEND_DIR) && $(GOTEST) -v ./...
	@echo "Backend tests completed!"

## test-frontend: Run frontend tests
test-frontend:
	@echo "Running frontend tests..."
	cd $(FRONTEND_DIR) && $(NPM) test
	@echo "Frontend tests completed!"

## test-coverage: Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	cd $(BACKEND_DIR) && $(GOTEST) -coverprofile=coverage.out ./...
	cd $(BACKEND_DIR) && $(GO) tool cover -html=coverage.out -o coverage.html
	cd $(FRONTEND_DIR) && $(NPM) run test:coverage
	@echo "Coverage reports generated!"

## lint: Run linters
lint: lint-backend lint-frontend

## lint-backend: Run backend linter
lint-backend:
	@echo "Running backend linter..."
	cd $(BACKEND_DIR) && $(GOLINT) run
	@echo "Backend linting completed!"

## lint-frontend: Run frontend linter
lint-frontend:
	@echo "Running frontend linter..."
	cd $(FRONTEND_DIR) && $(NPM) run lint
	@echo "Frontend linting completed!"

## format: Format code
format: format-backend format-frontend

## format-backend: Format backend code
format-backend:
	@echo "Formatting backend code..."
	cd $(BACKEND_DIR) && $(GOFMT) -w .
	@echo "Backend code formatted!"

## format-frontend: Format frontend code
format-frontend:
	@echo "Formatting frontend code..."
	cd $(FRONTEND_DIR) && $(NPM) run format
	@echo "Frontend code formatted!"

## dev: Start development servers
dev: docker-up dev-backend dev-frontend

## dev-backend: Start backend development server
dev-backend:
	@echo "Starting backend development server..."
	cd $(BACKEND_DIR) && $(GO) run cmd/api/main.go

## dev-frontend: Start frontend development server
dev-frontend:
	@echo "Starting frontend development server..."
	cd $(FRONTEND_DIR) && $(NPM) run dev

## migrate: Run database migrations
migrate:
	@echo "Running database migrations..."
	cd $(BACKEND_DIR) && $(GO) run cmd/migrate/main.go
	@echo "Migrations completed!"

## install: Install dependencies
install: install-backend install-frontend

## install-backend: Install backend dependencies
install-backend:
	@echo "Installing backend dependencies..."
	cd $(BACKEND_DIR) && $(GO) mod tidy
	@echo "Backend dependencies installed!"

## install-frontend: Install frontend dependencies
install-frontend:
	@echo "Installing frontend dependencies..."
	cd $(FRONTEND_DIR) && $(NPM) install
	@echo "Frontend dependencies installed!"

## docker-up: Start Docker containers
docker-up:
	@echo "Starting Docker containers..."
	docker-compose -f docker-compose.dev.yml up -d
	@echo "Docker containers started!"

## docker-down: Stop Docker containers
docker-down:
	@echo "Stopping Docker containers..."
	docker-compose -f docker-compose.dev.yml down
	@echo "Docker containers stopped!"

## deploy: Deploy to production
deploy: build
	@echo "Deploying to production..."
	./deploy/install.sh
	@echo "Deployment completed!"

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

## status: Show deployment status
status:
	./deploy/scripts/deploy.sh status

## restart: Restart services
restart:
	@echo "Restarting services..."
	./deploy/scripts/deploy.sh restart
	@echo "Services restarted!"

## install-systemd: Install systemd service files
install-systemd:
	@echo "Installing systemd services..."
	sudo cp deploy/systemd/vkai-panel-api.service /etc/systemd/system/
	sudo cp deploy/systemd/vkai-panel-frontend.service /etc/systemd/system/
	sudo systemctl daemon-reload
	sudo systemctl enable vkai-panel-api vkai-panel-frontend
	@echo "Systemd services installed!"

## uninstall-systemd: Uninstall systemd service files
uninstall-systemd:
	@echo "Uninstalling systemd services..."
	sudo systemctl disable vkai-panel-api vkai-panel-frontend
	sudo systemctl stop vkai-panel-api vkai-panel-frontend
	sudo rm -f /etc/systemd/system/vkai-panel-api.service
	sudo rm -f /etc/systemd/system/vkai-panel-frontend.service
	sudo systemctl daemon-reload
	@echo "Systemd services uninstalled!"

## setup: Setup development environment
setup:
	@echo "Setting up development environment..."
	./setup-dev.sh
	@echo "Development environment setup completed!"

## check: Run all checks
check: lint test
	@echo "All checks passed!"

## watch: Watch for changes and rebuild
watch-backend:
	@echo "Watching for backend changes..."
	cd $(BACKEND_DIR) && air

watch-frontend:
	@echo "Watching for frontend changes..."
	cd $(FRONTEND_DIR) && $(NPM) run dev

## db-backup: Backup database
db-backup:
	@echo "Backing up database..."
	./scripts/backup-db.sh
	@echo "Database backup completed!"

## db-restore: Restore database
db-restore:
	@echo "Restoring database..."
	./scripts/restore-db.sh
	@echo "Database restore completed!"

## ssl-issue: Issue SSL certificate
ssl-issue:
	@echo "Issuing SSL certificate..."
	./scripts/ssl-issue.sh
	@echo "SSL certificate issued!"

## ssl-renew: Renew SSL certificates
ssl-renew:
	@echo "Renewing SSL certificates..."
	./scripts/ssl-renew.sh
	@echo "SSL certificates renewed!"

## status: Show service status
status:
	@echo "Service Status:"
	@echo "==============="
	@systemctl status vkai-api --no-pager
	@echo ""
	@systemctl status vkai-frontend --no-pager
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
	@journalctl -u vkai-api -f

## version: Show version
version:
	@echo "vKAI Panel v1.0.0"
	@echo "Backend: $(shell cd $(BACKEND_DIR) && $(GO) version)"
	@echo "Frontend: $(shell cd $(FRONTEND_DIR) && $(NODE) --version)"
	@echo "Agent: $(shell cd $(AGENT_DIR) && $(GO) version)"
