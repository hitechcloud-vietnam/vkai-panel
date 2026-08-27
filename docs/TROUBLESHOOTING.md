# VKAI Panel Troubleshooting Guide

## Table of Contents

1. [Panel Access Issues](#panel-access-issues)
2. [Common Issues](#common-issues)
3. [Installation Issues](#installation-issues)
4. [Database Issues](#database-issues)
5. [Authentication Issues](#authentication-issues)
6. [Service Issues](#service-issues)
7. [Performance Issues](#performance-issues)
8. [Network Issues](#network-issues)
9. [SSL Issues](#ssl-issues)
10. [Panel UI Issues](#panel-ui-issues)
11. [Agent Issues](#agent-issues)
12. [Logs and Debugging](#logs-and-debugging)
13. [FAQ](#faq)

---

## Panel Access Issues

The panel listens on its own port (`VKAI_PANEL_PORT`, default `8888`) behind a
security entrance. Ports 80 and 443 belong to the customer websites and never
answer for the panel. Every blocked request receives the same neutral 404, so
"nothing works" usually means one of four things.

### I do not know the URL, the port or the entrance

```bash
vkai panel info
sudo cat /vkai-panel/etc/panel_access.json
sudo journalctl -u vkai-api | grep -A20 "THONG TIN TRUY CAP"
```

Generate a new entrance if the old one leaked:

```bash
vkai panel entrance random
sudo systemctl restart vkai-api
```

### Everything returns 404

That is the design. Work through the three checks in order:

```bash
# 1. Is the panel listening on the port you are dialling?
sudo ss -tlnp | grep vkai-api

# 2. Is it reachable locally? /health always answers, entrance or not.
curl -sI http://127.0.0.1:8888/health

# 3. Why was the request refused?
sudo journalctl -u vkai-api | grep "panel access denied"
```

The log names the reason: host does not match `VKAI_PANEL_DOMAIN`, source IP is
outside `VKAI_PANEL_ALLOWED_IPS`, or the entrance path is wrong.

### The port is open locally but not from outside

The firewall is the usual culprit. Open the panel port, never 80/443 for the
panel:

```bash
sudo ufw allow 8888/tcp                                                   # Ubuntu / Debian
sudo firewall-cmd --permanent --add-port=8888/tcp && sudo firewall-cmd --reload   # RHEL family
```

Always open the new port **before** restarting `vkai-api` after changing
`VKAI_PANEL_PORT`.

### I locked myself out with the IP allow list

Get in over SSH or the provider console, then:

```bash
sudo vkai panel allow-ip --clear
sudo systemctl restart vkai-api
```

### The panel refuses to take the port I asked for

Ports 80, 443, 22, 25, 3306, 5432 and 6379 are rejected on purpose: they belong
to the customer websites and to the panel's own supporting services. Pick
another port, or `sudo vkai port random`.

---

## Common Issues

### Service Won't Start

**Symptoms:**
- Service fails to start
- Error messages in logs

**Solutions:**

```bash
# Check service status
systemctl status vkai-api

# View detailed logs
journalctl -u vkai-api -n 100

# Check configuration
sudo cat /vkai-panel/etc/.env

# Check port availability (panel port first, then the internal API)
sudo lsof -i :8888
sudo lsof -i :30110

# Show the current panel port and entrance
vkai panel info

# Check permissions
ls -la /vkai-panel/core/bin/
```

### Database Connection Failed

**Symptoms:**
- Cannot connect to database
- Connection timeout errors

**Solutions:**

```bash
# Check PostgreSQL status
systemctl status postgresql

# Test connection
psql -h localhost -U vkai -d vkai_panel

# Check PostgreSQL logs
tail -f /var/log/postgresql/postgresql-*.log

# Verify credentials
sudo grep '^VKAI_DB_' /vkai-panel/etc/.env

# Reset password if needed
sudo -u postgres psql -c "ALTER USER vkai PASSWORD 'new_password';"
```

### Redis Connection Failed

**Symptoms:**
- Cannot connect to Redis
- Session issues

**Solutions:**

```bash
# Check Redis status
systemctl status redis-server

# Test connection
redis-cli ping

# Check Redis logs
tail -f /var/log/redis/redis-server.log

# Restart Redis
sudo systemctl restart redis-server
```

---

## Installation Issues

### Go Installation Failed

**Symptoms:**
- `go: command not found`
- Version mismatch

**Solutions:**

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

### Node.js Installation Failed

**Symptoms:**
- `node: command not found`
- npm errors

**Solutions:**

```bash
# Add NodeSource repository
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -

# Install Node.js
sudo apt install -y nodejs

# Verify
node --version
npm --version
```

### PostgreSQL Installation Failed

**Symptoms:**
- Cannot install PostgreSQL
- Service won't start

**Solutions:**

```bash
# Install PostgreSQL
sudo apt install -y postgresql postgresql-contrib

# Start service
sudo systemctl start postgresql
sudo systemctl enable postgresql

# Check status
sudo systemctl status postgresql

# View logs
sudo journalctl -u postgresql
```

---

## Database Issues

### Migration Failed

**Symptoms:**
- Migration errors
- Schema mismatch

**Solutions:**

```bash
# Migrations are plain SQL files applied in order from core/migrations/.
# List them
ls core/migrations/

# Apply every migration
make migrate DATABASE_URL=postgres://vkai:PASSWORD@localhost:5432/vkai_panel

# Apply one file by hand
psql "postgres://vkai:PASSWORD@localhost:5432/vkai_panel" -v ON_ERROR_STOP=1 \
  -f core/migrations/001_initial_schema.sql

# Reset database (WARNING: data loss)
sudo -u postgres psql -c "DROP DATABASE vkai_panel;"
sudo -u postgres psql -c "CREATE DATABASE vkai_panel OWNER vkai;"
make migrate DATABASE_URL=postgres://vkai:PASSWORD@localhost:5432/vkai_panel
```

### Connection Pool Exhausted

**Symptoms:**
- Too many connections
- Connection timeout

**Solutions:**

```bash
# Check connections
sudo -u postgres psql -c "SELECT count(*) FROM pg_stat_activity;"

# Increase max connections
sudo -u postgres psql -c "ALTER SYSTEM SET max_connections = 200;"

# Restart PostgreSQL
sudo systemctl restart postgresql

# Configure connection pool in .env
DB_MAX_CONNECTIONS=25
DB_MAX_IDLE_CONNECTIONS=5
```

### Slow Queries

**Symptoms:**
- Slow response times
- High CPU usage

**Solutions:**

```bash
# Enable slow query logging
sudo -u postgres psql -c "ALTER SYSTEM SET log_min_duration_statement = 1000;"

# Restart PostgreSQL
sudo systemctl restart postgresql

# Analyze queries
sudo -u postgres psql -c "SELECT * FROM pg_stat_statements ORDER BY total_time DESC LIMIT 10;"

# Add indexes
sudo -u postgres psql -d vkai_panel -c "CREATE INDEX idx_websites_domain ON websites(domain);"
```

---

## Authentication Issues

### JWT Token Invalid

**Symptoms:**
- 401 Unauthorized errors
- Token expired messages

**Solutions:**

```bash
# Check JWT secret
sudo grep '^VKAI_JWT_SECRET' /vkai-panel/etc/.env

# Verify token
curl -H "Authorization: Bearer YOUR_TOKEN" http://localhost:30110/api/v1/auth/verify

# Clear Redis cache
redis-cli FLUSHDB

# Restart API server
sudo systemctl restart vkai-api
```

### Login Failed

**Symptoms:**
- Cannot login
- Invalid credentials

**Solutions:**

```bash
# Check user exists
sudo -u postgres psql -d vkai_panel -c "SELECT * FROM users WHERE username = 'admin';"

# Reset password
sudo -u postgres psql -d vkai_panel -c "UPDATE users SET password_hash = 'NEW_HASH' WHERE username = 'admin';"

# Check account status
sudo -u postgres psql -d vkai_panel -c "SELECT status FROM users WHERE username = 'admin';"
```

### Session Expired

**Symptoms:**
- Frequent logouts
- Token refresh failed

**Solutions:**

```bash
# Check token lifetime (minutes)
sudo grep '^VKAI_JWT_ACCESS_EXPIRY' /vkai-panel/etc/.env

# Increase it
VKAI_JWT_ACCESS_EXPIRY=30

# Restart API server
sudo systemctl restart vkai-api
```

---

## Service Issues

### Systemd Service Failed

**Symptoms:**
- Service won't start
- Service crashes

**Solutions:**

```bash
# Check service status
systemctl status vkai-api

# View detailed logs
journalctl -u vkai-api -n 100

# Check service file
cat /etc/systemd/system/vkai-api.service

# Reload systemd
sudo systemctl daemon-reload

# Restart service
sudo systemctl restart vkai-api
```

### Port Already in Use

**Symptoms:**
- Port conflict errors
- Cannot bind to port

**Solutions:**

```bash
# Find process using port
lsof -i :30110

# Kill process
kill -9 PID

# Or change the internal API port in /vkai-panel/etc/.env
VKAI_SERVER_PORT=30111

# To change the PANEL port, use the CLI and open the firewall first
vkai panel port 9001

# Restart service
sudo systemctl restart vkai-api
```

### Memory Issues

**Symptoms:**
- Out of memory errors
- Service crashes

**Solutions:**

```bash
# Check memory usage
free -h

# Check process memory
ps aux | grep vkai-api

# Increase memory limits in service file
# /etc/systemd/system/vkai-api.service
[Service]
MemoryLimit=2G

# Reload and restart
sudo systemctl daemon-reload
sudo systemctl restart vkai-api
```

---

## Performance Issues

### High CPU Usage

**Symptoms:**
- Slow response times
- High CPU usage

**Solutions:**

```bash
# Check CPU usage
top

# Find CPU-intensive processes
ps aux --sort=-%cpu | head 10

# Check for infinite loops
# Review code for performance issues

# Optimize database queries
# Add indexes
# Use connection pooling
```

### High Memory Usage

**Symptoms:**
- Out of memory errors
- Slow performance

**Solutions:**

```bash
# Check memory usage
free -h

# Find memory-intensive processes
ps aux --sort=-%mem | head 10

# Check for memory leaks
# Review code for memory leaks

# Increase swap space
sudo fallocate -l 4G /swapfile
sudo chmod 600 /swapfile
sudo mkswap /swapfile
sudo swapon /swapfile
```

### Slow Database Queries

**Symptoms:**
- Slow API responses
- Database timeouts

**Solutions:**

```bash
# Enable slow query logging
sudo -u postgres psql -c "ALTER SYSTEM SET log_min_duration_statement = 1000;"

# Restart PostgreSQL
sudo systemctl restart postgresql

# Analyze queries
sudo -u postgres psql -d vkai_panel -c "EXPLAIN ANALYZE SELECT * FROM websites WHERE domain = 'example.com';"

# Add indexes
sudo -u postgres psql -d vkai_panel -c "CREATE INDEX idx_websites_domain ON websites(domain);"
```

---

## Network Issues

### Cannot Access Panel

**Symptoms:**
- Cannot connect to panel
- Connection refused

**Solutions:**

```bash
# Check Nginx status
systemctl status nginx

# Check Nginx configuration
nginx -t

# Check firewall
sudo ufw status

# Allow ports
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp

# Check Nginx logs
tail -f /var/log/nginx/error.log
```

### DNS Issues

**Symptoms:**
- Domain not resolving
- DNS errors

**Solutions:**

```bash
# Check DNS resolution
nslookup your-domain.com

# Check DNS records
dig your-domain.com

# Verify DNS configuration
cat /etc/resolv.conf

# Test with different DNS
nslookup your-domain.com 8.8.8.8
```

### Firewall Blocking Connections

**Symptoms:**
- Connection timeout
- Port blocked

**Solutions:**

```bash
# Check firewall status
sudo ufw status

# Allow specific ports
sudo ufw allow 30110/tcp
sudo ufw allow 3000/tcp

# Check iptables rules
sudo iptables -L -n

# Reset firewall
sudo ufw reset
sudo ufw enable
```

---

## SSL Issues

### Certificate Not Working

**Symptoms:**
- SSL errors
- Certificate warnings

**Solutions:**

```bash
# Check certificate
openssl s_client -connect your-domain.com:443

# Verify certificate files
ls -la /vkai-panel/ssl/

# Check Nginx SSL configuration
cat /etc/nginx/sites-available/vkai-panel

# Renew certificate
sudo certbot renew

# Test SSL
curl -I https://your-domain.com
```

### Let's Encrypt Failed

**Symptoms:**
- Cannot obtain certificate
- Validation failed

**Solutions:**

```bash
# Check Certbot logs
sudo journalctl -u certbot

# Verify domain DNS
nslookup your-domain.com

# Check webroot
ls -la /vkai-panel/www/default

# Manual verification
sudo certbot certonly --webroot -w /vkai-panel/www/default -d your-domain.com

# Check firewall
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
```

### Certificate Expired

**Symptoms:**
- Certificate expired warnings
- SSL errors

**Solutions:**

```bash
# Check certificate expiration
openssl x509 -enddate -noout -in /vkai-panel/ssl/your-domain.com/your-domain.crt

# Renew certificate
sudo certbot renew

# Setup auto-renewal
echo "0 0 1 * * certbot renew" | sudo crontab -

# Restart Nginx
sudo systemctl restart nginx
```

---

## Panel UI Issues

The UI is the `panel/` directory, served by the `vkai-ui` service on
`127.0.0.1:3000` and reached only through the panel port and the security
entrance.

### Build Failed

**Symptoms:**
- npm build errors
- TypeScript errors

**Solutions:**

```bash
# Clear cache
rm -rf node_modules .next

# Reinstall dependencies
npm install

# Check TypeScript errors
npm run build

# Fix TypeScript errors
# Review error messages and fix code
```

### Page Not Loading

**Symptoms:**
- Blank page
- JavaScript errors

**Solutions:**

```bash
# Check browser console
# Open DevTools (F12) and check Console tab

# Check API connection
curl http://localhost:30110/health

# Check environment variables
cat /vkai-panel/panel/.env.local

# Clear browser cache
# Ctrl+Shift+Delete (Chrome)
```

### API Connection Failed

**Symptoms:**
- Cannot connect to API
- CORS errors

**Solutions:**

```bash
# Check API URL
cat /vkai-panel/panel/.env.local

# Test API connection
curl http://localhost:30110/health

# Check CORS configuration: the origin the browser uses must be listed
sudo grep '^VKAI_CORS_ALLOWED_ORIGINS' /vkai-panel/etc/.env

# NEXT_PUBLIC_API_URL must point at the panel port, entrance included
sudo grep 'NEXT_PUBLIC_API_URL' /vkai-panel/etc/.env /vkai-panel/panel/.env.local

# A neutral 404 on every request means the entrance, host or source IP is wrong
vkai panel info
sudo journalctl -u vkai-api | grep "panel access denied"
```

---

## Agent Issues

### Agent Won't Start

**Symptoms:**
- Agent service fails
- Connection refused

**Solutions:**

```bash
# Check agent status
systemctl status vkai-agent

# View agent logs
journalctl -u vkai-agent -n 100

# Check agent configuration
cat /vkai-panel/agent/.env

# Test agent connection
curl http://localhost:30111/health
```

### Agent Not Reporting

**Symptoms:**
- No heartbeat
- Server offline

**Solutions:**

```bash
# Check agent logs
journalctl -u vkai-agent -f

# Check network connectivity
ping api-server-ip

# Check firewall
sudo ufw status

# Restart agent
sudo systemctl restart vkai-agent
```

### Agent Command Failed

**Symptoms:**
- Commands not executing
- Timeout errors

**Solutions:**

```bash
# Check agent permissions
ls -la /vkai-panel/agent/

# Check agent logs
journalctl -u vkai-agent -n 100

# Test command execution
# Review agent code for errors

# Increase timeout
# Edit agent configuration
```

---

## Logs and Debugging

### Viewing Logs

```bash
# API logs
journalctl -u vkai-api -f

# UI logs
journalctl -u vkai-ui -f

# Panel logs on disk
tail -f /vkai-panel/logs/api.log

# Web server logs of one customer site
tail -f /vkai-panel/logs/sites/example.com/access.log
tail -f /vkai-panel/logs/sites/example.com/error.log

# PostgreSQL logs
tail -f /var/log/postgresql/postgresql-*.log

# Redis logs
tail -f /var/log/redis/redis-server.log

# System logs
journalctl -f
```

### Debug Mode

```bash
# Enable debug logging
VKAI_LOG_LEVEL=debug

# Restart services
sudo systemctl restart vkai-api

# View debug logs
journalctl -u vkai-api -f | grep DEBUG
```

### Log Rotation

```bash
# Configure log rotation
sudo nano /etc/logrotate.d/vkai

# Add configuration
/vkai-panel/logs/*.log {
    daily
    missingok
    rotate 14
    compress
    delaycompress
    notifempty
    create 0640 vkai vkai
    sharedscripts
    postrotate
        systemctl reload vkai-api
    endscript
}
```

---

## FAQ

### How do I reset the admin password?

```bash
# Generate new password hash
go run -e 'fmt.Println(bcrypt.GenerateFromPassword([]byte("new_password"), 14))'

# Update database
sudo -u postgres psql -d vkai_panel -c "UPDATE users SET password_hash = 'NEW_HASH' WHERE username = 'admin';"

# Clear Redis cache
redis-cli FLUSHDB
```

### How do I backup the database?

```bash
# Backup
pg_dump -U vkai -d vkai_panel > backup.sql

# Backup with compression
pg_dump -U vkai -d vkai_panel | gzip > backup.sql.gz

# Restore
psql -U vkai -d vkai_panel < backup.sql

# Restore compressed
gunzip -c backup.sql.gz | psql -U vkai -d vkai_panel
```

### How do I update VKAI Panel?

```bash
# Backup database
pg_dump -U vkai -d vkai_panel > backup.sql

# Pull latest code
cd /vkai-panel
git pull origin main

# Run migrations
make migrate DATABASE_URL=postgres://vkai:PASSWORD@localhost:5432/vkai_panel

# Rebuild the API
go build -o bin/vkai-api ./cmd/api/

# Rebuild the UI
cd ../panel
npm ci
npm run build

# Restart services
sudo systemctl restart vkai-api vkai-ui
```

### How do I add a new website?

1. Login to the panel
2. Go to Websites
3. Click "Add Website"
4. Enter domain name
5. Select server
6. Configure settings
7. Click "Create"

### How do I issue an SSL certificate?

1. Go to SSL Certificates
2. Click "Issue Certificate"
3. Enter domain name
4. Select validation method
5. Click "Issue"
6. Wait for validation

### How do I create a database?

1. Go to Databases
2. Click "Create Database"
3. Enter database name
4. Select database server
5. Create database user
6. Click "Create"

### How do I setup email notifications?

1. Go to Settings
2. Click "Notifications"
3. Configure SMTP settings
4. Test email delivery
5. Enable notifications

### How do I monitor server resources?

1. Go to Servers
2. Select server
3. View metrics:
   - CPU usage
   - Memory usage
   - Disk usage
   - Network traffic

---

## Getting Help

### Documentation

- [API Documentation](API.md)
- [User Guide](USER_GUIDE.md)
- [Developer Guide](DEVELOPER_GUIDE.md)
- [Configuration Guide](CONFIGURATION.md)
- [Deployment Guide](DEPLOYMENT.md)

### Community

- **GitHub Issues**: https://github.com/hitechcloud-vietnam/vkai-panel/issues
- **Discussions**: https://github.com/hitechcloud-vietnam/vkai-panel/discussions
- **Email**: support@hitechcloud.vn

### Reporting Issues

When reporting issues, please include:

1. **Description**: Clear description of the problem
2. **Steps to Reproduce**: How to reproduce the issue
3. **Expected Behavior**: What you expected to happen
4. **Actual Behavior**: What actually happened
5. **Environment**: OS, browser, version
6. **Logs**: Relevant log entries
7. **Screenshots**: If applicable

---

## Support

- **Email**: support@hitechcloud.vn
- **Website**: https://hitechcloud.vn
- **Documentation**: https://docs.vkai.vn
