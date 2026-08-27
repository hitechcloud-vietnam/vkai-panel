# vKAI Panel

**Enterprise Multi-Server Hosting & Web Control Panel** by HiTechCloud

## Overview

vKAI Panel is a comprehensive server management platform designed for hosting providers and DevOps teams. It provides a modern web interface for managing servers, websites, databases, DNS, SSL certificates, Docker containers, and more.

## Features

### Core Features
- 🖥️ **Multi-Server Management** - Manage unlimited servers from a single panel
- 🌐 **Website Management** - PHP, Node.js, Python, Reverse Proxy, Static sites
- 🗄️ **Database Management** - MySQL, MariaDB, PostgreSQL, Redis, MongoDB
- 🔒 **SSL/TLS** - Let's Encrypt, custom certificates, auto-renewal
- 📝 **DNS Management** - BIND, PowerDNS integration
- 🐳 **Docker Management** - Containers, images, volumes, compose
- 📁 **File Manager** - Web-based file editor with syntax highlighting
- ⏰ **Cron Jobs** - Visual cron job management
- 🔥 **Firewall** - UFW, firewalld, CSF integration
- 💾 **Backups** - Automated backups to S3, FTP, SFTP, Dropbox
- 🚀 **Deployments** - Git-based deployments with webhooks
- 📊 **Monitoring** - Real-time server metrics and alerts
- 🔐 **Security** - SSH key management, 2FA, IP whitelisting
- 👥 **Multi-Tenant** - Complete tenant isolation with RBAC

### Supported Web Servers
- Nginx
- Apache
- OpenLiteSpeed
- LiteSpeed Enterprise
- Caddy
- Traefik

## Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Frontend      │     │   Backend API   │     │   vKAI Agent    │
│   (Next.js)     │────▶│   (Golang)      │────▶│   (Golang)      │
│   Port: 3000    │     │   Port: 30110   │     │   Port: 30111   │
└─────────────────┘     └─────────────────┘     └─────────────────┘
                              │                         │
                              ▼                         ▼
                        ┌──────────┐            ┌──────────────┐
                        │PostgreSQL│            │  Managed     │
                        │  Redis   │            │  Servers     │
                        └──────────┘            └──────────────┘
```

## Quick Start

### Prerequisites
- Go 1.22+
- Node.js 20+
- PostgreSQL 16+
- Redis 7+

### Development Setup

```bash
# Clone the repository
git clone https://github.com/hitechcloud/vkai-panel.git
cd vkai-panel

# Run setup script
chmod +x scripts/setup.sh
./scripts/setup.sh

# Start with Docker
docker-compose up -d

# Or start manually:
# Terminal 1: Backend
cd backend
go run cmd/api/main.go

# Terminal 2: Frontend
cd frontend
npm run dev

# Terminal 3: Agent (optional)
cd agent
VKAI_PANEL_URL=http://localhost:30110 VKAI_AGENT_TOKEN=your-token go run cmd/main.go
```

### Default Credentials
- **URL**: http://localhost:3000
- **Username**: admin
- **Password**: admin123

⚠️ **Change the default password immediately in production!**

## Project Structure

```
vkai-panel/
├── backend/                 # Go API server
│   ├── cmd/api/            # Entry point
│   ├── internal/           # Internal packages
│   │   ├── auth/          # JWT authentication
│   │   ├── config/        # Configuration
│   │   ├── database/      # Database connections
│   │   ├── handler/       # HTTP handlers
│   │   ├── middleware/     # HTTP middleware
│   │   ├── models/        # Data models
│   │   ├── rbac/          # Role-based access control
│   │   ├── repository/    # Data access layer
│   │   ├── service/       # Business logic
│   │   ├── utils/         # Utilities
│   │   └── webserver/     # Web server adapters
│   ├── migrations/        # SQL migrations
│   └── config.yaml        # Configuration file
├── frontend/               # Next.js frontend
│   ├── src/
│   │   ├── app/           # Next.js app router
│   │   ├── components/    # React components
│   │   ├── services/      # API services
│   │   ├── store/         # Zustand stores
│   │   └── styles/        # CSS styles
│   └── package.json
├── agent/                  # vKAI Agent
│   └── cmd/main.go       # Agent entry point
├── docker/                 # Docker configurations
├── scripts/               # Utility scripts
├── docs/                  # Documentation
├── Dockerfile             # Multi-stage Docker build
└── docker-compose.yml     # Docker Compose config
```

## API Documentation

### Authentication
```bash
# Login
POST /api/v1/auth/login
{
  "username": "admin",
  "password": "admin123"
}

# Refresh Token
POST /api/v1/auth/refresh
{
  "refresh_token": "..."
}
```

### Servers
```bash
# List servers
GET /api/v1/servers

# Create server
POST /api/v1/servers
{
  "name": "Web Server 01",
  "hostname": "web01.example.com",
  "ip_address": "192.168.1.100"
}

# Get server details
GET /api/v1/servers/:id

# Get server metrics
GET /api/v1/servers/:id/metrics
```

### Websites
```bash
# List websites
GET /api/v1/websites

# Create website
POST /api/v1/websites
{
  "domain": "example.com",
  "type": "php",
  "server_id": "..."
}
```

## Configuration

Configuration is managed through `config.yaml` or environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `VKAI_SERVER_PORT` | API server port | `30110` |
| `VKAI_DB_HOST` | PostgreSQL host | `localhost` |
| `VKAI_DB_PORT` | PostgreSQL port | `5432` |
| `VKAI_DB_USER` | PostgreSQL user | `vkai` |
| `VKAI_DB_PASSWORD` | PostgreSQL password | `vkai_password` |
| `VKAI_DB_NAME` | Database name | `vkai_panel` |
| `VKAI_REDIS_HOST` | Redis host | `localhost` |
| `VKAI_REDIS_PORT` | Redis port | `6379` |
| `VKAI_JWT_SECRET` | JWT secret key | (required) |

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

Copyright © 2024 HiTechCloud. All rights reserved.

## Support

- Documentation: https://docs.vkai.cloud
- Issues: https://github.com/hitechcloud/vkai-panel/issues
- Email: support@hitechcloud.vn
