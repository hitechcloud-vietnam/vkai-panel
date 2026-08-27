#!/bin/bash
# ============================================================
# vKAI Panel - Development Setup Script
# Uses Docker only for databases (PostgreSQL, Redis)
# ============================================================

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

print_banner() {
    echo -e "${BLUE}"
    echo "╔═══════════════════════════════════════════════════════════╗"
    echo "║           vKAI Panel - Development Setup                 ║"
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

start_databases() {
    echo -e "${BLUE}Starting databases...${NC}"
    
    # Start PostgreSQL and Redis
    docker-compose -f docker-compose.dev.yml up -d
    
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

setup_backend() {
    echo -e "${BLUE}Setting up backend...${NC}"
    
    cd backend
    
    # Install dependencies
    go mod tidy
    
    # Create .env file if not exists
    if [ ! -f .env ]; then
        cat > .env << EOF
# Development Configuration
SERVER_HOST=0.0.0.0
SERVER_PORT=30110

# Database
DB_HOST=localhost
DB_PORT=5432
DB_NAME=vkai_panel
DB_USER=vkai
DB_PASSWORD=vkai_dev_password
DB_SSLMODE=disable

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0

# JWT
JWT_SECRET=dev-secret-key-change-in-production
JWT_ACCESS_TOKEN_TTL=15m
JWT_REFRESH_TOKEN_TTL=168h
JWT_ISSUER=vkai-panel

# Logging
LOG_LEVEL=debug
LOG_MAX_SIZE=100
LOG_MAX_BACKUPS=5
LOG_MAX_AGE=30
LOG_COMPRESS=false
EOF
    fi
    
    echo -e "${GREEN}Backend setup complete${NC}"
}

setup_frontend() {
    echo -e "${BLUE}Setting up frontend...${NC}"
    
    cd ../frontend
    
    # Install dependencies
    npm install
    
    # Create .env.local file if not exists
    if [ ! -f .env.local ]; then
        cat > .env.local << EOF
NEXT_PUBLIC_API_URL=http://localhost:30110
EOF
    fi
    
    echo -e "${GREEN}Frontend setup complete${NC}"
}

print_usage() {
    echo ""
    echo -e "${GREEN}Development setup complete!${NC}"
    echo ""
    echo -e "${BLUE}Start development:${NC}"
    echo ""
    echo "  # Terminal 1 - Start backend"
    echo "  cd backend"
    echo "  go run cmd/api/main.go"
    echo ""
    echo "  # Terminal 2 - Start frontend"
    echo "  cd frontend"
    echo "  npm run dev"
    echo ""
    echo -e "${BLUE}Access:${NC}"
    echo "  Frontend: http://localhost:3000"
    echo "  API:      http://localhost:30110"
    echo ""
    echo -e "${BLUE}Database:${NC}"
    echo "  PostgreSQL: localhost:5432"
    echo "  Redis:      localhost:6379"
    echo ""
    echo -e "${BLUE}Stop databases:${NC}"
    echo "  docker-compose -f docker-compose.dev.yml down"
    echo ""
}

# Main
print_banner
check_dependencies
start_databases
setup_backend
setup_frontend
print_usage
