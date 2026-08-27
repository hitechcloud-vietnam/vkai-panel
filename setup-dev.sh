#!/bin/bash
# ============================================================
# VKAI Panel - Development Setup Script (HiTechCloud)
# Uses Docker only for databases (PostgreSQL, Redis)
#
# Thu muc ma nguon: core/ (Go API), panel/ (Next.js UI), agent/ (Go agent)
# ============================================================

set -e

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CORE_DIR="$ROOT_DIR/core"
PANEL_DIR="$ROOT_DIR/panel"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

print_banner() {
    echo -e "${BLUE}"
    echo "╔═══════════════════════════════════════════════════════════╗"
    echo "║           VKAI Panel - Development Setup                  ║"
    echo "║           HiTechCloud - hitechcloud.vn                   ║"
    echo "╚═══════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
}

check_dependencies() {
    echo -e "${BLUE}Checking dependencies...${NC}"

    # Check Docker
    if ! command -v docker &> /dev/null; then
        echo -e "${RED}Docker is not installed${NC}"
        echo "Install Docker: https://docs.docker.com/get-docker/"
        exit 1
    fi

    # Check Docker Compose
    if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
        echo -e "${RED}Docker Compose is not installed${NC}"
        exit 1
    fi

    # Check Go
    if ! command -v go &> /dev/null; then
        echo -e "${RED}Go is not installed${NC}"
        echo "Install Go: https://golang.org/doc/install"
        exit 1
    fi

    # Check Node.js
    if ! command -v node &> /dev/null; then
        echo -e "${RED}Node.js is not installed${NC}"
        echo "Install Node.js: https://nodejs.org/"
        exit 1
    fi

    echo -e "${GREEN}All dependencies found${NC}"
}

setup_hooks() {
    echo -e "${BLUE}Configuring git hooks...${NC}"
    if [ -d "$ROOT_DIR/.git" ] && [ -d "$ROOT_DIR/githooks" ]; then
        git -C "$ROOT_DIR" config core.hooksPath githooks
        echo -e "${GREEN}core.hooksPath=githooks${NC}"
    else
        echo -e "${YELLOW}Skipped (not a git checkout, or githooks/ missing)${NC}"
    fi
}

start_databases() {
    echo -e "${BLUE}Starting databases...${NC}"

    # Start PostgreSQL and Redis
    docker compose -f "$ROOT_DIR/docker-compose.dev.yml" up -d

    # Wait for databases to be ready
    echo -e "${YELLOW}Waiting for databases to be ready...${NC}"
    sleep 5

    # Check PostgreSQL
    until docker exec vkai-postgres pg_isready -U vkai -d vkai_panel > /dev/null 2>&1; do
        echo -n "."
        sleep 1
    done
    echo ""
    echo -e "${GREEN}PostgreSQL is ready${NC}"

    # Check Redis
    until docker exec vkai-redis redis-cli ping > /dev/null 2>&1; do
        echo -n "."
        sleep 1
    done
    echo ""
    echo -e "${GREEN}Redis is ready${NC}"
}

setup_core() {
    echo -e "${BLUE}Setting up core (API)...${NC}"

    cd "$CORE_DIR"

    # Install dependencies
    go mod tidy

    # Create .env file if not exists.
    # Cau hinh doc qua viper voi SetEnvPrefix("VKAI"), nen chi bien co tien to
    # VKAI_ moi duoc doc.
    if [ ! -f .env ]; then
        cat > .env <<'COREENV'
# VKAI Panel - core/ development configuration
VKAI_PANEL_ENABLED=true
VKAI_PANEL_PORT=8888
VKAI_PANEL_BIND=127.0.0.1
# De trong: loi vao an toan se duoc sinh va in ra trong banner khoi dong.
VKAI_PANEL_ENTRANCE=
VKAI_PANEL_CONFIG_FILE=./.panel_access.json

VKAI_SERVER_HOST=0.0.0.0
VKAI_SERVER_PORT=30110

# Database (docker-compose.dev.yml)
VKAI_DB_HOST=localhost
VKAI_DB_PORT=5432
VKAI_DB_NAME=vkai_panel
VKAI_DB_USER=vkai
VKAI_DB_PASSWORD=vkai_dev_password
VKAI_DB_SSLMODE=disable

# Redis (docker-compose.dev.yml)
VKAI_REDIS_HOST=localhost
VKAI_REDIS_PORT=6379
VKAI_REDIS_PASSWORD=
VKAI_REDIS_DB=0

# JWT - chi dung cho may phat trien, KHONG dung tren may that.
VKAI_JWT_SECRET=dev-only-a1b2c3d4e5f60718293a4b5c6d7e8f90
VKAI_JWT_ISSUER=vkai-panel

# Duong dan chuan (ban phat trien tro vao thu muc trong repo)
VKAI_PANEL_ROOT=./.dev/vkai-panel
VKAI_WEB_ROOT=./.dev/vkai-panel/www/domains
VKAI_FILEMANAGER_ROOT=./.dev/vkai-panel/www/domains
VKAI_BACKUP_ROOT=./.dev/vkai-panel/www/backup

# Logging
VKAI_LOG_LEVEL=debug
VKAI_LOG_FORMAT=text
COREENV
    fi

    echo -e "${GREEN}Core setup complete${NC}"
}

setup_panel() {
    echo -e "${BLUE}Setting up panel (UI)...${NC}"

    cd "$PANEL_DIR"

    # Install dependencies
    npm install

    # Create .env.local file if not exists.
    # LUU Y: NEXT_PUBLIC_* duoc nhung vao bundle LUC BUILD. Tren may phat trien
    # `next dev` doc lai tep nay moi lan khoi dong nen doi gia tri phai restart.
    if [ ! -f .env.local ]; then
        cat > .env.local <<'PANELENV'
# URL API tinh theo goc nhin cua TRINH DUYET (khong phai hostname noi bo Docker).
NEXT_PUBLIC_API_URL=http://localhost:30110
PANELENV
    fi

    echo -e "${GREEN}Panel setup complete${NC}"
}

print_usage() {
    echo ""
    echo -e "${GREEN}Development setup complete!${NC}"
    echo ""
    echo -e "${BLUE}Start development:${NC}"
    echo ""
    echo "  # Terminal 1 - Start the API (core/)"
    echo "  cd core"
    echo "  go run ./cmd/api"
    echo ""
    echo "  # Terminal 2 - Start the UI (panel/)"
    echo "  cd panel"
    echo "  npm run dev"
    echo ""
    echo -e "${BLUE}Access:${NC}"
    echo "  Panel UI: http://localhost:3000"
    echo "  API:      http://localhost:30110"
    echo "  Panel port (khi bat access gate): http://127.0.0.1:8888/<loi-vao>/"
    echo ""
    echo -e "${YELLOW}  Cong 80/443 danh RIENG cho website cua khach, khong dung cho panel.${NC}"
    echo ""
    echo -e "${BLUE}Database:${NC}"
    echo "  PostgreSQL: localhost:5432"
    echo "  Redis:      localhost:6379"
    echo ""
    echo -e "${BLUE}Stop databases:${NC}"
    echo "  docker compose -f docker-compose.dev.yml down"
    echo ""
}

# Main
print_banner
check_dependencies
setup_hooks
start_databases
setup_core
setup_panel
print_usage
