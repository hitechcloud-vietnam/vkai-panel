# VKAI Panel Roadmap

**Last verified:** 28 August 2026, against working tree at commit `0116f53`.
**Product version:** `0.5.0` (the `VERSION` file at the repository root is the
single source of truth).
**Companion document:** [RELEASE_READINESS.md](RELEASE_READINESS.md) answers the
question this roadmap does not: *can we release 1.0 today?*

---

## How to read this document

Every item carries one of three marks. The mark is a claim about the shipped
product, not about how much code was written.

| Mark | Meaning |
|---|---|
| `- [x]` | **Done.** The code exists, it is reachable in the shipped binary (a route mounted in `core/internal/handler/router.go` or a `Register*Routes` function that `router.go` or `cmd/api/main.go` calls, a registered CLI command, or a UI component a real page imports), and it changes something outside the database. |
| `- [~]` | **Partial.** The substance exists but something real is missing. The note says exactly what. |
| `- [ ]` | **Not started.** No implementation, or an implementation that only writes database rows for a feature whose whole point is to act on a server. |

Three rules were applied while marking, because this project has been bitten by
each of them:

1. **Code that nothing imports or mounts is not done.** `SiteManagePanel.tsx`
   (57 KB) is still imported by no page. `core/internal/upgrade/` is a complete,
   tested upgrade engine that no file in the module imports.
2. **A route that returns success without acting is worse than a missing
   route.** `internal/job/queue.go` has nine task handlers that `time.Sleep`
   and return `nil`; the worker that runs them is started in `main.go`.
3. **"There is a table for it" is not "it works."** A load balancer row with
   `status = "active"` and no HAProxy configuration behind it is a lie the
   product tells its operator.

Where a claim could not be settled, it says so and names what would settle it.

---

## Correction of the record

The previous version of this file was substantially fictional. It is recorded
here rather than quietly deleted, because the same errors are still repeated in
other documents.

| Previous claim | Reality |
|---|---|
| "Version 1.0.0 (Stable), Release Date Q1 2024, Status: Released" | No 1.0.0 has ever been released. `VERSION` says `0.5.0`. The first CHANGELOG entry is `0.1.0`, dated 26 August 2026. |
| Milestones for "Q2 2024", "Q3 2024", "Q4 2024", "2025" | All in the past by up to two and a half years. This document now uses ordered milestones with no invented dates. |
| 62 unchecked items, 0 checked | Roughly a third of them were already built and shipping. See below. |
| "PHP multi-version management — Status: In Progress" | Real host-level install/pool/version-switch machinery exists and is mounted. Shipped. |
| "Node.js application support — Planned", "with PM2 process management" | Node.js apps ship, driven by **systemd**, not PM2. There is no PM2 code anywhere in the repository. The promise of PM2 should be withdrawn, not deferred. |
| "Monitoring dashboard — Planned" | Collection ships and runs. Alerting exists but delivers to nobody. |
| "Audit logging — Planned" | A cryptographic hash chain enforced by PostgreSQL triggers ships, with an independent offline verifier. It covers only eight call sites. |
| `**Last Updated**: $(date)` | An unexpanded shell substitution had been committed and published. |
| A feedback survey at `https://forms.gle/xxx` | Not a real URL. Removed. |

---

## Where the product actually is

**Verified facts, 28 August 2026.**

- `go build ./...`, `go vet ./...` and `go test ./...` all pass in `core/`
  (132 test files, every package `ok`).
- `panel/` has **zero** tests: `package.json` defines
  `"test": "echo \"no frontend tests yet\" && exit 0"`. The frontend CI test job
  therefore always passes and proves nothing.
- 613 route registrations exist across `core/internal/handler/`, 320 of them
  parameterised. Not all are reachable: at least one registrar
  (`RegisterMultiWebServerRoutes`) is called from nowhere while its UI page
  ships.
- The demo host serves a valid Let's Encrypt certificate for **`control.vkai.vn`**
  (issuer `Let's Encrypt YE1`, valid 27 Aug 2026 – 25 Nov 2026), verified with
  `curl` without `-k`. Two corrections to the record: the host is
  `control.vkai.vn`, not `panel.vkai.vn`; and the certificate is managed by
  **certbot** through a hand-written nginx vhost, *not* by the panel's own ACME
  client or by `deploy/install.sh`. The panel's own certificate path is
  therefore unproven in production.
- The demo host does **not** run the code in this repository. Its API binary
  (`/opt/vkai-panel/api`, started 02:18 today) answers `404` on `/api/v1/version`
  and emits `Access-Control-Allow-Origin: *`, neither of which matches current
  source. Its systemd units are `vkai-panel-api.service` /
  `vkai-panel-frontend.service`, which are not the units in `deploy/systemd/`.
  Nothing about the demo host validates the product's own install path.
- The repository is **public** (`gh repo view` reports `"visibility":"PUBLIC"`),
  and source encryption is armed but never applied: `vkai-crypt status` reports
  **0 files encrypted in the repository, 759 in scope and still plaintext**.

---

## 1. Shipped

Each of these is reachable and does real work on the host.

### Platform and access

- [x] **Multi-tenant data model with `tenant_id` on every table.** `core/internal/models/`, enforced in the repository layer. Caveat: enforcement is by convention, not by construction — see §3.
- [x] **JWT authentication with refresh tokens.** `core/internal/auth/jwt.go`, `middleware/auth.go`; `POST /api/v1/auth/login|refresh|logout`.
- [x] **RBAC, deny by default.** `core/internal/middleware/rbac.go` (`RequirePermission`, `RequireExactPermission`, `RequireAdmin`); applied to every route group in `router.go`. No tests exist for `internal/rbac`.
- [x] **Panel access guard: host pinning, IP allow list, secret entrance path.** `core/internal/middleware/panel_access.go`, wrapped around the whole engine (including the Next.js UI) by `reload.NewGuardSwitch` at `cmd/api/main.go:665`. CLI: `vkai panel info|port|entrance|allow-ip|domain`.
- [x] **Session binding to device and network.** `middleware.BindSessions`, `core/internal/auth/sessionbinding.go`, installed engine-wide at `cmd/api/main.go:616`. Tested.
- [x] **Layered brute-force defence.** `core/internal/ratelimit/` (Redis-backed, per address+account, per address, per account), installed engine-wide as `middleware.ProtectCredentialEndpoints` at `router.go:245`, plus a global 600/min per-IP limiter at `router.go:232`. Fails **closed**: if Redis is down, nobody can sign in. Extensive tests, including tests that read the shipped fail2ban filter and match it against real log lines.
- [x] **Scoped API keys with rotation and revocation.** `core/internal/service/apikey.go`; `POST /api-keys/:id/rotate|revoke` and `GET /access/scopes` mounted by `RegisterAccessRoutes` at `cmd/api/main.go:530`. Keys are peppered HMAC-SHA256; plaintext rows are refused. `TestIntegrationRoutesAllDeclareAScope` keeps the scoped surface honest.
- [x] **mTLS between panel and agent, with certificate rotation and revocation.** `core/internal/agentpki/`, `agent/internal/pki/`. Mutual verification checks chain, role OID and a pushed deny list; renewal proves possession of the current key and keeps an overlap window so an interrupted rotation cannot brick an agent. Mounted at `router.go:1120`. The two old unauthenticated `/agent/heartbeat` and `/agent/register` stubs were deleted, not left in place.
- [x] **Security headers on the API.** `middleware.SecurityHeaders()` at `router.go:230`: CSP, `frame-ancestors 'none'`, `nosniff`, COOP/CORP, `Permissions-Policy`, HSTS over real TLS. Not tested — see §3 for the UI, which has no CSP.
- [x] **CORS restricted to an allowlist.** `middleware.CORS`; no wildcard, no reflection of unknown origins.
- [x] **Parameterised SQL throughout.** `jmoiron/sqlx` with `$N` placeholders; the nine `fmt.Sprintf` sites near SQL build placeholder indices, never interpolate caller data; identifiers go through `utils.QuoteSQLIdentifier`.

### Hosting features

- [x] **Websites, domains and vhosts on nginx.** `core/internal/service/website.go`, `core/internal/webserver/nginx.go` — writes the vhost and runs `systemctl reload nginx`.
- [x] **SSL: Let's Encrypt issuance and custom certificate upload.** `service/ssl.go`, `handler/ssl.go`; `/api/v1/ssl/*`.
- [x] **The panel's own certificate, from the command line.** `vkai panel cert issue|renew|status` — `core/internal/cli/cert.go`, registered at `core/internal/cli/panel.go:45`, with `cert_test.go`. This did not exist before today. The `vkai-cert-renew` timer is reported to have been failing on every run with `unknown flag: --identifier`; that could not be confirmed here, because the units in `deploy/systemd/` are not installed on the demo host at all (see below). Running the timer once on a freshly installed machine would settle it.
- [x] **Databases: MySQL/MariaDB, PostgreSQL, Redis.** `service/database.go`. Stored credentials are AES-encrypted and fail closed with no key.
- [x] **DNS zones and records.** `service/dns.go`; `/api/v1/dns/*`.
- [x] **File manager.** `service/file_manager.go` with traversal defence and denied roots; 12 routes at `router.go:478`; UI at `panel/src/app/(dashboard)/files/`. `Compress`, `Extract` and `ChangeOwner` exist in the service with no route.
- [x] **Cron jobs.** `service/cron.go`, quota-enforced.
- [x] **Firewall (iptables/UFW).** `service/firewall.go`.
- [x] **systemd service management.** `service/service_manager.go`; start/stop/restart/enable/disable/logs.
- [x] **Docker: containers, images, networks, volumes, compose.** `core/internal/docker/` speaks HTTP over the Docker socket rather than shelling out. Two route sets, the second registered at `cmd/api/main.go:554`; a test asserts that registration line still exists and names what goes dark if it is removed. Full UI.
- [x] **PHP multi-version management, for real.** `core/internal/phpfpm/` installs and removes PHP from Ondrej/Remi, writes FPM pool files, validates with `php-fpm -t`, reloads, and rolls back a failed reload. `service/php_runtime.go`; routes `GET /php/system`, `POST /php/install|uninstall`, `GET|PUT /php/pools/:id/settings`, `GET|PUT /php/sites/:website_id/version` mounted via `RegisterPHPWordPressRuntimeRoutes`. UI: `app-store/php`.
- [x] **WordPress installation, WP-CLI and staging.** `core/internal/wpcli/` runs `wp` as the site's own user, refuses to run as root, and rejects shell metacharacters. `service/wordpress_runtime.go` performs a genuine `wp core install`; `staging.go` clones, `search-replace`s and pushes back with an explicit database decision. UI: `wp-toolkit`, `wp-toolkit/add`, `wp-toolkit/data-copy`. The best-tested area of the codebase.
- [x] **App Store for OS packages.** `core/internal/appstore/` — a catalogue (currently 13 applications; only PHP offers a version choice), a plan/preview step, a job runner, real `apt-get`/`dnf` execution. `RegisterAppStoreRoutes` at `router.go:1155`; UI at `app-store/`. **This is being extended by other work in progress; treat the count as a snapshot, not a commitment.**
- [x] **Hosting packages and quota enforcement.** `core/internal/quota/` — `Enforcer.Check` is called *before* creation in `website.go` (sites and subdomains), `database.go`, `cron.go` and `mail_server.go`, and is constructor-injected so a new call site cannot forget it. A sampler goroutine runs from `main.go:182`; over-quota tenants can have their vhosts disabled. Only layer (a) of three — see §2.
- [x] **Web terminal with a real shell.** `core/internal/terminal/` allocates a PTY by hand (`/dev/ptmx`, `TIOCSPTLCK`, `TIOCGPTN`) and runs a login shell with a controlling TTY; `handler/terminal_ws.go` bridges it to the browser with base64 framing. Access requires a single-use ticket with a 30-second TTL, bound to its session, and an administrator check performed *before* the socket upgrade. Non-Linux builds return `ErrUnsupported` rather than pretending. Tested at every layer, including a test that the old broken `/api/ws` path is not aliased back. Before today there was no PTY behind this socket at all.
- [x] **Metrics collection.** `core/internal/collector/` — polls enrolled agents over mTLS and samples the local host, started at `cmd/api/main.go:582`. Four metrics: `cpu_percent`, `ram_used_percent`, `disk_used_percent`, `load1`.
- [x] **Tamper-evident audit chain.** `migrations/pending/audit_chain.sql` maintains a SHA-256 chain by database trigger, revokes `UPDATE`/`DELETE`/`TRUNCATE`, and seals checkpoints; `core/internal/audit/` re-derives the canonical bytes independently, and a standalone Python verifier is served at `GET /audit/chain/verifier` so a third party can check an export without trusting the panel. Tampering answers `409`. Mounted at `router.go:1152`.
- [x] **Offsite backup engine.** `core/internal/backup/` — tar.gz with a manifest and SHA-256, client-side AES-256-GCM with a wrapped data key, hand-rolled SigV4 for S3-compatible storage, extraction with path-traversal defence, retention, and `verify.go`, which performs a genuine restore into a scratch directory, re-hashes every file and imports the SQL dump. Mounted at `router.go:1153`. It has no UI and nothing schedules it — see §2.
- [x] **Configuration snapshots and rollback.** `service/config.go`; `/api/v1/config/*`.
- [x] **Recovery CLI.** `cmd/panelctl` plus `vkai user reset-password`, both talking straight to PostgreSQL, so an operator locked out of the web panel has a way back in.
- [x] **One-command installer.** `deploy/install.sh` (3,245 lines): distro and architecture detection, preflight checks on RAM/disk/ports/other panels/SELinux, idempotent re-runs, randomly generated database password, JWT secret, agent token and a 20-character admin password, an install log, and `--uninstall [--purge]`. Two caveats in §2.

---

## 2. Partial

The substance exists. What is missing is named exactly.

- [~] **Node.js applications.** `service/nodeapp.go` and `core/internal/nodeapp/systemd.go` really install, start, stop, restart and tail a systemd unit with `Restart=always`. Missing: **no PM2** (the roadmap promised it; nothing in the tree mentions it); **no Node version management** (`NodeApp.NodeVersion` is a recorded string, `systemd.go` hardcodes `/usr/bin/node`); **no build step** (nothing runs `npm run build` or `npm install` — "dependencies" are rows); units run as a hardcoded `www-data`, not the site owner. No tests in `internal/nodeapp` at all.
- [~] **Git deployment.** `service/gitdeployment.go` genuinely runs pre-hook, `git pull`, deploy script, post-hook and `git rev-parse`. Missing: **no webhook receive endpoint anywhere** (`WebhookSecret` and `WebhookURL` are stored and verified by nothing, so `auto_deploy` cannot fire); **no rollback**; **no initial `git clone`**, so the first deploy fails on any path that is not already a repository; `DeployKey` is never used, so private repositories cannot work; each deploy writes two history rows instead of updating one. Hooks run through `bash -c` as the panel process. No tests.
- [~] **WordPress management.** Install, staging, and live plugin/theme/core *updates* are real (see §1). Missing: **there is no plugin or theme install endpoint at all**; the legacy `POST /wordpress/:id/plugins` and `/themes` routes at `router.go:683` write a row, log "WordPress plugin installed", and install nothing — they should be removed or made real before release. Three of seven wp-toolkit pages (`migrate`, `panel-migrate`, `sets`) render an honest "not built yet" card naming the missing routes. No core write-lock, no automatic security updates, no vulnerable-plugin scanning.
- [~] **Monitoring.** Collection runs (§1) and the monitoring page shows real data with no mock values. Missing: **alerts reach nobody.** `service/monitoring.go:320` compares thresholds, writes a `monitoring_alert_logs` row and a `zap.Warn`, and never calls the notification service. No process monitoring, no website-uptime monitoring, no service monitoring in the collector. The collector's own `/sample`, `/series` and `/system/uptime` endpoints exist, are mounted at `cmd/api/main.go:571`, and are called by no UI code — the header still renders an em dash for uptime. `MonitoringService` and its alert evaluation have zero tests.
- [~] **Notifications.** Four real transports with retry, dead-lettering, deduplication, quiet windows and secret redaction: `core/internal/notify/{email,telegram,zalo,webhook}.go`, well tested. Missing: **`NotificationService.AttachDelivery` and `RunDispatcher` are never called outside tests**, so `Notify` returns `ErrDeliveryNotConfigured` on every call and nothing drains the outbox; and **no subsystem publishes events** — the only caller of `Notify` is a manual HTTP endpoint. The panel today can send exactly zero messages. There is no Slack channel (the roadmap named one); Slack would go through the generic webhook sender with no Slack payload formatting. The UI never reaches channel create/update/delete or preferences.
- [~] **Log management.** Search and filtering are real and the UI is substantial. Missing: **no live tail** (the UI polls; there is no SSE or log websocket in the Go code); **no server-side download** (the UI builds a Blob from rows already fetched); **rotation does nothing** — `log_rotations` stores `MaxSizeMB`, `MaxAgeDays`, `MaxFiles`, `CompressOld` and no code ever reads those rows. No tests for `LogService` or `LogHandler`.
- [~] **Audit logging.** The chain is genuinely strong (§1). Missing: **there is no audit middleware, and only eight call sites write audit rows** — sign-in, sessions, API keys, panel settings, upgrade and two-factor. Creating or deleting a website, database, backup job, cluster, firewall rule, SSL certificate or Docker container leaves no audit row, and neither does triggering a failover. A hash chain over a fraction of the write surface is a login log with a hash chain, not a compliance audit trail.
- [~] **Backup.** The engine and restore verification are excellent (§1). Missing: **there is no backup UI** — `backups/page.tsx` is 35 lines of static markup whose "Create Backup" button has no handler, and `backupApi` in `services/api.ts` is imported by nothing; **nothing is scheduled** — `BackupJob.Schedule` is stored and never read, and `StartRestorabilityChecks`, whose own comment says it is one line in `main.go`, is called from nowhere; `SetLogger`, `AttachJobQueue` and `SetDatabaseImporter` are likewise never called, so offsite backups log to a no-op logger and never appear under `/jobs`; **S3 and local only** — no Google Drive, no FTP/SFTP, no incremental backup; restore refuses a remote target node; there is no automatic pre-overwrite snapshot; and `service/backup.go:253 RestoreBackup` still returns `"restore not yet implemented"` (unmounted, at least).
- [~] **Two-factor authentication.** Enrolment, TOTP, bcrypt-hashed recovery codes, an AES-GCM-sealed secret, rate limiting, routes and a settings page are all real and tested. Missing, and severe: **`AuthService.SetTwoFactor` is never called outside tests**, so the login gate's verifier is `nil`. Enrolment sets `users.mfa_enabled = true`, and the login path then fails closed with "sign-in is temporarily unavailable for this account". **Any operator who enrols in 2FA on a shipped build is locked out.** One line in `cmd/api/main.go` fixes it. There is also no policy to *require* 2FA — it is strictly opt-in per user.
- [~] **Internationalisation.** The machinery is good: `panel/src/i18n/` with `useT`/`useTn`/`useLocale`, `vi-VN` formatters, missing-key fallback, a language switcher. `en.json` and `vi.json` are exactly in step at 278 keys each. Missing: **only 7 of 51 pages import the layer at all**, and only 2 of 49 dashboard pages; the other 44 hardcode English. The default locale is `en`, not `vi`, which contradicts the product's stated positioning; the choice is stored in `localStorage` and not on the user profile; and there is no backend error-code lookup table.
- [~] **Hosting packages and quota.** Layer (a), the pre-creation check, is real and enforced (§1). Missing: layer (b), filesystem quota (`setquota`/`repquota` appear nowhere), and layer (c), cgroup v2 / `MemoryMax` / `LimitNPROC` (likewise nowhere). And there is **no UI** — nothing under `panel/src` calls `/packages` or `/quota`.
- [~] **Tamper-proof file integrity.** `service/tamper_proof.go` really walks trees, computes SHA-256 baselines, detects changes, raises alerts and writes its own audit rows. Missing: it runs **only on demand** — nothing schedules `ScanAll` — and it has **no test file at all**.
- [~] **Public API.** `docs/API.md` is hand-written and substantial, and the scoped `/api/v1/integration/*` surface behind `APIKeyAuth` + `RequireScope` is real. Missing: **there is no OpenAPI specification anywhere in the repository**, and there are **no outbound event webhooks** (the only webhook senders are the alert channel and git-deploy hooks).
- [~] **Multi-node.** Real: agent enrolment over mTLS, per-node metric samples, `internal/localnode` registering the panel host as the first managed node, and a `vkai node` CLI. Missing: **no host-changing operation is ever sent to an agent.** The agent's entire capability set is `system.info`, `system.metrics`, `service.list/status/control`, `log.list/read`, `disk.usage`, `agent.info`, `pki.sync`. `WebsiteService.Create` ignores `server.ID` and always acts on the panel host; restore explicitly refuses a target node. **The panel is effectively single-node**, and every roadmap item that assumes remote execution is blocked behind extending `agent/internal/ops`.
- [~] **Installer.** Real and thorough (§1). Two caveats: there is no separate `deploy/uninstall.sh` (it is a `--uninstall` flag, which is fine but contradicts the enterprise roadmap text); and if neither a Go toolchain nor `python3-bcrypt` is available to compute a hash, the installer *warns* and leaves the seeded `admin/admin123` account in force. There is no forced password change on first login — `must_change_password` does not exist.
- [~] **Source protection.** `tools/protect/` is written and documented: `vkai-crypt`, a Git clean/smudge filter using AES-256-CTR + HMAC-SHA256 with a deterministic IV, and `build-protected.sh`, a `garble`-based release obfuscator. The filter is *configured* on this machine (`filter.vkai-crypt.clean`, `.required true`) and `.gitattributes` claims 759 files. Neither layer is active: `vkai-crypt status` reports **0 encrypted, 759 plaintext**, and `grep` finds no reference to `build-protected.sh` or `garble` in the `Makefile` or in any CI workflow.
- [~] **CI.** `.github/workflows/ci.yml` builds and tests `core/`, `panel/` and `agent/` against real PostgreSQL 16 and Redis 7, and there is a version-sync check. But `golangci-lint` is **non-blocking** with 113 pre-existing findings, and the frontend "test" step runs an `echo`.

---

## 3. Not started

- [ ] **Reverse proxy that proxies anything.** `service/reverseproxy.go` is 218 lines of pure CRUD. Nothing writes a proxy configuration and nothing reloads a web server; the model's `LoadBalancer`, `HealthCheck`, `WebSocket` and SSL-termination fields have no consumer. To its credit, `ProxyProjects.tsx` says so on screen: *"A proxy created here is recorded, not yet served."* The route group is mounted, so the CRUD works — as CRUD.
- [ ] **Cluster support, load balancing, high availability and failover.** `service/cluster.go` is 226 lines of pass-through. `grep` for `haproxy` or `keepalived` across `core/` and `panel/` finds nothing outside an unrelated test string. `TriggerFailover` swaps two columns in the `ha_pairs` row and sets `status = "failed-over"`; no VIP moves, no daemon is signalled, no health check runs — and the panel then displays the wrong machine as primary. `POST /api/v1/ha-pairs/:id/failover` returns `200`. The clusters page is read-only despite importing create/delete icons. No tests.
- [ ] **Website malware scanning.** `service/security.go:357 runScan` sleeps five seconds and writes `Score = 85, TotalChecks = 50, PassedChecks = 42`. `POST /api/v1/security/scans` is mounted. An operator reads a clean bill of health from a scan that examined nothing. There is no antivirus integration of any kind (`clamav`, `maldet`, `rkhunter`, `yara`: zero hits).
- [ ] **File protection / quarantine.** `service/file_protection.go` is fourteen CRUD methods over `rules`, `events` and `quarantine`. There is no watcher, no scanner and no quarantine mover; nothing ever writes an event or a quarantine item. The routes are mounted and a complete UI page ships. The tables will stay empty forever.
- [ ] **Job queue execution.** Nine handlers in `internal/job/queue.go` — backup, restore, deploy, ssl, cleanup, health check, metrics, log rotation, notification — are `// TODO` + `time.Sleep` + `return nil`. `cmd/api/main.go:281` starts the worker, so these jobs report **success** having done nothing, on live routes under `/api/v1/jobs`.
- [ ] **Upgrade engine, as shipped.** `core/internal/upgrade/` is a complete, documented, tested in-place upgrade engine — releases directory, staging, preflight, database dump, symlink promotion, rollback, lock, prune. **Nothing in the module imports it.** `internal/cli/upgrade.go` returns `stubUpgrader{}` at the line marked `SWAP HERE`, and `router.go:193` constructs `service.NewUpgradeService(nil, ...)`. Both halves of the upgrade path are dead while the engine sits finished next to them.
- [ ] **Multi-web-server management.** `RegisterMultiWebServerRoutes` (`handler/webserver.go:140`) is called from nowhere, yet `panel/src/app/(dashboard)/multi-webserver/page.tsx` ships and its endpoints answer 404. The adapters for Apache, Caddy, OpenLiteSpeed, LiteSpeed and Traefik exist in skeleton with TODOs; only nginx is complete.
- [ ] **FTP accounts.** No FTP route, handler, service or model exists. The FTP page is honest about it and wires what is real instead: systemd control of the daemon and firewall rules. Accounts, quotas and chroot: nothing.
- [ ] **Reseller / account hierarchy.** No `parent_tenant_id`; `migrations/pending/packages.sql` says outright that the reseller hierarchy is not built here.
- [ ] **cPanel / DirectAdmin migration.** A "not built yet" screen naming the endpoints that would be needed.
- [ ] **Billing and invoicing (WHMCS or otherwise).** No code.
- [ ] **Full business email.** `service/mail_dns.go` is real — live SPF/DKIM/DMARC/rDNS and blocklist checks. `service/mail_server.go` contains **zero** `exec.Command`: no Postfix or Dovecot configuration is ever written. See RELEASE_READINESS.md for two security defects in this area.
- [ ] **Public status page and SLA measurement.**
- [ ] **CDN, caching and acceleration.** No `fastcgi_cache`, `proxy_cache`, `varnish` or CDN integration.
- [ ] **Third-party plugin system.** No SDK, loader or manifest. (The App Store, which does ship, installs OS packages — a different thing.)
- [ ] **Mobile application and push notifications.**
- [ ] **Compliance reporting and certification.** The audit chain is the only compliance-adjacent artefact.
- [ ] **Python application manager.**
- [ ] **Emergency break-glass access.** `panelctl` and `vkai user reset-password` cover local recovery, but there is no one-time out-of-band token.
- [ ] **CSRF protection.** No middleware exists. This is **deliberate and correct** as currently architected: the API takes a Bearer token in the `Authorization` header, a cross-site request cannot set that header, and CORS reflects only an allowlist. It is listed here so that anyone who later moves the session to a cookie knows the guard is missing.
- [ ] **AI-assisted operations, predictive scaling, self-healing, blockchain, edge computing.** These were listed as "3.0 vision" in the previous roadmap. Nothing exists, and nothing should until the items above are true.

---

## Ordered milestones

No dates. Every date in the previous version of this document was wrong, and a
project that has missed every date it published should earn the right to publish
another one. Milestones are ordered; each is complete when everything in it is
`[x]` and the verification checklist in
[RELEASE_READINESS.md](RELEASE_READINESS.md) passes.

### Milestone A — Truthfulness (blocks any public release)

Nothing here is a new feature. It is the removal of claims the product cannot
back, and the connection of finished code to the process that runs it.

- [ ] Remove or make honest every endpoint that reports success without acting: the nine job handlers, the security scanner's hardcoded score, the WordPress plugin/theme install routes, the reverse-proxy and cluster/HA "active" statuses.
- [ ] Wire the six missing lines in `cmd/api/main.go`: `SetTwoFactor`, `AttachDelivery`, `RunDispatcher`, `StartRestorabilityChecks`, `SetLogger`/`AttachJobQueue` on the backup service, and `RegisterMultiWebServerRoutes`.
- [ ] Wire the upgrade engine into both the CLI and the API, or delete `internal/upgrade/` and say plainly that the product cannot upgrade itself.
- [ ] Close the two cross-tenant mail deletions and finish the sweep of the remaining parameterised routes.
- [ ] Fold `migrations/pending/` into the numbered sequence so that upgrading an installation creates the same schema as installing one.
- [ ] Make the panel repository private and activate source encryption, or decide publicly that the product is open source.

### Milestone B — 1.0

- [ ] Alerts that reach a human: monitoring thresholds → `Notify` → Telegram/Zalo/email, with the dedupe and quiet windows that are already written.
- [ ] A backup UI, and a scheduler that runs backup jobs and restorability checks.
- [ ] Audit middleware, so that every write route produces an audit row instead of eight of them.
- [ ] A 2FA policy: an administrator can require it for a tenant.
- [ ] i18n across all 49 dashboard pages, with `vi` as the default locale and the choice stored on the user profile.
- [ ] Frontend tests, and a CI job that fails when they fail.
- [ ] A CSP and HSTS on the UI surface, not only on the API.

### Milestone C — 1.1

- [ ] Reverse proxy that writes and reloads real configuration.
- [ ] Git deployment: clone, deploy keys, webhooks, rollback.
- [ ] Node.js: version management, build step, per-site user.
- [ ] WordPress plugin and theme installation; core write-lock.
- [ ] Log rotation that rotates, live tail, server-side download.
- [ ] FTP accounts.
- [ ] Filesystem and cgroup quota (layers b and c).
- [ ] An OpenAPI specification and outbound event webhooks.

### Milestone D — 1.2

- [ ] Remote execution: extend the agent so a site, database or PHP pool can be created on a node that is not the panel host. Everything below depends on this.
- [ ] Reseller hierarchy.
- [ ] Real load balancing and HA (HAProxy/keepalived), replacing the CRUD.
- [ ] Website malware scanning with a real engine.
- [ ] Public status page and SLA measurement.

### Milestone E — Later, on market signal

Full business email (Postfix/Dovecot/Rspamd), cPanel/DirectAdmin migration,
billing integration, CDN and caching, a plugin system, a mobile application,
compliance reporting. Each of these is measured in months, not weeks; none
should start before Milestone C is done.

### Explicitly not planned

Kubernetes, microservices, GraphQL, blockchain, edge computing and
"AI-powered automation" appeared in the previous roadmap's 2.0/3.0 sections.
They are removed. None of them addresses a problem this product has, and listing
them made the roadmap read as marketing.

---

## Technical improvements

### Performance

- [x] Connection pooling — `internal/database/database.go` sets `MaxOpenConns` and `MaxIdleConns`.
- [~] Database query optimisation — the 23 numbered migrations produce 103 tables and 373 indexes (`CHANGELOG.md`, 0.5.0), but there is no benchmark, no slow-query log and no measured baseline. Unverified either way.
- [ ] Response caching — no cache layer; Redis is used for rate limiting and the job queue only.
- [ ] CDN support.
- [ ] Image optimisation.
- [x] Code splitting — Next.js App Router with dynamic imports (xterm.js is loaded this way).

### Security

- [~] Two-factor authentication — built; login gate unwired. See §2.
- [x] IP allow list.
- [x] Rate limiting.
- [ ] CSRF protection — absent by design; see §3.
- [~] XSS prevention — React escaping only, no `dangerouslySetInnerHTML` anywhere, but no CSP on the UI surface that holds the session token in `localStorage`.
- [x] SQL injection prevention.
- [~] Security headers — complete on the API, absent (CSP, HSTS) on the UI and in `deploy/nginx/vkai-panel.conf`. No test covers the middleware, so a removal would be silent.
- [~] Audit logging — chain real, coverage narrow. See §2.

### Scalability

- [ ] Horizontal scaling — the panel is single-node in practice (§2).
- [ ] Load balancing — CRUD only (§3).
- [ ] Database replication.
- [ ] Redis clustering.
- [ ] Microservices architecture — not planned.
- [ ] Kubernetes support — not planned.

### Developer experience

- [~] API documentation — `docs/API.md` exists and is maintained by hand; no OpenAPI specification.
- [ ] SDK.
- [x] CLI tools — `vkai` (`backup`, `cert`, `config`, `db`, `firewall`, `node`, `panel`, `server`, `service`, `site`, `ssl`, `upgrade`, `user`) and the standalone `panelctl`.
- [~] Testing framework — Go side is real (132 test files, all passing). Frontend has none. Several of the strongest backend tests are `*_live_test.go` and skip unless a DSN environment variable is set, so a default `go test ./...` does not run them.
- [~] CI/CD pipeline — builds, tests and packages; lint is non-blocking, frontend tests are an `echo`.
- [~] Code quality tools — `golangci-lint` configured with 113 pre-existing findings and set not to fail the build.

---

## How to contribute

### Feature requests

Open a discussion describing the use case, the proposed solution and the
alternatives considered. A feature request that names the file it would change
gets read first.

### Bug reports

Open an issue with a description, steps to reproduce, expected and actual
behaviour, and the output of `vkai version` plus the distribution and version.

### Code contributions

Fork, branch, change, **write a test**, update the documentation, open a pull
request. See [CONTRIBUTING.md](CONTRIBUTING.md).

Two house rules that this roadmap exists to enforce:

1. **A feature is not finished until something reaches it.** If you add a route,
   add the line that mounts it and a test that fails when the line is deleted —
   `handler/router_test.go` and `handler/docker_routes_test.go` show the pattern.
   The same applies to a background worker: if it needs a line in
   `cmd/api/main.go`, that line is part of the change, not a follow-up.
2. **Do not mark anything done in this file without naming the evidence** — the
   file and symbol, and the route, command or page that reaches it.

---

## Contact

- **Website**: https://hitechcloud.vn
- **Email**: support@hitechcloud.vn
- **GitHub**: https://github.com/hitechcloud-vietnam/vkai-panel
