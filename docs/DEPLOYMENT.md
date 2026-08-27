# vKAI Panel Deployment Guide

## Table of Contents

1. [Production Deployment](#production-deployment)
2. [System Requirements](#system-requirements)
3. [Installation](#installation)
4. [Configuration](#configuration)
5. [SSL Setup](#ssl-setup)
6. [Domain Setup](#domain-setup)
7. [Backup Strategy](#backup-strategy)
8. [Monitoring](#monitoring)
9. [Security Hardening](#security-hardening)
10. [Performance Optimization](#performance-optimization)
11. [High Availability](#high-availability)
12. [Troubleshooting](#troubleshooting)

---

## Production Deployment

### Overview

vKAI Panel is designed to run as systemd services without Docker. This guide covers production deployment on Ubuntu/Debian systems.

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Nginx (Reverse Proxy)                  │
│                    Port 80/443 (HTTP/HTTPS)                 │
└─────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┴───────────────┐
              │                               │
              ▼                               ▼
┌─────────────────────────┐     ┌─────────────────────────┐
│   Frontend (Next.js)    │     │    API Server (Go)      │
│      Port 3000          │     │     Port 30110          │
└─────────────────────────┘     └─────────────────────────┘
                                          │
                        ┌─────────────────┼─────────────────┐
                        │                 │                 │
                        ▼                 ▼                 ▼
              ┌─────────────┐   ┌─────────────┐   ┌─────────────┐
              │ PostgreSQL  │   │    Redis    │   │   Agent     │
              │  Port 5432  │   │  Port 6379  │   │ Port 30111  │
              └─────────────┘   └─────────────┘   └─────────────┘
```

---

## System Requirements

### Minimum Requirements

- **OS**: Ubuntu 22.04 LTS or Debian 12
- **CPU**: 2 cores
- **RAM**: 4 GB
- **Disk**: 50 GB SSD
- **Network**: 1 Gbps

### Recommended Requirements

- **OS**: Ubuntu 22.04 LTS
- **CPU**: 4 cores
- **RAM**: 8 GB
- **Disk**: 100 GB SSD
- **Network**: 1 Gbps

### Software Requirements

- Go 1.22+
- Node.js 20 LTS
- PostgreSQL 16+
- Redis 7+
- Nginx
- Certbot (for SSL)

---

## Installation

### Quick Installation

```bash
# Clone repository
git clone https://github.com/hitechcloud-vietnam/vkai-panel.git /opt/vkai-panel
cd /opt/vkai-panel

# Run installation script
chmod +x deploy/install.sh
sudo ./deploy/install.sh
```

### Manual Installation

#### 1. System Dependencies

```bash
# Update system
sudo apt update && sudo apt upgrade -y

# Install dependencies
sudo apt install -y \
  curl wget git unzip \
  build-essential \
  postgresql postgresql-contrib \
  redis-server \
  nginx \
  certbot python3-certbot-nginx \
  iptables \
  logrotate \
  jq
```

#### 2. Install Go

```bash
# Download Go
wget https://go.dev/dl/go1.22.0.linux-amd64.tar.gz

# Extract
sudo tar -C /usr/local -xzf go1.22.0.linux-amd64.tar.gz

# Add to PATH
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.profile
source ~/.profile

# Verify
go version
```

#### 3. Install Node.js

```bash
# Add NodeSource repository
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -

# Install Node.js
sudo apt install -y nodejs

# Verify
node --version
npm --version
```

#### 4. Create System User

```bash
# Create vkai user
sudo useradd -r -m -d /home/vkai -s /bin/bash vkai

# Add to necessary groups
sudo usermod -aG www-data vkai
```

#### 5. Setup Directories

```bash
# Create directories
sudo mkdir -p /opt/vkai-panel/{backend,frontend,config}
sudo mkdir -p /var/log/vkai
sudo mkdir -p /var/backups/vkai
sudo mkdir -p /var/www
sudo mkdir -p /etc/ssl/vkai

# Set permissions
sudo chown -R vkai:vkai /opt/vkai-panel
sudo chown -R vkai:vkai /var/log/vkai
sudo chown -R vkai:vkai /var/backups/vkai
sudo chown -R vkai:vkai /var/www
```

#### 6. Setup PostgreSQL

```bash
# Start PostgreSQL
sudo systemctl enable postgresql
sudo systemctl start postgresql

# Create database and user
sudo -u postgres psql -c "CREATE USER vkai WITH PASSWORD 'your_secure_password';"
sudo -u postgres psql -c "CREATE DATABASE vkai_panel OWNER vkai;"
sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE vkai_panel TO vkai;"

# Run migrations
cd /opt/vkai-panel/backend
sudo -u vkai go run cmd/migrate/main.go
```

#### 7. Setup Redis

```bash
# Start Redis
sudo systemctl enable redis-server
sudo systemctl start redis-server
```

#### 8. Build Backend

```bash
cd /opt/vkai-panel/backend

# Install dependencies
sudo -u vkai go mod tidy

# Build
sudo -u vkai go build -o bin/vkai-api ./cmd/api/
```

#### 9. Build Frontend

```bash
cd /opt/vkai-panel/frontend

# Install dependencies
sudo -u vkai npm install

# Build
sudo -u vkai npm run build
```

#### 10. Configure Environment

```bash
# Create environment file
sudo -u vkai tee /opt/vkai-panel/.env << EOF
SERVER_HOST=0.0.0.0
SERVER_PORT=30110

DB_HOST=localhost
DB_PORT=5432
DB_NAME=vkai_panel
DB_USER=vkai
DB_PASSWORD=your_secure_password
DB_SSLMODE=disable

REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0

JWT_SECRET=$(openssl rand -base64 64)
JWT_ACCESS_TOKEN_TTL=15m
JWT_REFRESH_TOKEN_TTL=168h
JWT_ISSUER=vkai-panel

LOG_LEVEL=info
LOG_MAX_SIZE=100
LOG_MAX_BACKUPS=5
LOG_MAX_AGE=30
LOG_COMPRESS=true

NEXT_PUBLIC_API_URL=http://localhost:30110
EOF

# Set permissions
sudo chmod 600 /opt/vkai-panel/.env
```

#### 11. Install Systemd Services

```bash
# Copy service files
sudo cp /opt/vkai-panel/deploy/systemd/vkai-api.service /etc/systemd/system/
sudo cp /opt/vkai-panel/deploy/systemd/vkai-frontend.service /etc/systemd/system/

# Reload systemd
sudo systemctl daemon-reload

# Enable services
sudo systemctl enable vkai-api
sudo systemctl enable vkai-frontend

# Start services
sudo systemctl start vkai-api
sudo systemctl start vkai-frontend
```

#### 12. Configure Nginx

```bash
# Copy Nginx configuration
sudo tee /etc/nginx/sites-available/vkai-panel << 'EOF'
server {
    listen 80;
    server_name _;

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

# Enable site
sudo ln -sf /etc/nginx/sites-available/vkai-panel /etc/nginx/sites-enabled/
sudo rm -f /etc/nginx/sites-enabled/default

# Test and reload
sudo nginx -t
sudo systemctl reload nginx
```

---

## Configuration

### Environment Variables

Edit `/opt/vkai-panel/.env`:

```bash
# Server
SERVER_HOST=0.0.0.0
SERVER_PORT=30110

# Database
DB_HOST=localhost
DB_PORT=5432
DB_NAME=vkai_panel
DB_USER=vkai
DB_PASSWORD=your_secure_password
DB_SSLMODE=disable
DB_MAX_CONNECTIONS=25
DB_MAX_IDLE_CONNECTIONS=5

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0

# JWT
JWT_SECRET=your_jwt_secret_key_at_least_32_chars
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
```

### Frontend Configuration

Edit `/opt/vkai-panel/frontend/.env.local`:

```bash
NEXT_PUBLIC_API_URL=http://localhost:30110
NEXT_PUBLIC_APP_NAME=vKAI Panel
NEXT_PUBLIC_APP_VERSION=1.0.0
```

---

## SSL Setup

### Let's Encrypt

```bash
# Install Certbot
sudo apt install certbot python3-certbot-nginx

# Obtain certificate
sudo certbot --nginx -d your-domain.com

# Test renewal
sudo certbot renew --dry-run

# Add auto-renewal to crontab
echo "0 0 1 * * certbot renew" | sudo crontab -
```

### Custom Certificate

```bash
# Copy certificate files
sudo cp your-domain.crt /etc/ssl/vkai/
sudo cp your-domain.key /etc/ssl/vkai/

# Set permissions
sudo chmod 600 /etc/ssl/vkai/your-domain.key
sudo chmod 644 /etc/ssl/vkai/your-domain.crt

# Update Nginx configuration
sudo nano /etc/nginx/sites-available/vkai-panel
```

Add SSL configuration:

```nginx
server {
    listen 443 ssl http2;
    server_name your-domain.com;

    ssl_certificate /etc/ssl/vkai/your-domain.crt;
    ssl_certificate_key /etc/ssl/vkai/your-domain.key;

    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-RSA-AES256-GCM-SHA512:DHE-RSA-AES256-GCM-SHA512;
    ssl_prefer_server_ciphers off;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 10m;

    # ... rest of configuration
}

server {
    listen 80;
    server_name your-domain.com;
    return 301 https://$server_name$request_uri;
}
```

---

## Domain Setup

### DNS Records

Add the following DNS records:

```
Type    Name    Value           TTL
A       @       your-server-ip  3600
A       www     your-server-ip  3600
```

### Nginx Configuration

Update `/etc/nginx/sites-available/vkai-panel`:

```nginx
server {
    listen 80;
    server_name your-domain.com www.your-domain.com;

    # ... configuration
}
```

---

## Backup Strategy

### Automated Backups

Create backup script `/opt/vkai-panel/scripts/backup.sh`:

```bash
#!/bin/bash
BACKUP_DIR="/var/backups/vkai"
DATE=$(date +%Y%m%d_%H%M%S)

# Backup database
pg_dump -U vkai -d vkai_panel | gzip > "$BACKUP_DIR/db_$DATE.sql.gz"

# Backup websites
tar -czf "$BACKUP_DIR/websites_$DATE.tar.gz" /var/www

# Backup configuration
tar -czf "$BACKUP_DIR/config_$DATE.tar.gz" /opt/vkai-panel/.env /etc/nginx/sites-available

# Cleanup old backups (keep 30 days)
find $BACKUP_DIR -name "*.gz" -mtime +30 -delete
```

Add to crontab:

```bash
# Daily backup at 2 AM
0 2 * * * /opt/vkai-panel/scripts/backup.sh

# Weekly full backup
0 3 * * 0 tar -czf /var/backups/vkai/full_$(date +\%Y\%m\%d).tar.gz /opt/vkai-panel /var/www
```

### Remote Backups

#### S3 Backup

```bash
# Install AWS CLI
sudo apt install awscli

# Configure AWS
aws configure

# Backup to S3
aws s3 sync /var/backups/vkai s3://your-bucket/backups/
```

#### SFTP Backup

```bash
# Create backup script
#!/bin/bash
sftp backup-user@backup-server << EOF
cd /backups/vkai
put /var/backups/vkai/*.gz
EOF
```

---

## Monitoring

### System Monitoring

```bash
# Install monitoring tools
sudo apt install -y htop iotop nethogs

# Monitor CPU/Memory
htop

# Monitor disk I/O
sudo iotop

# Monitor network
sudo nethogs
```

### Service Monitoring

```bash
# Check service status
systemctl status vkai-api
systemctl status vkai-frontend
systemctl status nginx
systemctl status postgresql
systemctl status redis-server

# View service logs
journalctl -u vkai-api -f
journalctl -u vkai-frontend -f
```

### Log Monitoring

```bash
# View API logs
tail -f /var/log/vkai/api.log

# Search for errors
grep -i error /var/log/vkai/api.log

# View Nginx logs
tail -f /var/log/nginx/access.log
tail -f /var/log/nginx/error.log
```

### Health Checks

Create health check script `/opt/vkai-panel/scripts/health-check.sh`:

```bash
#!/bin/bash

# Check API
if ! curl -s http://localhost:30110/health > /dev/null; then
    echo "API is down"
    systemctl restart vkai-api
fi

# Check Frontend
if ! curl -s http://localhost:3000 > /dev/null; then
    echo "Frontend is down"
    systemctl restart vkai-frontend
fi

# Check PostgreSQL
if ! pg_isready -h localhost -p 5432 > /dev/null; then
    echo "PostgreSQL is down"
    systemctl restart postgresql
fi

# Check Redis
if ! redis-cli ping > /dev/null; then
    echo "Redis is down"
    systemctl restart redis-server
fi
```

Add to crontab:

```bash
# Health check every 5 minutes
*/5 * * * * /opt/vkai-panel/scripts/health-check.sh
```

---

## Security Hardening

### Firewall Configuration

```bash
# Install UFW
sudo apt install ufw

# Allow SSH
sudo ufw allow 22/tcp

# Allow HTTP/HTTPS
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp

# Enable firewall
sudo ufw enable

# Check status
sudo ufw status
```

### SSH Hardening

Edit `/etc/ssh/sshd_config`:

```bash
# Disable root login
PermitRootLogin no

# Disable password authentication
PasswordAuthentication no

# Use SSH keys only
PubkeyAuthentication yes

# Change SSH port
Port 2222

# Limit login attempts
MaxAuthTries 3
```

### Fail2Ban

```bash
# Install Fail2Ban
sudo apt install fail2ban

# Configure
sudo cp /etc/fail2ban/jail.conf /etc/fail2ban/jail.local

# Edit configuration
sudo nano /etc/fail2ban/jail.local
```

Add configuration:

```ini
[sshd]
enabled = true
port = ssh
filter = sshd
logpath = /var/log/auth.log
maxretry = 3
bantime = 3600

[nginx-http-auth]
enabled = true
port = http,https
filter = nginx-http-auth
logpath = /var/log/nginx/error.log
maxretry = 3
bantime = 3600
```

### SSL Hardening

```nginx
# Strong SSL configuration
ssl_protocols TLSv1.2 TLSv1.3;
ssl_ciphers ECDHE-RSA-AES256-GCM-SHA512:DHE-RSA-AES256-GCM-SHA512;
ssl_prefer_server_ciphers off;
ssl_session_cache shared:SSL:10m;
ssl_session_timeout 10m;
ssl_stapling on;
ssl_stapling_verify on;

# Security headers
add_header X-Frame-Options "SAMEORIGIN" always;
add_header X-Content-Type-Options "nosniff" always;
add_header X-XSS-Protection "1; mode=block" always;
add_header Referrer-Policy "no-referrer-when-downgrade" always;
add_header Content-Security-Policy "default-src 'self' http: https: ws: wss: data: blob: 'unsafe-inline'; frame-ancestors 'self';" always;
add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
```

---

## Performance Optimization

### PostgreSQL Optimization

Edit `/etc/postgresql/16/main/postgresql.conf`:

```ini
# Memory
shared_buffers = 256MB
effective_cache_size = 768MB
work_mem = 4MB
maintenance_work_mem = 64MB

# Connections
max_connections = 100

# WAL
wal_buffers = 16MB
checkpoint_completion_target = 0.9
max_wal_size = 1GB
min_wal_size = 80MB

# Query planning
random_page_cost = 1.1
effective_io_concurrency = 200
```

### Redis Optimization

Edit `/etc/redis/redis.conf`:

```ini
# Memory
maxmemory 256mb
maxmemory-policy allkeys-lru

# Performance
tcp-keepalive 300
timeout 0

# Persistence
save 900 1
save 300 10
save 60 10000
```

### Nginx Optimization

```nginx
# Worker processes
worker_processes auto;
worker_connections 1024;

# Keepalive
keepalive_timeout 65;

# Buffers
client_max_body_size 100M;
proxy_buffering on;
proxy_buffer_size 4k;
proxy_buffers 8 4k;

# Gzip
gzip on;
gzip_vary on;
gzip_proxied any;
gzip_comp_level 6;
gzip_types text/plain text/css text/xml text/javascript application/json application/javascript application/xml+rss application/atom+xml image/svg+xml;
```

### Application Optimization

```bash
# Connection pool
DB_MAX_CONNECTIONS=25
DB_MAX_IDLE_CONNECTIONS=5

# Redis pool
REDIS_POOL_SIZE=10

# Rate limiting
RATE_LIMIT=100
```

---

## High Availability

### Load Balancing

```nginx
upstream vkai_backend {
    server 10.0.0.1:30110;
    server 10.0.0.2:30110;
    server 10.0.0.3:30110;
}

upstream vkai_frontend {
    server 10.0.0.1:3000;
    server 10.0.0.2:3000;
    server 10.0.0.3:3000;
}

server {
    listen 80;
    server_name your-domain.com;

    location / {
        proxy_pass http://vkai_frontend;
    }

    location /api/ {
        proxy_pass http://vkai_backend;
    }
}
```

### Database Replication

```bash
# Primary server
sudo -u postgres psql -c "ALTER SYSTEM SET wal_level = 'replica';"
sudo -u postgres psql -c "ALTER SYSTEM SET max_wal_senders = 3;"
sudo -u postgres psql -c "ALTER SYSTEM SET wal_keep_segments = 10;"

# Replica server
sudo -u postgres pg_basebackup -h primary-server -D /var/lib/postgresql/16/main -U replicator -P -v
```

---

## Troubleshooting

### Common Issues

#### Service Won't Start

```bash
# Check service status
systemctl status vkai-api

# View detailed logs
journalctl -u vkai-api -n 100

# Check configuration
cat /opt/vkai-panel/.env

# Check port availability
lsof -i :30110
```

#### Database Connection Issues

```bash
# Check PostgreSQL status
systemctl status postgresql

# Test connection
psql -h localhost -U vkai -d vkai_panel

# Check logs
tail -f /var/log/postgresql/postgresql-*.log

# Reset password
sudo -u postgres psql -c "ALTER USER vkai PASSWORD 'new_password';"
```

#### Nginx Issues

```bash
# Test configuration
nginx -t

# Check status
systemctl status nginx

# View logs
tail -f /var/log/nginx/error.log

# Reload configuration
systemctl reload nginx
```

#### SSL Issues

```bash
# Check certificate
certbot certificates

# Renew certificate
certbot renew

# Test SSL
openssl s_client -connect your-domain.com:443
```

### Performance Issues

```bash
# Check CPU/Memory
htop

# Check disk usage
df -h

# Check network
iftop

# Check database connections
sudo -u postgres psql -c "SELECT count(*) FROM pg_stat_activity;"
```

---

## Maintenance

### Regular Maintenance Tasks

```bash
# Update system packages
sudo apt update && sudo apt upgrade -y

# Update vKAI Panel
cd /opt/vkai-panel
git pull origin main
sudo systemctl restart vkai-api
sudo systemctl restart vkai-frontend

# Clean up old logs
find /var/log/vkai -name "*.log" -mtime +30 -delete

# Vacuum PostgreSQL
sudo -u postgres vacuumdb --all --analyze
```

### Backup Verification

```bash
# Test backup restoration
pg_dump -U vkai -d vkai_panel | psql -U vkai -d vkai_panel_test

# Verify backup integrity
gunzip -t /var/backups/vkai/db_*.sql.gz
```

---

## Support

- **Documentation**: https://docs.vkai.vn
- **Issues**: https://github.com/hitechcloud-vietnam/vkai-panel/issues
- **Email**: support@hitechcloud.vn
