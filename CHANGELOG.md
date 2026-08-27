# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Removed - Docker is no longer how the panel is built or run

The panel now builds and runs **bare-metal** on Linux: a Go binary for the API, a
Next.js standalone build for the UI, a Go binary for the agent, all supervised by
systemd. PostgreSQL, Redis and nginx are installed natively by the system package
manager. The host running the panel no longer needs Docker Engine.

Removed from the repository:

- `Dockerfile` at the repository root and in `core/`, `panel/` and `agent/`.
- `docker-compose.yml`, `docker-compose.dev.yml`, the root `.dockerignore` and
  the per-component `.dockerignore` files.
- The `docker/` directory, which only ever served the container build of the
  panel.
- The "Docker Build" job and every `docker/*` step in the GitHub Actions
  workflows.
- All documentation describing how to install or run the panel with `docker` or
  `docker compose`.

What replaces them:

- **Installation**: one command through `deploy/install.sh`, which detects the OS
  family and installs PostgreSQL, Redis and nginx natively.
- **Development**: `setup-dev.sh` installs the databases natively and prepares
  the dev `.env` files; no container runtime in the development loop.
- **Deployment**: a packaged `.tar.gz` release unpacked into
  `/vkai-panel/releases/<version>/` with a `current` symlink naming the live
  release, driven by `deploy/scripts/deploy.sh` (validate, back up, migrate,
  switch, restart, health-check, auto-rollback on failure).

> **Docker management remains a full product feature.** This change removes only
> the use of Docker *to build and run the panel itself*. The Docker screen in the
> UI, the `/api/v1/docker/*` API, the Docker service layer and the `docker:*`
> RBAC permissions are all unchanged, and customers continue to manage their own
> containers, images, volumes, networks and compose stacks from the panel. In
> short: the panel does not run in Docker, the panel manages Docker. Install
> Docker Engine on the host only if you intend to use that feature.

### Documentation - bare-metal build and run model

- `README.md` gained a prominent section distinguishing the two meanings of
  "Docker" in this project, plus sections on release deployment and day-to-day
  operations. The source tree listing no longer shows `Dockerfile`,
  `docker-compose.yml` or `docker/`.
- `docs/ARCHITECTURE.md` documents the release-directory layout
  (`/vkai-panel/releases/<version>` plus the `current` symlink), the shared state
  that lives outside releases, the corrected `vkai-ui` unit (Next.js standalone
  started by `node`, not `npm run start`), and the nginx panel vhost.
- `docs/DEPLOYMENT.md` gained "Release Deployment and Rollback" and "Operations":
  package layout, the exact deploy sequence, rollback semantics and the fact that
  database migrations are never reversed, reading logs with `journalctl` for
  `vkai-api` / `vkai-ui` / `vkai-agent`, the `/health`, `/ready` and `/live`
  endpoints, and a quick incident lookup table.
- `docs/DEVELOPMENT.md`, `docs/DEVELOPER_GUIDE.md` and `docs/CONTRIBUTING.md`
  describe the development databases as natively installed via `setup-dev.sh`.
- `docs/PANEL_ACCESS.md` replaced its Docker Compose section with an internal
  ports and systemd section.
- `docs/SECURITY.md` replaced the container hardening guidance with the systemd
  sandboxing actually shipped in `deploy/systemd/`, and notes that `docker:*`
  permissions are root-equivalent on the host.
- `docs/TESTING.md` describes test databases on the locally installed PostgreSQL
  and Redis instead of a `docker-compose.test.yml`.
- `deploy/README.md` and `docs/FAQ.md` state the two roles of Docker explicitly
  so the change is not misread as dropping container management.

### Changed - whitelabel to VKAI Panel (HiTechCloud)

- Product name is **VKAI Panel**, vendor **HiTechCloud** (hitechcloud.vn).
- Source directories renamed: `backend/` is now `core/`, `frontend/` is now
  `panel/`. `agent/` is unchanged. **Go module paths are unchanged**
  (`github.com/hitechcloud-vietnam/vkai-panel`), so no import needs editing.
- Installed layout moved off `/opt/vkai-panel` and `/var/www`:

  | Path | Contents |
  |------|----------|
  | `/vkai-panel/` | Installation root (`VKAI_PANEL_ROOT`) |
  | `/vkai-panel/core/` | API code and binaries |
  | `/vkai-panel/panel/` | Built UI |
  | `/vkai-panel/www/domains/<domain>/` | Customer website document roots |
  | `/vkai-panel/www/backup/` | Website and database backups |
  | `/vkai-panel/www/default/` | Catch-all vhost |
  | `/vkai-panel/logs/` | Panel logs |
  | `/vkai-panel/logs/sites/<domain>/` | Per-site web server logs |
  | `/vkai-panel/etc/` | `.env`, `config.yaml`, `panel_access.json` |
  | `/vkai-panel/ssl/` | Certificates (`ssl/panel/` for the panel itself) |
  | `/vkai-panel/tmp/` | Panel-owned scratch space |

- Systemd units are `vkai-api`, `vkai-ui` and `vkai-agent`; the admin command is
  `vkai`. The former `vkai-frontend` / `vkai-panel-api` / `vkai-panel-frontend`
  unit names are gone.
- Every environment variable carries the `VKAI_` prefix
  (`VKAI_PANEL_PORT`, `VKAI_PANEL_ENTRANCE`, `VKAI_PANEL_BIND`, `VKAI_DB_HOST`,
  ...). The pre-whitelabel names are still read for backward compatibility and
  the `VKAI_` form wins when both are set; the legacy names are deprecated.
- Documentation states explicitly that the panel runs on its own port
  (default `8888`) behind a security entrance and never on 80/443, which are
  reserved for the customer websites.

### Documentation

- README rewritten: product introduction, supported OS matrix, one-line
  installer, interface overview, new directory layout, standard server paths,
  and the contribution rules (side branch plus Pull Request, direct pushes to
  `main` forbidden).
- Pull request template rewritten in Vietnamese with a required checklist:
  green CI, no direct push to `main`, evidence of testing, UI screenshots,
  security impact, migration impact.
- Removed references to `cmd/migrate`, which has never existed; migrations are
  the SQL files in `core/migrations/`, applied with `make migrate`.

## [0.2.1] - 2026-08-27

### Fixed
- Fix Gin route wildcard conflicts (zoneId→id, scanId→id, phpVersionId→id)
- Fix migration 012/013/014 permissions INSERT to use (resource, action) columns
- Add pgx/v5/stdlib driver import for database connection
- Fix DNS handler to use consistent parameter names
- Fix security handler to use consistent parameter names
- Fix PHP handler to use consistent parameter names

## [0.2.0] - 2026-08-27

### Added
- Phase 8: WebSocket real-time communication
- Phase 9: Job queue system with asynq
- Phase 10: Config rollback management
- Phase 11: Node.js app systemd integration
- Phase 12: CI/CD pipeline with GitHub Actions

### Fixed
- Go compilation errors (duplicate types, missing imports)
- Frontend build errors (missing UI components)
- TypeScript errors in Sidebar component
- Lumberjack import typo

## [0.1.0] - 2026-08-26

### Added
- Initial release with Phases 1-7
- Multi-tenant architecture
- RBAC with 8 roles
- DNS management
- SSL certificate management
- PHP version management
- Node.js app management
- Reverse proxy management
- Git deployment management
- WordPress management
- Security scanning
- Monitoring and alerting
- Log management
- Notification system
- Audit logging
- Cluster and HA management
