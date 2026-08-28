# Upgrading VKAI Panel

This document is for the operator of a server that already runs VKAI Panel.
It describes how an upgrade works, what happens when one fails, how to upgrade a
machine with no outbound internet access, how to hold a machine on one version,
and - the part worth reading before you need it - how to bring the panel back by
hand when both the upgrade and its automatic rollback have failed.

Installing the panel for the first time is a different job:
see [DEPLOYMENT.md](DEPLOYMENT.md).

---

## Table of contents

1. [The short version](#the-short-version)
2. [What an upgrade actually does](#what-an-upgrade-actually-does)
3. [The release channel](#the-release-channel)
4. [The daily update check](#the-daily-update-check)
5. [Command reference](#command-reference)
6. [When an upgrade fails](#when-an-upgrade-fails)
7. [Rolling back on purpose](#rolling-back-on-purpose)
8. [Upgrading offline from a downloaded tarball](#upgrading-offline-from-a-downloaded-tarball)
9. [Pinning a version](#pinning-a-version)
10. [Manual recovery](#manual-recovery)
11. [What an upgrade never touches](#what-an-upgrade-never-touches)

---

## The short version

```bash
vkai version              # what is installed, on which channel, since when
vkai upgrade --check      # is there something newer? changes nothing
sudo vkai upgrade         # shows the plan, asks, then upgrades
```

Everything else on this page is the detail behind those three lines.

---

## What an upgrade actually does

### The release-directory model

The panel is never upgraded by overwriting the files that are currently running.
Each release is unpacked into its own directory, and one symlink decides which
of them is live:

```
/vkai-panel/
├── releases/
│   ├── 20260315_101500/                 # previous release, kept for rollback
│   └── 20260316_143000/                 # the release just published
├── current -> releases/20260316_143000  # what the systemd units run
├── etc/                                 # .env, version.json    - SHARED
├── logs/                                #                       - SHARED
├── www/                                 # customer sites, backups - SHARED
└── ssl/                                 # certificates          - SHARED
```

The systemd units point at paths under `/vkai-panel/current`, never at a release
directory by name. Switching to a new release is therefore *moving one symlink
and restarting two services*, and going back is moving it the other way. Both
take about a second, and neither can leave a half-copied file tree behind.

Only **code** is versioned. `etc/`, `logs/`, `www/` and `ssl/` live outside every
release: an upgrade does not overwrite them, and a rollback does not take them
back in time.

The running release plus the five most recent old ones are kept. Older ones are
deleted after a successful upgrade.

### The steps, in order

1. Resolve the newest version on the configured channel (or the one given to
   `--to`).
2. Download the release package and verify it.
3. Unpack it into a **new** directory `/vkai-panel/releases/<id>/`.
4. Validate the unpacked release before touching anything that is running - the
   API binary must be executable, and the UI build must contain both
   `panel/.next/standalone/server.js` and `panel/.next/standalone/.next/static`.
   A package missing the static assets is rejected here rather than becoming a
   panel that loads and then shows *"Application error: a client-side exception
   has occurred"*.
5. Link the shared configuration into the new release
   (`/vkai-panel/etc/.env` -> `<release>/panel/.env`); Next.js only reads `.env`
   from its project root.
6. Dump the database to `/vkai-panel/www/backup/predeploy_<timestamp>.sql.gz`.
7. Run the migrations the **new** release brings, **before** the symlink moves.
   A failing migration stops the upgrade while the old release is still serving.
   Applied migrations are recorded in `/vkai-panel/etc/migrations.applied`.
8. Repoint `/vkai-panel/current`, `systemctl restart vkai-api vkai-ui` (plus
   `vkai-agent` when enabled) and reload nginx.
9. Health check **both** the API and the UI, retrying for about 30 seconds.
10. On failure: repoint `current` at the previous release and health check again
    (see [When an upgrade fails](#when-an-upgrade-fails)).
11. On success: rewrite `/vkai-panel/etc/version.json` and delete releases older
    than the five kept.

The panel is unreachable for the few seconds of step 8. **Customer websites are
served by nginx from `/vkai-panel/www/domains/` and stay up throughout** - they
do not depend on `vkai-api` or `vkai-ui`.

### Where the version is recorded

`/vkai-panel/etc/version.json` (mode `0644`, root-owned, readable by the panel):

```json
{
  "version": "1.0.0",
  "channel": "stable",
  "pin": "",
  "installed_at": "2026-08-28T05:12:00Z",
  "updated_at": "2026-08-28T05:12:00Z",
  "release": "in-place",
  "install_mode": "fresh",
  "os": "Ubuntu 24.04.1 LTS",
  "arch": "amd64"
}
```

It is written by `deploy/install.sh` and rewritten by every successful upgrade.
It is deliberately **not** derived from the binaries: on a machine where somebody
replaced a binary by hand, a version guessed from that binary would be wrong
exactly where the answer matters. `installed_at` survives every upgrade, so it
still says when this panel was first installed.

---

## The release channel

A channel is the stream of releases this machine follows.

| Channel  | Contents                                                        |
|----------|-----------------------------------------------------------------|
| `stable` | Production releases. The default, and the only one for customer servers. |
| `beta`   | Pre-releases, for a staging machine you can afford to break.     |

The channel is chosen at install time and recorded in `version.json`:

```bash
sudo bash deploy/install.sh --channel stable      # the default
sudo bash deploy/install.sh --channel beta
```

To move an existing machine to another channel, edit the `channel` field of
`/vkai-panel/etc/version.json` and run `vkai upgrade --check`:

```bash
sudo sed -i 's/"channel": "stable"/"channel": "beta"/' /vkai-panel/etc/version.json
vkai upgrade --check
```

Switching from `beta` back to `stable` does **not** downgrade anything by itself.
The machine simply stops being offered pre-releases and waits for a stable
release with a higher version. To actually go back to a stable build now, use
`--to` with that version, or roll back to the previous release directory.

---

## The daily update check

`vkai-upgrade-check.timer` runs `vkai upgrade --check --quiet` once a day
(04:00 plus a randomised delay of up to four hours, so a fleet does not ask the
same question in the same second), and again about 20 minutes after every boot.

**It only checks. It never installs anything.** Unattended upgrades of a hosting
control panel are how a whole fleet goes down at once: one bad release, every
customer's panel restarted into it overnight, nobody watching. A panel upgrade
also runs database migrations and restarts the API and the UI, so it happens when
a human is there to read the result. The timer's whole job is to answer *"is
there something newer?"* and write the answer down.

The answer lands in `/vkai-panel/etc/upgrade-check.json`, which is what the panel
reads for its "an update is available" banner:

```json
{
  "checked_at": "2026-08-28T04:13:52Z",
  "status": "update-available",
  "installed_version": "1.0.0",
  "latest_version": "1.1.0",
  "channel": "stable",
  "pinned": "",
  "release_notes_url": "https://github.com/hitechcloud-vietnam/vkai-panel/releases",
  "detail": ""
}
```

`status` is one of `up-to-date`, `update-available`, `pinned`, `unsupported`
(this build has no upgrade engine) or `error` (the check itself failed - the
`detail` field says why).

Managing the timer:

```bash
systemctl status  vkai-upgrade-check.timer
systemctl list-timers vkai-upgrade-check.timer
sudo systemctl start vkai-upgrade-check.service   # check right now
journalctl -u vkai-upgrade-check -n 20 --no-pager
sudo systemctl disable --now vkai-upgrade-check.timer   # stop checking entirely
```

Exit code 10 ("an update is available") is declared as `SuccessExitStatus=10` in
the unit, so a machine with a pending update is not reported as a failed systemd
unit. A genuinely failed check exits 1 and does show up as a unit failure.

### Driving monitoring from the check

`vkai upgrade --check` changes nothing and exits non-zero when an update is
available, so it can be a check command as it stands:

```bash
# Nagios/Icinga style: 0 = ok, 1 = warning, 2 = critical
vkai upgrade --check --quiet
case $? in
  0)  echo "OK - panel is up to date";        exit 0 ;;
  10) echo "WARNING - panel update available"; exit 1 ;;
  *)  echo "CRITICAL - update check failed";   exit 2 ;;
esac
```

To read the last recorded result without running a new check - from a collector
that must not touch the network - parse `/vkai-panel/etc/upgrade-check.json`, or
use the machine-readable form of the command:

```bash
vkai upgrade --check --json
```

---

## Command reference

### `vkai version`

Prints what is installed, from `version.json`, plus the release the symlink
points at and the result of the last update check. It reports the **installed
panel**, not the version compiled into one binary, so it stays right on a machine
where a binary was replaced by hand.

### `vkai upgrade [--check] [--to <version>] [--yes]`

| Flag | Meaning |
|------|---------|
| `--check` | Report only. Nothing on the machine changes except the check record. |
| `--to <version>` | Upgrade to exactly this version instead of the newest on the channel. Also the way to reinstall the running version, or to move to a named version after a channel change. |
| `--yes`, `-y` | Skip the confirmation. Exists so an operator's own automation can run the command; the panel itself never uses it. |
| `--json` | With `--check`: print the result as JSON instead of a table. |
| `--quiet`, `-q` | One line of output. What the daily timer uses. |

Without `--yes`, `vkai upgrade` prints the plan - which version, which channel,
what will happen to the database and the services, how long the panel is
unreachable - and waits for you to type `yes`. If stdin is not a terminal it
refuses instead of assuming consent, so a cron job that forgot `--yes` stops
rather than upgrading a fleet unattended.

Upgrading requires root; `--check` does not (though only root can update the
record the panel displays).

### Exit codes

These are interface, not implementation detail - monitoring depends on them.

| Code | Meaning |
|------|---------|
| `0` | Up to date, or an update exists but this machine is pinned, or the upgrade succeeded. |
| `10` | An update is available (`--check` only). |
| `2` | This build has no upgrade engine compiled in. Upgrade from a tarball instead - see [offline upgrades](#upgrading-offline-from-a-downloaded-tarball). |
| `1` | The check or the upgrade failed. |

---

## When an upgrade fails

Failure is handled differently depending on where it happens, and the ordering of
the steps above is what makes that possible.

| Fails at | What happens | State afterwards |
|----------|--------------|------------------|
| Download or package validation (steps 2-4) | The upgrade stops before anything running is touched. | The old release is still serving. Nothing changed. |
| A database migration (step 7) | The migration run stops at the failing file. The symlink has **not** moved. | The old release is still serving, against a partly migrated database. See below. |
| Health check after the switch (step 9) | **Automatic rollback:** `current` is repointed at the previous release, the services are restarted, and the health check runs again. | The previous release is serving. The broken release is left in `/vkai-panel/releases/` for you to inspect. |
| The rollback's own health check | Nothing more is attempted automatically. | Both releases are unhealthy. Go to [Manual recovery](#manual-recovery). |

Two things a rollback does **not** undo:

- **Database migrations are not rolled back.** Older code against a newer schema
  usually works, because migrations are written to be backward compatible for one
  release. When it does not, restore the dump taken in step 6:

  ```bash
  sudo gunzip -c /vkai-panel/www/backup/predeploy_20260316_143000.sql.gz \
    | psql -h 127.0.0.1 -U vkai -d vkai_panel
  ```

  The password is `VKAI_DB_PASSWORD` in `/vkai-panel/etc/.env`; export it as
  `PGPASSWORD` first, or run the command as the `postgres` system user.

- **Shared state is not rolled back.** `etc/`, `www/`, `logs/` and `ssl/` are
  outside the release directories on purpose. Configuration you changed, sites
  you created and certificates you issued survive both the upgrade and the
  rollback.

After any failure, the first two commands are always:

```bash
vkai version && vkai status
sudo journalctl -u vkai-api -n 80 --no-pager
sudo journalctl -u vkai-ui  -n 80 --no-pager
```

---

## Rolling back on purpose

The automatic rollback only covers a release that fails its health check within
about 30 seconds. For a release that starts fine and turns out to be wrong an
hour later, roll back by hand:

```bash
sudo /vkai-panel/bin/vkai-deploy list       # what is on disk, "->" marks the live one
sudo /vkai-panel/bin/vkai-deploy rollback   # go back one release
sudo /vkai-panel/bin/vkai-deploy status
```

`rollback` finds the release immediately before the running one, checks that it
is complete, repoints `current`, restarts the services and health checks them.
`/vkai-panel/bin/vkai-deploy` is root-owned and is the only privileged entry
point that moves the symlink.

To go back **more** than one release, deploy that release's package again
(`vkai-deploy deploy <file>`), or repoint the symlink by hand as described below.

---

## Upgrading offline from a downloaded tarball

A panel behind a firewall with no outbound access, or one where you want to
control exactly which bytes are installed, is upgraded from a release package.
This is also the path to use when `vkai upgrade` reports exit code 2.

On a machine that does have internet access, download the release package and
its checksum from the releases page, then verify and copy it across:

```bash
# on the workstation
sha256sum -c vkai-panel-1.1.0.tar.gz.sha256
scp vkai-panel-1.1.0.tar.gz root@server:/tmp/
```

On the server:

```bash
# 1. Verify the package landed intact (compare against the published checksum).
sha256sum /tmp/vkai-panel-1.1.0.tar.gz

# 2. Deploy it. This is the same code path "vkai upgrade" drives:
#    unpack, validate, back up the database, migrate, switch, health check,
#    and roll back automatically if the new release does not answer.
sudo /vkai-panel/bin/vkai-deploy deploy /tmp/vkai-panel-1.1.0.tar.gz

# 3. Confirm what is running.
vkai version
vkai status
```

A release package must contain:

```
core/bin/vkai-api                       # the API binary, executable
core/migrations/*.sql                   # migrations
panel/.next/standalone/server.js        # the UI build
panel/.next/standalone/.next/static     # MANDATORY - the UI is broken without it
agent/bin/vkai-agent                    # optional
```

Building one on a workstation (never on a customer server):

```bash
make build
tar -czf vkai-panel-1.1.0.tar.gz -C dist .
```

`vkai-deploy` does not know the version number of a package, so record it
afterwards if you deployed by hand:

```bash
sudo sed -i 's/"version": "[^"]*"/"version": "1.1.0"/' /vkai-panel/etc/version.json
```

### The other offline path: rebuilding in place

On a single machine with the source tree present, `sudo vkai update` rebuilds
`core/` and `panel/` where they stand and restarts the services. It is quicker to
reach for, but it creates **no release directory**, so `vkai-deploy rollback`
cannot undo it. Prefer the tarball on any server with customers on it.

---

## Pinning a version

Pinning holds a machine at one version - for a server under change freeze, one
that needs a specific version for a customer integration, or one you want left
alone until you have tested a release elsewhere.

```bash
# Pin to the version that is running now.
sudo sed -i 's/"pin": "[^"]*"/"pin": "1.0.0"/' /vkai-panel/etc/version.json
vkai upgrade --check
```

With a pin set:

- `vkai upgrade --check` reports `status: pinned` when a newer release exists,
  and **exits 0**. That is deliberate: the operator asked for this version, so
  monitoring must not page anybody about it.
- `/vkai-panel/etc/upgrade-check.json` carries `"status": "pinned"` and the
  pinned version, which is what the panel displays instead of an "update
  available" banner.
- `vkai upgrade` will not move off the pinned version.
- `vkai upgrade --to <version>` still works and warns that it is overriding the
  pin for that one run. The pin field itself is not changed by the run.
- `deploy/install.sh` preserves the pin when it re-runs on the machine.

Remove the pin to follow the channel again:

```bash
sudo sed -i 's/"pin": "[^"]*"/"pin": ""/' /vkai-panel/etc/version.json
vkai upgrade --check
```

---

## Manual recovery

**Use this when the upgrade failed AND the automatic rollback failed** - the
panel is down, `vkai upgrade` and `vkai-deploy rollback` are not getting it back,
and you need to put the symlink somewhere that works with your own hands.

Customer websites are served by nginx and are almost certainly still up. Check
before you assume the outage is bigger than it is:

```bash
systemctl status nginx --no-pager
curl -I http://127.0.0.1/ -H 'Host: one-customer-domain.example'
```

### Step 1 - see what you have

```bash
readlink -f /vkai-panel/current        # which release is linked right now
ls -1t /vkai-panel/releases/           # every release kept, newest first
sudo journalctl -u vkai-api -n 80 --no-pager
sudo journalctl -u vkai-ui  -n 80 --no-pager
```

### Step 2 - pick a release that is actually complete

Check the candidate before pointing anything at it. All three must succeed:

```bash
REL=/vkai-panel/releases/20260315_101500        # the release you intend to run

test -x "$REL/core/bin/vkai-api"                        && echo "API binary OK"
test -f "$REL/panel/.next/standalone/server.js"         && echo "UI server OK"
test -d "$REL/panel/.next/standalone/.next/static"      && echo "UI assets OK"
```

If none of the release directories passes, skip to
[Step 6](#step-6---when-no-release-is-usable).

### Step 3 - repoint the symlink by hand

```bash
sudo systemctl stop vkai-ui vkai-api

# The shared configuration lives outside the release and has to be linked in;
# Next.js reads .env only from its own project root.
sudo ln -sfn /vkai-panel/etc/.env "$REL/panel/.env"
sudo ln -sfn /vkai-panel/etc/.env "$REL/core/.env"

# Move the pointer. -n stops ln from creating a link INSIDE the old target,
# and -f replaces the existing one. Both flags matter here.
sudo ln -sfn "$REL" /vkai-panel/current
sudo chown -h vkai:vkai /vkai-panel/current

readlink -f /vkai-panel/current         # confirm it points where you meant
```

### Step 4 - start the services again

```bash
sudo systemctl daemon-reload
sudo systemctl start vkai-api
sudo systemctl start vkai-ui
sudo systemctl reload nginx

systemctl status vkai-api --no-pager
systemctl status vkai-ui  --no-pager
```

### Step 5 - prove it is really back

```bash
# The API on its loopback port (VKAI_SERVER_PORT in .env, 30110 by default)
curl -fsS http://127.0.0.1:30110/health && echo

# The UI on its loopback port (PORT in .env, 3000 by default)
curl -fsS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:3000/

# The panel as a user reaches it - port and entrance from "vkai info"
vkai info
vkai status
vkai version
```

Then record what you did, so the next upgrade knows what it is upgrading from:

```bash
sudo sed -i 's/"version": "[^"]*"/"version": "1.0.0"/' /vkai-panel/etc/version.json
vkai version
```

### Step 6 - when no release is usable

In order of increasing disruption:

1. **Deploy a known-good package.** If you have any release tarball on disk or
   can copy one over, this is still the fastest route back:

   ```bash
   sudo /vkai-panel/bin/vkai-deploy deploy /tmp/vkai-panel-1.0.0.tar.gz
   ```

2. **Rebuild in place from the source tree**, if the machine has one plus Go and
   Node:

   ```bash
   sudo vkai update /path/to/source-tree
   ```

3. **Re-run the installer.** It detects the existing installation and upgrades in
   place; the database, the certificates, the security entrance, `.env` and every
   customer site are preserved. It rebuilds the code and rewrites the systemd
   units, which repairs a broken unit file as well as a broken build:

   ```bash
   sudo bash /path/to/vkai-panel/deploy/install.sh
   ```

   Never pass `--purge`. That is the uninstaller's flag and it drops the database
   and `/vkai-panel/www`.

4. **Restore the database** only if the schema is the problem - a rolled-back
   panel refusing to start against a migrated database, for instance:

   ```bash
   sudo ls -1t /vkai-panel/www/backup/predeploy_*.sql.gz | head
   sudo gunzip -c /vkai-panel/www/backup/predeploy_20260316_143000.sql.gz \
     | sudo -u postgres psql -d vkai_panel
   ```

   Restoring the database rolls back customer records too - sites, users and
   settings created since that dump. Do it last, not first.

If step 3 restarts cleanly but the browser shows *"Application error: a
client-side exception has occurred"*, the UI build in that release is missing
`.next/static`. Point `current` at a different release, or rebuild the UI.

---

## What an upgrade never touches

| Kept across every upgrade and rollback | Where |
|----------------------------------------|-------|
| Customer website documents | `/vkai-panel/www/domains/<domain>/` |
| Backups | `/vkai-panel/www/backup/` |
| Panel configuration and secrets | `/vkai-panel/etc/.env` (mode `600`) |
| Panel port, security entrance, IP allow list | `/vkai-panel/etc/panel_access.json` |
| Version record and its pin | `/vkai-panel/etc/version.json` |
| Certificates | `/vkai-panel/ssl/` |
| Logs | `/vkai-panel/logs/` |
| The database | PostgreSQL, dumped before every upgrade |

The panel port and the security entrance in particular do **not** change during
an upgrade. If the panel appears to be gone after one, you are almost certainly
on the wrong URL - `vkai info` prints the right one.

---

## See also

- [DEPLOYMENT.md](DEPLOYMENT.md) - first installation, the release model, day-to-day operations
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) - symptoms and fixes
- [PANEL_ACCESS.md](PANEL_ACCESS.md) - port, entrance and IP allow list
- [CONFIGURATION.md](CONFIGURATION.md) - every environment variable
