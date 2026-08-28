# VKAI Panel

**English** · [Tiếng Việt](README.vi.md)

**A multi-server hosting and server control panel** — built by **HiTechCloud** ([hitechcloud.vn](https://hitechcloud.vn)).

VKAI Panel manages servers, websites, databases, DNS, TLS certificates, containers,
firewalls, backups and monitoring from a single web interface. The panel listens on
**its own port (8888 by default)** behind a **security entrance**; ports **80 and 443
stay reserved for customer websites**.

---

## Contents

- [What makes it different](#what-makes-it-different)
- [Docker in VKAI Panel: two different roles](#docker-in-vkai-panel-two-different-roles)
- [Features](#features)
- [Supported operating systems](#supported-operating-systems)
- [One-line install](#one-line-install)
- [First access to the panel](#first-access-to-the-panel)
- [The interface](#the-interface)
- [Architecture](#architecture)
- [Repository layout](#repository-layout)
- [Standard paths on the server](#standard-paths-on-the-server)
- [systemd services](#systemd-services)
- [Releases and deployment](#releases-and-deployment)
- [Day-to-day operation](#day-to-day-operation)
- [The `vkai` administration command](#the-vkai-administration-command)
- [Configuration](#configuration)
- [Development environment](#development-environment)
- [Contributing](#contributing)
- [Documentation](#documentation)
- [Licence and support](#licence-and-support)

---

## What makes it different

| | VKAI Panel |
|---|---|
| Administration port | Its own port, `8888` by default — **never** takes 80/443 |
| Entrance | A secret path such as `/vkai_a1b2c3d4`; a wrong path gets a neutral 404 |
| IP and domain restriction | Yes, checked before the entrance itself |
| Customer websites | Have 80/443 entirely to themselves, fully separate from the panel |
| Deployment | Plain systemd (`vkai-api`, `vkai-ui`, `vkai-agent`) — a Go binary and a Next.js standalone build, **no Docker** |
| Multi-server | One panel drives many nodes through `vkai-agent` |

## Docker in VKAI Panel: two different roles

This is the easiest thing to misread, so it is stated up front. The word "Docker"
means **two completely separate things** in this project.

**1. Docker as the infrastructure that runs the panel — removed.**
The core API, the interface and the agent all build and run **directly** on Linux:
a Go binary, a Next.js standalone build, supervised by systemd. The repository
contains no `Dockerfile`, no `docker-compose.yml`, no `.dockerignore` and no
`docker compose up` instructions for standing the panel up. PostgreSQL and Redis
are installed **on the host** by `deploy/install.sh`. A server running the panel
**does not need** Docker Engine.

**2. Docker as a feature for customers — kept, in full.**
Customers use the panel to manage **their own** containers, images, volumes,
networks and compose stacks. The Docker screens in the interface, the
`/api/v1/docker/*` API group and the `docker:*` RBAC permissions are **unchanged**.
Nothing about this feature was cut.

| | Docker to run the panel | Docker as a customer feature |
|---|---|---|
| Status | **Removed** | **Kept and fully supported** |
| Where it shows in the code | `Dockerfile`, `docker-compose.yml` (deleted) | Docker screens, `/api/v1/docker/*`, `docker:*` permissions |
| Replaced by | `deploy/install.sh` + systemd | Not replaced — still a first-class feature |
| Docker Engine needed on the server? | No | Yes, but only if the customer wants to use it |

In one line: **the panel does not run in Docker, but the panel manages Docker.**

## Features

### Main areas

- **Multi-server management** — drive many nodes from one panel.
- **Websites** — PHP, Node.js, Python, reverse proxy, static sites, WordPress.
- **Databases** — MySQL, MariaDB, PostgreSQL, Redis, MongoDB.
- **SSL/TLS** — Let's Encrypt, self-signed certificates, automatic renewal.
- **DNS** — BIND and PowerDNS integration.
- **Docker** — containers, images, volumes, compose.
- **File manager** — a web editor with syntax highlighting, confined to a configurable root.
- **Cron** — scheduled tasks managed from the interface.
- **Firewall** — UFW, firewalld, CSF.
- **Backups** — automatic backups to S3, FTP, SFTP and Dropbox.
- **Deployment** — deploy from Git, with webhooks.
- **Monitoring** — live server metrics and alerting.
- **Security** — SSH key management, 2FA, IP allow lists, WAF, file tamper protection.
- **Multi-tenancy** — tenant isolation with eight RBAC roles.

### Supported web servers

Nginx, Apache, OpenLiteSpeed, LiteSpeed Enterprise, Caddy, Traefik.

> The Nginx adapter is complete; the others are scaffolded and being finished —
> see [docs/ENTERPRISE_ROADMAP.md](docs/ENTERPRISE_ROADMAP.md).

## Supported operating systems

The installer identifies the operating system from `/etc/os-release` and runs only
on the families below.

| Operating system | Recommended versions | Status | Notes |
|---|---|---|---|
| Ubuntu Server | 22.04 LTS, 24.04 LTS | Fully supported | Primary test platform |
| Ubuntu Server | 20.04 LTS | Supported | Needs the NodeSource Node.js 20 repository |
| Debian | 12 (Bookworm), 11 (Bullseye) | Fully supported | |
| Rocky Linux | 9, 8 | Supported | Uses the installer's `dnf`/`yum` path |
| AlmaLinux | 9, 8 | Supported | Uses the installer's `dnf`/`yum` path |
| RHEL | 9, 8 | Supported | Needs a valid repository subscription |
| CentOS Stream | 9 | Supported | CentOS 7 is end-of-life and unsupported |
| Any other Linux | — | Unsupported | The installer stops with a clear message |

| CPU architecture | Status |
|---|---|
| `x86_64` / `amd64` | Fully supported |
| `aarch64` / `arm64` | Supported |
| Anything else | Unsupported |

Minimum requirements: 2 vCPU, 4 GB RAM, 50 GB SSD, `root` access, and a fresh
server with no other panel installed (aaPanel, cPanel, Plesk and so on).
Recommended for production: 4 vCPU, 8 GB RAM, 100 GB SSD.

## One-line install

```bash
curl -sSL https://install.vkai.vn | sudo bash
```

The installer identifies the operating system and architecture, installs
PostgreSQL, Redis and Nginx, generates random passwords and secrets, installs the
panel binaries, creates the systemd services, and prints the access details.

Installing from source, when you need to build in place:

```bash
git clone https://github.com/hitechcloud-vietnam/vkai-panel.git
cd vkai-panel
sudo bash deploy/install.sh
```

> Read [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) carefully before installing on a
> server that is already carrying live traffic.

## First access to the panel

When the installation finishes, the panel prints the access details exactly once:

```
=============================================================================
 VKAI Panel v1.0.0 - INSTALLATION COMPLETE (fresh)
 HiTechCloud
 System  : Ubuntu 24.04.1 LTS (x86_64)
=============================================================================

PANEL ACCESS
  Full URL   : https://203.0.113.10:8888/vkai_91ac5b65/
  Port       : 8888   (80/443 stay reserved for the customer websites)
  Entrance   : /vkai_91ac5b65
  Domain     : (none - reached by IP 203.0.113.10)
  Allowed IPs: any source address
  Any other path returns a neutral 404. That is deliberate.

ADMINISTRATOR
  Username : admin
  Password : <generated>
  (!) This is the DEFAULT password - change it immediately after logging in.

CERTIFICATE
  Mode        : letsencrypt
  Source      : Let's Encrypt
  Expires     : 2026-11-26
  SHA-256     : <fingerprint>
```

(Abridged. The real summary also lists every path on disk, the systemd services,
the database and Redis, the firewall state, the update channel, and the fact that
this machine is already registered as the first managed node.)

Three things to do immediately:

1. **Open the firewall for the panel port before you close the console.**

   ```bash
   sudo ufw allow 8888/tcp                                                          # Ubuntu / Debian
   sudo firewall-cmd --permanent --add-port=8888/tcp && sudo firewall-cmd --reload  # RHEL / Rocky / Alma
   ```

2. **Save the URL together with its security entrance.** Any other path returns a
   neutral 404.
3. **Change the default administrator password** on your first sign-in.

You can print the details again at any time:

```bash
vkai panel info
```

For the full detail — changing the port or the entrance, restricting by IP, TLS,
running behind a reverse proxy — see [docs/PANEL_ACCESS.md](docs/PANEL_ACCESS.md).

## The interface

The interface is a Next.js 14 application (App Router) on a light theme, in two
brand colours, **navy `#0B398C`** and **cyan `#1791C8`**, with Inter for text and
JetBrains Mono for code and logs. The layout is a navigation sidebar on the left, a
top bar showing the selected server and notifications, and the screen's content in
the body.

Screenshots live in `docs/images/` and are embedded in the relevant documents.

| Screen | What it shows |
|---|---|
| Dashboard | Live CPU, memory, disk and bandwidth; open alerts; recent tasks |
| Servers | Node list, agent status, adding and removing servers |
| Websites | Sites, runtime type, TLS status, quick actions; WordPress management |
| Databases | Instances, databases, users, backups, a query console |
| SSL | Certificates, expiry dates, Let's Encrypt issuance and renewal |
| DNS | Zones and records |
| Docker | Containers, images, volumes, networks, compose |
| File manager | Browse, edit and transfer files within the configured root |
| Cron | Schedules, history and per-run logs |
| Security | Firewall, WAF, security scans, file protection, tamper detection |
| Monitoring and logs | Metric charts, alerts, panel and web server logs |
| Backups | Policies, restore points, remote destinations |
| Users and API keys | Accounts, RBAC roles, API keys, audit log |
| Terminal | A shell session on the selected server, in the browser |

## Architecture

```
                       Internet
                          |
        +-----------------+------------------+
        |                                    |
        v                                    v
  Ports 80 / 443                       Port 8888 (VKAI_PANEL_PORT)
  Customer websites                    Administration panel
  (nginx/apache/... vhosts)            + security entrance /vkai_xxxxxxxx
        |                                    |
        v                                    v
  /vkai-panel/www/domains/<domain>     +-----------------------------+
                                       |  vkai-api (Go, port 30110)  |
                                       |  the only way in is the     |
                                       |  entrance checked here      |
                                       +--------------+--------------+
                                                      | only past the entrance
                                                      v
                                       +-----------------------------+
                                       |  vkai-ui  (Next.js, 3000)   |
                                       +--------------+--------------+
                                                      |
                            +-------------------------+-------------------------+
                            |                         |                         |
                            v                         v                         v
                     +-------------+           +-------------+          +----------------+
                     | PostgreSQL  |           |    Redis    |          |   vkai-agent   |
                     |  port 5432  |           |  port 6379  |          |   port 30111   |
                     +-------------+           +-------------+          +----------------+
```

Port `30110` (API) and port `3000` (UI) listen on the loopback only; everything
from outside arrives on the panel port, `8888`. Nginx has exactly **one** upstream,
`vkai-api`: the security entrance is checked there, and only then does the API
forward the interface to Next.js.

### Technology

| Component | Technology |
|---|---|
| API (`core/`) | Go 1.22, Gin, JWT, pgx, go-redis, asynq |
| Interface (`panel/`) | Next.js 14, React 18, TypeScript, Tailwind CSS |
| Databases | PostgreSQL 16, Redis 7 |
| Agent (`agent/`) | A Go binary (`vkaid`) |
| Web servers | Nginx (default), Apache, OpenLiteSpeed, LiteSpeed, Caddy, Traefik |
| Process supervision | systemd — a Go binary and a Next.js standalone build, no Docker |

## Repository layout

```
vkai-panel/
├── core/                       # The Go API server (formerly backend/)
│   ├── cmd/
│   │   ├── api/                # Entry point for the vkai-api service
│   │   ├── cli/                # Administration commands
│   │   └── panelctl/           # vkai-panelctl: port, entrance, IPs, domain, certificate
│   ├── internal/
│   │   ├── acme/               # ACME (RFC 8555) client
│   │   ├── auth/               # JWT authentication
│   │   ├── config/             # Configuration, panel port and entrance
│   │   ├── database/           # Database connections
│   │   ├── handler/            # HTTP handlers
│   │   ├── middleware/         # HTTP middleware
│   │   ├── models/             # Data models
│   │   ├── rbac/               # Role-based access control
│   │   ├── repository/         # Data access
│   │   ├── service/            # Business logic
│   │   ├── terminal/           # Login shells on a pseudo-terminal
│   │   ├── utils/              # Utilities
│   │   └── webserver/          # Web server adapters
│   ├── migrations/             # SQL migrations
│   └── config.yaml             # Sample configuration
├── panel/                      # The Next.js interface (formerly frontend/)
│   ├── src/
│   │   ├── app/                # App Router
│   │   ├── components/         # React components
│   │   ├── services/           # API client
│   │   ├── store/              # Zustand stores
│   │   └── styles/             # CSS
│   └── package.json
├── agent/                      # The VKAI Agent, one per managed node
│   └── cmd/main.go
├── deploy/                     # install.sh, deploy.sh, systemd units, nginx config
│   ├── install.sh              # Multi-OS installer (installs onto the host)
│   ├── systemd/                # vkai-api.service, vkai-ui.service, vkai-agent.service
│   ├── nginx/                  # The vhost for the panel port
│   └── scripts/deploy.sh       # Release deployment and rollback
├── scripts/                    # Helper scripts
├── docs/                       # Documentation
├── setup-dev.sh                # Development environment setup
└── Makefile                    # build / test / lint / package
```

There is no `Dockerfile` and no `docker-compose.yml` in the repository: the panel is
built and run directly on the host. See
[Docker in VKAI Panel: two different roles](#docker-in-vkai-panel-two-different-roles).

> The Go import paths are **unchanged**: the module is still
> `github.com/hitechcloud-vietnam/vkai-panel` (and `.../agent`). Only the directory
> names on disk changed, from `backend/` to `core/` and `frontend/` to `panel/`.

## Standard paths on the server

| Path | Contents |
|---|---|
| `/vkai-panel/` | The panel's root after installation |
| `/vkai-panel/core/` | The API source and binary (`vkai-api`) |
| `/vkai-panel/panel/` | The interface build (`vkai-ui`) |
| `/vkai-panel/www/domains/<domain>/` | **Customer website code** |
| `/vkai-panel/www/backup/` | Website and database backups |
| `/vkai-panel/www/default/` | The default page for a vhost that matches no domain |
| `/vkai-panel/logs/` | Panel logs |
| `/vkai-panel/logs/sites/<domain>/` | Web server logs, separated per site |
| `/vkai-panel/etc/` | Panel configuration (`.env`, `config.yaml`) |
| `/vkai-panel/ssl/` | TLS certificates |
| `/vkai-panel/tmp/` | Temporary files |

The generated port and security entrance are stored in
`/vkai-panel/etc/panel_access.json`, mode `0600`. Changing `VKAI_PANEL_ROOT` moves
the whole tree above; changing `VKAI_WEB_ROOT`, `VKAI_BACKUP_ROOT`, `VKAI_LOG_ROOT`,
`VKAI_ETC_ROOT`, `VKAI_SSL_ROOT` or `VKAI_TMP_ROOT` moves only that branch — which
is how you put backups or logs on their own disk.

## systemd services

| Service | Role | Ports |
|---|---|---|
| `vkai-api` | The Go API; also serves the panel port and the security entrance | 8888 (public), 30110 (internal) |
| `vkai-ui` | The Next.js interface | 3000 (internal only) |
| `vkai-agent` | The agent on each managed node | 30111 |

```bash
sudo systemctl status vkai-api vkai-ui vkai-agent
sudo journalctl -u vkai-api -f
```

The panel runs as the system user **`vkai`**, not as `root`. `vkai-agent` is the
exception: it runs as `root` because it has to act on the system, and it is an
**optional** service. All three units are hardened: `NoNewPrivileges`,
`ProtectSystem=strict`, `ProtectHome`, `PrivateTmp`, and `ReadWritePaths` opened
only for the directories that genuinely need writing.

## Releases and deployment

A release is packaged as a `.tar.gz` and shipped to the server. Each deployment
unpacks into **its own versioned directory** and points the `current` symlink at it,
so rolling back is just moving the symlink.

```
/vkai-panel/releases/20250315_101500/    # the previous release, kept for rollback
/vkai-panel/releases/20250316_143000/    # the release just deployed
/vkai-panel/current -> /vkai-panel/releases/20250316_143000
```

`etc/`, `logs/`, `www/` and `ssl/` live **outside** the release: a deployment does
not overwrite them and a rollback does not revert them.

The package must have exactly this structure. CI builds it automatically; if you
build one by hand, follow this shape:

```
core/bin/vkai-api                    # the API binary
core/migrations/*.sql                # migrations
panel/.next/standalone/server.js     # the UI build
panel/.next/standalone/.next/static  # REQUIRED — without it the UI fails client-side
agent/bin/vkai-agent                 # optional
```

```bash
# On the build machine
make build                   # Go binaries and the Next.js standalone build
tar -czf vkai-panel-1.2.0.tar.gz -C dist .

# On the server
sudo bash deploy/scripts/deploy.sh deploy /tmp/vkai-panel-1.2.0.tar.gz
sudo bash deploy/scripts/deploy.sh list         # the releases being kept
sudo bash deploy/scripts/deploy.sh status
sudo bash deploy/scripts/deploy.sh rollback     # return to the previous release
```

`deploy` validates the package **before** it touches the running system, backs up
the database, runs the new release's migrations **before** moving the symlink, and
only then points `current` at the new release and restarts the services. The health
check covers **both the API and the interface**, and **a failure rolls back
automatically**. The running release plus the five most recent are kept.

> A rollback reverts **code only**. It does **not** revert database migrations.

See [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) for the detail.

## Day-to-day operation

```bash
# Logs
sudo journalctl -u vkai-api -f                 # follow the API
sudo journalctl -u vkai-ui -n 200 --no-pager   # last 200 lines of the interface
sudo journalctl -u vkai-agent --since "1 hour ago"
sudo journalctl -u vkai-api -p err --since today   # today's errors only

# Health
curl -fsS http://127.0.0.1:30110/health        # the API is alive
curl -fsS http://127.0.0.1:30110/ready         # ready (database and Redis connected)
curl -fsS http://127.0.0.1:3000/ -o /dev/null  # the interface responds
systemctl is-active vkai-api vkai-ui

# Roll back a release
sudo bash deploy/scripts/deploy.sh rollback
readlink -f /vkai-panel/current                # which release is running
```

## The `vkai` administration command

```bash
# Services
vkai start                  # Start vkai-api, vkai-ui and nginx
vkai stop                   # Stop vkai-ui and vkai-api
vkai restart                # Restart everything
vkai status                 # Service status and host resources

# Logs
vkai logs api               # vkai-api logs
vkai logs ui                # vkai-ui logs
vkai logs agent             # vkai-agent logs
vkai logs nginx             # web server logs
vkai logs install           # the most recent installation log

# Panel access: port and security entrance
vkai info                   # Panel URL, port, entrance and data paths
vkai port                   # Show the current port
vkai port 8888              # Change the panel port (80 and 443 are refused)
vkai port random            # A random port between 8000 and 65535
vkai entrance random        # Generate a new security entrance
vkai panel allow-ip 203.0.113.7,10.0.0.0/8
vkai panel domain panel.example.com

# The panel's own TLS certificate
vkai cert status            # Issuer, subject, expiry and days remaining
vkai cert issue             # Order a certificate from Let's Encrypt
vkai cert renew             # Renew if it is near expiry; otherwise do nothing

# Panel operations
vkai backup                 # Back up the database and configuration to /vkai-panel/www/backup
vkai update                 # Rebuild core/ and panel/, then restart the services
vkai upgrade --check        # Check whether a newer release exists (never installs)
vkai uninstall              # Uninstall

# Domain commands, delegated to vkai-cli
vkai site list
vkai site create example.com
vkai db backup
vkai db restore
vkai ssl request example.com
vkai ssl renew
vkai ssl list
vkai firewall list
vkai server status
vkai user list
```

`vkai cert renew` is what the `vkai-cert-renew` systemd timer runs twice a day. It
orders a new certificate only when the current one is near expiry, missing, or no
longer covers the configured identifier; otherwise it reports the days remaining
and exits successfully without contacting the certificate authority. A failed
renewal never removes the certificate already in place.

The older `vkai panel info` / `vkai panel port` / `vkai panel entrance` /
`vkai panel cert` forms still work, for backwards compatibility.

## Configuration

Configuration is read in increasing order of precedence: built-in defaults, then
`config.yaml`, then environment variables. **Environment variables always win.**

The configuration files are `/vkai-panel/etc/.env` and
`/vkai-panel/etc/config.yaml`, mode `0600`, owned by the `vkai` user.

### Main environment variables

Every variable is prefixed **`VKAI_`**.

| Variable | Description | Default |
|---|---|---|
| `VKAI_PANEL_PORT` | The administration panel's port. 80, 443, 22, 25, 3306, 5432 and 6379 are refused | `8888` |
| `VKAI_PANEL_BIND` | The address the panel listens on | `0.0.0.0` |
| `VKAI_PANEL_ENTRANCE` | The security entrance, for example `/vkai_a1b2c3d4`. Leave empty to generate one | (generated) |
| `VKAI_PANEL_ENTRANCE_ENABLED` | Whether the security entrance is enforced | `true` |
| `VKAI_PANEL_ALLOWED_IPS` | IP addresses and CIDRs allowed into the panel. Empty means any address | (empty) |
| `VKAI_PANEL_TRUSTED_PROXIES` | Trust `X-Forwarded-For` only from these addresses | (empty) |
| `VKAI_PANEL_DOMAIN` | Bind the panel to one domain name | (empty) |
| `VKAI_PANEL_TLS_CERT` / `VKAI_PANEL_TLS_KEY` | The panel's own TLS certificate and key | (empty) |
| `VKAI_PANEL_SESSION_TTL` | How long the entrance cookie is valid | `12h` |
| `VKAI_PANEL_CONFIG_FILE` | Where the generated port and entrance are stored | `/vkai-panel/etc/panel_access.json` |
| `VKAI_SERVER_PORT` | The internal API port | `30110` |
| `VKAI_DB_HOST` / `VKAI_DB_PORT` | PostgreSQL | `localhost` / `5432` |
| `VKAI_DB_USER` / `VKAI_DB_NAME` | Database user and database name | `vkai` / `vkai_panel` |
| `VKAI_DB_PASSWORD` | The PostgreSQL password | **required, no default** |
| `VKAI_DB_SSLMODE` | SSL mode for PostgreSQL | `require` |
| `VKAI_REDIS_HOST` / `VKAI_REDIS_PORT` | Redis | `localhost` / `6379` |
| `VKAI_JWT_SECRET` | The JWT signing key, at least 32 random characters | **required, no default** |
| `VKAI_SECRET_KEY` | The key that encrypts secrets held in the database (32 bytes, hex or base64) | **required to create or change database users** |
| `VKAI_CORS_ALLOWED_ORIGINS` | Browser origins the API accepts | (empty) |
| `VKAI_AGENT_PORT` / `VKAI_AGENT_ENROLMENT_TOKEN` | The agent control channel port, and the one-time enrolment token used only at the agent's first start. There is no shared secret: see [docs/AGENT_CHANNEL.md](docs/AGENT_CHANNEL.md) | `30111` / (empty) |

The complete list is in [`.env.example`](.env.example) and
[docs/CONFIGURATION.md](docs/CONFIGURATION.md).

### Backwards compatibility with older variable names

The panel still accepts the older names, but they are deprecated and will be
removed in a future major release. The `VKAI_`-prefixed name always wins.

| Current name | Older names still accepted |
|---|---|
| `VKAI_PANEL_PORT` | `PANEL_PORT` |
| `VKAI_PANEL_BIND` | `PANEL_BIND`, `PANEL_HOST`, `VKAI_PANEL_HOST` |
| `VKAI_PANEL_ENTRANCE` | `PANEL_ENTRANCE` |
| `VKAI_PANEL_ALLOWED_IPS` | `PANEL_ALLOWED_IPS`, `PANEL_ALLOW_IPS`, `VKAI_PANEL_ALLOW_IPS` |
| `VKAI_PANEL_TLS_CERT` / `VKAI_PANEL_TLS_KEY` | `PANEL_TLS_CERT_FILE`, `PANEL_TLS_KEY_FILE`, and their unprefixed variants |
| `VKAI_DB_HOST`, `VKAI_DB_PORT`, `VKAI_DB_USER`, `VKAI_DB_PASSWORD`, `VKAI_DB_NAME`, `VKAI_DB_SSLMODE` | `VKAI_DATABASE_HOST`, `VKAI_DATABASE_PORT`, `VKAI_DATABASE_USER`, `VKAI_DATABASE_PASSWORD`, `VKAI_DATABASE_DBNAME`, `VKAI_DATABASE_SSLMODE` |

## Development environment

```bash
git clone https://github.com/hitechcloud-vietnam/vkai-panel.git
cd vkai-panel

# Install PostgreSQL and Redis on the host, install dependencies, generate a dev .env
bash setup-dev.sh

# Terminal 1 — the API
cd core
cp ../.env.example ../.env      # fill in VKAI_DB_PASSWORD, VKAI_JWT_SECRET, VKAI_SECRET_KEY
go run ./cmd/api

# Terminal 2 — the interface
cd panel
npm install
npm run dev
```

In development the interface runs on `http://localhost:3000` and calls the API
through `NEXT_PUBLIC_API_URL`. On a real server the interface is reachable only
through the panel port, behind the security entrance.

Full instructions: [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) and
[docs/DEVELOPER_GUIDE.md](docs/DEVELOPER_GUIDE.md).

## Contributing

**Pushing straight to `main` is not allowed.** `main` is protected; every change
goes through a pull request and needs at least one approving review.

1. Branch from `main`:

   ```bash
   git checkout main
   git pull origin main
   git checkout -b feat/your-feature
   ```

   Branch naming: `feat/...`, `fix/...`, `docs/...`, `refactor/...`, `chore/...`.

2. Write commits in the Conventional Commits style:
   `feat(website): add Node.js 22 support`.

3. Run the tests and the linter before pushing:

   ```bash
   make lint
   make test
   ```

4. Push the branch and open a pull request against `main`:

   ```bash
   git push origin feat/your-feature
   ```

5. Fill in the [pull request template](.github/PULL_REQUEST_TEMPLATE.md): CI green,
   tested, screenshots for interface changes, and an assessment of the security and
   migration impact.

6. Wait for review, address the comments, then squash merge. The branch is deleted
   after merging.

See [docs/CONTRIBUTING.md](docs/CONTRIBUTING.md) and
[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

## Documentation

| Document | Contents |
|---|---|
| [docs/PANEL_ACCESS.md](docs/PANEL_ACCESS.md) | Panel port, security entrance, IP restriction, TLS |
| [docs/USER_GUIDE.md](docs/USER_GUIDE.md) | Guide for administrators |
| [docs/API.md](docs/API.md) | REST API reference |
| [docs/CONFIGURATION.md](docs/CONFIGURATION.md) | Every configuration option |
| [docs/INSTALL.md](docs/INSTALL.md) | Installation, in full |
| [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) | Deploying to a production server |
| [docs/UPGRADE.md](docs/UPGRADE.md) | Upgrading an existing installation |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | System architecture |
| [docs/AGENT_CHANNEL.md](docs/AGENT_CHANNEL.md) | The panel-to-agent channel and enrolment |
| [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) | Setting up a development environment |
| [docs/DEVELOPER_GUIDE.md](docs/DEVELOPER_GUIDE.md) | Guide for developers |
| [docs/TESTING.md](docs/TESTING.md) | Testing strategy |
| [docs/SECURITY.md](docs/SECURITY.md) | Operational security guidance |
| [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md) | Troubleshooting |
| [docs/FAQ.md](docs/FAQ.md) | Frequently asked questions |
| [docs/ROADMAP.md](docs/ROADMAP.md) · [docs/ENTERPRISE_ROADMAP.md](docs/ENTERPRISE_ROADMAP.md) | Roadmap |
| [CHANGELOG.md](CHANGELOG.md) | Change history |

## Licence and support

Released under the MIT Licence. Copyright (c) 2024 HiTechCloud Vietnam. See
[LICENSE](LICENSE).

- Website: https://hitechcloud.vn
- Documentation: https://docs.vkai.vn
- Bug reports: https://github.com/hitechcloud-vietnam/vkai-panel/issues
- Discussions: https://github.com/hitechcloud-vietnam/vkai-panel/discussions
- Security vulnerabilities: [SECURITY.md](SECURITY.md) — please do **not** open a
  public issue
- Support email: support@hitechcloud.vn
