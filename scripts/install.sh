#!/bin/bash
#
# vKAI Panel Auto-Setup Installer
# Usage: curl -sSL https://install.vkai.vn | bash
#   or:  bash install.sh
#
# This script will:
# 1. Detect system environment
# 2. Install dependencies (PostgreSQL, Redis, Nginx)
# 3. Generate random credentials
# 4. Install vKAI Panel
# 5. Configure systemd services
# 6. Print access information
#

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
VKAI_VERSION="0.3.0"
VKAI_INSTALL_DIR="/opt/vkai-panel"
VKAI_DATA_DIR="/var/lib/vkai-panel"
VKAI_LOG_DIR="/var/log/vkai-panel"
VKAI_BACKUP_DIR="/opt/vkai-panel/backups"

# Functions
print_banner() {
    echo -e "${CYAN}"
    echo "╔════════════════════════════════════════════════════════════╗"
    echo "║                                                            ║"
    echo "║   ██╗   ██╗██╗  ██╗ █████╗ ██╗                            ║"
    echo "║   ██║   ██║██║ ██╔╝██╔══██╗██║                            ║"
    echo "║   ██║   ██║█████╔╝ ███████║██║                            ║"
    echo "║   ╚██╗ ██╔╝██╔═██╗ ██╔══██║██║                            ║"
    echo "║    ╚████╔╝ ██║  ██╗██║  ██║███████╗                       ║"
    echo "║     ╚═══╝  ╚═╝  ╚═╝╚═╝  ╚═╝╚══════╝                       ║"
    echo "║                                                            ║"
    echo "║   vKAI Panel Installer v${VKAI_VERSION}                            ║"
    echo "║   https://vkai.vn                                          ║"
    echo "║                                                            ║"
    echo "╚════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
}

print_step() {
    echo -e "${BLUE}[STEP]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[✓]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[!]${NC} $1"
}

print_error() {
    echo -e "${RED}[✗]${NC} $1"
    exit 1
}

generate_random_string() {
    local length=$1
    cat /dev/urandom | tr -dc 'a-zA-Z0-9' | fold -w $length | head -n 1
}

generate_random_port() {
    # Generate random port between 9000-65000
    echo $((RANDOM % 56000 + 9000))
}

check_root() {
    if [[ $EUID -ne 0 ]]; then
        print_error "This script must be run as root"
    fi
}

check_os() {
    if [[ -f /etc/os-release ]]; then
        . /etc/os-release
        OS=$ID
        OS_VERSION=$VERSION_ID
    else
        print_error "Cannot detect operating system"
    fi

    case $OS in
        ubuntu|debian)
            PKG_MANAGER="apt"
            ;;
        centos|rhel|rocky|almalinux)
            PKG_MANAGER="yum"
            ;;
        *)
            print_error "Unsupported operating system: $OS"
            ;;
    esac

    print_success "Detected OS: $OS $OS_VERSION"
}

check_architecture() {
    ARCH=$(uname -m)
    case $ARCH in
        x86_64|amd64)
            ARCH="amd64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        *)
            print_error "Unsupported architecture: $ARCH"
            ;;
    esac
    print_success "Architecture: $ARCH"
}

install_dependencies() {
    print_step "Installing dependencies..."

    case $PKG_MANAGER in
        apt)
            apt-get update -qq
            apt-get install -y -qq \
                curl \
                wget \
                gnupg2 \
                lsb-release \
                ca-certificates \
                apt-transport-https \
                software-properties-common \
                unzip \
                tar \
                jq \
                > /dev/null 2>&1
            ;;
        yum)
            yum install -y -q \
                curl \
                wget \
                gnupg2 \
                ca-certificates \
                unzip \
                tar \
                jq \
                > /dev/null 2>&1
            ;;
    esac

    print_success "Dependencies installed"
}

install_postgresql() {
    print_step "Installing PostgreSQL..."

    if command -v psql &> /dev/null; then
        print_warning "PostgreSQL already installed"
        return
    fi

    case $PKG_MANAGER in
        apt)
            # Add PostgreSQL repository
            curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc | gpg --dearmor -o /usr/share/keyrings/postgresql-keyring.gpg
            echo "deb [signed-by=/usr/share/keyrings/postgresql-keyring.gpg] http://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main" > /etc/apt/sources.list.d/pgdg.list
            apt-get update -qq
            apt-get install -y -qq postgresql-16 postgresql-client-16 > /dev/null 2>&1
            ;;
        yum)
            yum install -y -q postgresql-server postgresql > /dev/null 2>&1
            postgresql-setup --initdb
            ;;
    esac

    systemctl enable postgresql
    systemctl start postgresql

    print_success "PostgreSQL installed"
}

install_redis() {
    print_step "Installing Redis..."

    if command -v redis-cli &> /dev/null; then
        print_warning "Redis already installed"
        return
    fi

    case $PKG_MANAGER in
        apt)
            apt-get install -y -qq redis-server > /dev/null 2>&1
            ;;
        yum)
            yum install -y -q redis > /dev/null 2>&1
            ;;
    esac

    systemctl enable redis-server || systemctl enable redis
    systemctl start redis-server || systemctl start redis

    print_success "Redis installed"
}

install_nginx() {
    print_step "Installing Nginx..."

    if command -v nginx &> /dev/null; then
        print_warning "Nginx already installed"
        return
    fi

    case $PKG_MANAGER in
        apt)
            apt-get install -y -qq nginx > /dev/null 2>&1
            ;;
        yum)
            yum install -y -q nginx > /dev/null 2>&1
            ;;
    esac

    systemctl enable nginx
    systemctl start nginx

    print_success "Nginx installed"
}

generate_credentials() {
    print_step "Generating random credentials..."

    # Generate random credentials
    DB_NAME="vkai_panel_$(generate_random_string 6)"
    DB_USER="vkai_$(generate_random_string 6)"
    DB_PASSWORD=$(generate_random_string 24)
    
    ADMIN_USERNAME="admin_$(generate_random_string 4)"
    ADMIN_PASSWORD=$(generate_random_string 16)
    ADMIN_EMAIL="admin@$(hostname -f 2>/dev/null || echo 'localhost')"
    
    JWT_SECRET=$(generate_random_string 64)
    
    # Generate random ports
    API_PORT=$(generate_random_port)
    FRONTEND_PORT=$(generate_random_port)
    
    # Generate random panel URL path
    PANEL_PATH="/$(generate_random_string 8)"

    print_success "Credentials generated"
}

setup_database() {
    print_step "Setting up database..."

    # Create database and user
    sudo -u postgres psql -c "CREATE USER $DB_USER WITH PASSWORD '$DB_PASSWORD';" 2>/dev/null || true
    sudo -u postgres psql -c "CREATE DATABASE $DB_NAME OWNER $DB_USER;" 2>/dev/null || true
    sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE $DB_NAME TO $DB_USER;" 2>/dev/null || true

    # Run migrations
    print_step "Running database migrations..."
    
    # Create tables
    sudo -u postgres psql -d $DB_NAME << 'EOF'
-- Tenants
CREATE TABLE IF NOT EXISTS tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(100) UNIQUE NOT NULL,
    status VARCHAR(50) DEFAULT 'active',
    plan VARCHAR(50) DEFAULT 'free',
    settings JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Users
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id),
    username VARCHAR(100) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    first_name VARCHAR(100),
    last_name VARCHAR(100),
    status VARCHAR(50) DEFAULT 'active',
    last_login_at TIMESTAMP WITH TIME ZONE,
    last_login_ip VARCHAR(45),
    mfa_enabled BOOLEAN DEFAULT false,
    mfa_secret VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Websites
CREATE TABLE IF NOT EXISTS websites (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id),
    domain VARCHAR(255) NOT NULL,
    document_root VARCHAR(500),
    php_version VARCHAR(20),
    status VARCHAR(50) DEFAULT 'active',
    ssl_enabled BOOLEAN DEFAULT false,
    ssl_cert_path VARCHAR(500),
    ssl_key_path VARCHAR(500),
    settings JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Firewall Rules
CREATE TABLE IF NOT EXISTS firewall_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id),
    port VARCHAR(20) NOT NULL,
    protocol VARCHAR(10) DEFAULT 'tcp',
    source_ip VARCHAR(45),
    action VARCHAR(20) DEFAULT 'allow',
    comment TEXT,
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Cron Jobs
CREATE TABLE IF NOT EXISTS cron_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id),
    name VARCHAR(255) NOT NULL,
    command TEXT NOT NULL,
    schedule VARCHAR(100) NOT NULL,
    enabled BOOLEAN DEFAULT true,
    last_run_at TIMESTAMP WITH TIME ZONE,
    next_run_at TIMESTAMP WITH TIME ZONE,
    status VARCHAR(50) DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- DNS Zones
CREATE TABLE IF NOT EXISTS dns_zones (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id),
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) DEFAULT 'master',
    master_ip VARCHAR(45),
    status VARCHAR(50) DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- DNS Records
CREATE TABLE IF NOT EXISTS dns_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    zone_id UUID REFERENCES dns_zones(id),
    name VARCHAR(255) NOT NULL,
    type VARCHAR(10) NOT NULL,
    value TEXT NOT NULL,
    ttl INTEGER DEFAULT 3600,
    priority INTEGER,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Databases
CREATE TABLE IF NOT EXISTS databases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id),
    name VARCHAR(100) NOT NULL,
    db_type VARCHAR(50) DEFAULT 'postgresql',
    status VARCHAR(50) DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Database Users
CREATE TABLE IF NOT EXISTS database_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id),
    username VARCHAR(100) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    status VARCHAR(50) DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- API Keys
CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id),
    user_id UUID REFERENCES users(id),
    name VARCHAR(255) NOT NULL,
    key_hash VARCHAR(255) NOT NULL,
    key_prefix VARCHAR(20) NOT NULL,
    permissions JSONB DEFAULT '[]',
    last_used_at TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE,
    status VARCHAR(50) DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Git Deployments
CREATE TABLE IF NOT EXISTS git_deployments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id),
    website_id UUID REFERENCES websites(id),
    repo_url VARCHAR(500) NOT NULL,
    branch VARCHAR(100) DEFAULT 'main',
    deploy_path VARCHAR(500),
    status VARCHAR(50) DEFAULT 'pending',
    last_deploy_at TIMESTAMP WITH TIME ZONE,
    auto_deploy BOOLEAN DEFAULT false,
    webhook_secret VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Security Scans
CREATE TABLE IF NOT EXISTS security_scans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id),
    scan_type VARCHAR(50) NOT NULL,
    status VARCHAR(50) DEFAULT 'pending',
    results JSONB DEFAULT '{}',
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_users_tenant_id ON users(tenant_id);
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_websites_tenant_id ON websites(tenant_id);
CREATE INDEX IF NOT EXISTS idx_websites_domain ON websites(domain);
CREATE INDEX IF NOT EXISTS idx_firewall_rules_tenant_id ON firewall_rules(tenant_id);
CREATE INDEX IF NOT EXISTS idx_cron_jobs_tenant_id ON cron_jobs(tenant_id);
CREATE INDEX IF NOT EXISTS idx_dns_zones_tenant_id ON dns_zones(tenant_id);
CREATE INDEX IF NOT EXISTS idx_dns_records_zone_id ON dns_records(zone_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_tenant_id ON api_keys(tenant_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys(user_id);
CREATE INDEX IF NOT EXISTS idx_git_deployments_tenant_id ON git_deployments(tenant_id);
CREATE INDEX IF NOT EXISTS idx_security_scans_tenant_id ON security_scans(tenant_id);
EOF

    print_success "Database setup complete"
}

install_vkai_panel() {
    print_step "Installing vKAI Panel..."

    # Create directories
    mkdir -p $VKAI_INSTALL_DIR
    mkdir -p $VKAI_DATA_DIR
    mkdir -p $VKAI_LOG_DIR
    mkdir -p $VKAI_BACKUP_DIR

    # Download binaries
    print_step "Downloading vKAI Panel v${VKAI_VERSION}..."
    
    # For now, we'll copy from local build
    # In production, this would download from GitHub releases
    if [[ -f /home/vkai-panel/backend/vkai-panel-api ]]; then
        cp /home/vkai-panel/backend/vkai-panel-api $VKAI_INSTALL_DIR/api
    else
        print_error "vKAI Panel binary not found. Please build from source first."
    fi

    if [[ -f /home/vkai-panel/backend/vkai ]]; then
        cp /home/vkai-panel/backend/vkai $VKAI_INSTALL_DIR/vkai
    fi

    # Copy frontend build
    if [[ -d /home/vkai-panel/frontend/.next ]]; then
        cp -r /home/vkai-panel/frontend/.next $VKAI_INSTALL_DIR/frontend
        cp /home/vkai-panel/frontend/package.json $VKAI_INSTALL_DIR/frontend/
    fi

    # Make binaries executable
    chmod +x $VKAI_INSTALL_DIR/api
    chmod +x $VKAI_INSTALL_DIR/vkai

    # Create symlink for CLI
    ln -sf $VKAI_INSTALL_DIR/vkai /usr/local/bin/vkai

    print_success "vKAI Panel installed"
}

create_config() {
    print_step "Creating configuration..."

    # Create config file
    cat > $VKAI_INSTALL_DIR/config.yaml << EOF
server:
  host: "0.0.0.0"
  port: $API_PORT
  mode: "release"

database:
  host: "localhost"
  port: 5432
  name: "$DB_NAME"
  user: "$DB_USER"
  password: "$DB_PASSWORD"
  sslmode: "disable"
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: 300

redis:
  host: "localhost"
  port: 6379
  password: ""
  db: 0

jwt:
  secret: "$JWT_SECRET"
  access_token_ttl: 15
  refresh_token_ttl: 10080

log:
  level: "info"
  format: "json"
  output: "file"
  file: "$VKAI_LOG_DIR/api.log"
  max_size: 100
  max_backups: 3
  max_age: 30
EOF

    # Create environment file
    cat > $VKAI_INSTALL_DIR/.env << EOF
VKAI_DATABASE_URL=postgres://$DB_USER:$DB_PASSWORD@localhost:5432/$DB_NAME?sslmode=disable
VKAI_REDIS_URL=redis://localhost:6379/0
VKAI_JWT_SECRET=$JWT_SECRET
VKAI_SERVER_PORT=$API_PORT
VKAI_SERVER_HOST=0.0.0.0
VKAI_LOG_LEVEL=info
VKAI_LOG_FILE=$VKAI_LOG_DIR/api.log
EOF

    # Set permissions
    chmod 600 $VKAI_INSTALL_DIR/.env

    print_success "Configuration created"
}

create_systemd_services() {
    print_step "Creating systemd services..."

    # API service
    cat > /etc/systemd/system/vkai-panel-api.service << EOF
[Unit]
Description=vKAI Panel API Server
After=network.target postgresql.service redis.service
Requires=postgresql.service redis.service

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=$VKAI_INSTALL_DIR
ExecStart=$VKAI_INSTALL_DIR/api
EnvironmentFile=$VKAI_INSTALL_DIR/.env
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=vkai-panel-api

[Install]
WantedBy=multi-user.target
EOF

    # Frontend service
    cat > /etc/systemd/system/vkai-panel-frontend.service << EOF
[Unit]
Description=vKAI Panel Frontend
After=network.target

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=$VKAI_INSTALL_DIR/frontend
ExecStart=/usr/bin/node $VKAI_INSTALL_DIR/frontend/node_modules/.bin/next start -p $FRONTEND_PORT
Environment=NODE_ENV=production
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=vkai-panel-frontend

[Install]
WantedBy=multi-user.target
EOF

    # Reload systemd
    systemctl daemon-reload

    # Enable services
    systemctl enable vkai-panel-api
    systemctl enable vkai-panel-frontend

    print_success "Systemd services created"
}

configure_nginx() {
    print_step "Configuring Nginx..."

    # Get server IP
    SERVER_IP=$(curl -s ifconfig.me 2>/dev/null || curl -s ipinfo.io/ip 2>/dev/null || echo "localhost")

    # Create Nginx config
    cat > /etc/nginx/sites-available/vkai-panel << EOF
server {
    listen 80;
    server_name $SERVER_IP;

    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "no-referrer-when-downgrade" always;

    # API proxy
    location /api/ {
        proxy_pass http://127.0.0.1:$API_PORT;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_cache_bypass \$http_upgrade;
        proxy_read_timeout 86400;
    }

    # WebSocket proxy
    location /ws {
        proxy_pass http://127.0.0.1:$API_PORT;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_read_timeout 86400;
    }

    # Frontend proxy
    location / {
        proxy_pass http://127.0.0.1:$FRONTEND_PORT;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_cache_bypass \$http_upgrade;
    }

    # Increase upload size
    client_max_body_size 100M;
}
EOF

    # Enable site
    ln -sf /etc/nginx/sites-available/vkai-panel /etc/nginx/sites-enabled/
    
    # Remove default site
    rm -f /etc/nginx/sites-enabled/default

    # Test and reload Nginx
    nginx -t && systemctl reload nginx

    print_success "Nginx configured"
}

create_admin_user() {
    print_step "Creating admin user..."

    # Generate password hash
    ADMIN_PASSWORD_HASH=$(python3 -c "
import bcrypt
password = '$ADMIN_PASSWORD'.encode('utf-8')
salt = bcrypt.gensalt(10, prefix=b'2a')
hashed = bcrypt.hashpw(password, salt)
print(hashed.decode('utf-8'))
" 2>/dev/null || echo "")

    if [[ -z "$ADMIN_PASSWORD_HASH" ]]; then
        # Fallback: use pre-generated hash for 'admin123'
        ADMIN_PASSWORD_HASH='$2a$10$.tqjuZcezymaHIy8AHcK1uXlszd6W7eOcbHFzWoU9pDArdjJdItD.'
        ADMIN_PASSWORD="admin123"
        print_warning "Using default admin password (bcrypt not available)"
    fi

    # Create admin user
    sudo -u postgres psql -d $DB_NAME << EOF
INSERT INTO tenants (id, name, slug, status, plan) 
VALUES ('a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11', 'Default', 'default', 'active', 'enterprise')
ON CONFLICT (id) DO NOTHING;

INSERT INTO users (id, tenant_id, username, email, password_hash, first_name, last_name, status)
VALUES (
    'b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
    'a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11',
    '$ADMIN_USERNAME',
    '$ADMIN_EMAIL',
    '$ADMIN_PASSWORD_HASH',
    'System',
    'Admin',
    'active'
)
ON CONFLICT (id) DO UPDATE SET
    username = EXCLUDED.username,
    email = EXCLUDED.email,
    password_hash = EXCLUDED.password_hash;
EOF

    print_success "Admin user created"
}

start_services() {
    print_step "Starting services..."

    systemctl start vkai-panel-api
    systemctl start vkai-panel-frontend

    # Wait for services to start
    sleep 3

    # Check if services are running
    if systemctl is-active --quiet vkai-panel-api; then
        print_success "API server started"
    else
        print_error "Failed to start API server"
    fi

    if systemctl is-active --quiet vkai-panel-frontend; then
        print_success "Frontend started"
    else
        print_error "Failed to start frontend"
    fi
}

print_installation_info() {
    echo ""
    echo -e "${GREEN}╔════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║                                                            ║${NC}"
    echo -e "${GREEN}║   vKAI Panel Installation Complete!                        ║${NC}"
    echo -e "${GREEN}║                                                            ║${NC}"
    echo -e "${GREEN}╚════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "${CYAN}Access Information:${NC}"
    echo -e "  URL:      http://$SERVER_IP"
    echo -e "  Username: ${YELLOW}$ADMIN_USERNAME${NC}"
    echo -e "  Password: ${YELLOW}$ADMIN_PASSWORD${NC}"
    echo ""
    echo -e "${CYAN}Service Ports:${NC}"
    echo -e "  API:      $API_PORT"
    echo -e "  Frontend: $FRONTEND_PORT"
    echo ""
    echo -e "${CYAN}Database:${NC}"
    echo -e "  Name:     $DB_NAME"
    echo -e "  User:     $DB_USER"
    echo -e "  Password: ${YELLOW}$DB_PASSWORD${NC}"
    echo ""
    echo -e "${CYAN}CLI Tool:${NC}"
    echo -e "  Run:      vkai --help"
    echo ""
    echo -e "${CYAN}Configuration:${NC}"
    echo -e "  Config:   $VKAI_INSTALL_DIR/config.yaml"
    echo -e "  Env:      $VKAI_INSTALL_DIR/.env"
    echo -e "  Logs:     $VKAI_LOG_DIR/"
    echo ""
    echo -e "${YELLOW}⚠ Important: Save these credentials! They will not be shown again.${NC}"
    echo ""
    echo -e "${CYAN}Useful Commands:${NC}"
    echo -e "  vkai server status          Show server status"
    echo -e "  vkai user list              List all users"
    echo -e "  vkai service list           List all services"
    echo -e "  systemctl status vkai-panel-api    Check API status"
    echo -e "  journalctl -u vkai-panel-api -f    View API logs"
    echo ""
}

save_credentials() {
    # Save credentials to file
    cat > $VKAI_INSTALL_DIR/credentials.txt << EOF
vKAI Panel Installation Credentials
Generated: $(date)

URL: http://$SERVER_IP
Username: $ADMIN_USERNAME
Password: $ADMIN_PASSWORD

Database:
  Name: $DB_NAME
  User: $DB_USER
  Password: $DB_PASSWORD

API Port: $API_PORT
Frontend Port: $FRONTEND_PORT

JWT Secret: $JWT_SECRET
EOF

    chmod 600 $VKAI_INSTALL_DIR/credentials.txt
    print_success "Credentials saved to $VKAI_INSTALL_DIR/credentials.txt"
}

# Main installation flow
main() {
    print_banner
    
    check_root
    check_os
    check_architecture
    
    install_dependencies
    install_postgresql
    install_redis
    install_nginx
    
    generate_credentials
    setup_database
    install_vkai_panel
    create_config
    create_systemd_services
    configure_nginx
    create_admin_user
    start_services
    
    save_credentials
    print_installation_info
}

# Run main function
main "$@"
