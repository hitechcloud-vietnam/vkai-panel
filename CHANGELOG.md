# Changelog

All notable changes to this project will be documented in this file.

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
