# Installing VKAI Panel

> One command installs the panel **and makes the machine you installed it on a
> managed node**. When the installer finishes you can create a website, a
> database and a certificate on that same box. A second machine is optional and
> is never a precondition.

- [What you need](#what-you-need)
- [Install](#install)
- [What the installer does, in order](#what-the-installer-does-in-order)
- [The machine you installed on is the first node](#the-machine-you-installed-on-is-the-first-node)
- [Checking that it worked](#checking-that-it-worked)
- [If the node is missing: repairing by hand](#if-the-node-is-missing-repairing-by-hand)
- [Adding another machine](#adding-another-machine)
- [Reinstalling over an existing database](#reinstalling-over-an-existing-database)
- [Uninstalling](#uninstalling)
- [Troubleshooting](#troubleshooting)

---

## What you need

| | |
|---|---|
| Operating system | Debian/Ubuntu, RHEL/Rocky/Alma, or openSUSE with systemd |
| Privileges | root |
| RAM | 900 MB minimum |
| Disk | 5 GB free |
| Ports | one panel port of your choosing (never 80 or 443 — those belong to the customer websites), plus port 80 for ACME |

The installer brings its own Go and Node.js when the machine has none that are
new enough. PostgreSQL, Redis and nginx are installed from the distribution.

---

## Install

From a source tree:

```bash
sudo bash deploy/install.sh
```

Common options:

```bash
sudo bash deploy/install.sh \
    --port 8443 \                 # the public panel port (never 80/443)
    --domain panel.example.com \  # pins the panel to one host name
    --tls-mode letsencrypt \      # or self-signed (default), or none
    --allow-ip 203.0.113.7        # restrict the panel to your own addresses
```

`--allow-ip` always keeps loopback on the list, whatever you pass. That is not a
hole — only a process already on this machine can present `127.0.0.1` as its
source address — and without it the panel host would be locked out of its own
panel, starting with the agent that runs there.

The final table prints the URL, the security entrance, the administrator
password and the node this panel now manages. It is written to
`/vkai-panel/etc/install-summary.txt`, mode 0600. **The password is not
recoverable**; write it down.

---

## What the installer does, in order

1. Checks the machine: root, systemd, RAM, disk, free ports, no competing panel.
2. Installs the dependencies, and Go/Node.js if needed.
3. Creates the `vkai` account and the `/vkai-panel` tree.
4. Writes `/vkai-panel/etc/.env` — **before** the UI build, because the UI
   inlines its API URL at build time.
5. Prepares PostgreSQL, then applies the migrations.
6. Builds the API, the agent and the UI.
7. Creates the first administrator.
8. Installs the systemd units, nginx, logrotate, SELinux rules and the firewall.
9. Starts `vkai-api` and `vkai-ui` and waits for `/health`.
10. **Registers this machine as the first managed node and enrols its agent.**
11. Prints the summary table.

### About the migrations

`core/migrations/001…023` is a contiguous, verified sequence. It is applied
first, and a failure there stops the install: a half-migrated database is not
something to continue on top of.

`core/migrations/pending/` holds schema that is written but not yet numbered into
that sequence. It is applied afterwards, because the alternative is shipping
features whose tables never exist on any installed panel — the panel host cannot
be a managed node without `server_local_node`, and would say so at every start.
Every file there creates its tables with `IF NOT EXISTS` and never alters an
existing one, so applying it twice is a no-op. A staged file that fails is a
**warning**, not a fatal error: it costs the feature that wanted it, not the
install, and it is retried on the next install or upgrade.

---

## The machine you installed on is the first node

Before this, VKAI Panel was a control plane with nothing under it. It installed,
it ran, an administrator could sign in — and it managed no machine at all, not
even the one it was on. Creating a single website meant finding a second server.
That is no longer true.

### The node row

The machine gets a row in `servers` like any other node, with its facts read
from the running system — hostname, IPv4 and IPv6, OS, kernel, CPU cores, total
RAM, total disk — rather than left null. Its `role` is `panel`, and its
`agent_token` carries the `retired-` prefix, because a node registered today
joins by holding a certificate and never by holding a string.

Nothing branches on that string. What actually marks the row as this machine is a
separate table, `server_local_node`, deliberately out of reach of the
operator-facing `PUT /servers/:id`: a flag that decides whether the panel runs a
command locally must not be one an API call can set.

### The identity it is keyed on

Not the hostname. Hostnames change under a running installation — a rename, a
cloud image rewriting it on first boot, a DHCP client — and keying on one would
turn a single machine into two rows the first time somebody ran `hostnamectl`.

The key is a **node id**: a UUID generated once at first registration and kept at
`/vkai-panel/etc/node.json`, mode 0600. It is that row's primary key, which is
what makes registration an upsert rather than a search.

Beside it is a **witness**: a salted SHA-256 of `/etc/machine-id`, recorded both
in `node.json` and on the row. The node id answers "which node is this row"; the
witness answers "is this row the machine I am standing on". The pair covers the
two ways that belief goes wrong:

- the database is restored onto another machine — the panel there has its own
  `node.json`, so no row claims to be it;
- the machine is cloned — `node.json` comes along, but the machine id underneath
  it does not, so the witness fails and the panel refuses the local route.

`/etc/machine-id` is the witness rather than the key on purpose: it is absent in
some minimal containers, regenerated by some cloud images on first boot, carried
along by a cloned disk, and documented by systemd as a value that should not be
exposed — which is why only its salted hash is ever written down.

### The agent

A row is only a record. What makes the machine manageable is a running agent
holding a certificate this panel issued. The installer, through
`vkai node register`:

1. creates `/vkai-panel/ssl/agent`, root-owned, mode 0700 — the panel account has
   no business reading the private key of the thing it authenticates;
2. asks the running panel for a **single-use enrolment token**;
3. writes `/vkai-panel/etc/agent.env` with the panel URL and that token;
4. runs `systemctl enable --now vkai-agent`;
5. waits for the agent to trade the token for a certificate;
6. **removes the spent token from the file.**

#### Why minting that token locally is safe here

Enrolment needs a single-use token because the token is the only thing that
authenticates a joining agent before it has a certificate. Across a network it
must be carried by a human, over a channel the panel does not control, to a
machine the panel cannot yet identify. Every risk in that sentence is about the
carrying: a bearer secret in transit, on a screen, in a clipboard, in shell
history.

Here there is no carrying. The panel and the agent are the same machine. The
token is requested over the loopback interface, written to a file only root can
read, and spent by a process on that machine seconds later. It never reaches a
network interface and never reaches a terminal. The only party who could read it
is root on this host — who already holds the CA private key, the database
password and the panel's TLS key — so it grants nothing that was not already
held, and it is deleted as soon as it is spent.

**None of that survives a move to a second machine.** The moment the token has to
reach another host it is a bearer secret in transit again, and it goes back
through the operator: mint it in the panel, paste it there. See
[Adding another machine](#adding-another-machine).

### `/vkai-panel/etc/agent.env`

Kept apart from `.env` because the installer rewrites `.env` wholesale on every
run, and what makes this machine a node has to survive that.
`vkai-agent.service` reads it *after* `.env`, so its values win.

| Variable | What it is |
|---|---|
| `VKAI_PANEL_URL` | Where the agent reaches this panel. Loopback plus the security entrance; when `VKAI_PANEL_DOMAIN` is pinned, the same URL a human uses, because the access gate checks the `Host` header |
| `VKAI_AGENT_STATE_DIR` | Where the agent's key and certificate live — outside the release tree, so an upgrade does not take the identity with it |
| `VKAI_PANEL_CA_FILE` | The panel's certificate, when the URL is https. On a default install it is self-signed and in no system trust store; verification is never switched off |
| `VKAI_AGENT_ENROLMENT_TOKEN` | Present only between minting and spending |

---

## Checking that it worked

```bash
vkai node list
```

```
ID        HOSTNAME          ADDRESS       ROLE   STATUS  AGENT   CPU  RAM      LAST SEEN         THIS HOST
--        --------          -------       ----   ------  -----   ---  ---      ---------         ---------
6bb68aff  panel.example.com 203.0.113.10  panel  active  online  24   7.6 GiB  2026-08-28 14:14  yes

Total: 1 node(s)
```

Also useful:

```bash
vkai status          # every service, including vkai-agent
vkai info            # URL, entrance, paths, and this machine's node id
systemctl is-active vkai-agent
```

In the panel, the machine appears under **Servers** with a `local_node` block on
it. Anywhere a server id is accepted, the alias `local` means this machine.

---

## If the node is missing: repairing by hand

Registration is deliberately **non-fatal**. Everything before it has already
produced a working panel; refusing to finish the install because the node could
not be registered would trade a panel that works and says what is missing for one
that does not exist. The summary table names the state and the command.

```bash
sudo vkai node register
```

It is idempotent: run it as often as you like. It never creates a second row for
this machine.

What it leaves alone, because it belongs to you: the node's **tags**,
**location**, **SSH port**, and — once the row exists — its **role** and
**status**.

What it refreshes every time, because it is measured on the machine and the
machine is the authority on it: **hostname**, **IPv4** and **IPv6**, **OS**,
**kernel**, **CPU cores**, **RAM** and **disk**. Note that hostname is among
them: renaming this node in the panel does not stick, because the next
registration reads the machine's own hostname again. Rename the machine
(`hostnamectl set-hostname`) rather than the row.

| Exit | Meaning |
|---|---|
| `0` | The node is registered and its agent is enrolled |
| `3` | The row is in place, the agent is **not** enrolled. The command printed what to do |
| other | Nothing was registered. The message says why |

Useful options:

| Option | When |
|---|---|
| `--enrolment-token <token>` | You would rather mint the token yourself in the panel (**Servers → Add agent**) than have the command do it |
| `--panel-url <url>` | The derived URL is wrong — usually a security entrance that changed. Compare with `vkai entrance` |
| `--skip-agent` | Write the row only, leave the service alone |
| `--rebind` | The panel's configuration was restored onto **rebuilt hardware** and the machine witness no longer matches. Off by default, so an unattended restart can never rebind |
| `--timeout 5m` | A slow machine |

---

## Adding another machine

Optional. Additional nodes, high availability and clustering are a layer you add
when you need one, not a precondition for the product being useful.

1. In the panel: **Servers → Add agent**. It mints a single-use token that
   expires in thirty minutes and carries this panel's CA fingerprint.
2. On the other machine, install the agent and start it with:

   ```bash
   VKAI_PANEL_URL=https://panel.example.com:8443/<entrance>
   VKAI_AGENT_ENROLMENT_TOKEN=vkai-enrol.v1....
   ```

3. The agent generates a key that never leaves that machine, trades the token for
   a certificate, and pins the CA the token named. The token is dead from that
   moment.

Full detail, including rotation and revocation: [AGENT_CHANNEL.md](AGENT_CHANNEL.md).

---

## Reinstalling over an existing database

Re-running the installer on a machine that already has one is safe and expected.

- The administrator's password is **not** reset.
- The panel port, the security entrance and the generated secrets are reused.
- Existing databases and customer data are untouched.
- The node row is found by its node id and refreshed, not duplicated — including
  after a rename.
- If `/vkai-panel/etc/node.json` was lost but the database still holds a node
  this machine registered before, that node is **adopted** rather than a second
  one created. The machine witness is what makes the match.
- The agent keeps the certificate it holds; it is not re-enrolled.

If the panel's configuration was restored onto **different hardware**,
registration refuses rather than adopting the row of another machine. That is the
one case that needs a decision from you: `sudo vkai node register --rebind` if
the move was deliberate, and an investigation if it was not.

---

## Uninstalling

```bash
sudo bash deploy/install.sh --uninstall            # program files and services
sudo bash deploy/install.sh --uninstall --purge    # everything, including data
```

Without `--purge`, `/vkai-panel/etc` survives — with it the node identity, the
agent's configuration and the panel's secrets — so a reinstall recognises the
same machine and the same node.

---

## Troubleshooting

| Symptom | What it means | What to do |
|---|---|---|
| Summary says *agent NOT enrolled* | The row exists; the agent has no certificate | `journalctl -u vkai-agent -n 80`, then `sudo vkai node register` |
| `vkai node list` shows nothing | No node was registered | `sudo vkai node register` |
| `vkai node list` shows nodes but none is `THIS HOST` | `node.json` is missing or names a node no row carries | `sudo vkai node register` |
| *"no server_local_node table"* | The staged migration did not apply | `psql -d vkai_panel -f /vkai-panel/core/migrations/pending/local_node.sql`, then `sudo vkai node register` |
| *"the stored node identity does not belong to this machine"* | The witness disagrees: restored configuration, or a clone | `--rebind` only if the move onto new hardware was deliberate |
| Minting answers **404** | The URL carries the wrong security entrance | Compare with `vkai entrance`, then `sudo vkai node register --panel-url <url>` |
| Minting answers **401/403** | `VKAI_JWT_SECRET` does not match the running API, or there is no active administrator | Mint in the panel and pass `--enrolment-token` |
| `vkai-agent` restarts every ten seconds | It is not enrolled and has no token | `sudo vkai node register` |
| The panel is unreachable from your browser | The allow list or the entrance | `vkai info`, `vkai entrance`, `vkai port` |

Further reading: [DEPLOYMENT.md](DEPLOYMENT.md) · [CONFIGURATION.md](CONFIGURATION.md) ·
[AGENT_CHANNEL.md](AGENT_CHANNEL.md) · [UPGRADE.md](UPGRADE.md) ·
[TROUBLESHOOTING.md](TROUBLESHOOTING.md)
