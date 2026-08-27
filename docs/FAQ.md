# vKAI Panel FAQ

## General Questions

### What is vKAI Panel?

vKAI Panel is an enterprise-grade multi-server hosting control panel designed for hosting providers and DevOps teams. It provides a modern web interface for managing servers, websites, databases, DNS, SSL certificates, and more.

### What are the system requirements?

**Minimum Requirements:**
- OS: Ubuntu 22.04 LTS or Debian 12
- CPU: 2 cores
- RAM: 4 GB
- Disk: 50 GB SSD
- Network: 1 Gbps

**Recommended Requirements:**
- OS: Ubuntu 22.04 LTS
- CPU: 4 cores
- RAM: 8 GB
- Disk: 100 GB SSD
- Network: 1 Gbps

### What technologies does vKAI Panel use?

- **Backend**: Go 1.22, Gin framework, JWT authentication
- **Frontend**: Next.js 14, React 18, TypeScript, Tailwind CSS
- **Database**: PostgreSQL 16, Redis 7
- **Agent**: Go binary (vkaid)
- **Reverse Proxy**: Nginx
- **Services**: systemd (no Docker required)

### Is vKAI Panel free?

Yes, vKAI Panel is open-source and free to use. It's licensed under the MIT License.

### Can I use vKAI Panel in production?

Yes, vKAI Panel is designed for production use. It includes:
- Systemd services for reliability
- Security best practices
- Performance optimization
- Comprehensive documentation

---

## Installation

### How do I install vKAI Panel?

```bash
# Clone repository
git clone https://github.com/hitechcloud-vietnam/vkai-panel.git
cd vkai-panel

# Run installation script
chmod +x deploy/install.sh
sudo ./deploy/install.sh
```

### What does the installation script do?

The installation script:
1. Installs system dependencies
2. Installs Go and Node.js
3. Creates system user
4. Sets up directories
5. Configures PostgreSQL and Redis
6. Builds backend and frontend
7. Configures environment
8. Installs systemd services
9. Configures Nginx
10. Sets up firewall

### Can I install vKAI Panel manually?

Yes, see the [Deployment Guide](DEPLOYMENT.md) for manual installation instructions.

### How do I update vKAI Panel?

```bash
# Backup database
pg_dump -U vkai -d vkai_panel > backup.sql

# Pull latest code
cd /opt/vkai-panel
git pull origin main

# Run migrations
cd backend
go run cmd/migrate/main.go

# Rebuild
go build -o bin/vkai-api ./cmd/api/
cd ../frontend
npm install
npm run build

# Restart services
sudo systemctl restart vkai-api
sudo systemctl restart vkai-frontend
```

### How do I uninstall vKAI Panel?

```bash
# Stop services
sudo systemctl stop vkai-api
sudo systemctl stop vkai-frontend

# Disable services
sudo systemctl disable vkai-api
sudo systemctl disable vkai-frontend

# Remove service files
sudo rm /etc/systemd/system/vkai-api.service
sudo rm /etc/systemd/system/vkai-frontend.service

# Remove files
sudo rm -rf /opt/vkai-panel

# Remove user
sudo userdel -r vkai

# Remove database
sudo -u postgres psql -c "DROP DATABASE vkai_panel;"
sudo -u postgres psql -c "DROP USER vkai;"
```

---

## Configuration

### Where is the configuration file?

The main configuration file is `/opt/vkai-panel/.env`.

### What configuration options are available?

See the [Configuration Guide](CONFIGURATION.md) for all available options.

### How do I change the API port?

Edit `/opt/vkai-panel/.env`:
```bash
SERVER_PORT=30111
```

Then restart the API server:
```bash
sudo systemctl restart vkai-api
```

### How do I change the database password?

1. Update PostgreSQL:
```bash
sudo -u postgres psql -c "ALTER USER vkai PASSWORD 'new_password';"
```

2. Update `.env`:
```bash
DB_PASSWORD=new_password
```

3. Restart services:
```bash
sudo systemctl restart vkai-api
```

### How do I enable debug logging?

Edit `/opt/vkai-panel/.env`:
```bash
LOG_LEVEL=debug
```

Then restart:
```bash
sudo systemctl restart vkai-api
```

---

## Authentication

### What are the default credentials?

- **URL**: http://localhost:3000 (development) or http://your-server-ip (production)
- **Username**: admin
- **Password**: admin123

⚠️ **Change the default password immediately in production!**

### How do I reset the admin password?

```bash
# Generate new password hash
go run -e 'fmt.Println(bcrypt.GenerateFromPassword([]byte("new_password"), 14))'

# Update database
sudo -u postgres psql -d vkai_panel -c "UPDATE users SET password_hash = 'NEW_HASH' WHERE username = 'admin';"

# Clear Redis cache
redis-cli FLUSHDB
```

### How do I create a new user?

1. Login to the panel
2. Go to Users
3. Click "Add User"
4. Enter user details
5. Assign role
6. Click "Create"

### How do I enable two-factor authentication?

1. Go to Settings
2. Click "Security"
3. Enable 2FA
4. Scan QR code with authenticator app
5. Enter verification code
6. Save backup codes

### How do I manage API tokens?

1. Go to Settings
2. Click "API Tokens"
3. Click "Generate Token"
4. Set permissions
5. Copy token (shown only once)

---

## Websites

### How do I add a new website?

1. Login to the panel
2. Go to Websites
3. Click "Add Website"
4. Enter domain name
5. Select server
6. Configure settings
7. Click "Create"

### How do I configure SSL for a website?

1. Go to Websites
2. Select website
3. Go to SSL tab
4. Click "Issue Certificate"
5. Select validation method
6. Click "Issue"
7. Wait for validation

### How do I enable HTTPS redirect?

1. Go to Websites
2. Select website
3. Go to SSL tab
4. Enable "Force HTTPS"
5. Save changes

### How do I configure a custom domain?

1. Go to DNS management
2. Add A record pointing to server IP
3. Add website in panel
4. Issue SSL certificate

### How do me delete a website?

1. Go to Websites
2. Select website
3. Click "Delete"
4. Confirm deletion

---

## Databases

### How do I create a database?

1. Go to Databases
2. Click "Create Database"
3. Enter database name
4. Select database server
5. Create database user
6. Click "Create"

### How do I backup a database?

1. Go to Databases
2. Select database
3. Click "Backup"
4. Choose backup location
5. Click "Backup"

### How do I restore a database?

1. Go to Databases
2. Select database
3. Click "Restore"
4. Select backup file
5. Click "Restore"

### How do I manage database users?

1. Go to Databases
2. Select database
3. Go to Users tab
4. Add/remove users
5. Set permissions

---

## SSL Certificates

### How do I issue an SSL certificate?

1. Go to SSL Certificates
2. Click "Issue Certificate"
3. Enter domain name
4. Select validation method:
   - HTTP validation
   - DNS validation
5. Click "Issue"
6. Wait for validation

### How do I renew an SSL certificate?

Certificates are automatically renewed. To manually renew:

```bash
sudo certbot renew
```

### How do I upload a custom certificate?

1. Go to SSL Certificates
2. Click "Upload Certificate"
3. Upload certificate file
4. Upload private key
5. Click "Upload"

### How do I check certificate expiration?

```bash
openssl x509 -enddate -noout -in /etc/ssl/vkai/your-domain.crt
```

---

## Services

### How do I manage systemd services?

```bash
# Start service
sudo systemctl start vkai-api

# Stop service
sudo systemctl stop vkai-api

# Restart service
sudo systemctl restart vkai-api

# Check status
sudo systemctl status vkai-api

# View logs
journalctl -u vkai-api -f
```

### How do I create a custom service?

1. Go to Services
2. Click "Add Service"
3. Enter service details
4. Configure command
5. Set environment variables
6. Click "Create"

### How do me enable/disable a service?

1. Go to Services
2. Select service
3. Click "Enable" or "Disable"

---

## File Manager

### How do I upload files?

1. Go to File Manager
2. Navigate to directory
3. Click "Upload"
4. Select files
5. Click "Upload"

### How do I edit files?

1. Go to File Manager
2. Navigate to file
3. Click on file
4. Edit content
5. Click "Save"

### How do I change file permissions?

1. Go to File Manager
2. Right-click on file
3. Select "Permissions"
4. Set permissions
5. Click "Save"

---

## Cron Jobs

### How do I create a cron job?

1. Go to Cron Jobs
2. Click "Add Cron Job"
3. Enter command
4. Set schedule (cron expression)
5. Click "Create"

### How do I test a cron job?

1. Go to Cron Jobs
2. Select cron job
3. Click "Run Now"
4. Check output

### What are common cron expressions?

- `* * * * *` - Every minute
- `0 * * * *` - Every hour
- `0 0 * * *` - Every day at midnight
- `0 0 * * 0` - Every Sunday at midnight
- `0 0 1 * *` - First day of every month

---

## Firewall

### How do I add a firewall rule?

1. Go to Firewall
2. Click "Add Rule"
3. Select action (Allow/Deny)
4. Enter port
5. Enter IP (optional)
6. Click "Add"

### How do I block an IP address?

1. Go to Firewall
2. Click "Add Rule"
3. Select "Deny"
4. Enter IP address
5. Click "Add"

### How do I allow a port?

1. Go to Firewall
2. Click "Add Rule"
3. Select "Allow"
4. Enter port number
5. Click "Add"

---

## Backups

### How do I create a backup?

1. Go to Backups
2. Click "Create Backup"
3. Select what to backup
4. Choose destination
5. Click "Backup"

### How do I schedule backups?

1. Go to Backups
2. Click "Schedule"
3. Set schedule
4. Select what to backup
5. Choose destination
6. Click "Save"

### How do I restore a backup?

1. Go to Backups
2. Select backup
3. Click "Restore"
4. Select what to restore
5. Click "Restore"

---

## Monitoring

### How do I monitor server resources?

1. Go to Servers
2. Select server
3. View metrics:
   - CPU usage
   - Memory usage
   - Disk usage
   - Network traffic

### How do I set up alerts?

1. Go to Settings
2. Click "Alerts"
3. Configure alert rules
4. Set notification channels
5. Click "Save"

---

## Troubleshooting

### The panel is not loading

1. Check Nginx status:
```bash
sudo systemctl status nginx
```

2. Check API server status:
```bash
sudo systemctl status vkai-api
```

3. Check logs:
```bash
journalctl -u vkai-api -n 100
tail -f /var/log/nginx/error.log
```

### I cannot login

1. Check credentials
2. Check user status
3. Check Redis connection
4. Clear browser cache

### The API is returning errors

1. Check API logs:
```bash
journalctl -u vkai-api -f
```

2. Check database connection:
```bash
psql -h localhost -U vkai -d vkai_panel
```

3. Check Redis connection:
```bash
redis-cli ping
```

### Services are not starting

1. Check service status:
```bash
sudo systemctl status vkai-api
```

2. Check logs:
```bash
journalctl -u vkai-api -n 100
```

3. Check configuration:
```bash
cat /opt/vkai-panel/.env
```

---

## Development

### How do I set up a development environment?

```bash
# Clone repository
git clone https://github.com/hitechcloud-vietnam/vkai-panel.git
cd vkai-panel

# Run setup script
chmod +x setup-dev.sh
./setup-dev.sh

# Start development
make dev
```

### How do I run tests?

```bash
# Backend tests
cd backend
go test ./...

# Frontend tests
cd frontend
npm test

# All tests
make test
```

### How do I contribute?

See the [Contributing Guide](CONTRIBUTING.md) for instructions.

---

## Support

### Where can I get help?

- **Documentation**: https://docs.vkai.vn
- **GitHub Issues**: https://github.com/hitechcloud-vietnam/vkai-panel/issues
- **Discussions**: https://github.com/hitechcloud-vietnam/vkai-panel/discussions
- **Email**: support@hitechcloud.vn

### How do I report a bug?

1. Check existing issues
2. Create new issue with:
   - Description
   - Steps to reproduce
   - Expected behavior
   - Actual behavior
   - Environment details
   - Logs

### How do I request a feature?

1. Check existing discussions
2. Create new discussion with:
   - Feature description
   - Use case
   - Proposed solution
   - Alternatives considered

---

## License

### What license is vKAI Panel under?

vKAI Panel is licensed under the MIT License. See [LICENSE](../LICENSE) for details.

### Can I use vKAI Panel commercially?

Yes, the MIT License allows commercial use.

### Do I need to attribute vKAI Panel?

While not required, attribution is appreciated:
```markdown
This project uses [vKAI Panel](https://github.com/hitechcloud-vietnam/vkai-panel) by [HiTechCloud Vietnam](https://hitechcloud.vn).
```

---

## Contact

- **Website**: https://hitechcloud.vn
- **Email**: support@hitechcloud.vn
- **GitHub**: https://github.com/hitechcloud-vietnam/vkai-panel
