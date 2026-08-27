# VKAI Panel Architecture

## Table of Contents

1. [Overview](#overview)
2. [System Architecture](#system-architecture)
3. [Core API Architecture (`core/`)](#core-api-architecture-core)
4. [Panel UI Architecture (`panel/`)](#panel-ui-architecture-panel)
5. [Agent Architecture](#agent-architecture)
6. [Database Architecture](#database-architecture)
7. [Security Architecture](#security-architecture)
8. [Deployment Architecture](#deployment-architecture)
9. [Scalability](#scalability)
10. [Performance](#performance)

---

## Overview

VKAI Panel is an enterprise-grade multi-server hosting control panel designed for hosting providers and DevOps teams. It provides a modern web interface for managing servers, websites, databases, DNS, SSL certificates, and more.

### Design Principles

1. **Modularity**: Each feature is a separate module
2. **Scalability**: Horizontal and vertical scaling support
3. **Security**: Defense in depth approach
4. **Performance**: Optimized for high concurrency
5. **Reliability**: Fault tolerance and recovery
6. **Maintainability**: Clean code and documentation
7. **Bare-metal runtime**: the panel itself ships as native Linux processes -- a
   Go binary and a Next.js standalone build supervised by systemd. No container
   runtime is involved in running, building or installing the panel.

### Two Different Meanings of "Docker"

This distinction matters throughout the document, so it is stated once here.

| | Docker as panel infrastructure | Docker as a customer-facing feature |
|---|---|---|
| Status | **Removed** | **Kept, fully supported** |
| What it was / is | `Dockerfile`, `docker-compose.yml` used to build and run the panel | The Docker screen, `/api/v1/docker/*`, `docker:*` RBAC permissions |
| Replaced by | `deploy/install.sh` + systemd units | Nothing -- it remains a first-class feature |
| Docker Engine required on the panel host? | No | Only if the customer wants to use the feature |

The panel **does not run inside Docker**; the panel **manages Docker** on behalf
of its users. Removing the container-based deployment path did not remove any
container-management functionality.

---

## System Architecture

### High-Level Architecture

```
                                 Internet
                                    │
              ┌─────────────────────┴─────────────────────┐
              │                                           │
              ▼                                           ▼
┌───────────────────────────┐             ┌───────────────────────────┐
│  Port 80 / 443            │             │  Port 8888 (VKAI_PANEL_   │
│  Customer websites        │             │  PORT) + security         │
│  (nginx / apache / ...)   │             │  entrance /vkai_xxxxxxxx  │
│  Web root:                │             │  ADMIN PANEL ONLY         │
│  /vkai-panel/www/domains/ │             │                           │
└───────────────────────────┘             └─────────────┬─────────────┘
                                                        │
                                      ┌─────────────────┴─────────────────┐
                                      │                                   │
                                      ▼                                   ▼
                        ┌─────────────────────────┐     ┌─────────────────────────┐
                        │   vkai-ui (Next.js)     │     │  vkai-api (Go)          │
                        │   Port 3000, localhost  │     │  Port 30110, localhost  │
                        └─────────────────────────┘     └─────────────────────────┘
                                                                    │
                                                  ┌─────────────────┼─────────────────┐
                                                  │                 │                 │
                                                  ▼                 ▼                 ▼
                                        ┌─────────────┐   ┌─────────────┐   ┌─────────────┐
                                        │ PostgreSQL  │   │    Redis    │   │ vkai-agent  │
                                        │  Port 5432  │   │  Port 6379  │   │ Port 30111  │
                                        └─────────────┘   └─────────────┘   └─────────────┘
```

The panel never listens on 80 or 443: those ports belong to the customer
websites this server hosts. `vkai-api` owns the panel port itself and serves
both the API and the UI behind the security entrance; ports 30110 and 3000 are
bound to localhost and are not reachable from the Internet.

### Component Overview

| Component | Technology | Purpose |
|-----------|------------|---------|
| UI (`panel/`, `vkai-ui`) | Next.js 14, React 18 | User interface |
| API Server | Go 1.22, Gin | Business logic |
| Database | PostgreSQL 16 | Data storage |
| Cache | Redis 7 | Session, cache |
| Agent | Go binary | Server management |
| Reverse Proxy | Nginx | Load balancing, SSL |

---

## Core API Architecture (`core/`)

The Go API lives in the **`core/`** directory (formerly `backend/`) and ships as
the `vkai-api` systemd service. The Go module path is unchanged:
`github.com/hitechcloud-vietnam/vkai-panel`.

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
core/
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

## Panel UI Architecture (`panel/`)

The Next.js UI lives in the **`panel/`** directory (formerly `frontend/`) and
ships as the `vkai-ui` systemd service. It is only reachable through the panel
port and the security entrance, never directly from the Internet.

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
panel/
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
        Customer traffic                      Administrator traffic
              │                                          │
              ▼                                          ▼
┌─────────────────────────────┐        ┌─────────────────────────────┐
│  nginx / apache vhosts      │        │  vkai-api on port 8888      │
│  Port 80 / 443              │        │  + entrance /vkai_xxxxxxxx  │
│  Root: /vkai-panel/www/     │        │  + IP allow list, TLS       │
│        domains/<domain>     │        │  (systemd: vkai-api)        │
│  Logs: /vkai-panel/logs/    │        └──────────────┬──────────────┘
│        sites/<domain>       │                       │
└─────────────────────────────┘        ┌──────────────┴──────────────┐
                                       │                             │
                                       ▼                             ▼
                         ┌─────────────────────────┐   ┌─────────────────────────┐
                         │   vkai-ui (Next.js)     │   │   vkai-api internal API │
                         │   127.0.0.1:3000        │   │   127.0.0.1:30110       │
                         │   (systemd: vkai-ui)    │   │                         │
                         └─────────────────────────┘   └────────────┬────────────┘
                                                                    │
                                                  ┌─────────────────┼─────────────────┐
                                                  │                 │                 │
                                                  ▼                 ▼                 ▼
                                        ┌─────────────┐   ┌─────────────┐   ┌─────────────┐
                                        │ PostgreSQL  │   │    Redis    │   │ vkai-agent  │
                                        │  Port 5432  │   │  Port 6379  │   │ Port 30111  │
                                        │ (systemd)   │   │ (systemd)   │   │ (systemd)   │
                                        └─────────────┘   └─────────────┘   └─────────────┘
```

Ports 80 and 443 are reserved for customer websites. The panel is reached only
through `VKAI_PANEL_PORT` (default `8888`) plus the security entrance; anything
that arrives on the panel port without the correct host, source IP and entrance
path receives a neutral 404. See [PANEL_ACCESS.md](PANEL_ACCESS.md).

### Installed Layout

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

### Systemd Services

| Unit | Role | Ports |
|------|------|-------|
| `vkai-api` | Go API; also owns the panel port and the security entrance | 8888 public, 30110 localhost |
| `vkai-ui` | Next.js UI | 3000 localhost only |
| `vkai-agent` | Node agent on each managed server | 30111 |

```ini
# /etc/systemd/system/vkai-api.service
[Unit]
Description=VKAI Panel API Server
After=network.target postgresql.service redis.service

[Service]
Type=simple
User=vkai
Group=vkai
WorkingDirectory=/vkai-panel/core
EnvironmentFile=/vkai-panel/etc/.env
ExecStart=/vkai-panel/core/bin/vkai-api
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```ini
# /etc/systemd/system/vkai-ui.service
[Unit]
Description=VKAI Panel UI (Next.js standalone)
After=network-online.target vkai-api.service

[Service]
Type=simple
User=vkai
Group=vkai
WorkingDirectory=/vkai-panel/panel
Environment=NODE_ENV=production
Environment=PORT=3000
Environment=HOSTNAME=127.0.0.1
EnvironmentFile=/vkai-panel/etc/.env
ExecStart=/usr/bin/node /vkai-panel/panel/.next/standalone/server.js
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

The UI is served from the Next.js **standalone** output, started directly by
`node`. The build step must copy `.next/static` and `public/` into
`.next/standalone`; without them the page renders but every `/_next/static/*.js`
returns 404 and the browser shows *"Application error: a client-side exception
has occurred"*.

The authoritative unit files live in `deploy/systemd/`. All three carry systemd
hardening (`NoNewPrivileges`, `ProtectSystem=strict`, `ProtectHome`,
`PrivateTmp`, and a narrow `ReadWritePaths` allow-list).

### Release Layout and Rollback

Deployments are directory-based. Each release is unpacked into its own versioned
directory under `/vkai-panel/releases/`, and the `current` symlink names the
release that is live. Switching releases is repointing that symlink and
restarting the units; rolling back is repointing it the other way.

```
/vkai-panel/
├── releases/
│   ├── 20250315_101500/                    # previous release, kept for rollback
│   │   ├── core/bin/vkai-api
│   │   ├── core/migrations/*.sql
│   │   ├── panel/.next/standalone/server.js
│   │   └── agent/bin/vkai-agent
│   └── 20250316_143000/                    # newly deployed release
├── current -> releases/20250316_143000     # the active release
├── etc/                                    # .env, panel_access.json  -- shared
├── logs/                                   # shared
├── www/                                    # customer sites, backups -- shared
└── ssl/                                    # shared
```

Only the code is versioned. `etc/`, `logs/`, `www/` and `ssl/` are **shared
state**: they live outside every release, so a deploy never overwrites them and a
rollback never reverts them. `deploy.sh` links `/vkai-panel/etc/.env` into each
release tree, because Next.js only reads `.env` from a project root.

The systemd units start the panel from `/vkai-panel/core` and
`/vkai-panel/panel`. Those paths must resolve to the release that `current`
points at -- otherwise switching the symlink has no effect on what actually runs.

`deploy/scripts/deploy.sh` owns the lifecycle:

1. Unpack the package into a new `releases/<timestamp>/` directory.
2. **Validate** it -- API binary present and executable, `server.js` present, and
   `.next/standalone/.next/static` present -- before touching the running system.
3. Dump the database to `www/backup/predeploy_<timestamp>.sql.gz`.
4. Apply pending migrations **from the new release, before the switch**, so a
   failing migration aborts while the old release is still serving.
5. Repoint `current` and restart `vkai-api`, `vkai-ui` (and `vkai-agent` if
   enabled), then reload nginx.
6. Health-check both the API (`/health`) and the UI, retrying for about 30s.
7. On failure, **switch back to the previous release automatically** and
   health-check again.
8. Keep the active release plus the five most recent older ones; delete the rest.

Rollback restores **code only**. Database migrations are never reversed, which is
why the pre-deploy dump in step 3 is the real recovery path for a bad migration.

### nginx

nginx terminates the panel port and proxies to the two loopback services. The
vhost is installed as `/etc/nginx/conf.d/vkai-panel.conf` from
`deploy/nginx/vkai-panel.conf`:

```nginx
upstream vkai_ui  { server 127.0.0.1:3000;  keepalive 32; }
upstream vkai_api { server 127.0.0.1:30110; keepalive 32; }

server {
    listen 8888;              # VKAI_PANEL_PORT -- never 80 or 443
    listen [::]:8888;
    ...
}
```

This file contains no `listen 80` and no `listen 443`, by design: those ports
belong to customer vhosts, which nginx serves from entirely separate server
blocks. The panel port is a separate listener on the same nginx instance.

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
