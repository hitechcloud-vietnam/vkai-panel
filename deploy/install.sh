#!/bin/bash
# ============================================================
# vKAI Panel - Production Installation Script
# HiTechCloud - No Docker Required
# ============================================================

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
VKAI_VERSION="1.0.0"
VKAI_USER="vkai"
VKAI_GROUP="vkai"
VKAI_HOME="/opt/vkai-panel"
VKAI_LOG_DIR="/var/log/vkai"
VKAI_BACKUP_DIR="/var/backups/vkai"
DB_NAME="vkai_panel"
DB_USER="vkai"
DB_PASS=""

print_banner() {
    echo -e "${BLUE}"
    echo "╔═══════════════════════════════════════════════════════════╗"
    echo "║           vKAI Panel Installation Script                 ║"
    echo "║           HiTechCloud - Version ${VKAI_VERSION}                    ║"
    echo "╚═══════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
}

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_root() {
    if [ "$EUID" -ne 0 ]; then
        log_error "This script must be run as root"
        exit 1
    fi
}

detect_os() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS=$ID
        OS_VERSION=$VERSION_ID
    else
        log_error "Cannot detect OS"
        exit 1
    fi
    log_info "Detected OS: $OS $OS_VERSION"
}

install_dependencies() {
    log_info "Installing system dependencies..."

    case $OS in
        ubuntu|debian)
            apt-get update
            apt-get install -y \
                curl wget git unzip \
                build-essential \
                postgresql postgresql-contrib \
                redis-server \
                nginx \
                certbot python3-certbot-nginx \
                iptables \
                logrotate \
                jq
            ;;
        centos|rhel|fedora)
            yum install -y \
                curl wget git unzip \
                gcc gcc-c++ make \
                postgresql-server postgresql-contrib \
                redis \
                nginx \
                certbot python3-certbot-nginx \
                iptables-services \
                logrotate \
                jq
            ;;
        *)
            log_error "Unsupported OS: $OS"
            exit 1
            ;;
    esac

    log_info "Dependencies installed"
}

install_golang() {
    if command -v go &> /dev/null; then
        log_info "Go already installed: $(go version)"
        return
    fi

    log_info "Installing Go 1.22..."
    wget -q https://go.dev/dl/go1.22.0.linux-amd64.tar.gz
    tar -C /usr/local -xzf go1.22.0.linux-amd64.tar.gz
    rm go1.22.0.linux-amd64.tar.gz
    echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile.d/golang.sh
    source /etc/profile.d/golang.sh
    log_info "Go installed: $(go version)"
}

install_nodejs() {
    if command -v node &> /dev/null; then
        log_info "Node.js already installed: $(node --version)"
        return
    fi

    log_info "Installing Node.js 20 LTS..."
    curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
    apt-get install -y nodejs
    log_info "Node.js installed: $(node --version)"
}

setup_user() {
    if id "$VKAI_USER" &>/dev/null; then
        log_info "User $VKAI_USER already exists"
    else
        log_info "Creating user $VKAI_USER..."
        useradd -r -m -d /home/$VKAI_USER -s /bin/bash $VKAI_USER
    fi
}

setup_directories() {
    log_info "Creating directories..."

    mkdir -p $VKAI_HOME/{backend,frontend,config}
    mkdir -p $VKAI_LOG_DIR
    mkdir -p $VKAI_BACKUP_DIR
    mkdir -p /var/www
    mkdir -p /etc/ssl/vkai
    mkdir -p /etc/vkai

    chown -R $VKAI_USER:$VKAI_GROUP $VKAI_HOME
    chown -R $VKAI_USER:$VKAI_GROUP $VKAI_LOG_DIR
    chown -R $VKAI_USER:$VKAI_GROUP $VKAI_BACKUP_DIR
    chown -R $VKAI_USER:$VKAI_GROUP /var/www

    log_info "Directories created"
}

setup_database() {
    log_info "Setting up PostgreSQL..."

    # Start PostgreSQL
    systemctl enable postgresql
    systemctl start postgresql

    # Generate random password if not set
    if [ -z "$DB_PASS" ]; then
        DB_PASS=$(openssl rand -base64 32)
    fi

    # Create database and user
    sudo -u postgres psql -c "CREATE USER $DB_USER WITH PASSWORD '$DB_PASS';" 2>/dev/null || true
    sudo -u postgres psql -c "CREATE DATABASE $DB_NAME OWNER $DB_USER;" 2>/dev/null || true
    sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE $DB_NAME TO $DB_USER;" 2>/dev/null || true

    # Run migrations
    if [ -f "$VKAI_HOME/backend/migrations/001_initial_schema.sql" ]; then
        sudo -u postgres psql -d $DB_NAME -f "$VKAI_HOME/backend/migrations/001_initial_schema.sql"
    fi

    log_info "Database setup complete"
}

setup_redis() {
    log_info "Setting up Redis..."

    systemctl enable redis-server
    systemctl start redis-server

    log_info "Redis setup complete"
}

build_backend() {
    log_info "Building backend..."

    cd $VKAI_HOME/backend
    sudo -u $VKAI_USER go mod tidy
    sudo -u $VKAI_USER go build -o bin/vkai-api ./cmd/api/

    log_info "Backend built successfully"
}

build_frontend() {
    log_info "Building frontend..."

    cd $VKAI_HOME/frontend
    sudo -u $VKAI_USER npm install
    sudo -u $VKAI_USER npm run build

    log_info "Frontend built successfully"
}

build_agent() {
    log_info "Building agent..."

    cd $VKAI_HOME/agent
    sudo -u $VKAI_USER go mod tidy
    sudo -u $VKAI_USER go build -o bin/vkaid ./cmd/

    log_info "Agent built successfully"
}

setup_config() {
    log_info "Setting up configuration..."

    # Generate JWT secret
    JWT_SECRET=$(openssl rand -base64 64)

    # Create environment file
    cat > $VKAI_HOME/.env << EOF
# vKAI Panel Configuration
# Generated: $(date)

# Server
SERVER_HOST=0.0.0.0
SERVER_PORT=30110

# Database
DB_HOST=localhost
DB_PORT=5432
DB_NAME=$DB_NAME
DB_USER=$DB_USER
DB_PASSWORD=$DB_PASS
DB_SSLMODE=disable
DB_MAX_CONNECTIONS=25
DB_MAX_IDLE_CONNECTIONS=5

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0

# JWT
JWT_SECRET=$JWT_SECRET
JWT_ACCESS_TOKEN_TTL=15m
JWT_REFRESH_TOKEN_TTL=168h
JWT_ISSUER=vkai-panel

# Logging
LOG_LEVEL=info
LOG_MAX_SIZE=100
LOG_MAX_BACKUPS=5
LOG_MAX_AGE=30
LOG_COMPRESS=true

# Frontend
NEXT_PUBLIC_API_URL=http://localhost:30110
EOF

    chown $VKAI_USER:$VKAI_GROUP $VKAI_HOME/.env
    chmod 600 $VKAI_HOME/.env

    log_info "Configuration created at $VKAI_HOME/.env"
}

install_systemd_services() {
    log_info "Installing systemd services..."

    # Copy service files
    cp $VKAI_HOME/deploy/systemd/vkai-api.service /etc/systemd/system/
    cp $VKAI_HOME/deploy/systemd/vkai-frontend.service /etc/systemd/system/

    # Reload systemd
    systemctl daemon-reload

    # Enable services
    systemctl enable vkai-api
    systemctl enable vkai-frontend

    log_info "Systemd services installed"
}

setup_nginx() {
    log_info "Setting up Nginx reverse proxy..."

    cat > /etc/nginx/sites-available/vkai-panel << 'EOF'
server {
    listen 80;
    server_name _;

    # Frontend
    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache_bypass $http_upgrade;
    }

    # API
    location /api/ {
        proxy_pass http://127.0.0.1:30110;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache_bypass $http_upgrade;
    }

    # WebSocket for terminal
    location /ws/ {
        proxy_pass http://127.0.0.1:30110;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_read_timeout 86400;
    }
}
EOF

    ln -sf /etc/nginx/sites-available/vkai-panel /etc/nginx/sites-enabled/
    rm -f /etc/nginx/sites-enabled/default

    nginx -t && systemctl reload nginx

    log_info "Nginx configured"
}

setup_logrotate() {
    log_info "Setting up log rotation..."

    cat > /etc/logrotate.d/vkai << EOF
$VKAI_LOG_DIR/*.log {
    daily
    missingok
    rotate 14
    compress
    delaycompress
    notifempty
    create 0640 $VKAI_USER $VKAI_GROUP
    sharedscripts
    postrotate
        systemctl reload vkai-api > /dev/null 2>&1 || true
    endscript
}
EOF

    log_info "Log rotation configured"
}

setup_firewall() {
    log_info "Setting up firewall..."

    # Allow SSH
    iptables -A INPUT -p tcp --dport 22 -j ACCEPT

    # Allow HTTP/HTTPS
    iptables -A INPUT -p tcp --dport 80 -j ACCEPT
    iptables -A INPUT -p tcp --dport 443 -j ACCEPT

    # Allow loopback
    iptables -A INPUT -i lo -j ACCEPT

    # Allow established connections
    iptables -A INPUT -m state --state ESTABLISHED,RELATED -j ACCEPT

    # Drop other traffic
    iptables -A INPUT -j DROP

    # Save rules
    iptables-save > /etc/iptables/rules.v4

    log_info "Firewall configured"
}

start_services() {
    log_info "Starting services..."

    systemctl start vkai-api
    systemctl start vkai-frontend
    systemctl start nginx

    log_info "Services started"
}

print_summary() {
    echo ""
    echo -e "${GREEN}╔═══════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║           vKAI Panel Installation Complete!              ║${NC}"
    echo -e "${GREEN}╚═══════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "${BLUE}Access URLs:${NC}"
    echo -e "  Panel:     http://$(hostname -I | awk '{print $1}')"
    echo -e "  API:       http://$(hostname -I | awk '{print $1}'):30110"
    echo ""
    echo -e "${BLUE}Default Credentials:${NC}"
    echo -e "  Username:  admin"
    echo -e "  Password:  admin123"
    echo ""
    echo -e "${YELLOW}⚠  IMPORTANT: Change the default password immediately!${NC}"
    echo ""
    echo -e "${BLUE}Service Management:${NC}"
    echo -e "  systemctl status vkai-api      # Check API status"
    echo -e "  systemctl status vkai-frontend  # Check frontend status"
    echo -e "  journalctl -u vkai-api -f       # View API logs"
    echo ""
    echo -e "${BLUE}Configuration:${NC}"
    echo -e "  $VKAI_HOME/.env"
    echo ""
}

# Main installation
main() {
    print_banner
    check_root
    detect_os

    install_dependencies
    install_golang
    install_nodejs
    setup_user
    setup_directories
    setup_database
    setup_redis
    build_backend
    build_frontend
    build_agent
    setup_config
    install_systemd_services
    setup_nginx
    setup_logrotate
    start_services
    print_summary
}

# Run installation
main "$@"
