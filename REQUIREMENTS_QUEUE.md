# Pending requirements, recorded so nothing is lost between rounds

## Website creation: the document root fills itself in

The backend already does this. `service.CreateWebsite` calls `config.SiteRoot(req.Domain)`, which
produces `/vkai-panel/www/domains/<domain>`, and only overrides it when the caller supplies
`root_dir` - validated as absolute, traversal-free and inside the web root.

The interface is what is wrong: the create form asks the operator to type a path.

Required behaviour, matching aaPanel:
- The document root fills itself in from the domain as the operator types it, before submit.
- It stays editable. An operator who wants a different path sets one, and that is the only case where
  `root_dir` is sent at all.
- If the operator edits it and then changes the domain, do not silently overwrite what they typed.
  Track whether the field was touched; overwrite only while it is still the generated value.
- Show the path that will actually be created, not a placeholder, so there is no guessing about
  whether a trailing segment gets added.

## Header: operating system and uptime

- The header calls `/api/v1/monitoring/system`, which does not exist. The nearest real route is
  `/api/v1/system`, and it returns Go runtime facts - `os: "linux"`, `arch: "amd64"`, heap statistics -
  not the host's operating system.
- The correct source for the OS is the `servers` row, which already holds
  `Ubuntu 24.04.3 LTS` and kernel `6.8.0-79-generic` for the panel host.
- Uptime has no source until the collector lands; it comes from the agent's `system.info`.

## Dashboard: Traffic and Software panels

Both are empty and say so honestly. `SOFTWARE_ITEMS` is a hardcoded empty array; the traffic panel
takes series props nothing supplies. The App Store work owns the software inventory and the collector
owns the traffic series. Wire the panels to them once both land - do not fabricate either.

---

## Found on the live demo server (116.118.2.44), 2026-08-28

Measured from the host's own journal and TLS handshake, not inferred.

### Confirmed working

- **Panel TLS is a real Let's Encrypt certificate.** `issuer=C=US, O=Let's Encrypt, CN=YE1`,
  `subject=CN=panel.vkai.vn`, valid `Aug 28 10:29 2026` → `Nov 26 10:29 2026`.
  `curl` without `-k` reports `ssl_verify=0`. No self-signed fallback is in play.
- `vkai-api`, `vkai-ui` and `vkai-agent` are all active and running.
  (The unit names are `vkai-api` / `vkai-ui`, not `vkai-core` / `vkai-panel`.)

### Broken

1. **The certificate cannot renew.** `vkai-cert-renew.service` has failed on every run
   (07:05 and 15:12 today) with `unknown flag: --identifier`. `deploy/vkai.sh` calls
   `vkai panel cert issue|renew`, and that subcommand does not exist — `NewPanelCmd()`
   has only `info`, `port`, `entrance`, `allow-ip`, `domain`. The script's `rc == 2`
   branch was meant to catch "no such command", but cobra exits **1** for an unknown
   flag, so it never fires. The certificate on the host is real but is a dead end: it
   expires 26 Nov and nothing can replace it.
   → being implemented against the existing `internal/acme` + `internal/tlsmanager`.

2. **`vkai-upgrade-check.service` fails daily.** `internal/cli/upgrade.go` ships a
   deliberate stub (`stubUpgrader`, marked `SWAP HERE`) that returns
   `errUpgradeNotBuilt` and exit 2. A unit that goes red every day for a permanent,
   known, documented condition is not news — and it sits directly next to the
   cert-renew failure, which *is* news. Reporting "unsupported" as a successful check
   (exit 0, recorded in the state file, surfaced in the panel as "update checking is
   not configured" rather than "up to date") is the honest behaviour until a release
   feed exists. Deferred only to avoid editing `internal/cli/` while it is being
   translated.

3. **Vietnamese strings ship to GitHub in the CLI.** `Loi: unknown flag: --identifier`
   in the system journal is the proof. `cmd/panelctl/main.go:30,41` and roughly eight
   sites in `internal/cli/panel.go`. The rule is English everywhere on GitHub, with
   Vietnamese only in the UI through i18n.

### Web terminal — fixed, with the real cause recorded

The terminal was reported broken three times. The socket path was only the first
layer:

- the browser connected to `/api/ws`, which is mounted nowhere (`/api/v1/ws` is);
- it put the **access token in the query string**, which nginx writes to its access
  log and the browser keeps in history — replaced by a single-use 30-second ticket;
- and behind the socket **there was no shell at all**. `internal/websocket` is a
  broadcast hub: no PTY, no `terminal_input` handling, nothing. Fixing only the path
  would have produced a terminal that connects and ignores every keystroke, which
  looks like it works and is harder to diagnose than a disconnection.

`internal/terminal` now runs a real login shell on a pseudo-terminal, with tests that
start one, assert `stty size` reflects a resize, and assert the process dies when the
socket closes.
