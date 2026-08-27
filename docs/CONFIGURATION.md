# VKAI Panel Configuration Guide

## Table of Contents

1. [Standard Paths](#standard-paths)
2. [Environment Variables](#environment-variables)
3. [Database Configuration](#database-configuration)
4. [Redis Configuration](#redis-configuration)
5. [JWT Configuration](#jwt-configuration)
6. [Logging Configuration](#logging-configuration)
7. [Server Configuration](#server-configuration)
8. [Nginx Configuration](#nginx-configuration)
9. [SSL Configuration](#ssl-configuration)
10. [Backup Configuration](#backup-configuration)
11. [Notification Configuration](#notification-configuration)
12. [Security Configuration](#security-configuration)
13. [Performance Tuning](#performance-tuning)

---

## Standard Paths

Every absolute path the panel uses is derived from a single installation root,
so an operator can relocate the whole tree with one variable.

| Path | Contents | Variable |
|------|----------|----------|
| `/vkai-panel/` | Installation root | `VKAI_PANEL_ROOT` |
| `/vkai-panel/core/` | API code and binaries (`vkai-api`) | derived |
| `/vkai-panel/panel/` | Built UI (`vkai-ui`) | derived |
| `/vkai-panel/www/domains/<domain>/` | Customer website document roots | `VKAI_WEB_ROOT` |
| `/vkai-panel/www/backup/` | Website and database backups | `VKAI_BACKUP_ROOT` |
| `/vkai-panel/www/default/` | Catch-all vhost for unmatched hosts | derived |
| `/vkai-panel/logs/` | Panel logs | `VKAI_LOG_ROOT` |
| `/vkai-panel/logs/sites/<domain>/` | Per-site web server logs | derived |
| `/vkai-panel/etc/` | `.env`, `config.yaml`, `panel_access.json` | `VKAI_ETC_ROOT` |
| `/vkai-panel/ssl/` | Certificates (`ssl/panel/` for the panel itself) | `VKAI_SSL_ROOT` |
| `/vkai-panel/tmp/` | Panel-owned scratch space | `VKAI_TMP_ROOT` |

Overriding `VKAI_PANEL_ROOT` moves every derived path with it. Overriding one of
the specific variables moves only that subtree, which is how a separate backup
volume or log partition is mounted in. Legacy unprefixed names (`PANEL_ROOT`,
`WEB_ROOT`, `BACKUP_ROOT`, `LOG_ROOT`, `ETC_ROOT`, `SSL_ROOT`, `TMP_ROOT`) are
still honoured for upgrades.

`/opt/vkai-panel` and `/var/www` are the pre-whitelabel locations. They are no
longer used.

---

## Environment Variables

Every panel variable carries the **`VKAI_`** prefix. Precedence is
**defaults < `config.yaml` < environment**, so a variable always wins over the
file.

### Core API Configuration (`core/`, service `vkai-api`)

Create `/vkai-panel/etc/.env`:

```bash
# ===========================================
# Panel access gate - own port, never 80/443
# ===========================================
VKAI_PANEL_ENABLED=true
VKAI_PANEL_PORT=8888
VKAI_PANEL_BIND=0.0.0.0
# Empty on first start: an entrance is generated and printed once.
VKAI_PANEL_ENTRANCE=
VKAI_PANEL_ENTRANCE_ENABLED=true
VKAI_PANEL_SESSION_TTL=12h
VKAI_PANEL_RANDOM_PORT=false
VKAI_PANEL_ALLOWED_IPS=
VKAI_PANEL_TRUSTED_PROXIES=
VKAI_PANEL_DOMAIN=
VKAI_PANEL_TLS_CERT=
VKAI_PANEL_TLS_KEY=
VKAI_PANEL_TLS_SELF_SIGNED=false
VKAI_PANEL_CONFIG_FILE=/vkai-panel/etc/panel_access.json

# ===========================================
# Filesystem layout
# ===========================================
VKAI_PANEL_ROOT=/vkai-panel
VKAI_WEB_ROOT=/vkai-panel/www/domains
VKAI_BACKUP_ROOT=/vkai-panel/www/backup
VKAI_LOG_ROOT=/vkai-panel/logs
VKAI_ETC_ROOT=/vkai-panel/etc
VKAI_SSL_ROOT=/vkai-panel/ssl
VKAI_TMP_ROOT=/vkai-panel/tmp

# ===========================================
# Internal API - localhost only when the panel gate is on
# ===========================================
VKAI_SERVER_HOST=127.0.0.1
VKAI_SERVER_PORT=30110
VKAI_SERVER_MODE=release
VKAI_SERVER_READ_TIMEOUT=30s
VKAI_SERVER_WRITE_TIMEOUT=30s
VKAI_SERVER_IDLE_TIMEOUT=120s

# ===========================================
# Database
# ===========================================
VKAI_DB_HOST=localhost
VKAI_DB_PORT=5432
VKAI_DB_NAME=vkai_panel
VKAI_DB_USER=vkai
VKAI_DB_PASSWORD=your_secure_password
# "require" or stronger. "disable" only for a database on this same host.
VKAI_DB_SSLMODE=require
VKAI_DATABASE_MAX_OPEN=25
VKAI_DATABASE_MAX_IDLE=5

# ===========================================
# Redis
# ===========================================
VKAI_REDIS_HOST=localhost
VKAI_REDIS_PORT=6379
VKAI_REDIS_PASSWORD=
VKAI_REDIS_DB=0

# ===========================================
# Secrets - no defaults, the panel refuses to start without them
# ===========================================
VKAI_JWT_SECRET=            # openssl rand -hex 32
VKAI_SECRET_KEY=            # openssl rand -hex 32
VKAI_AGENT_TOKEN=           # openssl rand -base64 24
VKAI_JWT_ACCESS_EXPIRY=15
VKAI_JWT_REFRESH_EXPIRY=10080
VKAI_JWT_ISSUER=vkai-panel

# ===========================================
# Authorization and CORS
# ===========================================
VKAI_RBAC_ENFORCE=true
VKAI_CORS_ALLOWED_ORIGINS=https://panel.example.com:8888

# ===========================================
# Logging
# ===========================================
VKAI_LOG_LEVEL=info
VKAI_LOG_FORMAT=json
VKAI_LOG_MAX_SIZE=100
VKAI_LOG_MAX_BACKUPS=5
VKAI_LOG_MAX_AGE=30
VKAI_LOG_COMPRESS=true

# ===========================================
# Agent
# ===========================================
VKAI_AGENT_PORT=30111

# ===========================================
# UI - must point at the panel port, entrance included
# ===========================================
NEXT_PUBLIC_API_URL=https://panel.example.com:8888
```

The annotated reference copy is [`.env.example`](../.env.example) in the
repository root.

### Backward compatibility with older names

Existing installations keep booting on their old environment file. When both
forms are present the `VKAI_` name wins.

| Current name | Still accepted |
|--------------|----------------|
| `VKAI_SERVER_HOST`, `VKAI_SERVER_PORT` | `PANEL_SERVER_HOST`, `PANEL_SERVER_PORT`, `SERVER_HOST`, `SERVER_PORT` |
| `VKAI_DB_HOST`, `VKAI_DB_PORT`, `VKAI_DB_USER`, `VKAI_DB_PASSWORD`, `VKAI_DB_NAME`, `VKAI_DB_SSLMODE` | `VKAI_DATABASE_*` and the unprefixed `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE` |
| `VKAI_REDIS_HOST`, `VKAI_REDIS_PORT`, `VKAI_REDIS_PASSWORD`, `VKAI_REDIS_DB` | `REDIS_HOST`, `REDIS_PORT`, `REDIS_PASSWORD`, `REDIS_DB` |
| `VKAI_JWT_SECRET`, `VKAI_JWT_ISSUER` | `JWT_SECRET`, `JWT_ISSUER` |
| `VKAI_LOG_LEVEL` | `LOG_LEVEL` |
| `VKAI_PANEL_*` (port, bind, entrance, allowed IPs, TLS, ...) | The same names without the prefix: `PANEL_PORT`, `PANEL_BIND`, `PANEL_HOST`, `PANEL_ENTRANCE`, `PANEL_ALLOW_IPS`, `PANEL_TLS_CERT_FILE`, `PANEL_TLS_KEY_FILE`, `PANEL_SSL`, `PANEL_STATE_FILE` |
| `VKAI_PANEL_ROOT`, `VKAI_WEB_ROOT`, `VKAI_BACKUP_ROOT`, `VKAI_LOG_ROOT`, `VKAI_ETC_ROOT`, `VKAI_SSL_ROOT`, `VKAI_TMP_ROOT` | `PANEL_ROOT`, `WEB_ROOT`, `BACKUP_ROOT`, `LOG_ROOT`, `ETC_ROOT`, `SSL_ROOT`, `TMP_ROOT` |

Prefer the `VKAI_` names: the legacy forms are deprecated and will be removed in
a future major release.

### UI Configuration (`panel/`, service `vkai-ui`)

Create `/vkai-panel/panel/.env.local`:

```bash
# API URL - the panel port, plus the entrance when one is configured
NEXT_PUBLIC_API_URL=https://panel.example.com:8888/vkai_a1b2c3d4

# App Configuration
NEXT_PUBLIC_APP_NAME=VKAI Panel
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
cd /vkai-panel
make migrate DATABASE_URL=postgres://vkai:PASSWORD@localhost:5432/vkai_panel
```

### Database Backup

```bash
# Backup
pg_dump -U vkai -d vkai_panel > backup.sql

# Restore
psql -U vkai -d vkai_panel < backup.sql

# Automated backup script
#!/bin/bash
BACKUP_DIR="/vkai-panel/www/backup"
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
# Access token lifetime, in minutes (short-lived)
VKAI_JWT_ACCESS_EXPIRY=15

# Refresh token lifetime, in minutes (long-lived; 10080 = 7 days)
VKAI_JWT_REFRESH_EXPIRY=10080

# Token issuer
VKAI_JWT_ISSUER=vkai-panel
```

The same values can be pinned in `/vkai-panel/etc/config.yaml` as durations
under `jwt.access_ttl` and `jwt.refresh_ttl`. Rotating `VKAI_JWT_SECRET`
invalidates every issued access and refresh token.

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
VKAI_LOG_FORMAT=json

# Text format (for development)
VKAI_LOG_FORMAT=text
```

### Log Rotation

```bash
# Maximum log file size (MB)
VKAI_LOG_MAX_SIZE=100

# Maximum number of old log files
VKAI_LOG_MAX_BACKUPS=5

# Maximum days to retain old log files
VKAI_LOG_MAX_AGE=30

# Compress old log files
VKAI_LOG_COMPRESS=true
```

### Log Files

```
/vkai-panel/logs/                    # VKAI_LOG_ROOT
├── api.log                          # API server logs
├── api.log.1                        # Rotated log
├── api.log.2.gz                     # Compressed rotated log
└── sites/                           # Web server logs, one directory per site
    └── example.com/
        ├── access.log
        └── error.log
```

### Viewing Logs

```bash
# View API logs
journalctl -u vkai-api -f

# View specific log file
tail -f /vkai-panel/logs/api.log

# Search logs
grep "ERROR" /vkai-panel/logs/api.log

# View last 100 lines
tail -n 100 /vkai-panel/logs/api.log

# Web server logs of one customer site
tail -f /vkai-panel/logs/sites/example.com/error.log
```

---

## Server Configuration

### Server Settings

These settings describe the **internal** API listener. With the panel access
gate on (`VKAI_PANEL_ENABLED=true`, the default) the public listener is
`VKAI_PANEL_BIND:VKAI_PANEL_PORT` instead, and the internal listener should stay
on localhost.

```bash
# Internal API host - keep on localhost when the panel gate is on
VKAI_SERVER_HOST=127.0.0.1

# Internal API port
VKAI_SERVER_PORT=30110

# Gin mode: release, debug, test
VKAI_SERVER_MODE=release

# Read timeout
VKAI_SERVER_READ_TIMEOUT=30s

# Write timeout
VKAI_SERVER_WRITE_TIMEOUT=30s

# Idle timeout
VKAI_SERVER_IDLE_TIMEOUT=120s
```

### Timeouts

- **Read Timeout**: Maximum time to read request
- **Write Timeout**: Maximum time to write response
- **Idle Timeout**: Maximum time for keep-alive connections

### Connection Limits

Concurrent-connection and request-rate limits are enforced by the rate limiting
middleware and by the web server in front of the customer sites, not by a panel
environment variable. Database pool sizing is the setting you tune here:

```bash
VKAI_DATABASE_MAX_OPEN=25
VKAI_DATABASE_MAX_IDLE=5
```

---

## Nginx Configuration

### The panel is not proxied on 80/443

`vkai-api` serves the panel itself on `VKAI_PANEL_PORT` (default `8888`) behind
the security entrance. Nginx on this server is for the **customer websites**.
Never add a vhost on 80 or 443 that proxies the panel: it would put the admin
interface back on the ports every scanner probes.

### Customer website vhost

Create `/etc/nginx/sites-available/example.com`:

```nginx
server {
    listen 80;
    server_name example.com www.example.com;

    root /vkai-panel/www/domains/example.com;
    index index.php index.html;

    access_log /vkai-panel/logs/sites/example.com/access.log;
    error_log  /vkai-panel/logs/sites/example.com/error.log;

    location / {
        try_files $uri $uri/ /index.php?$query_string;
    }

    location ~ \.php$ {
        include snippets/fastcgi-php.conf;
        fastcgi_pass unix:/run/php/php8.2-fpm.sock;
    }
}
```

### Optional reverse proxy in front of the panel port

Only when you need TLS termination or a hostname for the panel. It must listen
on the panel port, and `VKAI_PANEL_TRUSTED_PROXIES` must name the proxy so the
IP allow list still sees the real client address.

```nginx
server {
    listen 8888 ssl http2;
    server_name panel.example.com;

    ssl_certificate     /vkai-panel/ssl/panel/panel.crt;
    ssl_certificate_key /vkai-panel/ssl/panel/panel.key;

    location / {
        proxy_pass http://127.0.0.1:8888;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 86400;
    }
}
```

A ready-made file ships as `deploy/nginx/vkai-panel.conf`. Set
`VKAI_PANEL_BIND=127.0.0.1` on the node when a proxy on the same host fronts it.

### Enable Site

```bash
sudo ln -s /etc/nginx/sites-available/example.com /etc/nginx/sites-enabled/
sudo mkdir -p /vkai-panel/logs/sites/example.com
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

Panel certificates and customer-site certificates are kept apart:

| | Location | Configured by |
|---|---|---|
| Panel | `/vkai-panel/ssl/panel/` | `VKAI_PANEL_TLS_CERT`, `VKAI_PANEL_TLS_KEY`, `VKAI_PANEL_TLS_SELF_SIGNED` |
| Customer site | `/vkai-panel/ssl/<domain>/` | the site's nginx/apache vhost |

A site renewal can therefore never overwrite the panel's key.

### Let's Encrypt (customer sites)

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
sudo mkdir -p /vkai-panel/ssl/your-domain.com
sudo cp your-domain.crt /vkai-panel/ssl/your-domain.com/
sudo cp your-domain.key /vkai-panel/ssl/your-domain.com/

# Set permissions
sudo chown -R vkai:vkai /vkai-panel/ssl/your-domain.com
sudo chmod 600 /vkai-panel/ssl/your-domain.com/your-domain.key
sudo chmod 644 /vkai-panel/ssl/your-domain.com/your-domain.crt
```

#### 3. Configure Nginx

```nginx
server {
    listen 443 ssl http2;
    server_name your-domain.com;

    root /vkai-panel/www/domains/your-domain.com;

    ssl_certificate /vkai-panel/ssl/your-domain.com/your-domain.crt;
    ssl_certificate_key /vkai-panel/ssl/your-domain.com/your-domain.key;

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
BACKUP_DIR=/vkai-panel/www/backup

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
# /vkai-panel/scripts/backup.sh

BACKUP_DIR="/vkai-panel/www/backup"
DATE=$(date +%Y%m%d_%H%M%S)

# Backup database
pg_dump -U vkai -d vkai_panel | gzip > "$BACKUP_DIR/db_$DATE.sql.gz"

# Backup websites
tar -czf "$BACKUP_DIR/websites_$DATE.tar.gz" /vkai-panel/www/domains

# Backup configuration
tar -czf "$BACKUP_DIR/config_$DATE.tar.gz" /vkai-panel/etc/.env /etc/nginx/sites-available

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

These settings control the panel's **Docker management feature** -- the Docker
screen and the `/api/v1/docker/*` API that let users manage their own containers,
images, volumes, networks and compose stacks. They have nothing to do with how
the panel itself runs: the panel runs bare-metal under systemd and is never
containerised.

Leave `ENABLE_DOCKER=false` on hosts where no one manages containers; the panel
runs perfectly well without Docker Engine installed.

```bash
# Enable Docker management (customer-facing feature)
ENABLE_DOCKER=true

# Docker socket the panel talks to on behalf of users
DOCKER_SOCKET=/var/run/docker.sock
```

> Access to this feature is gated by the `docker:*` RBAC permissions. The Docker
> socket is effectively root-equivalent on the host, so grant them only to roles
> that are meant to control containers.

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

#### UI Not Loading

```bash
# Check UI service status
sudo systemctl status vkai-ui

# Check logs
journalctl -u vkai-ui -n 100

# Check port
lsof -i :3000
```

---

## Configuration Files Reference

| File | Description |
|------|-------------|
| `/vkai-panel/etc/.env` | Main environment configuration (mode `0600`, owner `vkai`) |
| `/vkai-panel/etc/config.yaml` | Optional YAML configuration |
| `/vkai-panel/etc/panel_access.json` | Generated panel port and entrance (mode `0600`) |
| `/vkai-panel/panel/.env.local` | UI configuration |
| `/etc/nginx/sites-available/<domain>` | Per-site vhost for a customer website |
| `deploy/nginx/vkai-panel.conf` | Optional reverse proxy for the panel port |
| `/etc/postgresql/16/main/postgresql.conf` | PostgreSQL configuration |
| `/etc/redis/redis.conf` | Redis configuration |
| `/etc/systemd/system/vkai-api.service` | API service (`vkai-api`) |
| `/etc/systemd/system/vkai-ui.service` | UI service (`vkai-ui`) |
| `/etc/systemd/system/vkai-agent.service` | Agent service (`vkai-agent`) |

---

## Support

- **Documentation**: https://docs.vkai.vn
- **Issues**: https://github.com/hitechcloud-vietnam/vkai-panel/issues
- **Email**: support@hitechcloud.vn
