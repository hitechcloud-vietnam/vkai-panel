# vKAI Panel Architecture

## Table of Contents

1. [Overview](#overview)
2. [System Architecture](#system-architecture)
3. [Backend Architecture](#backend-architecture)
4. [Frontend Architecture](#frontend-architecture)
5. [Agent Architecture](#agent-architecture)
6. [Database Architecture](#database-architecture)
7. [Security Architecture](#security-architecture)
8. [Deployment Architecture](#deployment-architecture)
9. [Scalability](#scalability)
10. [Performance](#performance)

---

## Overview

vKAI Panel is an enterprise-grade multi-server hosting control panel designed for hosting providers and DevOps teams. It provides a modern web interface for managing servers, websites, databases, DNS, SSL certificates, and more.

### Design Principles

1. **Modularity**: Each feature is a separate module
2. **Scalability**: Horizontal and vertical scaling support
3. **Security**: Defense in depth approach
4. **Performance**: Optimized for high concurrency
5. **Reliability**: Fault tolerance and recovery
6. **Maintainability**: Clean code and documentation

---

## System Architecture

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                           Load Balancer                             │
│                        (Nginx/HAProxy)                              │
└─────────────────────────────────────────────────────────────────────┘
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

### Component Overview

| Component | Technology | Purpose |
|-----------|------------|---------|
| Frontend | Next.js 14, React 18 | User interface |
| API Server | Go 1.22, Gin | Business logic |
| Database | PostgreSQL 16 | Data storage |
| Cache | Redis 7 | Session, cache |
| Agent | Go binary | Server management |
| Reverse Proxy | Nginx | Load balancing, SSL |

---

## Backend Architecture

### Layered Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    HTTP Layer (Handlers)                     │
├─────────────────────────────────────────────────────────────┤
│                   Business Layer (Services)                  │
├─────────────────────────────────────────────────────────────┤
│                   Data Layer (Repositories)                  │
├─────────────────────────────────────────────────────────────┤
│                   Database Layer (PostgreSQL)                │
└─────────────────────────────────────────────────────────────┘
```

### Package Structure

```
backend/
├── cmd/
│   ├── api/                    # API server entry point
│   └── migrate/                # Database migration tool
├── internal/
│   ├── auth/                   # JWT authentication
│   │   ├── jwt.go             # JWT service
│   │   └── password.go        # Password hashing
│   ├── config/                 # Configuration
│   │   └── config.go          # Config struct
│   ├── database/               # Database connections
│   │   ├── postgres.go        # PostgreSQL connection
│   │   └── redis.go           # Redis connection
│   ├── handler/                # HTTP handlers
│   │   ├── auth.go            # Auth handlers
│   │   ├── website.go         # Website handlers
│   │   ├── database.go        # Database handlers
│   │   └── router.go          # Route definitions
│   ├── middleware/              # HTTP middleware
│   │   ├── auth.go            # Auth middleware
│   │   ├── cors.go            # CORS middleware
│   │   ├── rate_limit.go      # Rate limiting
│   │   └── tenant.go          # Tenant isolation
│   ├── models/                 # Data models
│   │   └── models.go          # All models
│   ├── rbac/                   # Role-based access control
│   │   └── rbac.go            # RBAC service
│   ├── repository/             # Data access layer
│   │   ├── website.go         # Website repository
│   │   ├── database.go        # Database repository
│   │   └── user.go            # User repository
│   ├── service/                # Business logic
│   │   ├── website.go         # Website service
│   │   ├── database.go        # Database service
│   │   └── auth.go            # Auth service
│   ├── utils/                  # Utilities
│   │   ├── validator.go       # Input validation
│   │   └── helpers.go         # Helper functions
│   └── webserver/              # Web server adapters
│       ├── adapter.go         # Adapter interface
│       └── nginx.go           # Nginx adapter
└── migrations/                 # SQL migrations
    ├── 001_initial_schema.sql
    └── 002_add_features.sql
```

### Request Flow

```
Client Request
      │
      ▼
┌─────────────┐
│   Router    │
└─────────────┘
      │
      ▼
┌─────────────┐
│ Middleware  │
│ - CORS      │
│ - Auth      │
│ - Tenant    │
│ - Rate Limit│
└─────────────┘
      │
      ▼
┌─────────────┐
│   Handler   │
│ - Validate  │
│ - Parse     │
└─────────────┘
      │
      ▼
┌─────────────┐
│   Service   │
│ - Business  │
│ - Logic     │
└─────────────┘
      │
      ▼
┌─────────────┐
│ Repository  │
│ - SQL       │
│ - Queries   │
└─────────────┘
      │
      ▼
┌─────────────┐
│  Database   │
└─────────────┘
```

### Dependency Injection

```go
// cmd/api/main.go
func main() {
    // Load config
    cfg := config.Load()
    
    // Initialize database
    db := database.NewPostgres(cfg.Database)
    redis := database.NewRedis(cfg.Redis)
    
    // Initialize repositories
    userRepo := repository.NewUserRepository(db)
    websiteRepo := repository.NewWebsiteRepository(db)
    
    // Initialize services
    authService := service.NewAuthService(userRepo, cfg.JWT)
    websiteService := service.NewWebsiteService(websiteRepo)
    
    // Initialize handlers
    authHandler := handler.NewAuthHandler(authService)
    websiteHandler := handler.NewWebsiteHandler(websiteService)
    
    // Setup router
    router := handler.SetupRouter(authHandler, websiteHandler)
    
    // Start server
    router.Run(cfg.Server.Address)
}
```

---

## Frontend Architecture

### Component Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Pages (App Router)                      │
├─────────────────────────────────────────────────────────────┤
│                    Components (React)                        │
├─────────────────────────────────────────────────────────────┤
│                      Hooks (Custom)                          │
├─────────────────────────────────────────────────────────────┤
│                    Store (Zustand)                           │
├─────────────────────────────────────────────────────────────┤
│                   Services (API)                             │
└─────────────────────────────────────────────────────────────┘
```

### Directory Structure

```
frontend/
├── src/
│   ├── app/                    # Next.js app router
│   │   ├── layout.tsx         # Root layout
│   │   ├── page.tsx           # Home page
│   │   ├── login/             # Login page
│   │   ├── register/          # Register page
│   │   └── dashboard/         # Dashboard pages
│   │       ├── layout.tsx     # Dashboard layout
│   │       ├── page.tsx       # Dashboard home
│   │       ├── websites/      # Website management
│   │       ├── databases/     # Database management
│   │       └── settings/      # Settings
│   ├── components/             # React components
│   │   ├── ui/                # UI components
│   │   │   ├── Button.tsx
│   │   │   ├── Input.tsx
│   │   │   └── Modal.tsx
│   │   ├── layout/            # Layout components
│   │   │   ├── Sidebar.tsx
│   │   │   └── Header.tsx
│   │   └── features/          # Feature components
│   │       ├── WebsiteList.tsx
│   │       └── DatabaseList.tsx
│   ├── hooks/                  # Custom hooks
│   │   ├── useAuth.ts
│   │   ├── useWebsites.ts
│   │   └── useDatabases.ts
│   ├── lib/                    # Utility libraries
│   │   ├── api.ts             # API client
│   │   └── utils.ts           # Utilities
│   ├── services/               # API services
│   │   ├── auth.ts
│   │   ├── websites.ts
│   │   └── databases.ts
│   ├── store/                  # Zustand stores
│   │   ├── authStore.ts
│   │   ├── websiteStore.ts
│   │   └── databaseStore.ts
│   └── styles/                 # CSS styles
│       └── globals.css
└── package.json
```

### State Management

```typescript
// store/websiteStore.ts
import { create } from 'zustand';
import { websiteService, Website } from '@/services/websites';

interface WebsiteState {
  websites: Website[];
  loading: boolean;
  error: string | null;
  fetchWebsites: () => Promise<void>;
  createWebsite: (data: CreateWebsiteRequest) => Promise<void>;
  deleteWebsite: (id: number) => Promise<void>;
}

export const useWebsiteStore = create<WebsiteState>((set) => ({
  websites: [],
  loading: false,
  error: null,
  
  fetchWebsites: async () => {
    set({ loading: true, error: null });
    try {
      const { data } = await websiteService.getAll();
      set({ websites: data, loading: false });
    } catch (error) {
      set({ error: 'Failed to fetch websites', loading: false });
    }
  },
  
  // ... other actions
}));
```

### Component Pattern

```typescript
// components/WebsiteList.tsx
'use client';

import { useEffect } from 'react';
import { useWebsiteStore } from '@/store/websiteStore';

export function WebsiteList() {
  const { websites, loading, error, fetchWebsites, deleteWebsite } = useWebsiteStore();
  
  useEffect(() => {
    fetchWebsites();
  }, [fetchWebsites]);
  
  if (loading) return <LoadingSpinner />;
  if (error) return <ErrorMessage error={error} />;
  
  return (
    <div className="space-y-4">
      {websites.map((website) => (
        <WebsiteCard key={website.id} website={website} onDelete={deleteWebsite} />
      ))}
    </div>
  );
}
```

---

## Agent Architecture

### Agent Structure

```
agent/
├── cmd/
│   └── main.go                # Entry point
├── internal/
│   ├── collector/             # System info collectors
│   │   ├── cpu.go            # CPU metrics
│   │   ├── memory.go         # Memory metrics
│   │   ├── disk.go           # Disk metrics
│   │   └── network.go        # Network metrics
│   ├── heartbeat/             # Heartbeat mechanism
│   │   └── heartbeat.go
│   ├── command/               # Command execution
│   │   └── executor.go
│   └── config/                # Configuration
│       └── config.go
└── go.mod
```

### Agent Communication

```
┌─────────────┐     HTTP/JSON     ┌─────────────┐
│   Agent     │ ◄──────────────► │  API Server │
│  (vkaid)    │                  │             │
└─────────────┘                  └─────────────┘
      │
      ▼
┌─────────────┐
│ System APIs │
│ - CPU       │
│ - Memory    │
│ - Disk      │
│ - Network   │
└─────────────┘
```

### Heartbeat Flow

```
Agent ──[Heartbeat]──► API Server
  │                        │
  │                        ▼
  │                  Update Server Status
  │                        │
  │                        ▼
  ◄──────[Commands]────────┘
  │
  ▼
Execute Commands
```

---

## Database Architecture

### Schema Design

```sql
-- Core tables
tenants
users
roles
permissions
user_roles
role_permissions

-- Server tables
servers
server_metrics

-- Website tables
websites
website_domains
website_configs

-- Database tables
database_servers
databases
database_users

-- SSL tables
ssl_certificates
ssl_renewals

-- DNS tables
dns_zones
dns_records

-- Cron tables
cron_jobs
cron_logs

-- Firewall tables
firewall_rules

-- Backup tables
backup_jobs
backup_files
```

### Multi-Tenant Isolation

```sql
-- All queries include tenant_id
SELECT * FROM websites WHERE tenant_id = $1;

-- Row-level security
ALTER TABLE websites ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON websites
    USING (tenant_id = current_setting('app.current_tenant')::bigint);
```

### Indexing Strategy

```sql
-- Primary keys (automatic)
-- Foreign keys
CREATE INDEX idx_websites_tenant_id ON websites(tenant_id);
CREATE INDEX idx_websites_server_id ON websites(server_id);

-- Query patterns
CREATE INDEX idx_websites_domain ON websites(domain);
CREATE INDEX idx_websites_status ON websites(status);

-- Composite indexes
CREATE INDEX idx_websites_tenant_status ON websites(tenant_id, status);
```

---

## Security Architecture

### Authentication Flow

```
Client ──[Login]──► API Server
  │                      │
  │                      ▼
  │                Validate Credentials
  │                      │
  │                      ▼
  │                Generate JWT Token
  │                      │
  ◄──────[Token]─────────┘
  │
  ▼
Store Token
  │
  ▼
[API Request + Token] ──► API Server
                              │
                              ▼
                        Validate Token
                              │
                              ▼
                        Extract Claims
                              │
                              ▼
                        Process Request
```

### Authorization Flow

```
Request ──► Auth Middleware ──► RBAC Check ──► Handler
                │                   │
                │                   ▼
                │            Check Permissions
                │                   │
                │                   ▼
                │            Allow/Deny
                ▼
          Validate Token
```

### Security Layers

```
┌─────────────────────────────────────────────────────────────┐
│                    Network Security                          │
│                    (Firewall, IDS/IPS)                       │
├─────────────────────────────────────────────────────────────┤
│                    Transport Security                        │
│                    (TLS/SSL)                                 │
├─────────────────────────────────────────────────────────────┤
│                    Application Security                      │
│                    (WAF, Rate Limiting)                      │
├─────────────────────────────────────────────────────────────┤
│                    Authentication                            │
│                    (JWT, MFA)                                │
├─────────────────────────────────────────────────────────────┤
│                    Authorization                             │
│                    (RBAC, ABAC)                              │
├─────────────────────────────────────────────────────────────┤
│                    Data Security                             │
│                    (Encryption, Masking)                     │
└─────────────────────────────────────────────────────────────┘
```

---

## Deployment Architecture

### Production Deployment

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
│    (systemd service)    │     │    (systemd service)    │
└─────────────────────────┘     └─────────────────────────┘
                                          │
                        ┌─────────────────┼─────────────────┐
                        │                 │                 │
                        ▼                 ▼                 ▼
              ┌─────────────┐   ┌─────────────┐   ┌─────────────┐
              │ PostgreSQL  │   │    Redis    │   │   Agent     │
              │  Port 5432  │   │  Port 6379  │   │ Port 30111  │
              │ (systemd)   │   │ (systemd)   │   │ (systemd)   │
              └─────────────┘   └─────────────┘   └─────────────┘
```

### Systemd Services

```ini
# vkai-api.service
[Unit]
Description=vKAI Panel API Server
After=network.target postgresql.service redis.service

[Service]
Type=simple
User=vkai
Group=vkai
WorkingDirectory=/opt/vkai-panel/backend
ExecStart=/opt/vkai-panel/backend/bin/vkai-api
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

---

## Scalability

### Horizontal Scaling

```
┌─────────────────┐
│ Load Balancer   │
└─────────────────┘
         │
    ┌────┴────┐
    │         │
    ▼         ▼
┌───────┐ ┌───────┐
│ API 1 │ │ API 2 │
└───────┘ └───────┘
    │         │
    └────┬────┘
         │
         ▼
┌─────────────────┐
│   Database      │
│   (Primary)     │
└─────────────────┘
         │
         ▼
┌─────────────────┐
│   Database      │
│   (Replica)     │
└─────────────────┘
```

### Vertical Scaling

- Increase CPU cores
- Increase RAM
- Use SSD storage
- Optimize database queries
- Use connection pooling

---

## Performance

### Optimization Strategies

1. **Database**
   - Connection pooling
   - Query optimization
   - Indexing
   - Caching

2. **Application**
   - Goroutines for concurrency
   - Connection pooling
   - Response caching
   - Compression

3. **Frontend**
   - Code splitting
   - Image optimization
   - CDN caching
   - Lazy loading

4. **Network**
   - HTTP/2
   - Gzip compression
   - Keep-alive connections
   - CDN

### Monitoring

```
┌─────────────────────────────────────────────────────────────┐
│                    Monitoring Stack                          │
├─────────────────────────────────────────────────────────────┤
│  Prometheus ──► Grafana ──► Alerts                          │
│      │                                                      │
│      ▼                                                      │
│  Metrics Collection                                         │
│  - CPU, Memory, Disk                                        │
│  - Request latency                                          │
│  - Error rates                                              │
│  - Database queries                                         │
└─────────────────────────────────────────────────────────────┘
```

---

## Future Considerations

### Planned Improvements

1. **Microservices**: Split into smaller services
2. **Event Sourcing**: Use event-driven architecture
3. **CQRS**: Separate read/write models
4. **GraphQL**: Add GraphQL API
5. **WebSocket**: Real-time updates
6. **gRPC**: Internal service communication

### Technology Roadmap

- **v2.0**: Microservices architecture
- **v3.0**: Kubernetes deployment
- **v4.0**: Multi-cloud support

---

## Resources

- [Go Documentation](https://go.dev/doc/)
- [Next.js Documentation](https://nextjs.org/docs)
- [PostgreSQL Documentation](https://www.postgresql.org/docs/)
- [Redis Documentation](https://redis.io/documentation)
- [Nginx Documentation](https://nginx.org/en/docs/)

---

## Support

- **Issues**: https://github.com/hitechcloud-vietnam/vkai-panel/issues
- **Discussions**: https://github.com/hitechcloud-vietnam/vkai-panel/discussions
- **Email**: support@hitechcloud.vn
