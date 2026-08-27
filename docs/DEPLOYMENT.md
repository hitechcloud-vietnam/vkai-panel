# VKAI Panel Deployment Guide

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

VKAI Panel is designed to run as systemd services without Docker. This guide covers production deployment on Ubuntu/Debian systems.

### Architecture

```
        Customer traffic                      Administrator traffic
              │                                          │
              ▼                                          ▼
┌─────────────────────────────┐        ┌─────────────────────────────┐
│  nginx / apache vhosts      │        │  vkai-api on port 8888      │
│  Port 80 / 443              │        │  + entrance /vkai_xxxxxxxx  │
│  Root: /vkai-panel/www/     │        │  (systemd: vkai-api)        │
│        domains/<domain>     │        └──────────────┬──────────────┘
└─────────────────────────────┘                       │
                                       ┌──────────────┴──────────────┐
                                       │                             │
                                       ▼                             ▼
                         ┌─────────────────────────┐   ┌─────────────────────────┐
                         │   vkai-ui (Next.js)     │   │   vkai-api internal API │
                         │   127.0.0.1:3000        │   │   127.0.0.1:30110       │
                         └─────────────────────────┘   └────────────┬────────────┘
                                                                    │
                                                  ┌─────────────────┼─────────────────┐
                                                  │                 │                 │
                                                  ▼                 ▼                 ▼
                                        ┌─────────────┐   ┌─────────────┐   ┌─────────────┐
                                        │ PostgreSQL  │   │    Redis    │   │ vkai-agent  │
                                        │  Port 5432  │   │  Port 6379  │   │ Port 30111  │
                                        └─────────────┘   └─────────────┘   └─────────────┘
```

**Ports 80 and 443 are never used by the panel.** They belong to the customer
websites hosted on this server. The panel answers only on `VKAI_PANEL_PORT`
(default `8888`) behind the security entrance - see
[PANEL_ACCESS.md](PANEL_ACCESS.md).

### Standard Paths

| Path | Contents |
|------|----------|
| `/vkai-panel/` | Installation root |
| `/vkai-panel/core/` | API source and binaries (`vkai-api`) |
| `/vkai-panel/panel/` | Built UI served by `vkai-ui` |
| `/vkai-panel/www/domains/<domain>/` | Customer website document roots |
| `/vkai-panel/www/backup/` | Website and database backups |
| `/vkai-panel/www/default/` | Default page for unmatched vhosts |
| `/vkai-panel/logs/` | Panel logs |
| `/vkai-panel/logs/sites/<domain>/` | Per-site web server logs |
| `/vkai-panel/etc/` | Panel configuration (`.env`, `config.yaml`) |
| `/vkai-panel/ssl/` | TLS certificates |
| `/vkai-panel/tmp/` | Temporary files |
| `/vkai-panel/etc/panel_access.json` | Generated panel port and entrance (mode `0600`) |

---

## System Requirements

### Minimum Requirements

- **OS**: Ubuntu 22.04/24.04 LTS, Debian 11/12, Rocky Linux 8/9, AlmaLinux 8/9, RHEL 8/9, or CentOS Stream 9
- **Architecture**: `x86_64`/`amd64` or `aarch64`/`arm64`
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
git clone https://github.com/hitechcloud-vietnam/vkai-panel.git /vkai-panel
cd /vkai-panel

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
# Create the standard layout
sudo mkdir -p /vkai-panel/{core,panel,etc,ssl,tmp}
sudo mkdir -p /vkai-panel/www/{domains,backup,default}
sudo mkdir -p /vkai-panel/logs/sites

# Set ownership and permissions
sudo chown -R vkai:vkai /vkai-panel
sudo chmod 750 /vkai-panel/etc /vkai-panel/ssl
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
cd /vkai-panel
sudo -u vkai make migrate DATABASE_URL=postgres://vkai:PASSWORD@localhost:5432/vkai_panel
```

#### 7. Setup Redis

```bash
# Start Redis
sudo systemctl enable redis-server
sudo systemctl start redis-server
```

#### 8. Build the API (core/)

```bash
cd /vkai-panel/core

# Install dependencies
sudo -u vkai go mod tidy

# Build
sudo -u vkai go build -o bin/vkai-api ./cmd/api/
```

#### 9. Build the UI (panel/)

```bash
cd /vkai-panel/panel

# Install dependencies
sudo -u vkai npm install

# Build
sudo -u vkai npm run build
```

#### 10. Configure Environment

All panel settings use the **`VKAI_`** prefix. Start from
[`.env.example`](../.env.example) in the repository, which documents every
variable.

```bash
# Create environment file
sudo -u vkai tee /vkai-panel/etc/.env << EOF
# --- Panel access gate: own port + security entrance, never 80/443 ---
VKAI_PANEL_ENABLED=true
VKAI_PANEL_PORT=8888
VKAI_PANEL_BIND=0.0.0.0
# Leave empty on the first start: one is generated and printed once.
VKAI_PANEL_ENTRANCE=
VKAI_PANEL_ENTRANCE_ENABLED=true
VKAI_PANEL_SESSION_TTL=12h
VKAI_PANEL_ALLOWED_IPS=
VKAI_PANEL_TRUSTED_PROXIES=
VKAI_PANEL_DOMAIN=
VKAI_PANEL_CONFIG_FILE=/vkai-panel/etc/panel_access.json

# --- Internal API (localhost only when the panel gate is on) ---
VKAI_SERVER_HOST=127.0.0.1
VKAI_SERVER_PORT=30110
VKAI_SERVER_MODE=release

# --- PostgreSQL ---
VKAI_DB_HOST=localhost
VKAI_DB_PORT=5432
VKAI_DB_NAME=vkai_panel
VKAI_DB_USER=vkai
VKAI_DB_PASSWORD=your_secure_password
VKAI_DB_SSLMODE=require

# --- Redis ---
VKAI_REDIS_HOST=localhost
VKAI_REDIS_PORT=6379
VKAI_REDIS_PASSWORD=
VKAI_REDIS_DB=0

# --- Secrets (no defaults: the panel refuses to start without them) ---
VKAI_JWT_SECRET=$(openssl rand -hex 32)
VKAI_SECRET_KEY=$(openssl rand -hex 32)
VKAI_AGENT_TOKEN=$(openssl rand -base64 24)
VKAI_JWT_ACCESS_EXPIRY=15
VKAI_JWT_REFRESH_EXPIRY=10080

# --- Logging ---
VKAI_LOG_LEVEL=info
VKAI_LOG_MAX_SIZE=100
VKAI_LOG_MAX_BACKUPS=5
VKAI_LOG_MAX_AGE=30
VKAI_LOG_COMPRESS=true

# --- UI: must point at the panel port, entrance included ---
NEXT_PUBLIC_API_URL=https://panel.example.com:8888
EOF

# Set permissions
sudo chmod 600 /vkai-panel/etc/.env
sudo chown vkai:vkai /vkai-panel/etc/.env
```

**Legacy names.** For backward compatibility the panel still accepts the older
unprefixed panel-access names (`PANEL_PORT`, `PANEL_BIND`, `PANEL_ENTRANCE`,
`PANEL_ALLOWED_IPS`, `PANEL_TLS_CERT`, ...) and the older database names
(`VKAI_DATABASE_HOST`, `VKAI_DATABASE_PASSWORD`, `VKAI_DATABASE_DBNAME`, ...).
The `VKAI_` form always wins when both are set. Use the `VKAI_` names in new
installations; the legacy names are deprecated.

#### 11. Install Systemd Services

```bash
# Copy service files
sudo cp /vkai-panel/deploy/systemd/vkai-api.service /etc/systemd/system/
sudo cp /vkai-panel/deploy/systemd/vkai-ui.service /etc/systemd/system/

# Reload systemd
sudo systemctl daemon-reload

# Enable services
sudo systemctl enable vkai-api
sudo systemctl enable vkai-ui

# Start services
sudo systemctl start vkai-api
sudo systemctl start vkai-ui
```

#### 12. Configure Nginx for customer websites

Nginx on this server exists for the **customer websites**, not for the panel.
The panel is served by `vkai-api` itself on `VKAI_PANEL_PORT`; do not proxy it
from port 80 or 443.

```bash
# Default vhost: answer 444 for any hostname that has no site yet
sudo tee /etc/nginx/sites-available/vkai-default << 'EOF'
server {
    listen 80 default_server;
    listen [::]:80 default_server;
    server_name _;
    root /vkai-panel/www/default;
    index index.html;

    access_log /vkai-panel/logs/sites/default/access.log;
    error_log  /vkai-panel/logs/sites/default/error.log;

    location / {
        try_files $uri $uri/ =404;
    }
}
EOF

sudo mkdir -p /vkai-panel/logs/sites/default
sudo ln -sf /etc/nginx/sites-available/vkai-default /etc/nginx/sites-enabled/
sudo rm -f /etc/nginx/sites-enabled/default

sudo nginx -t
sudo systemctl reload nginx
```

Per-site vhosts created through the panel follow the same layout:

- document root `/vkai-panel/www/domains/<domain>`
- logs `/vkai-panel/logs/sites/<domain>/{access,error}.log`
- certificates under `/vkai-panel/ssl/`

#### 13. Open the panel port

```bash
# Ubuntu / Debian
sudo ufw allow 8888/tcp

# RHEL / Rocky / AlmaLinux
sudo firewall-cmd --permanent --add-port=8888/tcp && sudo firewall-cmd --reload
```

Then read the access banner printed once at first start:

```bash
vkai panel info
sudo journalctl -u vkai-api | grep -A20 "THONG TIN TRUY CAP"
```

The URL looks like `https://203.0.113.10:8888/vkai_91ac5b65/`. Anything that
reaches the panel port with the wrong host, source IP or entrance path gets a
neutral 404.

#### 14. Optional: reverse proxy in front of the panel port

Only if you need TLS termination or a hostname in front of the panel. The proxy
must listen on the **panel port**, never on 80/443, and you must set
`VKAI_PANEL_TRUSTED_PROXIES` to the proxy address so `X-Forwarded-For` is
trusted. Full instructions: [PANEL_ACCESS.md](PANEL_ACCESS.md).

---

## Configuration

### Environment Variables

Edit `/vkai-panel/etc/.env`. Every panel variable uses the **`VKAI_`** prefix.

| Variable | Description | Default |
|----------|-------------|---------|
| `VKAI_PANEL_PORT` | Panel port. 80, 443, 22, 25, 3306, 5432 and 6379 are rejected | `8888` |
| `VKAI_PANEL_BIND` | Interface the panel binds | `0.0.0.0` |
| `VKAI_PANEL_ENTRANCE` | Secret entrance path, e.g. `/vkai_a1b2c3d4`. Empty = generated on first start | (generated) |
| `VKAI_PANEL_ENTRANCE_ENABLED` | Enable the security entrance | `true` |
| `VKAI_PANEL_SESSION_TTL` | Entrance cookie lifetime | `12h` |
| `VKAI_PANEL_ALLOWED_IPS` | Comma separated IPs/CIDRs allowed to reach the panel. Empty = everyone | (empty) |
| `VKAI_PANEL_TRUSTED_PROXIES` | Addresses whose `X-Forwarded-For` is trusted | (empty) |
| `VKAI_PANEL_DOMAIN` | Pin the panel to one host name | (empty) |
| `VKAI_PANEL_TLS_CERT` / `VKAI_PANEL_TLS_KEY` | Panel TLS certificate and key | (empty) |
| `VKAI_PANEL_TLS_SELF_SIGNED` | Generate a self-signed panel certificate | `false` |
| `VKAI_PANEL_CONFIG_FILE` | Where the generated port/entrance are stored | `/vkai-panel/etc/panel_access.json` |
| `VKAI_SERVER_HOST` / `VKAI_SERVER_PORT` | Internal API bind address | `127.0.0.1` / `30110` |
| `VKAI_SERVER_MODE` | Gin mode: `release`, `debug`, `test` | `release` |
| `VKAI_DB_HOST` / `VKAI_DB_PORT` | PostgreSQL | `localhost` / `5432` |
| `VKAI_DB_USER` / `VKAI_DB_NAME` | Database user and name | `vkai` / `vkai_panel` |
| `VKAI_DB_PASSWORD` | Database password | **required, no default** |
| `VKAI_DB_SSLMODE` | `require` or stronger; `disable` only for a localhost database | `require` |
| `VKAI_REDIS_HOST` / `VKAI_REDIS_PORT` / `VKAI_REDIS_DB` | Redis | `localhost` / `6379` / `0` |
| `VKAI_JWT_SECRET` | JWT signing key, at least 32 random characters | **required, no default** |
| `VKAI_JWT_ACCESS_EXPIRY` / `VKAI_JWT_REFRESH_EXPIRY` | Token lifetimes in minutes | `15` / `10080` |
| `VKAI_SECRET_KEY` | 32-byte hex/base64 key encrypting stored secrets | **required** for managed DB users |
| `VKAI_CORS_ALLOWED_ORIGINS` | Browser origins allowed to call the API. No wildcard | (empty) |
| `VKAI_RBAC_ENFORCE` | Enforce permission checks | `true` |
| `VKAI_FILEMANAGER_ROOT` | Jail directory for the file manager | the web root, `/vkai-panel/www/domains` |
| `VKAI_BACKUP_ROOT` | Root every backup destination must resolve inside | `/vkai-panel/www/backup` |
| `VKAI_CRON_USER` | Account panel-managed cron jobs run as | `www-data` |
| `VKAI_AGENT_PORT` / `VKAI_AGENT_TOKEN` | Agent port and shared secret | `30111` / **required** |
| `VKAI_LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `info` |
| `VKAI_PANEL_ROOT` | Installation root; every other path is derived from it | `/vkai-panel` |
| `VKAI_WEB_ROOT` | Customer website document roots | `/vkai-panel/www/domains` |
| `VKAI_LOG_ROOT` | Panel logs (site logs go to `<log root>/sites/<domain>`) | `/vkai-panel/logs` |
| `VKAI_ETC_ROOT` | Configuration directory | `/vkai-panel/etc` |
| `VKAI_SSL_ROOT` | Certificates (`<ssl root>/panel` for the panel's own) | `/vkai-panel/ssl` |
| `VKAI_TMP_ROOT` | Panel-owned scratch space | `/vkai-panel/tmp` |
| `NEXT_PUBLIC_API_URL` | URL the UI calls; must include the panel port and entrance | (required) |

The complete annotated list is [`.env.example`](../.env.example).

#### Legacy variable names

Older names are still accepted so existing installations keep working, but they
are deprecated and the `VKAI_` form always takes precedence:

| Current | Still accepted |
|---------|----------------|
| `VKAI_PANEL_PORT`, `VKAI_PANEL_BIND`, `VKAI_PANEL_ENTRANCE`, `VKAI_PANEL_ALLOWED_IPS`, ... | The same names without the `VKAI_` prefix (`PANEL_PORT`, `PANEL_BIND`, `PANEL_HOST`, `PANEL_ENTRANCE`, `PANEL_ALLOW_IPS`, `PANEL_TLS_CERT_FILE`, `PANEL_TLS_KEY_FILE`, `PANEL_SSL`) |
| `VKAI_DB_HOST`, `VKAI_DB_PORT`, `VKAI_DB_USER`, `VKAI_DB_PASSWORD`, `VKAI_DB_NAME`, `VKAI_DB_SSLMODE` | `VKAI_DATABASE_HOST`, `VKAI_DATABASE_PORT`, `VKAI_DATABASE_USER`, `VKAI_DATABASE_PASSWORD`, `VKAI_DATABASE_DBNAME`, `VKAI_DATABASE_SSLMODE` |

Unprefixed application names such as `SERVER_PORT`, `DB_PASSWORD` or
`JWT_SECRET` are **not** read. Rename them to the `VKAI_` form when upgrading
from an old installation.

### UI Configuration

Edit `/vkai-panel/panel/.env.local`:

```bash
# Point at the panel port, entrance included when one is configured
NEXT_PUBLIC_API_URL=https://panel.example.com:8888/vkai_a1b2c3d4
NEXT_PUBLIC_APP_NAME=VKAI Panel
NEXT_PUBLIC_APP_VERSION=1.0.0
```

---

## SSL Setup

Certificates come in two flavours and they are kept apart:

- **Customer website certificates** - issued per domain by certbot/Let's Encrypt
  and used by the nginx/apache vhosts on 80/443.
- **Panel certificate** - used only by `vkai-api` on the panel port, configured
  with `VKAI_PANEL_TLS_CERT` / `VKAI_PANEL_TLS_KEY`, or generated on first start
  when `VKAI_PANEL_TLS_SELF_SIGNED=true`.

### Let's Encrypt (customer websites)

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
sudo cp your-domain.crt /vkai-panel/ssl/
sudo cp your-domain.key /vkai-panel/ssl/

# Set permissions
sudo chown vkai:vkai /vkai-panel/ssl/your-domain.*
sudo chmod 600 /vkai-panel/ssl/your-domain.key
sudo chmod 644 /vkai-panel/ssl/your-domain.crt

# Update the customer website vhost
sudo nano /etc/nginx/sites-available/your-domain.com
```

Add SSL configuration:

```nginx
server {
    listen 443 ssl http2;
    server_name your-domain.com;

    root /vkai-panel/www/domains/your-domain.com;

    access_log /vkai-panel/logs/sites/your-domain.com/access.log;
    error_log  /vkai-panel/logs/sites/your-domain.com/error.log;

    ssl_certificate /vkai-panel/ssl/your-domain.crt;
    ssl_certificate_key /vkai-panel/ssl/your-domain.key;

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

To use a host name for the **panel** instead, point a record at the server and
set `VKAI_PANEL_DOMAIN=panel.example.com`. The panel still answers on its own
port: `https://panel.example.com:8888/vkai_a1b2c3d4/`.

### Nginx Configuration (customer website)

Create `/etc/nginx/sites-available/your-domain.com`:

```nginx
server {
    listen 80;
    server_name your-domain.com www.your-domain.com;

    root /vkai-panel/www/domains/your-domain.com;
    index index.php index.html;

    access_log /vkai-panel/logs/sites/your-domain.com/access.log;
    error_log  /vkai-panel/logs/sites/your-domain.com/error.log;

    # ... rest of the site configuration
}
```

---

## Backup Strategy

### Automated Backups

Create backup script `/vkai-panel/scripts/backup.sh`:

```bash
#!/bin/bash
BACKUP_DIR="/vkai-panel/www/backup"
DATE=$(date +%Y%m%d_%H%M%S)

# Backup database
pg_dump -U vkai -d vkai_panel | gzip > "$BACKUP_DIR/db_$DATE.sql.gz"

# Backup websites
tar -czf "$BACKUP_DIR/websites_$DATE.tar.gz" /vkai-panel/www/domains

# Backup configuration
tar -czf "$BACKUP_DIR/config_$DATE.tar.gz" /vkai-panel/etc /etc/nginx/sites-available

# Cleanup old backups (keep 30 days)
find $BACKUP_DIR -name "*.gz" -mtime +30 -delete
```

Add to crontab:

```bash
# Daily backup at 2 AM
0 2 * * * /vkai-panel/scripts/backup.sh

# Weekly full backup
0 3 * * 0 tar -czf /vkai-panel/www/backup/full_$(date +\%Y\%m\%d).tar.gz --exclude=/vkai-panel/www/backup /vkai-panel
```

### Remote Backups

#### S3 Backup

```bash
# Install AWS CLI
sudo apt install awscli

# Configure AWS
aws configure

# Backup to S3
aws s3 sync /vkai-panel/www/backup s3://your-bucket/backups/
```

#### SFTP Backup

```bash
# Create backup script
#!/bin/bash
sftp backup-user@backup-server << EOF
cd /backups/vkai
put /vkai-panel/www/backup/*.gz
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
systemctl status vkai-ui
systemctl status nginx
systemctl status postgresql
systemctl status redis-server

# View service logs
journalctl -u vkai-api -f
journalctl -u vkai-ui -f
```

### Log Monitoring

```bash
# View panel logs
tail -f /vkai-panel/logs/api.log

# Search for errors
grep -i error /vkai-panel/logs/api.log

# View the web server logs of one customer site
tail -f /vkai-panel/logs/sites/your-domain.com/access.log
tail -f /vkai-panel/logs/sites/your-domain.com/error.log
```

### Health Checks

Create health check script `/vkai-panel/scripts/health-check.sh`:

```bash
#!/bin/bash

# Check API
if ! curl -s http://127.0.0.1:30110/health > /dev/null; then
    echo "API is down"
    systemctl restart vkai-api
fi

# Check the UI
if ! curl -s http://127.0.0.1:3000 > /dev/null; then
    echo "UI is down"
    systemctl restart vkai-ui
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
*/5 * * * * /vkai-panel/scripts/health-check.sh
```

---

## Security Hardening

### Firewall Configuration

```bash
# Install UFW
sudo apt install ufw

# Allow SSH
sudo ufw allow 22/tcp

# Allow HTTP/HTTPS - customer websites only
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp

# Allow the panel port. Restrict it to the addresses you administer from.
sudo ufw allow from 203.0.113.0/24 to any port 8888 proto tcp

# Enable firewall
sudo ufw enable

# Check status
sudo ufw status
```

Do **not** expose 30110 (internal API) or 3000 (UI) to the Internet: both bind
to localhost and are reached through the panel port. Open the panel port
**before** restarting `vkai-api` after changing `VKAI_PANEL_PORT`, otherwise you
lock yourself out.

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

In `/vkai-panel/etc/.env`:

```bash
# PostgreSQL connection pool
VKAI_DATABASE_MAX_OPEN=25
VKAI_DATABASE_MAX_IDLE=5
```

The same values can be set in `/vkai-panel/etc/config.yaml` under
`database.max_open` and `database.max_idle`; the environment variable wins.

---

## High Availability

### Load Balancing

Several panel nodes can sit behind one load balancer. The balancer listens on
the **panel port**, not on 80/443, and every node must trust it via
`VKAI_PANEL_TRUSTED_PROXIES` so the IP allow list keeps working.

```nginx
upstream vkai_panel_nodes {
    server 10.0.0.1:8888;
    server 10.0.0.2:8888;
    server 10.0.0.3:8888;
}

server {
    listen 8888 ssl http2;
    server_name panel.example.com;

    ssl_certificate     /vkai-panel/ssl/panel.crt;
    ssl_certificate_key /vkai-panel/ssl/panel.key;

    location / {
        proxy_pass http://vkai_panel_nodes;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

On each node:

```bash
VKAI_PANEL_BIND=10.0.0.1
VKAI_PANEL_TRUSTED_PROXIES=10.0.0.10
VKAI_PANEL_DOMAIN=panel.example.com
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
sudo cat /vkai-panel/etc/.env

# Check port availability
sudo lsof -i :8888
sudo lsof -i :30110

# Show the current panel port and entrance
vkai panel info
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

# Update VKAI Panel
vkai update
# or, from a source checkout:
#   cd /vkai-panel && git pull origin main
#   cd core  && sudo -u vkai go build -o bin/vkai-api ./cmd/api/
#   cd ../panel && sudo -u vkai npm ci && sudo -u vkai npm run build
sudo systemctl restart vkai-api vkai-ui

# Clean up old logs
find /vkai-panel/logs -name "*.log" -mtime +30 -delete

# Vacuum PostgreSQL
sudo -u postgres vacuumdb --all --analyze
```

### Backup Verification

```bash
# Test backup restoration
pg_dump -U vkai -d vkai_panel | psql -U vkai -d vkai_panel_test

# Verify backup integrity
gunzip -t /vkai-panel/www/backup/db_*.sql.gz
```

---

## Support

- **Documentation**: https://docs.vkai.vn
- **Issues**: https://github.com/hitechcloud-vietnam/vkai-panel/issues
- **Email**: support@hitechcloud.vn
