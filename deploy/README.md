# vKAI Panel Deployment

## Quick Start

### 1. Installation

```bash
# Clone the repository
git clone https://github.com/hitechcloud-vietnam/vkai-panel.git /opt/vkai-panel
cd /opt/vkai-panel

# Run installation script
chmod +x deploy/install.sh
sudo ./deploy/install.sh
```

### 2. Access

- **Panel URL**: http://your-server-ip
- **Default Credentials**:
  - Username: `admin`
  - Password: `admin123`

⚠️ **Change the default password immediately after first login!**

## Architecture

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

## Services

| Service | Description | Port | Systemd Service |
|---------|-------------|------|-----------------|
| API Server | Backend API (Go) | 30110 | `vkai-api` |
| Frontend | Web UI (Next.js) | 3000 | `vkai-frontend` |
| Agent | System agent | 30111 | `vkai-agent` |
| PostgreSQL | Database | 5432 | `postgresql` |
| Redis | Cache | 6379 | `redis-server` |
| Nginx | Reverse proxy | 80/443 | `nginx` |

## Management Commands

Install the management script:

```bash
sudo cp /opt/vkai-panel/deploy/vkai.sh /usr/local/bin/vkai
sudo chmod +x /usr/local/bin/vkai
```

### Service Management

```bash
# Start all services
vkai start

# Stop all services
vkai stop

# Restart all services
vkai restart

# View service status
vkai status

# View logs
vkai logs api        # API server logs
vkai logs frontend   # Frontend logs
vkai logs agent      # Agent logs
```

### Database Management

```bash
# Backup database
vkai db backup

# Restore database
vkai db restore /var/backups/vkai/db_20240101_120000.sql.gz

# Open PostgreSQL console
vkai db console
```

### SSL Management

```bash
# Issue SSL certificate
vkai ssl issue example.com

# Renew all certificates
vkai ssl renew

# List certificates
vkai ssl list
```

### Update & Uninstall

```bash
# Update vKAI Panel
vkai update

# Uninstall vKAI Panel
vkai uninstall
```

## Configuration

### Environment Variables

Configuration file: `/opt/vkai-panel/.env`

```bash
# Server
SERVER_HOST=0.0.0.0
SERVER_PORT=30110

# Database
DB_HOST=localhost
DB_PORT=5432
DB_NAME=vkai_panel
DB_USER=vkai
DB_PASSWORD=<generated>

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379

# JWT
JWT_SECRET=<generated>
JWT_ACCESS_TOKEN_TTL=15m
JWT_REFRESH_TOKEN_TTL=168h

# Logging
LOG_LEVEL=info
```

### Nginx Configuration

Nginx config: `/etc/nginx/sites-available/vkai-panel`

To add SSL:

```bash
sudo certbot --nginx -d your-domain.com
```

## File Locations

| Path | Description |
|------|-------------|
| `/opt/vkai-panel/` | Main installation directory |
| `/opt/vkai-panel/.env` | Environment configuration |
| `/var/log/vkai/` | Log files |
| `/var/backups/vkai/` | Backup files |
| `/var/www/` | Website files |
| `/etc/ssl/vkai/` | SSL certificates |
| `/etc/nginx/sites-available/` | Nginx site configs |

## Troubleshooting

### Service won't start

```bash
# Check service status
systemctl status vkai-api

# View detailed logs
journalctl -u vkai-api -n 100

# Check configuration
cat /opt/vkai-panel/.env
```

### Database connection issues

```bash
# Check PostgreSQL status
systemctl status postgresql

# Test connection
psql -h localhost -U vkai -d vkai_panel

# Reset password
sudo -u postgres psql -c "ALTER USER vkai PASSWORD 'new_password';"
```

### Port already in use

```bash
# Find process using port
lsof -i :30110

# Kill process
kill -9 <PID>
```

## Security Recommendations

1. **Change default password** immediately after installation
2. **Enable firewall** and only open necessary ports
3. **Use SSL** for all connections
4. **Regular backups** of database and files
5. **Keep system updated** with security patches
6. **Monitor logs** for suspicious activity

## Backup Strategy

### Automated Backups

Add to crontab:

```bash
# Daily database backup at 2 AM
0 2 * * * /usr/local/bin/vkai db backup

# Weekly full backup
0 3 * * 0 tar -czf /var/backups/vkai/full_$(date +\%Y\%m\%d).tar.gz /opt/vkai-panel /var/www
```

### Manual Backup

```bash
# Backup database
vkai db backup

# Backup everything
tar -czf /var/backups/vkai/manual_$(date +%Y%m%d_%H%M%S).tar.gz \
    /opt/vkai-panel \
    /var/www \
    /etc/nginx/sites-available
```

## Support

- **Documentation**: https://docs.vkai.vn
- **Issues**: https://github.com/hitechcloud-vietnam/vkai-panel/issues
- **Email**: support@hitechcloud.vn
