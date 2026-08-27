# vKAI Panel Configuration Guide

## Table of Contents

1. [Environment Variables](#environment-variables)
2. [Database Configuration](#database-configuration)
3. [Redis Configuration](#redis-configuration)
4. [JWT Configuration](#jwt-configuration)
5. [Logging Configuration](#logging-configuration)
6. [Server Configuration](#server-configuration)
7. [Nginx Configuration](#nginx-configuration)
8. [SSL Configuration](#ssl-configuration)
9. [Backup Configuration](#backup-configuration)
10. [Notification Configuration](#notification-configuration)
11. [Security Configuration](#security-configuration)
12. [Performance Tuning](#performance-tuning)

---

## Environment Variables

### Backend Configuration

Create `/opt/vkai-panel/.env`:

```bash
# ===========================================
# Server Configuration
# ===========================================
SERVER_HOST=0.0.0.0
SERVER_PORT=30110
SERVER_READ_TIMEOUT=30s
SERVER_WRITE_TIMEOUT=30s
SERVER_IDLE_TIMEOUT=120s

# ===========================================
# Database Configuration
# ===========================================
DB_HOST=localhost
DB_PORT=5432
DB_NAME=vkai_panel
DB_USER=vkai
DB_PASSWORD=your_secure_password
DB_SSLMODE=disable
DB_MAX_CONNECTIONS=25
DB_MAX_IDLE_CONNECTIONS=5
DB_CONNECTION_MAX_LIFETIME=5m

# ===========================================
# Redis Configuration
# ===========================================
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0
REDIS_MAX_RETRIES=3
REDIS_POOL_SIZE=10

# ===========================================
# JWT Configuration
# ===========================================
JWT_SECRET=your_jwt_secret_key_at_least_32_chars
JWT_ACCESS_TOKEN_TTL=15m
JWT_REFRESH_TOKEN_TTL=168h
JWT_ISSUER=vkai-panel

# ===========================================
# Logging Configuration
# ===========================================
LOG_LEVEL=info
LOG_FORMAT=json
LOG_MAX_SIZE=100
LOG_MAX_BACKUPS=5
LOG_MAX_AGE=30
LOG_COMPRESS=true

# ===========================================
# Frontend Configuration
# ===========================================
NEXT_PUBLIC_API_URL=http://localhost:30110
```

### Frontend Configuration

Create `/opt/vkai-panel/frontend/.env.local`:

```bash
# API URL
NEXT_PUBLIC_API_URL=http://localhost:30110

# App Configuration
NEXT_PUBLIC_APP_NAME=vKAI Panel
NEXT_PUBLIC_APP_VERSION=1.0.0

# Features
NEXT_PUBLIC_ENABLE_ANALYTICS=false
NEXT_PUBLIC_ENABLE_SENTRY=false
```

---

## Database Configuration

### PostgreSQL Setup

#### 1. Install PostgreSQL

```bash
# Ubuntu/Debian
sudo apt update
sudo apt install postgresql postgresql-contrib

# CentOS/RHEL
sudo yum install postgresql-server postgresql-contrib
sudo postgresql-setup --initdb
```

#### 2. Configure PostgreSQL

Edit `/etc/postgresql/16/main/postgresql.conf`:

```ini
# Connection Settings
listen_addresses = 'localhost'
port = 5432
max_connections = 100

# Memory Settings
shared_buffers = 256MB
effective_cache_size = 768MB
work_mem = 4MB
maintenance_work_mem = 64MB

# Write Ahead Log
wal_buffers = 16MB
checkpoint_completion_target = 0.9
max_wal_size = 1GB
min_wal_size = 80MB

# Query Planning
random_page_cost = 1.1
effective_io_concurrency = 200
```

#### 3. Configure Authentication

Edit `/etc/postgresql/16/main/pg_hba.conf`:

```ini
# TYPE  DATABASE        USER            ADDRESS                 METHOD
local   all             postgres                                peer
local   all             all                                     peer
host    all             all             127.0.0.1/32            scram-sha-256
host    all             all             ::1/128                 scram-sha-256
```

#### 4. Create Database and User

```bash
# Switch to postgres user
sudo -u postgres psql

# Create user
CREATE USER vkai WITH PASSWORD 'your_secure_password';

# Create database
CREATE DATABASE vkai_panel OWNER vkai;

# Grant privileges
GRANT ALL PRIVILEGES ON DATABASE vkai_panel TO vkai;

# Exit
\q
```

#### 5. Run Migrations

```bash
cd /opt/vkai-panel/backend
go run cmd/migrate/main.go
```

### Database Backup

```bash
# Backup
pg_dump -U vkai -d vkai_panel > backup.sql

# Restore
psql -U vkai -d vkai_panel < backup.sql

# Automated backup script
#!/bin/bash
BACKUP_DIR="/var/backups/vkai"
DATE=$(date +%Y%m%d_%H%M%S)
pg_dump -U vkai -d vkai_panel | gzip > "$BACKUP_DIR/vkai_panel_$DATE.sql.gz"
find $BACKUP_DIR -name "*.sql.gz" -mtime +30 -delete
```

---

## Redis Configuration

### Redis Setup

#### 1. Install Redis

```bash
# Ubuntu/Debian
sudo apt update
sudo apt install redis-server

# CentOS/RHEL
sudo yum install redis
```

#### 2. Configure Redis

Edit `/etc/redis/redis.conf`:

```ini
# Network
bind 127.0.0.1
port 6379
protected-mode yes

# General
daemonize yes
supervised systemd
pidfile /var/run/redis/redis-server.pid

# Memory
maxmemory 256mb
maxmemory-policy allkeys-lru

# Persistence
save 900 1
save 300 10
save 60 10000

# Logging
loglevel notice
logfile /var/log/redis/redis-server.log

# Security
requirepass your_redis_password
```

#### 3. Start Redis

```bash
sudo systemctl enable redis-server
sudo systemctl start redis-server
```

### Redis Commands

```bash
# Connect to Redis
redis-cli

# Authenticate
AUTH your_redis_password

# Check connection
PING

# View all keys
KEYS *

# Flush database
FLUSHDB
```

---

## JWT Configuration

### JWT Secret

Generate a secure JWT secret:

```bash
# Generate random secret
openssl rand -base64 64

# Or use Python
python3 -c "import secrets; print(secrets.token_urlsafe(64))"
```

### Token Configuration

```bash
# Access token TTL (short-lived)
JWT_ACCESS_TOKEN_TTL=15m

# Refresh token TTL (long-lived)
JWT_REFRESH_TOKEN_TTL=168h

# Token issuer
JWT_ISSUER=vkai-panel
```

### Token Refresh Flow

1. Client sends request with expired access token
2. Server returns 401 Unauthorized
3. Client sends refresh token to `/api/v1/auth/refresh`
4. Server validates refresh token and returns new access token
5. Client retries original request with new access token

---

## Logging Configuration

### Log Levels

- `debug`: Detailed debug information
- `info`: General information
- `warn`: Warning messages
- `error`: Error messages
- `fatal`: Fatal errors (exits application)

### Log Format

```bash
# JSON format (recommended for production)
LOG_FORMAT=json

# Text format (for development)
LOG_FORMAT=text
```

### Log Rotation

```bash
# Maximum log file size (MB)
LOG_MAX_SIZE=100

# Maximum number of old log files
LOG_MAX_BACKUPS=5

# Maximum days to retain old log files
LOG_MAX_AGE=30

# Compress old log files
LOG_COMPRESS=true
```

### Log Files

```
/var/log/vkai/
├── api.log          # API server logs
├── api.log.1        # Rotated log
├── api.log.2.gz     # Compressed rotated log
└── ...
```

### Viewing Logs

```bash
# View API logs
journalctl -u vkai-api -f

# View specific log file
tail -f /var/log/vkai/api.log

# Search logs
grep "ERROR" /var/log/vkai/api.log

# View last 100 lines
tail -n 100 /var/log/vkai/api.log
```

---

## Server Configuration

### Server Settings

```bash
# Server host
SERVER_HOST=0.0.0.0

# Server port
SERVER_PORT=30110

# Read timeout
SERVER_READ_TIMEOUT=30s

# Write timeout
SERVER_WRITE_TIMEOUT=30s

# Idle timeout
SERVER_IDLE_TIMEOUT=120s
```

### Timeouts

- **Read Timeout**: Maximum time to read request
- **Write Timeout**: Maximum time to write response
- **Idle Timeout**: Maximum time for keep-alive connections

### Connection Limits

```bash
# Maximum concurrent connections
SERVER_MAX_CONNECTIONS=1000

# Maximum requests per second
SERVER_RATE_LIMIT=100
```

---

## Nginx Configuration

### Reverse Proxy Configuration

Create `/etc/nginx/sites-available/vkai-panel`:

```nginx
server {
    listen 80;
    server_name your-domain.com;

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

    # WebSocket
    location /ws/ {
        proxy_pass http://127.0.0.1:30110;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_read_timeout 86400;
    }
}
```

### Enable Site

```bash
sudo ln -s /etc/nginx/sites-available/vkai-panel /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

### SSL Configuration

```bash
# Install Certbot
sudo apt install certbot python3-certbot-nginx

# Obtain SSL certificate
sudo certbot --nginx -d your-domain.com

# Auto-renewal
sudo certbot renew --dry-run
```

---

## SSL Configuration

### Let's Encrypt

#### 1. Install Certbot

```bash
sudo apt install certbot python3-certbot-nginx
```

#### 2. Obtain Certificate

```bash
sudo certbot --nginx -d your-domain.com
```

#### 3. Auto-Renewal

```bash
# Test renewal
sudo certbot renew --dry-run

# Add to crontab
0 0 1 * * certbot renew
```

### Custom Certificates

#### 1. Generate CSR

```bash
openssl req -new -newkey rsa:2048 -nodes \
  -keyout your-domain.key \
  -out your-domain.csr
```

#### 2. Install Certificate

```bash
# Copy certificate files
sudo cp your-domain.crt /etc/ssl/vkai/
sudo cp your-domain.key /etc/ssl/vkai/

# Set permissions
sudo chmod 600 /etc/ssl/vkai/your-domain.key
sudo chmod 644 /etc/ssl/vkai/your-domain.crt
```

#### 3. Configure Nginx

```nginx
server {
    listen 443 ssl http2;
    server_name your-domain.com;

    ssl_certificate /etc/ssl/vkai/your-domain.crt;
    ssl_certificate_key /etc/ssl/vkai/your-domain.key;

    # SSL settings
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-RSA-AES256-GCM-SHA512:DHE-RSA-AES256-GCM-SHA512;
    ssl_prefer_server_ciphers off;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 10m;

    # ... rest of configuration
}
```

---

## Backup Configuration

### Local Backups

```bash
# Backup directory
BACKUP_DIR=/var/backups/vkai

# Retention days
RETENTION_DAYS=30
```

### S3 Backups

```bash
# AWS credentials
AWS_ACCESS_KEY_ID=your_access_key
AWS_SECRET_ACCESS_KEY=your_secret_key
AWS_REGION=us-east-1
AWS_BUCKET=vkai-backups
```

### SFTP Backups

```bash
# SFTP configuration
SFTP_HOST=backup-server.example.com
SFTP_PORT=22
SFTP_USER=backup_user
SFTP_PASSWORD=backup_password
SFTP_PATH=/backups/vkai
```

### Backup Script

```bash
#!/bin/bash
# /opt/vkai-panel/scripts/backup.sh

BACKUP_DIR="/var/backups/vkai"
DATE=$(date +%Y%m%d_%H%M%S)

# Backup database
pg_dump -U vkai -d vkai_panel | gzip > "$BACKUP_DIR/db_$DATE.sql.gz"

# Backup websites
tar -czf "$BACKUP_DIR/websites_$DATE.tar.gz" /var/www

# Backup configuration
tar -czf "$BACKUP_DIR/config_$DATE.tar.gz" /opt/vkai-panel/.env /etc/nginx/sites-available

# Cleanup old backups
find $BACKUP_DIR -name "*.gz" -mtime +30 -delete
```

---

## Notification Configuration

### Email Notifications

```bash
# SMTP configuration
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=your-email@gmail.com
SMTP_PASSWORD=your-app-password
SMTP_FROM=noreply@your-domain.com
SMTP_TLS=true
```

### Webhook Notifications

```bash
# Webhook URL
WEBHOOK_URL=https://hooks.slack.com/services/xxx/yyy/zzz

# Webhook events
WEBHOOK_EVENTS=server.down,backup.failed,ssl.expiring
```

### Telegram Notifications

```bash
# Telegram bot token
TELEGRAM_BOT_TOKEN=your_bot_token

# Telegram chat ID
TELEGRAM_CHAT_ID=your_chat_id
```

---

## Security Configuration

### Password Policy

```bash
# Minimum password length
PASSWORD_MIN_LENGTH=8

# Require uppercase
PASSWORD_REQUIRE_UPPERCASE=true

# Require lowercase
PASSWORD_REQUIRE_LOWERCASE=true

# Require numbers
PASSWORD_REQUIRE_NUMBERS=true

# Require special characters
PASSWORD_REQUIRE_SPECIAL=true
```

### Session Configuration

```bash
# Session timeout
SESSION_TIMEOUT=15m

# Maximum sessions per user
MAX_SESSIONS=5

# Enable two-factor authentication
ENABLE_2FA=true
```

### IP Whitelist

```bash
# Allowed IP addresses (comma-separated)
IP_WHITELIST=192.168.1.0/24,10.0.0.0/8

# Enable IP whitelist
ENABLE_IP_WHITELIST=false
```

### Rate Limiting

```bash
# Rate limit (requests per minute)
RATE_LIMIT=100

# Rate limit burst
RATE_LIMIT_BURST=20
```

---

## Performance Tuning

### Database Performance

```ini
# PostgreSQL performance tuning
shared_buffers = 256MB
effective_cache_size = 768MB
work_mem = 4MB
maintenance_work_mem = 64MB
max_connections = 100
```

### Redis Performance

```ini
# Redis performance tuning
maxmemory 256mb
maxmemory-policy allkeys-lru
tcp-keepalive 300
```

### Application Performance

```bash
# Connection pool
DB_MAX_CONNECTIONS=25
DB_MAX_IDLE_CONNECTIONS=5

# Redis pool
REDIS_POOL_SIZE=10

# Rate limiting
RATE_LIMIT=100
```

### Nginx Performance

```nginx
# Nginx performance tuning
worker_processes auto;
worker_connections 1024;
keepalive_timeout 65;
client_max_body_size 100M;
proxy_buffering on;
proxy_buffer_size 4k;
proxy_buffers 8 4k;
```

---

## Monitoring Configuration

### Prometheus Metrics

```bash
# Enable Prometheus metrics
ENABLE_METRICS=true
METRICS_PORT=9090
```

### Health Checks

```bash
# Health check endpoint
HEALTH_CHECK_PATH=/health

# Health check interval
HEALTH_CHECK_INTERVAL=30s
```

---

## Advanced Configuration

### Custom Web Servers

```bash
# Supported web servers
WEB_SERVERS=nginx,apache,openlitespeed,litespeed,caddy,traefik

# Default web server
DEFAULT_WEB_SERVER=nginx
```

### PHP Configuration

```bash
# Supported PHP versions
PHP_VERSIONS=7.4,8.0,8.1,8.2,8.3,8.4

# Default PHP version
DEFAULT_PHP_VERSION=8.2

# PHP-FPM socket path
PHP_FPM_SOCKET=/var/run/php/php{version}-fpm.sock
```

### Docker Configuration

```bash
# Enable Docker management
ENABLE_DOCKER=true

# Docker socket
DOCKER_SOCKET=/var/run/docker.sock
```

---

## Troubleshooting

### Common Issues

#### Database Connection Failed

```bash
# Check PostgreSQL status
sudo systemctl status postgresql

# Check connection
psql -h localhost -U vkai -d vkai_panel

# Check logs
sudo tail -f /var/log/postgresql/postgresql-*.log
```

#### Redis Connection Failed

```bash
# Check Redis status
sudo systemctl status redis-server

# Test connection
redis-cli ping

# Check logs
sudo tail -f /var/log/redis/redis-server.log
```

#### API Not Responding

```bash
# Check API status
sudo systemctl status vkai-api

# Check logs
journalctl -u vkai-api -n 100

# Check port
lsof -i :30110
```

#### Frontend Not Loading

```bash
# Check frontend status
sudo systemctl status vkai-frontend

# Check logs
journalctl -u vkai-frontend -n 100

# Check port
lsof -i :3000
```

---

## Configuration Files Reference

| File | Description |
|------|-------------|
| `/opt/vkai-panel/.env` | Main environment configuration |
| `/opt/vkai-panel/frontend/.env.local` | Frontend configuration |
| `/etc/nginx/sites-available/vkai-panel` | Nginx configuration |
| `/etc/postgresql/16/main/postgresql.conf` | PostgreSQL configuration |
| `/etc/redis/redis.conf` | Redis configuration |
| `/etc/systemd/system/vkai-api.service` | API service configuration |
| `/etc/systemd/system/vkai-frontend.service` | Frontend service configuration |

---

## Support

- **Documentation**: https://docs.vkai.vn
- **Issues**: https://github.com/hitechcloud-vietnam/vkai-panel/issues
- **Email**: support@hitechcloud.vn
