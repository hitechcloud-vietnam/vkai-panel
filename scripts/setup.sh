#!/bin/bash

# vKAI Panel - Development Setup Script
set -e

echo "========================================="
echo "  vKAI Panel - Development Setup"
echo "  HiTechCloud Server Management"
echo "========================================="

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

print_status() {
    echo -e "${GREEN}[✓]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[!]${NC} $1"
}

print_error() {
    echo -e "${RED}[✗]${NC} $1"
}

# Check prerequisites
echo ""
echo "Checking prerequisites..."

# Check Go
if command -v go &> /dev/null; then
    GO_VERSION=$(go version | awk '{print $3}')
    print_status "Go installed: $GO_VERSION"
else
    print_error "Go is not installed. Please install Go 1.22+"
    exit 1
fi

# Check Node.js
if command -v node &> /dev/null; then
    NODE_VERSION=$(node --version)
    print_status "Node.js installed: $NODE_VERSION"
else
    print_error "Node.js is not installed. Please install Node.js 20+"
    exit 1
fi

# Check PostgreSQL
if command -v psql &> /dev/null; then
    PG_VERSION=$(psql --version | awk '{print $3}')
    print_status "PostgreSQL installed: $PG_VERSION"
else
    print_warning "PostgreSQL not found. Using Docker instead."
fi

# Check Redis
if command -v redis-cli &> /dev/null; then
    REDIS_VERSION=$(redis-cli --version | awk '{print $2}')
    print_status "Redis installed: $REDIS_VERSION"
else
    print_warning "Redis not found. Using Docker instead."
fi

# Setup Backend
echo ""
echo "Setting up backend..."
cd backend

# Install Go dependencies
print_status "Installing Go dependencies..."
go mod tidy

# Copy config
if [ ! -f config.yaml ]; then
    cp config.yaml.example config.yaml 2>/dev/null || true
    print_status "Created config.yaml"
fi

cd ..

# Setup Frontend
echo ""
echo "Setting up frontend..."
cd frontend

# Install Node dependencies
print_status "Installing Node.js dependencies..."
npm install

cd ..

# Setup Agent
echo ""
echo "Setting up agent..."
cd agent

# Install Go dependencies
print_status "Installing Go dependencies..."
go mod tidy

cd ..

echo ""
echo "========================================="
echo "  Setup Complete!"
echo "========================================="
echo ""
echo "To start development:"
echo ""
echo "  1. Start database services:"
echo "     docker-compose up -d postgres redis"
echo ""
echo "  2. Run database migrations:"
echo "     cd backend && go run cmd/api/main.go"
echo ""
echo "  3. Start frontend:"
echo "     cd frontend && npm run dev"
echo ""
echo "  4. Start agent (optional):"
echo "     cd agent && VKAI_PANEL_URL=http://localhost:30110 VKAI_AGENT_TOKEN=your-token go run cmd/main.go"
echo ""
echo "Access the panel at: http://localhost:3000"
echo "Default login: admin / admin123"
echo ""
