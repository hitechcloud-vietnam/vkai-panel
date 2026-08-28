# Release readiness: can VKAI Panel ship as 1.0?

**Assessed:** 28 August 2026, 19:40 (+07), against the working tree at commit
`0116f53`.
**Current version:** `0.5.0`.
**Assessment: no. Not today.**

This document exists to make that a decision rather than a feeling. It lists what
must be fixed before any public release, what is not blocking but is embarrassing
in a commercial product, what 1.0 will honestly not do, and the exact commands
that decide whether a candidate build is releasable.

Everything here was verified by reading the code and running the commands in
§4. Where something could not be verified, it says so.

**A note on timing.** Several agents were changing code while this was written.
The findings below were confirmed against the tree at the timestamp above; before
acting on any single one, re-run the command given with it. Nothing here is
second-hand.

---

## 1. Blockers

Each of these must be fixed, or the feature removed, before a public release.
They are ordered by how much damage they do if a customer meets them first.

### B1. Enrolling in two-factor authentication locks the operator out

`AuthService.SetTwoFactor` is never called outside tests:

```
$ grep -rn "SetTwoFactor" core/ --include=*.go | grep -v _test
core/internal/service/auth.go:214:func (s *AuthService) SetTwoFactor(...)   # definition
core/internal/handler/auth.go:29:  # a doc comment showing the call
```

`cmd/api/main.go` constructs `NewAuthService(...)` and calls `SetAudit(...)`, and
never calls `SetTwoFactor`. So `s.twoFactor` is `nil` at runtime. Enrolment
nevertheless sets `users.mfa_enabled = true`
(`core/internal/twofactor/store_postgres.go:260`), and the login gate then takes
the `s.twoFactor == nil && user.MFAEnabled` branch in
`core/internal/service/auth.go:478` and returns `ErrTwoFactorUnavailable`.

**Why it blocks:** the security feature that most enterprise buyers check first
bricks the account of anyone who turns it on. It fails closed, so it is not an
authentication bypass — it is a self-inflicted denial of service on the
administrator, discovered after they have already enrolled and, on a fresh
install, possibly with no second administrator to recover with. The fix is one
line in `cmd/api/main.go`. There is no test covering `main.go` wiring, which is
precisely why this shipped.

### B2. Cross-tenant deletion is still reachable, and the audit is unfinished

Two routes delete another tenant's data given only a UUID:

```go
// core/internal/handler/mail_server.go:188  DeleteAlias      — no tenant check
// core/internal/handler/mail_server.go:214  DeleteQueueItem  — no tenant check
// core/internal/repository/mail_server.go:192  DELETE FROM mail_aliases WHERE id = $1
// core/internal/repository/mail_server.go:206  DELETE FROM mail_queue   WHERE id = $1
```

Both sit behind `RequirePermission("settings")` (`router.go:897`), so any tenant
holding that permission can destroy any other tenant's mail alias or queue item.
The guard they need — `ownedDomain` / `ownedAccount` — already exists in the
sibling file `core/internal/handler/mail_dns.go` and is used by five other
routes.

That is the residue of a class, not an isolated bug. Two cross-tenant defects of
this class were found and fixed earlier today (monitoring metrics readable across
tenants; mail domains and mailboxes deletable across tenants — both fixes
verified present). **320 parameterised route registrations** exist across
`core/internal/handler/*.go`, against **170** `requestTenant(c)` call sites,
**13** `requireTenantScopedRow` call sites and **5** `owned*` call sites. The
dominant pattern is to pass `tenantID` down to a repository whose `WHERE` clause
carries the predicate — sound, but it is a convention, and a convention is only
as good as its last reviewer.

**Why it blocks:** multi-tenancy is the product. One customer deleting another
customer's data is the failure a hosting control panel cannot survive, and it is
not a bug you get to fix after it happens.

**What settles it:** the in-flight audit of all parameterised routes completing,
plus a `*_tenant_scope_live_test.go` for mail aliases and mail queue — the six
existing files of that kind are the reason the other tables are known good, and
mail aliases were not among them.

### B3. The job queue reports success while doing nothing

All nine handlers in `core/internal/job/queue.go` are the same shape:

```go
// TODO: Implement actual restore logic
time.Sleep(3 * time.Second)
return nil
```

at lines 180, 196, 213, 230, 247, 263, 279, 295 and 312 — backup, restore,
deploy, SSL, cleanup, health check, metrics, log rotation and notification. The
worker really runs: `cmd/api/main.go:281` calls `jobQueue.StartWorker(10)`. The
routes are live: `POST /api/v1/jobs/backup|restore|deploy|ssl|cleanup`
(`router.go:791-795`). The frontend's `backupApi` points its restore at
`POST /api/v1/jobs/restore`.

**Why it blocks:** a customer clicks "restore", the panel says the job completed,
and nothing was restored. There is no failure mode worse than this in a hosting
product: it converts a recoverable incident into an unrecoverable one, and it
does so silently. Either implement the handlers or unmount the routes and delete
the queue's enqueue endpoints.

### B4. The security scanner invents its results

`core/internal/service/security.go:357`:

```go
// TODO: Implement actual security scanning logic
time.Sleep(5 * time.Second)
scan.Status = "completed"
scan.Score = 85
scan.TotalChecks = 50
scan.PassedChecks = 42
```

`POST /api/v1/security/scans` is mounted at `router.go:457` and there is a
security page in the UI.

**Why it blocks:** it does not merely fail to scan; it returns a clean bill of
health for a server it never looked at. An operator who acts on that score is
worse off than one with no scanner at all. Remove the route, or make it real.
There is no antivirus integration anywhere in the tree (`clamav`, `maldet`,
`rkhunter`, `yara`: zero hits), so "make it real" is not a small change.

### B5. There is no upgrade path — twice over

Two independent failures, and the second is worse than the missing engine.

**(a) The engine is finished and connected to nothing.**
`core/internal/upgrade/` is a complete in-place upgrade implementation — a
releases directory, staged extraction, preflight, a pre-upgrade database dump,
symlink promotion, a single explicit rollback path, a lock, pruning — with its
own tests, all passing. And:

```
$ grep -rn "vkai-panel/internal/upgrade" --include=*.go . | wc -l
0
```

Nothing imports it. `core/internal/cli/upgrade.go:123` returns `stubUpgrader{}`
at the line marked `SWAP HERE`, whose `Apply` returns "upgrade support is not
built into this binary"; and `core/internal/handler/router.go:193` constructs
`service.NewUpgradeService(nil, auditService, logger)` — a nil engine, so the
`/api/v1/upgrade/*` routes report unavailable. Both halves are two lines from
working.

**(b) Upgrading an installation does not create the schema the code needs.**
Fourteen migration files live in `core/migrations/pending/` — `2fa.sql`,
`agent_pki.sql`, `apikey_scopes.sql`, `appstore.sql`, `audit_chain.sql`,
`backup.sql`, `collector.sql`, `local_node.sql`, `multi_webserver.sql`,
`notify.sql`, `packages.sql`, `php_wordpress.sql`, `session_binding.sql`,
`waf_event_context.sql`. `deploy/install.sh:1864` applies them (in *tolerant*
mode, so a failure there silently costs a feature). But
`deploy/scripts/deploy.sh:241`, the release-upgrade script, globs
`find "$dir" -maxdepth 1 -name '*.sql'`, and `make migrate` globs
`migrations/*.sql`. **Neither applies `pending/`.** An installation upgraded
through the product's own release channel therefore ends up running new code
against a database with no tables for two-factor authentication, agent PKI, the
App Store, the audit chain, offsite backup, the metrics collector, notifications,
packages and quota, or the PHP/WordPress runtime.

**Why it blocks:** "cannot upgrade itself" is tolerable in a 1.0 if you say so.
"Upgrades into a broken state" is not. And a control panel that cannot ship a
security fix to its installed base has no way to respond to the next CVE.

### B6. The repository is public and source encryption is inert

```
$ gh repo view hitechcloud-vietnam/vkai-panel --json visibility
{"visibility":"PUBLIC"}

$ vkai-crypt status
Files in scope and ENCRYPTED in the repository: 0
Files in scope but still PLAINTEXT:            759
```

`tools/protect/` is written and documented — `vkai-crypt`, an AES-256-CTR +
HMAC-SHA256 Git clean/smudge filter, and `build-protected.sh`, a `garble`-based
release obfuscator. The filter is configured (`filter.vkai-crypt.required true`,
`.gitattributes` covers `*.go`, `*.ts`, `*.tsx`, …) and the key exists. It has
never been applied: every committed blob is plaintext. Neither
`build-protected.sh` nor `garble` is referenced by the `Makefile` or by any
workflow in `.github/workflows/`.

Compounding it, `core/migrations/001_initial_schema.sql:457` seeds a
`super_admin` account with the bcrypt hash of `admin123`, publicly, with the
comment naming the password. `deploy/install.sh:1995` normally replaces it with a
random 20-character password — but at `:2001` there is a fallback: if it can
compute no bcrypt hash (no Go toolchain, no `python3-bcrypt`), it *warns* and
leaves `admin/admin123` in force. There is no forced password change on first
login (`must_change_password` does not exist in the codebase).

**Why it blocks:** this is a commercial product being sold on a security
proposition, whose entire source is public, whose anti-copying tooling is
switched off, and whose default administrator credential is published in the same
repository. Whatever the commercial decision — private repository, or open source
by choice — it must be made deliberately before release, not discovered
afterwards.

### B7. Mailbox passwords are stored and returned in plaintext

`core/internal/service/mail_server.go:57`:

```go
// In production, hash the password here
return s.repo.CreateAccount(ctx, tenantID, req, req.Password)
```

`core/internal/repository/mail_server.go:100` inserts it directly into
`mail_accounts.password`; `ListAccounts` and `GetAccount` select the column back
out and the handler returns it over the API. The repository parameter is named
`hashedPwd`, which it is not.

Mailbox passwords legitimately need a scheme a mail server can verify — a
`{SHA512-CRYPT}` value, not bcrypt — so the fix is not simply "hash it". But
plaintext at rest, readable through the API by anyone with the `settings`
permission, is a disclosure defect, and it is in a table that a breach report
would have to name.

### B8. The panel cannot tell anyone that anything went wrong

Every transport is written and tested — `core/internal/notify/{email,telegram,zalo,webhook}.go`,
with retry, dead-lettering, deduplication, quiet windows and secret redaction —
and none of it runs:

```
$ grep -rn "AttachDelivery\|RunDispatcher\|StartRestorabilityChecks" \
      --include=*.go . | grep -v _test
# only definitions and doc comments
```

`NotificationService.Notify` therefore returns `ErrDeliveryNotConfigured` on
every call, nothing drains the outbox, and — separately — nothing publishes
events into it. `MonitoringService.checkAlerts`
(`core/internal/service/monitoring.go:320`) evaluates thresholds, writes a
`monitoring_alert_logs` row and a `zap.Warn`, and never calls `Notify`. Backup
failures, tamper-proof alerts and agent-offline detection do the same.

**Why it blocks:** an unattended hosting control panel whose alerting reaches
nobody is being sold on a promise it cannot keep. Customers will configure a
Telegram channel, see it saved, and hear nothing when their disk fills. Both the
plumbing and the publishing side are small changes on top of finished code.

### B9. Failover is a lie the panel tells with a 200

`POST /api/v1/ha-pairs/:id/failover` is mounted (`router.go:763`). It reaches
`core/internal/repository/cluster.go:471`, which swaps `primary_server_id` and
`secondary_server_id` and sets `status = 'failed-over'`. No VIP moves. No
keepalived is signalled. No health check runs. `grep` for `haproxy` or
`keepalived` across `core/` and `panel/` returns nothing but an unrelated string
in a test.

After an operator triggers failover during an incident, traffic is exactly where
it was and the panel now displays the wrong machine as primary — so the panel
actively misleads the person handling the outage. The same applies to
"load balancers", which are created with `status = "active"` and no configuration
behind them, and to the reverse proxy, whose UI at least says so on screen.

Either remove these routes for 1.0 and mark the feature as planned, or make them
real. Showing them as working is not an option.

### B10. The demo host does not run the product

The machine serving `control.vkai.vn` runs `/opt/vkai-panel/api` (started 02:18
today) under `vkai-panel-api.service` / `vkai-panel-frontend.service` — units that
do not exist in `deploy/systemd/` — behind a hand-written nginx vhost whose TLS
comes from **certbot** (`/etc/letsencrypt/live/control.vkai.vn/`), not from the
panel's own ACME client. The running binary answers `404` on `/api/v1/version`
and emits `Access-Control-Allow-Origin: *`, neither of which matches current
source; the current `middleware.CORS` reflects only an allowlist.

TLS itself is genuinely good: the certificate verifies without `-k`
(`curl --resolve control.vkai.vn:443:127.0.0.1`, `ssl_verify_result=0`), issued
by Let's Encrypt, valid to 25 November 2026. The problem is what it proves —
nothing about `deploy/install.sh`, the systemd units, the entrance guard, the
panel's own certificate path or the upgrade path, because none of them is in use.

**Why it blocks:** the install path is the first thing every customer runs and it
has never been exercised end to end on a machine anyone can point at. Note that
`CHANGELOG.md` records "a clean install could not complete" as fixed **today**,
in 0.5.0 — so the installer's happy path is days old and unproven in production.

**What settles it:** a fresh VPS, `deploy/install.sh` run once, and §4's
checklist passing against it.

---

## 2. Should fix

Not blocking. Each one is the sort of thing a customer or a reviewer finds and
mentions.

- **No CSP or HSTS on the UI.** The API has a full header set
  (`middleware.SecurityHeaders`, `router.go:230`), but the Next.js surface is
  served through `uiproxy` outside the gin engine and its headers come from
  `panel/next.config.js`, which sets `X-Frame-Options`, `nosniff`,
  `Referrer-Policy` and `Permissions-Policy` — no CSP, no HSTS.
  `deploy/nginx/vkai-panel.conf` adds neither. The session token lives in
  `localStorage`, so the page that holds it has the weakest headers in the
  system. Also: no test covers `SecurityHeaders`, so removing it would be silent.
- **No frontend tests at all.** `panel/package.json:17` is
  `"test": "echo \"no frontend tests yet\" && exit 0"`. The CI frontend test job
  passes unconditionally.
- **Lint is non-blocking** with 113 pre-existing `golangci-lint` findings
  (`.github/workflows/ci.yml:89`).
- **`internal/rbac` has no tests.** Neither do `internal/nodeapp`,
  `internal/job`, `internal/websocket`, `internal/models` or `internal/database`.
  For a package that decides who may do what, that is the wrong one to leave
  uncovered.
- **Orphaned UI components.** `panel/src/components/websites/SiteManagePanel.tsx`
  (57 KB), `SiteDefaultsDialog.tsx` (11.8 KB) and
  `email-server/DeliverabilityPane.tsx` (11.8 KB) are imported by nothing.
  (`CreateSiteDialog.tsx` was in this list and is now reached through
  `PhpProjects.tsx`.)
- **`/multi-webserver` ships as a page whose endpoints 404.**
  `RegisterMultiWebServerRoutes` (`handler/webserver.go:140`) is called from
  nowhere.
- **Four finished backends have no UI:** offsite backup, packages and quota,
  notification channels and preferences, agent enrolment and revocation. The
  backups page is 35 lines of static markup with a button that has no handler.
- **Two mounted WordPress routes install nothing.**
  `POST /wordpress/:id/plugins` and `/themes` (`router.go:683`) write a row and
  log "WordPress plugin installed". The frontend's own API client says so in a
  comment. There is no plugin or theme *install* endpoint anywhere.
- **Two parallel PHP surfaces.** The real one (`/php/system`, `/php/install`,
  `/php/pools/:id/settings`, `/php/sites/:id/version`) has no UI calling it; the
  old row-writing one (`/php/versions`, `/php/pools`, `/php/extensions`) is what
  the site settings screen uses, and it reports host state it does not have.
- **File protection ships a complete UI over tables nothing ever writes.** No
  watcher, no scanner, no quarantine mover.
- **Tamper-proof file integrity is real but runs only on demand and has no test
  file at all.**
- **Hardcoded infrastructure values.** `cmd/api/main.go:271` hardcodes
  `localhost:6379` for the job queue's Redis; `internal/cli/backup.go:250`
  hardcodes `PGPASSWORD=vkai_panel_2024`.
- **`FileManager.Compress`, `Extract` and `ChangeOwner` exist with no route.**
- **fail2ban is shipped but never enabled.** `deploy/fail2ban/` has a filter, a
  jail and `enable.sh`, and nothing in the installer, `Makefile` or CI calls it.
  (The log-format tests do read the shipped filter and match it against real log
  lines, so at least it cannot drift silently.)
- **i18n is 278 keys over 2 of 49 dashboard pages**, and the default locale is
  `en` for a product positioned on being Vietnamese.
- **The agent's bootstrap client honours `VKAI_PANEL_INSECURE`**
  (`agent/cmd/main.go:445` → `InsecureSkipVerify: true`). It is logged as a risk
  and only affects enrolment transport, but it is a foot-gun in a product whose
  headline security claim is mTLS.
- **`migrations/pending/` is applied in tolerant mode by the installer**, so a
  syntax error there costs one feature silently on a fresh install. Fold it into
  the numbered sequence (this is also B5b).

---

## 3. Known limitations for the release notes

State these up front. Every one of them is something a customer will otherwise
discover on their own, and discovering it is much more expensive than reading it.

**Scope**

1. **VKAI Panel 1.0 manages the machine it is installed on.** Remote servers can
   be enrolled and monitored over mTLS, but no host-changing operation is sent to
   an agent: the agent can do `system.info`, `system.metrics`,
   `service.list/status/control`, `log.list/read`, `disk.usage`, `agent.info`
   and `pki.sync`, and nothing else. Creating a site, database or PHP pool always
   acts on the panel host, whatever server is selected. Restore explicitly
   refuses a remote target.
2. **Clusters, load balancers and HA pairs are records, not configuration.** No
   HAProxy, keepalived or VRRP configuration is generated.
3. **The reverse proxy screen records a rule; it does not serve it.**
4. **nginx is the only complete web server adapter.** Apache, Caddy,
   OpenLiteSpeed, LiteSpeed and Traefik exist in skeleton.

**Features people will look for and not find**

5. **No FTP accounts.** The FTP screen starts and stops the daemon and manages
   firewall rules; it cannot create an account.
6. **Node.js applications run under systemd, not PM2.** No Node version
   management, no build step.
7. **Git deployment cannot clone, cannot use a deploy key, has no webhook
   receiver and cannot roll back.** It updates a checkout that already exists.
8. **WordPress: installation, staging and updates work; plugin and theme
   *installation* does not.**
9. **Offsite backup supports S3-compatible storage and local paths only.** No
   Google Drive, no FTP/SFTP, no incremental backup, and no automatic snapshot
   before an overwriting restore. Nothing is scheduled: backups and restorability
   checks run when something asks for them, and today nothing asks.
10. **Log rotation settings are stored and not acted on. There is no live tail
    (the UI polls) and no server-side log download.**
11. **The mail server is not a mail server.** DNS diagnostics — SPF, DKIM, DMARC,
    rDNS, blocklists — are real and useful. No Postfix or Dovecot configuration
    is ever written.
12. **No reseller hierarchy, no billing integration, no cPanel/DirectAdmin
    migration, no public status page, no CDN or caching layer, no plugin system,
    no mobile app.**
13. **The panel cannot upgrade itself** (see B5; if B5 is fixed, delete this
    line).

**Operational constraints**

14. **Redis is required, and the credential guard fails closed.** If Redis is
    unavailable, nobody can sign in. This is deliberate — a brute-force limiter
    that fails open is not a limiter — but it must be in the runbook.
15. **Quota is enforced at the API boundary only.** There is no filesystem quota
    and no cgroup limit, so a tenant can exceed a disk allowance between
    sampling runs and cannot be capped on CPU, RAM or IOPS.
16. **The audit chain is cryptographically strong and covers eight call sites:**
    sign-in, sessions, API keys, panel settings, upgrade and two-factor. Creating
    or deleting a website, database, backup, cluster, firewall rule or SSL
    certificate is not audited.
17. **Two-factor authentication is opt-in per user.** There is no policy to
    require it (and see B1).
18. **Session authentication uses a Bearer token in `localStorage`.** There is
    deliberately no CSRF protection, because a cross-site request cannot set the
    header. Anyone moving the session to a cookie must add CSRF defence in the
    same change.
19. **Monitoring records four metrics** — CPU, RAM, disk, load average. No
    process, website-uptime or service monitoring.

---

## 4. Release verification checklist

Run all of it. A release candidate passes only if every step passes. Steps 1–4
run anywhere; steps 5–12 must run on a **freshly installed VPS**, because the
install path is the thing least proven (B10).

### Build and test

```bash
# 1. The Go module builds, vets and tests clean.
cd core
go build ./...                 # PASS: no output, exit 0
go vet ./...                   # PASS: no output, exit 0
go test ./... -count=1         # PASS: every line "ok" or "[no test files]";
                               #       no FAIL, exit 0
```
As of this assessment all three pass: 132 test files, every package `ok`.

```bash
# 2. Live-database tests actually run. Several of the strongest tests
#    (*_live_test.go) skip silently without a DSN, including every
#    tenant-isolation test. A release must not be cut on the skipped set.
cd core
VKAI_TEST_DSN=postgres://vkai:...@127.0.0.1:5432/vkai_panel_test \
VKAI_NOTIFY_DSN=postgres://vkai:...@127.0.0.1:5432/vkai_panel_test \
  go test ./... -count=1 -v 2>&1 | grep -c -- "--- SKIP"
                               # PASS: 0 tenant-scope or notify tests skipped
```

```bash
# 3. The UI builds and type-checks.
cd panel
npm ci
npm run type-check             # PASS: exit 0, no errors
npm run build                  # PASS: exit 0
npm run lint                   # PASS: exit 0
npm test                       # CURRENTLY MEANINGLESS - it is an `echo`.
                               # This step is not a gate until real tests exist.
```

```bash
# 4. Version consistency: VERSION, the Go binaries and package.json agree.
make check-version-sync        # PASS: exit 0
cat VERSION                    # PASS: matches the tag being cut
```

### Install on a clean machine

```bash
# 5. One-command install on a fresh VPS (Ubuntu 22.04/24.04, Debian 12,
#    AlmaLinux/Rocky 8/9).
sudo bash deploy/install.sh
# PASS: exits 0; prints an administrator username and a 20-character
#       generated password; the log at the printed path contains no
#       "The default admin/admin123 account ... is still in force".
# FAIL: any occurrence of that warning - the machine has a published
#       default credential (B6).
```

```bash
# 6. Every service the installer created is running.
systemctl is-active vkai-api vkai-ui vkai-agent
# PASS: "active" three times
systemctl list-timers --all | grep vkai
# PASS: vkai-cert-renew.timer and vkai-upgrade-check.timer are listed
systemctl status vkai-cert-renew.service vkai-upgrade-check.service --no-pager
# PASS: last run "status=0/SUCCESS" or "inactive (dead)" having never
#       needed to run.
# FAIL: any "status=1/FAILURE" - the cert-renew timer was failing on every
#       run with `unknown flag: --identifier` until today, and
#       vkai-upgrade-check cannot succeed while the CLI ships stubUpgrader
#       (B5a).
```

```bash
# 7. Health and version, through the real front door.
curl -sS https://<panel-host>/health
# PASS: 200, and "version" equals the contents of VERSION.
#       It must NOT be a hardcoded "1.0.0" - that defect was fixed in
#       0.5.0 and is worth re-checking.
curl -sS https://<panel-host>/api/v1/version
# PASS: 200. A 404 here means the deployed binary is older than the
#       release being verified (this is exactly how the stale demo host
#       was detected).
```

```bash
# 8. TLS is real, from outside, with no -k.
curl -sS -o /dev/null -w '%{http_code} %{ssl_verify_result}\n' \
     https://<panel-host>/
# PASS: "200 0" - ssl_verify_result 0 means the chain verified.
echo | openssl s_client -connect <panel-host>:443 \
       -servername <panel-host> 2>/dev/null \
     | openssl x509 -noout -subject -issuer -dates
# PASS: subject CN matches the panel host, issuer is Let's Encrypt,
#       notAfter is more than 30 days away.
vkai panel cert status
# PASS: reports the certificate the panel is serving and the days
#       remaining. This command must exist - it did not before today.
```

```bash
# 9. The security entrance actually gates the whole panel, UI included.
curl -sS -o /dev/null -w '%{http_code}\n' https://<panel-host>/
# PASS: 404 (the neutral response) without the entrance path.
curl -sS -o /dev/null -w '%{http_code}\n' https://<panel-host>/<entrance>/
# PASS: 200.
# FAIL: a login form reachable at "/" - that was the state before the
#       single-front-door change and it must not regress.
```

### Verify the claims the product makes

```bash
# 10. Schema completeness. Every migration, including pending/, is applied.
sudo -u postgres psql -d vkai_panel -c "\dt" | wc -l
# PASS: the count matches a fresh install of the same release.
sudo -u postgres psql -d vkai_panel -c \
  "SELECT to_regclass('user_two_factor'), to_regclass('audit_chain'),
          to_regclass('hosting_packages'), to_regclass('backup_destinations'),
          to_regclass('notification_deliveries');"
# PASS: five non-null names.
# FAIL: any NULL - the pending/ migrations did not run, and 2FA, the audit
#       chain, quota, offsite backup or notifications will fail at runtime
#       (B5b). Check this after an UPGRADE too, not only after an install.
```

```bash
# 11. The audit chain verifies, and detects tampering.
curl -sS -H "Authorization: Bearer $TOKEN" \
     https://<panel-host>/api/v1/audit/chain/status
# PASS: 200, chain head present.
curl -sS -H "Authorization: Bearer $TOKEN" \
     https://<panel-host>/api/v1/audit/chain/verify
# PASS: 200 and a positive verification.
# Then, on a scratch database only, modify one audit row and repeat:
# PASS: 409, not 200.
```

```bash
# 12. Nothing reports success without acting. Manual, and mandatory.
#  a. Enrol an administrator in 2FA, sign out, sign in again.
#     PASS: the second factor is requested and accepted.
#     FAIL: "sign-in is temporarily unavailable for this account" (B1).
#  b. Configure a Telegram or email notification channel, send a test.
#     PASS: the message arrives.
#     FAIL: anything else (B8).
#  c. Trigger a backup, then restore it into a scratch directory.
#     PASS: files come back and hashes match.
#     FAIL: a job that completes in ~2-3 seconds having produced nothing (B3).
#  d. As tenant A, call DELETE /api/v1/mail-server/aliases/<id-owned-by-B>.
#     PASS: 403 or 404.
#     FAIL: 200 (B2).
#  e. Run a security scan.
#     PASS: the route is gone, or the score reflects real checks.
#     FAIL: score 85, 50 checks, 42 passed, every time (B4).
#  f. Upgrade the installation to the next release with `vkai upgrade`.
#     PASS: it completes, /health reports the new version, and step 10
#           still passes.
#     FAIL: "upgrade support is not built into this binary" (B5a).
```

### Release hygiene

```bash
# 13. Source protection matches the commercial decision.
vkai-crypt status
# PASS (closed-source): "still PLAINTEXT: 0"
# PASS (open-source):   a deliberate, recorded decision; remove the filter
#                       config and the .gitattributes entries so the repo
#                       stops claiming protection it does not apply.
# FAIL: today's state - 0 encrypted, 759 plaintext, filter armed (B6).

gh repo view hitechcloud-vietnam/vkai-panel --json visibility
# PASS: matches that same decision.
```

---

## 5. The recommendation

**Do not cut 1.0 from this tree.**

Cut it after Milestone A in [ROADMAP.md](ROADMAP.md) — which is almost entirely
subtraction and wiring, not new features. Six of the ten blockers (B1, B3, B4,
B5a, B8, B9) are resolved either by adding a handful of lines to
`cmd/api/main.go` or by deleting routes that report work they do not do. B2 is
the completion of an audit already under way. B5b is folding fourteen files into
a numbered sequence. B6 is a commercial decision plus one `git add --renormalize`.
B10 is one afternoon on a clean VPS.

That is days of work, not months, and the difference it makes is the difference
between a product that is smaller than the brochure and a product that lies. This
codebase is much better than its documentation has been: the audit chain, the
mTLS agent channel, the credential guard, the WP-CLI layer, the PHP-FPM manager
and the backup engine are genuinely good, carefully reasoned work with real
tests. What it has repeatedly failed at is the last inch — the line in `main.go`,
the mount in `router.go`, the import in the page. Every blocker above except B2,
B6 and B7 is an instance of that same last inch.

Fix the last inch, then ship.
