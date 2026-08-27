# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Changed - whitelabel to VKAI Panel (HiTech Cloud)

- Product name is **VKAI Panel**, vendor **HiTech Cloud** (hitechcloud.vn).
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
