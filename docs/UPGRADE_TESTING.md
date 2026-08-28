# Testing the self-upgrade

This page is for whoever has to prove that `vkai upgrade` works before a release
goes out. It builds a release feed from this repository, points a panel at it
over loopback, and walks an upgrade from 0.6.0 to a synthetic 0.6.1 and back
again.

Operating an installed panel is a different job: see [UPGRADE.md](UPGRADE.md).

---

## Table of contents

1. [What a release feed is](#what-a-release-feed-is)
2. [Where a panel gets its feed](#where-a-panel-gets-its-feed)
3. [The tools](#the-tools)
4. [Test 1: the sandbox, no panel required](#test-1-the-sandbox-no-panel-required)
5. [Test 2: on a real installation](#test-2-on-a-real-installation)
6. [The four outcomes of a check](#the-four-outcomes-of-a-check)
7. [The panel's Check for updates button](#the-panels-check-for-updates-button)
8. [What a production feed requires](#what-a-production-feed-requires)
9. [Things this test must never do](#things-this-test-must-never-do)

---

## What a release feed is

One JSON document listing published releases. `core/internal/upgrade` reads it
and nothing else decides what a panel installs.

```json
{
  "releases": [
    {
      "version": "0.6.1",
      "released_at": "2026-08-28T14:18:09Z",
      "min_upgrade_from": "",
      "tarball_url": "https://releases.example.com/vkai-panel-0.6.1.tar.gz",
      "sha256": "3e198c8d…",
      "size_bytes": 41052511,
      "changelog_url": "https://…/CHANGELOG.md",
      "signature": "3093ba9f…"
    }
  ]
}
```

The field names are a contract - `internal/upgrade/manifest.go` says so in as
many words. A single manifest object, or a bare array, is accepted too; the
`releases` list is what lets a refused upgrade name the intermediate release to
install first.

`signature` is an ed25519 signature over exactly these eight lines, each ending
in `\n` (`ManifestSigningPayload` in the same file):

```
vkai-panel-release-manifest-v1
<version>
<released_at, RFC3339 in UTC, empty when unset>
<min_upgrade_from>
<tarball_url>
<sha256, lowercase>
<size_bytes, decimal>
<changelog_url>
```

It is signed over the fields rather than over the JSON bytes, because two
encoders produce two different documents for the same manifest.

---

## Where a panel gets its feed

`/vkai-panel/etc/upgrade.env`, root-owned and readable by the panel:

```bash
VKAI_UPGRADE_FEED_URL=https://releases.example.com/vkai-panel/{channel}/releases.json
VKAI_UPGRADE_PUBLIC_KEYS=966cdd8230c79c104b784133297ef70a9eec8053a3580f7d14dfdf10fb468570
```

* `{channel}` is replaced with the `channel` field of
  `/vkai-panel/etc/version.json`, so one line serves `stable` and `beta`.
* `VKAI_UPGRADE_PUBLIC_KEYS` is a list of hex ed25519 public keys. Set it, and
  every manifest in the feed has to be signed by one of them. Leave it out and
  the feed is authenticated by TLS alone - which means its certificate, its CA,
  its DNS, its CDN and everyone with write access to the bucket behind it all
  choose what your customers install as root.
* The same variables are read from the process environment and from
  `/vkai-panel/etc/.env`, in that order of precedence. `upgrade.env` exists
  because `.env` is mode 0600 and `vkai upgrade --check` is allowed to run
  without root.

Other settings, all optional:

| Variable | Meaning |
|---|---|
| `VKAI_UPGRADE_ALLOW_INSECURE` | Permit a plaintext `http` feed or tarball. For a mirror on a network you control, and for the tests below. |
| `VKAI_UPGRADE_ALLOW_PRERELEASE` | Follow pre-releases such as `2.0.0-rc.1`. Off by default. |
| `VKAI_UPGRADE_SERVICES` | Units to restart and health-check. Default `vkai-api vkai-ui vkai-agent`. |
| `VKAI_UPGRADE_HEALTH_URL` | Probed with a GET after the restart, in addition to `systemctl is-active`. |
| `VKAI_UPGRADE_HEALTH_TIMEOUT` / `_INTERVAL` | How long the services get to come up before the upgrade is rolled back. Default `90s` / `3s`. |
| `VKAI_UPGRADE_KEEP_RELEASES` | Release directories to keep on prune. Default 5. |
| `VKAI_UPGRADE_DATABASE_BACKUP` | `off` disables the pre-upgrade dump. **Never on a production panel** - it is the only thing that makes a bad migration recoverable. |

The database dump is on automatically whenever the panel's database name is
known, and the dump command is handed `PGHOST`/`PGUSER`/`PGPASSWORD` from the
panel's own settings, so it works from an operator's shell as well as from
systemd.

---

## The tools

```
scripts/upgrade-test-feed.sh keygen                Generate an ed25519 signing pair
scripts/upgrade-test-feed.sh add --version 0.6.1   Build a release + signed manifest
scripts/upgrade-test-feed.sh serve                 Serve the feed on 127.0.0.1:8099
scripts/upgrade-test-feed.sh env                   Print the settings to export
scripts/upgrade-test-feed.sh sandbox               Build a throwaway installation
scripts/upgrade-test-feed.sh list                  Show what the feed contains
```

It needs `bash`, `openssl` (1.1.1 or newer), `sha256sum`, `tar`, `go`, and
`python3` for `serve`. The feed lives in `/tmp/vkai-test-feed` and the sandbox in
`/tmp/vkai-test-panel`; both move with `--dir` / `--sandbox` or
`VKAI_TEST_FEED_DIR` / `VKAI_TEST_SANDBOX`.

`add` builds the release package from the working tree: the Go binaries, the
migrations and `deploy/`. It does **not** build the Next.js interface, which
needs npm and several minutes - so a sandbox upgrade proves the upgrade path,
not that the UI serves. For a package that can actually serve, run
`make package VERSION=0.6.1` and pass it with `--from`.

---

## Test 1: the sandbox, no panel required

This runs on a laptop. Nothing is installed, no service is restarted, no root is
needed. The sandbox supplies a fake `systemctl` on `PATH`; everything inside the
engine - the checksum, the signature, the archive checks, the switch, the
rollback - is the real thing.

### 1. Build a signed feed with one release

```bash
cd /path/to/vkai-panel

scripts/upgrade-test-feed.sh keygen
scripts/upgrade-test-feed.sh add --version 0.6.1
scripts/upgrade-test-feed.sh list
```

`keygen` prints the public key and writes the private one to
`/tmp/vkai-test-feed/release-key.pem`. `add` builds
`vkai-panel-0.6.1.tar.gz`, hashes it, signs the manifest and rewrites
`releases.json`.

### 2. Serve it, in another terminal

```bash
scripts/upgrade-test-feed.sh serve
# serving /tmp/vkai-test-feed on http://127.0.0.1:8099/
```

### 3. Build a throwaway installation running 0.6.0

```bash
scripts/upgrade-test-feed.sh sandbox --version 0.6.0
```

It prints the settings to export. They are the same variables a real panel keeps
in `upgrade.env`:

```bash
export VKAI_PANEL_ROOT=/tmp/vkai-test-panel
export PATH=/tmp/vkai-test-panel/bin:$PATH
export VKAI_UPGRADE_FEED_URL=http://127.0.0.1:8099/releases.json
export VKAI_UPGRADE_PUBLIC_KEYS=$(cat /tmp/vkai-test-feed/release-key.pub.hex)
export VKAI_UPGRADE_ALLOW_INSECURE=1
export VKAI_UPGRADE_DATABASE_BACKUP=off
export VKAI_UPGRADE_HEALTH_TIMEOUT=6s
export VKAI_UPGRADE_HEALTH_INTERVAL=2s
```

### 4. Check

```bash
cd core && go build -o /tmp/vkai-cli ./cmd/cli && cd ..

/tmp/vkai-cli upgrade --check
echo "exit=$?"
```

```
Installed version : 0.6.0
Release channel   : stable
Latest version    : 0.6.1
Checked at        : 2026-08-28T14:23:37Z

An update is available: 0.6.0 -> 0.6.1
Install it with: sudo vkai upgrade
Release notes  : https://github.com/hitechcloud-vietnam/vkai-panel/blob/main/CHANGELOG.md
exit=10
```

`--check` changes nothing on disk. `--json` prints the record the panel reads.

### 5. Upgrade

```bash
/tmp/vkai-cli upgrade --yes
```

```
  -> Acquiring the upgrade lock...
     Holding /tmp/vkai-test-panel/etc/upgrade.lock
  -> Checking for a new release...
     0.6.1 is available (running 0.6.0)
  -> Downloading the release...
  -> Verifying the download checksum...
     sha256 matches the manifest
  -> Staging the new release...
  -> Running preflight checks...
     All preflight checks passed
  -> Switching to the new release...
     /tmp/vkai-test-panel/current now points at /tmp/vkai-test-panel/releases/0.6.1
  -> Restarting services...
  -> Waiting for services to become healthy...
     Services are healthy on 0.6.1
✓ Upgraded to 0.6.1.
```

Confirm the machine changed and the record was rewritten:

```bash
readlink /tmp/vkai-test-panel/current      # releases/0.6.1
cat /tmp/vkai-test-panel/etc/version.json  # "version": "0.6.1"
/tmp/vkai-cli upgrade --check; echo "exit=$?"   # up to date, exit=0
```

### 6. Roll back

Two rollbacks are worth testing, and they are not the same thing.

**a. The automatic one**, which is the one that matters: a release that will not
come up. Publish a 0.6.2 and arm the sandbox's health check to fail on it.

```bash
scripts/upgrade-test-feed.sh add --version 0.6.2
echo 0.6.2 > /tmp/vkai-test-panel/etc/health-fails-on

/tmp/vkai-cli upgrade --yes; echo "exit=$?"
```

```
  -> Switching to the new release...
     /tmp/vkai-test-panel/current now points at /tmp/vkai-test-panel/releases/0.6.2
  -> Restarting services...
  -> Waiting for services to become healthy...
     FAILED: The new release did not become healthy: services did not become
     healthy within 6s (3 attempts): service vkai-api is not active
  -> Rolling back to the previous release...
     Rolled back to /tmp/vkai-test-panel/releases/0.6.1
Error: the upgrade to 0.6.2 failed: … rolled back to …/releases/0.6.1, which is
running normally
exit=1
```

```bash
readlink /tmp/vkai-test-panel/current   # releases/0.6.1 - back where it started
rm /tmp/vkai-test-panel/etc/health-fails-on
```

**b. The deliberate one**: going back to a release that works fine. The upgrade
engine does not do this - it only rolls back a failure it caused. On a real
installation it is `sudo /vkai-panel/bin/vkai-deploy rollback`; in the sandbox it
is the same two steps by hand:

```bash
ln -sfn releases/0.6.0 /tmp/vkai-test-panel/current
systemctl restart vkai-api vkai-ui      # the sandbox's fake one
```

### 7. Prove the signature is load-bearing

A test that would pass with verification switched off has proved nothing:

```bash
# Publish an unsigned release into the feed…
scripts/upgrade-test-feed.sh add --version 0.6.3 --unsigned
/tmp/vkai-cli upgrade --check; echo "exit=$?"
```

With `VKAI_UPGRADE_PUBLIC_KEYS` set this reports an error and exits 1:
*"release 0.6.3 is not signed, or its signature is not 64 hex-encoded bytes"*.
Delete `manifest-0.6.3.json` from the feed directory and re-run
`scripts/upgrade-test-feed.sh add --version 0.6.1` to rebuild `releases.json`.

---

## Test 2: on a real installation

On the demo host, with a panel actually installed. This restarts the panel.

```bash
# 1. Build a full release package, interface included.
sudo make package VERSION=0.6.1

# 2. Publish it into a feed the host can reach. Do this on the host itself so
#    the feed stays on loopback.
sudo scripts/upgrade-test-feed.sh keygen
sudo scripts/upgrade-test-feed.sh add --version 0.6.1 \
     --from vkai-panel-0.6.1-linux-amd64.tar.gz
sudo scripts/upgrade-test-feed.sh serve &

# 3. Point the panel at it.
sudo tee /vkai-panel/etc/upgrade.env >/dev/null <<EOF
VKAI_UPGRADE_FEED_URL=http://127.0.0.1:8099/releases.json
VKAI_UPGRADE_PUBLIC_KEYS=$(cat /tmp/vkai-test-feed/release-key.pub.hex)
VKAI_UPGRADE_ALLOW_INSECURE=1
EOF
sudo chown root:vkai /vkai-panel/etc/upgrade.env
sudo chmod 0644 /vkai-panel/etc/upgrade.env

# 4. Check. This is also what the daily timer runs.
vkai upgrade --check
sudo systemctl start vkai-upgrade-check.service
systemctl status vkai-upgrade-check.service --no-pager
cat /vkai-panel/etc/upgrade-check.json

# 5. Upgrade. It dumps the database first; keep that dump.
sudo vkai upgrade

# 6. Confirm.
vkai version && vkai status
ls -l /vkai-panel/current
ls /vkai-panel/www/backup/databases/

# 7. Roll back on purpose.
sudo /vkai-panel/bin/vkai-deploy rollback
vkai version
```

Remove `/vkai-panel/etc/upgrade.env` (or replace it with the production feed)
when the test is over: leaving a loopback feed configured means the panel reports
an error every day once the test server is gone.

---

## The four outcomes of a check

`vkai upgrade --check` exists to be read by a machine. The exit code is the
answer, and `vkai-upgrade-check.service` declares `SuccessExitStatus=10`.

| Situation | `status` | Exit |
|---|---|---|
| No release feed configured | `unconfigured` | 0 |
| Feed answers, nothing newer | `up-to-date` | 0 |
| Feed answers, newer release exists | `update-available` | **10** |
| Feed answers, newer release exists, panel pinned | `pinned` | 0 |
| Feed configured, cannot be reached or would not verify | `error` | 1 |
| `vkai-cli` too old to have the command | `unsupported` | 2 |

`unconfigured` exits 0 on purpose. A panel with no feed is a permanent, known
condition; a daily unit that is red for a reason nobody can fix teaches an
operator to ignore red. It is reported as *"update checking is not configured"* -
never as "up to date" - in the state file, in the CLI and in the panel.

A feed that is configured and does not answer is the opposite case, and it stays
exit 1 and a red unit: the panel does not know what is published.

---

## The panel's Check for updates button

`POST /api/v1/upgrade/check` runs the same engine, built from the same
`upgrade.env`, so the button and `vkai upgrade --check` cannot disagree about a
machine. It is behind `RequirePlatformAdmin`: a customer's administrator gets
403, because a self-upgrade replaces every binary on the server.

`POST /api/v1/upgrade/start` answers 503 with *"This panel cannot upgrade itself
from the browser: the API runs as an unprivileged service. Run `sudo vkai
upgrade` on the server."* That is not a bug in the wiring - `vkai-api.service`
runs as the `vkai` user with `ProtectSystem=strict`, and replacing
`/vkai-panel/current` needs root. Making the button perform upgrades needs a
privileged helper; it is not something to fix by loosening that unit.

---

## What a production feed requires

1. **HTTPS.** Plaintext is refused unless `VKAI_UPGRADE_ALLOW_INSECURE` says
   otherwise, and so is a redirect from https to http.
2. **A signing key that is not on the release host.** Generate it with
   `scripts/upgrade-test-feed.sh keygen` (or any ed25519 keypair), keep the
   private half offline or in the CI secret store, and publish the 64-character
   public half in `upgrade.env` on every panel. Rotation is additive:
   `VKAI_UPGRADE_PUBLIC_KEYS` accepts a list, so the new key can be deployed
   before the old one stops signing.
3. **`signature` and `size_bytes` in every manifest.**
   `.github/workflows/release.yml` publishes `releases.json` today, but its
   manifests carry `tarball_size` rather than `size_bytes` and no `signature` at
   all. Both have to be added there before a panel with
   `VKAI_UPGRADE_PUBLIC_KEYS` set can upgrade from it - a manifest with no
   signature is refused, and without `size_bytes` the disk check before the
   download has nothing to work with.
4. **`min_upgrade_from` set on any release that must not be skipped**, and the
   feed serving the release *history*, not just the newest entry. It is read from
   every release the upgrade would step over, so an intermediate release that is
   missing from the feed cannot be named as the way forward.
5. **A stable URL per channel**, e.g.
   `https://releases.hitechcloud.vn/vkai-panel/stable/releases.json`, and
   `deploy/install.sh` writing `upgrade.env` at install time so a fresh panel is
   checking from day one.

---

## Things this test must never do

* **Do not turn signature verification off to make a test pass.** If the panel
  refuses a manifest, the manifest is wrong. Sign it.
* **Do not publish an unsigned feed as the default.** `--unsigned` exists to
  test the refusal path.
* **Do not set `VKAI_UPGRADE_DATABASE_BACKUP=off` anywhere but a sandbox with no
  database.** It is what makes a failed migration recoverable.
* **Do not leave `VKAI_UPGRADE_ALLOW_INSECURE=1` on a real panel.** Without
  signatures TLS is the only thing authenticating a root install; with them it is
  still the only thing authenticating the tarball's transport.
