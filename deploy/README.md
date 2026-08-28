# VKAI Panel — Installation and Deployment

HiTechCloud (hitechcloud.vn)

The panel **never** uses ports 80 or 443. Those belong to the customer websites.
The panel listens on a **port of its own** (`8888` by default) behind a
**security entrance** such as `/vkai_a1b2c3d4` — any other path returns a neutral
404.

### The panel runs natively, not in Docker

The whole panel is built and run **directly on the machine**: the API is a Go
binary, the UI is a Next.js standalone build run by `node`, the agent is a Go
binary — all three managed by **systemd**. PostgreSQL, Redis and nginx come from
the distribution's package manager. There is no image, no `docker-compose.yml`
and no `docker` step in installation or deployment. A server running the panel
does **not** need Docker Engine.

> **This is not the removal of the Docker feature.** Docker plays two completely
> different roles in VKAI Panel:
>
> | | Docker to run the panel | Docker as a feature for customers |
> |---|---|---|
> | Status | **Removed** | **Kept, fully supported** |
> | Artefacts | `Dockerfile`, `docker-compose.yml` | Docker screens, `/api/v1/docker/*`, `docker:*` permissions |
> | Replaced by | `deploy/install.sh` + systemd | Nothing — still a first class feature |
>
> Customers still manage **their** containers, images, volumes, networks and
> compose stacks through the panel. In short: **the panel does not run in
> Docker, but the panel manages Docker.** Install Docker Engine as an ordinary
> managed service when customers need that feature.

---

## 1. One command install

```bash
# On a clean server, as root
curl -fsSL https://raw.githubusercontent.com/hitechcloud-vietnam/vkai-panel/main/deploy/install.sh | sudo bash
```

or from a checkout:

```bash
git clone https://github.com/hitechcloud-vietnam/vkai-panel.git /usr/src/vkai-panel
cd /usr/src/vkai-panel
sudo bash deploy/install.sh
```

**The installer never asks a question.** Every decision is a flag with a sane
default, so the same command works from a terminal, from cloud-init and from a
pipe with no tty at all. When it is piped from `curl` and there is no source tree
next to it, it downloads one itself.

It finishes by printing the access table (full URL including the entrance, the
administrator credentials, the certificate fingerprint, the data paths and the
`vkai` command), writes the same content to `/vkai-panel/etc/install-summary.txt`
(mode 600) and logs everything to `/vkai-panel/logs/install.log`.

> `scripts/install.sh` is only a shortcut that forwards to `deploy/install.sh`.
> There is exactly **one** installer.

---

## 2. Supported operating systems

| Operating system | Versions | Family | Package manager |
|---|---|---|---|
| Ubuntu | 20.04, 22.04, 24.04 | debian | `apt-get` |
| Debian | 11, 12 | debian | `apt-get` |
| CentOS Stream | 8, 9 | rhel | `dnf` |
| RHEL | 8, 9 | rhel | `dnf` |
| Rocky Linux | 8, 9 | rhel | `dnf` |
| AlmaLinux | 8, 9 | rhel | `dnf` |
| Fedora | 38 and newer | rhel | `dnf` |
| openSUSE Leap | 15.x | suse | `zypper` |
| Amazon Linux | 2023 | rhel | `dnf` |

**Architectures:** `x86_64` (amd64) and `aarch64` (arm64).

When the distribution cannot be identified the installer **stops with a clear
message** instead of guessing. A version outside the matrix is refused unless
`--force-os` is given.

Handled per family:

- **apt** — `-o DPkg::Lock::Timeout=600` so the run does not fail while
  `unattended-upgrades` holds the dpkg lock.
- **RHEL/Rocky/Alma/CentOS** — enables **EPEL** and **CRB/PowerTools**.
- **Amazon Linux 2023** — uses `postgresql15*` and `redis6` instead of the
  standard package names.
- **openSUSE** — `git-core` and `gpg2` instead of `git` and `gnupg`.
- **SELinux** (RHEL/Rocky/Alma/Fedora) — `semanage port` for the panel port,
  `httpd_sys_rw_content_t` on `www/`, `httpd_can_network_connect` on.
- **Firewall** — opens the panel port and port 80 through `ufw` or `firewalld`;
  when neither is present it prints an explicit warning instead of staying quiet.

---

## 3. Command line flags

| Flag | Meaning |
|---|---|
| `--port <number>` | Public panel port (default 8888). 80 and 443 are refused. |
| `--entrance <path>` | Security entrance, e.g. `/vkai_a1b2c3d4`. Empty = generated. |
| `--domain <name>` | Domain the panel answers on. Empty = reached by IP. |
| `--admin-email <email>` | Administrator e-mail, also the ACME contact address. |
| `--tls-mode <mode>` | `self-signed` (default), `letsencrypt` or `none`. |
| `--acme-staging` | Use the Let's Encrypt staging directory (rehearsal, untrusted certificates). |
| `--allow-ip <list>` | Restrict panel access to these IPs/CIDRs. Repeatable, comma separated. |
| `--no-firewall` | Do not touch ufw/firewalld. |
| `--skip-deps` | Do not install system packages. |
| `--uninstall` | Remove services, binaries and configuration. |
| `--purge` | With `--uninstall`: also drop the database and `/vkai-panel/www`. |
| `-y`, `--yes` | Proceed past soft refusals (low RAM, another panel present). |
| `-q`, `--quiet` | Console shows warnings, errors and the final table only. |
| `--random-port` | Pick a free random port between 10000 and 60000. |
| `--admin-user <name>` | First administrator account name (default `admin`). |
| `--api-url <url>` | Hard-code `NEXT_PUBLIC_API_URL`. Empty = same origin through nginx. |
| `--source-url <url>` | Tarball to install from when the script runs on its own. |
| `--skip-checksum` | Skip Go/Node download checksum verification. |
| `--force-os` | Install on an OS release outside the tested matrix. |
| `-h`, `--help` / `-v`, `--version` | Help / version. |

Examples:

```bash
sudo bash deploy/install.sh --port 9001 --entrance /admin_x9f2
sudo bash deploy/install.sh --domain panel.example.com --admin-email ops@example.com --tls-mode letsencrypt
sudo bash deploy/install.sh --allow-ip 203.0.113.7 --allow-ip 10.0.0.0/8 --quiet
sudo bash deploy/install.sh --uninstall            # keeps customer data
sudo bash deploy/install.sh --uninstall --purge    # removes everything
```

---

## 4. Preflight: when the installer refuses

The installer stops, politely and with the way out, when:

- it is not running as **root**;
- **RAM** is below 900 MB (the Next.js build is what runs out of memory first) —
  `--yes` forces past it;
- **free disk** is below 5 GB — always fatal;
- **another control panel** is installed (cPanel, aaPanel/BT, Plesk,
  DirectAdmin, VestaCP, HestiaCP) — `--yes` forces past it;
- the chosen **panel port is already taken** by something that is not this
  panel's own nginx — pick another with `--port` or `--random-port`;
- the distribution is unknown, or outside the tested matrix without `--force-os`.

---

## 5. What the installer does, in order

1. Parse the flags, check **root**, detect **architecture** and **operating
   system** (`/etc/os-release` → `ID`/`VERSION_ID`/`ID_LIKE`, falling back to
   `/etc/redhat-release`).
2. Pick the package manager: `apt-get` | `dnf` | `yum` | `zypper`.
3. Preflight: systemd, RAM/disk minimums, other control panels.
4. Start logging into `/vkai-panel/logs/install.log`, detect whether this is a
   **fresh install or an upgrade in place**.
5. Fetch the source tree when the script was piped from `curl`.
6. Install the system dependencies with the correct package names per family
   (plus EPEL/CRB).
7. Install **Go** and **Node.js** for this architecture, with **SHA256
   verification** against the official checksum files; skipped when the machine
   already has a recent enough toolchain.
8. Create the `vkai` system user/group and the `/vkai-panel/**` tree, including
   the **ACME webroot** `/vkai-panel/www/default/.well-known/acme-challenge`
   owned by `vkai:vkai`.
9. Fix the **panel port and the entrance**, then check 80/443/panel port
   occupancy.
10. Copy the sources into `/vkai-panel/{core,panel,agent}`.
11. **Write `/vkai-panel/etc/.env`** — database password, JWT (≥64 characters),
    secret key, agent token, port, entrance, domain, TLS and ACME settings — and
    symlink it to `panel/.env`.
    > **This step must come BEFORE the UI build.** `NEXT_PUBLIC_API_URL` is
    > inlined into the bundle by `npm run build`, and Next.js only reads `.env`
    > from the **project root**.
12. Prepare the **panel certificate**: a self-signed pair is generated when none
    exists, with the machine IP and the pinned domain in the SAN list. An
    existing certificate is never overwritten.
13. Initialise PostgreSQL (initdb when needed, `pg_hba` for loopback), create the
    role and database, install `uuid-ossp`, apply the migrations in order
    (remembered in `etc/migrations.applied`). An existing database is left alone.
14. Start Redis.
15. Build `core/` (vkai-api, vkai-panelctl, vkai-cli) and `agent/`.
16. Build `panel/`, then **verify** that `.next/standalone/server.js` and
    `.next/standalone/.next/static` both exist, and abort with a clear message
    when they do not.
    > **Why the verification exists:** without that copy the panel returns HTML
    > while every `/_next/static/*.js` returns 404, and the browser shows
    > *"Application error: a client-side exception has occurred"* on every page.
17. Create the **first administrator** with a generated password (bcrypt cost 12).
    An administrator that already exists keeps their password.
18. Install the systemd units and the `vkai` command, configure nginx (panel port
    plus the ACME challenge server on port 80), logrotate, SELinux and the
    firewall.
19. With `--tls-mode letsencrypt`: self-test the challenge path and order the
    certificate.
20. Start the services, check `/health`, print the **access table** and write
    `/vkai-panel/etc/install-summary.txt`.

Re-running the installer is **idempotent**: the port, the entrance, the database
and its contents, the secrets, the certificates and the current administrator
password are all preserved.

---

## 6. TLS for the panel

The panel's certificate is completely separate from the customer site
certificates, which stay with `certbot` and `vkai ssl`.

| `--tls-mode` | What happens |
|---|---|
| `self-signed` (default) | nginx serves the panel port over HTTPS with a generated certificate whose SAN list contains the machine IP, so it works before any DNS record exists. The browser warns; the fingerprint printed in the access table is what to compare. |
| `letsencrypt` | Same bootstrap certificate, plus an ACME order for the pinned domain or, when there is none, for the machine's IP address. |
| `none` | Plain HTTP. Only for a panel already behind another TLS terminator. |

**Why the panel does not use the distribution's certbot for its own
certificate:** Ubuntu 24.04 still ships certbot 2.9.0, which knows nothing about
ACME profiles, and the panel has to install identically on nine OS families.

**Certificates for an IP address.** Let's Encrypt publishes the profiles
`classic`, `shortlived` and `tlsserver`, and a certificate for an **IP
identifier** is only issued through **`shortlived`**, which lasts about six days.
An IP identifier can be validated with **HTTP-01 (port 80)** or **TLS-ALPN-01
(port 443)** only — there is no DNS-01 for an IP. Port 443 belongs to the
customer sites, so HTTP-01 from
`/vkai-panel/www/default/.well-known/acme-challenge` is the only route, which is
why the installer opens port 80 and prepares that directory on every install.
Because the certificate lasts days rather than months, `vkai-cert-renew.timer`
runs twice a day.

The installer also writes `/etc/nginx/conf.d/vkai-acme-challenge.inc`. Include it
from any customer vhost on port 80 that must be able to answer a challenge:

```nginx
include /etc/nginx/conf.d/vkai-acme-challenge.inc;
```

Certificate commands:

```bash
vkai cert info      # path, subject, issuer, SAN, expiry, SHA-256 fingerprint
vkai cert renew     # what the timer runs; a no-op when nothing is due
vkai cert issue     # order a certificate now
```

---

## 7. Directory tree after installation

| Path | Contents |
|---|---|
| `/vkai-panel/` | Install root (751) |
| `/vkai-panel/core/` | API sources and binary (`bin/vkai-api`) |
| `/vkai-panel/panel/` | UI build (`.next/standalone/server.js`) |
| `/vkai-panel/agent/` | Node agent (`bin/vkai-agent`) |
| `/vkai-panel/www/domains/<domain>` | **Customer website documents** |
| `/vkai-panel/www/backup/` | Website and database backups |
| `/vkai-panel/www/default/` | Default site and the ACME webroot |
| `/vkai-panel/www/default/.well-known/acme-challenge/` | HTTP-01 challenge files (`vkai:vkai`, 755) |
| `/vkai-panel/logs/` | Panel logs (`install.log`) |
| `/vkai-panel/logs/sites/<domain>/` | Per site web server logs |
| `/vkai-panel/etc/` | Configuration (`.env`, `install-summary.txt`) — mode 700 |
| `/vkai-panel/ssl/` | Panel certificate (`panel.crt`, `panel.key`) |
| `/vkai-panel/tmp/` | Scratch space |

`/etc/vkai` is linked to `/vkai-panel/etc` so there is one source of truth.

---

## 8. Services

| Service | Role | Port | Unit |
|---|---|---|---|
| API (Go) | `core/bin/vkai-api` | 30110 (loopback) | `vkai-api` |
| UI (Next.js) | `panel/.next/standalone/server.js` | 3000 (loopback) | `vkai-ui` |
| Agent | `agent/bin/vkai-agent` | 30111 | `vkai-agent` (optional) |
| nginx — panel | Reverse proxy and TLS for the panel | **8888** (`VKAI_PANEL_PUBLIC_PORT`) | `nginx` |
| nginx — ACME | HTTP-01 challenge webroot | **80** | `nginx` |
| nginx — customer sites | Customer vhosts | **80/443** | `nginx` |
| Certificate renewal | `vkai cert renew` | — | `vkai-cert-renew.timer` |
| PostgreSQL | Database | 5432 | `postgresql` |
| Redis | Cache and queues | 6379 | `redis-server` \| `redis` \| `redis6` |

3000 / 30110 / 30111 / 5432 / 6379 **listen on loopback only** and are never
exposed to the internet.

The systemd units are hardened: `NoNewPrivileges`, `ProtectSystem=strict`,
`ProtectHome`, `PrivateTmp`, and `ReadWritePaths` opening only what is written.

---

## 9. The `vkai` command

```bash
vkai status                 # service status and machine resources
vkai start | stop | restart
vkai logs api|ui|agent|nginx|install

vkai info                   # full URL, port, entrance, data paths
vkai port 9001              # change the panel port (nginx + firewall + SELinux)
vkai port random
vkai entrance random        # generate a new security entrance

vkai cert info | renew | issue

vkai backup                 # database and configuration into www/backup
vkai update [source-path]   # rebuild core/ and panel/, restart
vkai uninstall [--purge]

vkai site create example.com   # domain commands - delegated to vkai-cli
vkai db | ssl | firewall | server ...
```

Lost the entrance? `vkai info` or `vkai-panelctl panel info`.

---

## 10. Deploying a packaged release

`deploy/scripts/deploy.sh` is for a CI pipeline pushing a `.tar.gz` to the
server. The layout is **release directory plus symlink**:

```
/vkai-panel/releases/<version>/       one directory per release
/vkai-panel/current -> releases/...   symlink to the running release
/vkai-panel/{etc,logs,www,ssl}        shared data, outside every release
```

```bash
sudo bash deploy/scripts/deploy.sh deploy /tmp/vkai-panel-1.2.0.tar.gz
sudo bash deploy/scripts/deploy.sh list       # releases kept
sudo bash deploy/scripts/deploy.sh status
sudo bash deploy/scripts/deploy.sh rollback
```

The package must contain `core/bin/vkai-api`, `core/migrations/*.sql`,
`panel/.next/standalone/server.js` **and** `panel/.next/standalone/.next/static`.

What `deploy` does, in order:

1. Unpack into `releases/<timestamp>/` and **validate the package** before
   touching the running system.
2. Link `/vkai-panel/etc/.env` into the new release.
3. Back the database up to `www/backup/predeploy_<timestamp>.sql.gz`.
4. Apply the missing migrations **from the new release, before the symlink
   moves** — a failed migration stops the deployment while the old release is
   still serving.
5. Point `current` at the new release, restart `vkai-api` and `vkai-ui` (and
   `vkai-agent` when enabled), reload nginx.
6. Health check **both the API and the UI**; **roll back automatically** when the
   new release is unhealthy.
7. Reconcile `/etc/nginx/conf.d/vkai-panel.conf`, once the release is proven
   healthy. The vhost is not part of a release, so a host installed before the
   panel gained its single front door still proxies `location /` straight to the
   Next.js service — which serves the interface, login form included, to anyone
   who finds the panel port. Only those `proxy_pass` lines are pointed at the
   API; `nginx -t` must pass, the previous file is kept as
   `vkai-panel.conf.pre-frontdoor.bak`, and a host that is already correct is
   left untouched.
8. Keep the running release plus the five most recent old ones.

> A rollback only reverts **code**, never database migrations. The backup from
> step 3 is the recovery path when a migration damages data.

---

## 11. Nginx

The installer renders `deploy/nginx/vkai-panel.conf` (a template with
`__VKAI_*__` tokens) into `/etc/nginx/conf.d/vkai-panel.conf`, filling in the
panel port, TLS, the allow list, the server name and the log directory. nginx
owns the public panel port and forwards everything to ONE loopback service, the
API:

```nginx
upstream vkai_api { server 127.0.0.1:30110; keepalive 16; }
```

There is no `vkai_ui` upstream, on purpose. The API is what enforces the
security entrance, so every request - the login page and every `/_next/` asset
included - has to pass through it; the API then forwards what it does not serve
itself to the Next.js service on `VKAI_UI_UPSTREAM`. When nginx sent `/`
straight to Next.js the entrance guarded the API and nothing else. The installer
refuses to install a rendered configuration that proxies to the UI directly.

That file contains **no** `listen 80` and **no** `listen 443`, and must never
contain them.

The second generated file, `/etc/nginx/conf.d/vkai-acme-challenge.conf`, is a
small port 80 server that serves the ACME challenge directory and returns 404 for
everything else, so it can never shadow a customer site. It only claims
`default_server` when nothing else on port 80 already has it; when the
configuration test fails it is removed again and the installer tells you to
include `vkai-acme-challenge.inc` in the vhost that owns port 80 instead.

---

## 12. Troubleshooting

**Services do not come up**

```bash
systemctl status vkai-api vkai-ui
journalctl -u vkai-api -n 100 --no-pager
cat /vkai-panel/etc/.env          # mode 600, readable by root/vkai only
```

**The panel shows "Application error: a client-side exception has occurred"**

The static assets are missing from the standalone build:

```bash
ls /vkai-panel/panel/.next/standalone/.next/static   # must exist and be non-empty
sudo vkai update                                     # rebuild properly
```

**The panel is unreachable from outside**

```bash
vkai info                        # the actual port and entrance
ss -ltnp | grep <panel-port>     # is nginx listening
ufw status | grep <panel-port>   # or: firewall-cmd --list-ports
```

Also check the provider's security group or firewall.

**The certificate order fails**

```bash
vkai cert info
curl -v http://<server-ip>/.well-known/acme-challenge/probe   # must reach this host
ls -la /vkai-panel/www/default/.well-known/acme-challenge     # vkai:vkai, 755
```

Let's Encrypt validates from the internet: port 80 has to reach this machine.
Behind NAT, the certificate is requested for the public address.

**Database connection errors**

```bash
systemctl status postgresql
PGPASSWORD=$(grep ^VKAI_DB_PASSWORD= /vkai-panel/etc/.env | cut -d= -f2-) \
  psql -h 127.0.0.1 -U vkai -d vkai_panel -c 'select 1'
```

---

## 13. Uninstalling

```bash
sudo bash deploy/install.sh --uninstall
```

Removes the services, the binaries, the nginx configuration, the systemd units,
logrotate and the `vkai` command. It **keeps** `/vkai-panel/www` (customer
website documents and backups), `/vkai-panel/etc`, `/vkai-panel/logs`,
`/vkai-panel/ssl` and the database, and says so in its output.

```bash
sudo bash deploy/install.sh --uninstall --purge
```

Additionally drops the database and its role, deletes `/vkai-panel` entirely
(including `/vkai-panel/www`) and removes the `vkai` account. There is no undo.

---

## 14. Security recommendations

1. Change the administrator password immediately after the first login.
2. Restrict the panel port to your administration IPs — `--allow-ip` at install
   time, or `VKAI_PANEL_ALLOWED_IPS` in `.env` plus the `allow`/`deny` block in
   `vkai-panel.conf`.
3. Keep the security entrance secret; rotate it with `vkai entrance random`.
4. **Never** move the panel onto port 80 or 443.
5. Keep panel TLS on, with a certificate separate from the customer sites.
6. Back up on a schedule: `0 2 * * * /usr/local/bin/vkai backup nightly`.
7. Keep the system and the panel updated; watch `vkai logs api`.
