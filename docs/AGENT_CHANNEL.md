# The panel-to-agent channel

How the panel and the `vkaid` agent authenticate each other, how an agent joins,
what to do when one loses its certificate, and what this design does not
protect against.

Audience: operators running a VKAI Panel installation, and developers touching
`core/internal/agentpki`, `core/internal/agentclient` or `agent/`.

---

## 1. What changed, and why it had to

Until this change the channel was one static string. `deploy/install.sh`
generated a single `VKAI_AGENT_TOKEN`, wrote it into `/vkai-panel/etc/.env`, and
every managed server got the same one. The agent sent it in an `X-Agent-Token`
header over plain HTTP, compared it with `!=`, and then served `/execute`, which
took a program name and an argument list and ran them as root.

The consequence was not subtle. Anyone who read that one string - from a config
file, a log line, a process listing, a backup, or the network - had root on every
server the panel managed, and there was nothing behind the credential to stop
them.

What replaces it:

| Before | Now |
|---|---|
| One shared secret, identical everywhere | A private key per agent, generated on that server, that never leaves it |
| Plain HTTP | TLS 1.3, both sides authenticated |
| Panel not authenticated to the agent at all beyond the token | Panel presents a client certificate from an internal CA the agent pins |
| Agent not authenticated to the panel beyond the token | Agent presents a certificate the panel itself issued to that agent |
| Secret never rotated | 24 hour certificates, renewed at half life |
| No way to cut off one server | Serial deny list, effective on the next handshake |
| `/execute`: arbitrary command as root | A closed set of named operations with typed arguments |

---

## 2. The pieces

```
panel host                                    managed server
----------                                    --------------
/vkai-panel/ssl/agent-pki/                    /vkai-panel/ssl/agent/
  ca.key      the CA private key, 0600          agent.key   this agent's key, 0600
  ca.crt      the CA certificate, 0600          agent.crt   its certificate, 0600
  panel-client.key  panel's own key, 0600       ca.crt      the panel CA, 0600
  panel-client.crt  panel's own cert, 0600      state.json  identity + deny list
  state.json  enrolments, agents, deny list
```

The CA private key is generated on the panel host on first use and never leaves
it. Nothing copies it to a managed server; agents receive only the CA
*certificate*, which is a public key.

Identity is the certificate, not the address. Neither side checks a host name,
because customer servers change IP addresses, move behind NAT and get rebuilt.
The panel accepts exactly the certificate it issued to that agent; the agent
accepts exactly a certificate its panel's CA issued that carries the panel role.

Certificate roles live in the subject's `OU`: `vkai-panel` or `vkai-agent`. That
is what stops a compromised agent from turning round and driving its neighbours -
its certificate is genuine, but it is the wrong role.

---

## 3. Enrolling an agent, end to end

1. **Mint a token.** An administrator calls
   `POST /api/v1/agent-pki/enrolments` (or uses the panel UI), optionally naming
   the server and hostname. The reply contains the token exactly once:

   ```
   vkai-enrol.v1.<id>.<secret>.<ca-fingerprint>
   ```

   The panel stores only SHA-256 of the secret, so it cannot show the token
   again, and a copy of the state file does not let its reader enrol. The token
   expires in 30 minutes by default (`ttl_seconds`, between 60 and 86400).

2. **Install the agent** on the target server and start it with:

   ```
   VKAI_PANEL_URL=https://panel.example.vn
   VKAI_AGENT_ENROLMENT_TOKEN=vkai-enrol.v1....
   ```

3. **The agent generates its key** on that server, builds a certificate signing
   request, and posts it with the token to `POST /api/v1/agent-pki/enrol`. Only
   a public key crosses the wire.

4. **The panel spends the token** - atomically, so two installers racing with
   one token produce one certificate and one failure - verifies the request's
   self-signature, and issues a 24 hour certificate.

5. **The agent checks what it was given.** The reply carries the CA certificate;
   the agent accepts it only if its public key matches the fingerprint that was
   inside the token the operator pasted. The operator is the trusted channel, so
   a machine in the middle cannot substitute its own CA.

6. **The agent writes** key, certificate and CA at 0600 in
   `/vkai-panel/ssl/agent/` and starts listening on port 30111 with mutual TLS.

The token is now dead. A restart uses the identity on disk; leaving the spent
token in the environment file changes nothing, but remove it anyway.

> **If the safe entrance is enabled** (`PANEL_ENTRANCE`), `VKAI_PANEL_URL` must
> include the entrance prefix, for example
> `https://panel.example.vn/vkai_7f3a`. The agent's later calls go to the same
> base URL, so nothing else has to change.

---

## 4. Rotation

The agent checks hourly and renews once it is past half life, calling
`POST /api/v1/agent-pki/renew` with a fresh certificate request, signed with the
private key of the certificate it currently holds. No enrolment token is
involved, which is what makes rotation unattended.

A rotation cannot lock an agent out:

- The panel keeps accepting the **previous** certificate for a 24 hour overlap
  window after it was superseded (and never past that certificate's own expiry).
  An agent that renewed but never received the answer - a dropped connection, a
  crash between two writes, a reverted snapshot - keeps working and simply
  renews again.
- The agent replaces its files only after the new certificate has been received
  and verified.
- A failed renewal is logged and retried on the next tick. At half life there
  are twelve hourly attempts left before anything expires.

Only one previous certificate is kept. After two rotations the oldest is
finished, overlap window or not.

---

## 5. Revocation

`POST /api/v1/agent-pki/agents/{agent_id}/revoke` puts every serial that agent
holds on a deny list. The list is consulted on **every** handshake and on every
signed request, so the next connection from that agent fails - it does not wait
for the certificate to expire.

`DELETE /api/v1/agent-pki/agents/{agent_id}` does the same and then forgets the
record. The serials stay denied: a record that is gone must not become a record
that is trusted again.

`GET /api/v1/agent-pki/deny-list` shows what is refused and why.

**What this does not do, stated plainly:**

- The deny list is authoritative *on the panel*. Agents learn about a revoked
  **panel** certificate when the panel next reaches them - on any status report,
  and through the `pki.sync` operation. An agent that is unreachable keeps
  accepting the old panel certificate until it expires, which is why the
  certificates are short.
- The list only grows. Expired entries could be pruned; they are kept because
  the list is small and a deny list that forgets is one you have to reason about.
- It is not a remedy for the CA key leaking. If `ca.key` leaks, every
  certificate is worthless: build a new CA and re-enrol every agent (section 7).

---

## 6. What the agent will do, and what it will not

The agent serves, behind mutual TLS and nothing else:

| Operation | Arguments | What it does |
|---|---|---|
| `system.info` | none | static host facts |
| `system.metrics` | none | current resource usage |
| `service.list` | none | status of every managed unit |
| `service.status` | `name` | status of one unit |
| `service.control` | `name`, `action` | `start`, `stop`, `restart` or `reload` one unit |
| `agent.info` | none | the agent's identity and certificate |
| `pki.sync` | `denied_serials` | accept the panel's deny list |

`name` must be a unit the agent manages (a fixed list, plus versioned
`phpX.Y-fpm`). `action` must be one of those four verbs. Nothing else reaches a
command line, and no operation takes a program name from the caller.

`GET /v1/operations` lists what a given agent build offers - useful when the
panel and the agent are different versions.

### The escape hatch

`VKAI_AGENT_ALLOW_RAW_EXEC=true` registers one more operation, `exec.raw`, which
runs an arbitrary command as root. It exists only because removing a capability
outright can strand a deployment that depends on it.

It is off by default, it is **absent** rather than merely refused when off, it
announces itself at agent startup, and every invocation is logged with the full
command line. Treat it as a temporary measure with a date on it. While it is on,
panel compromise is root on that server with nothing behind it - which is the
property this whole design exists to remove.

---

## 7. Runbook

### An agent lost its certificate (disk wiped, snapshot reverted, key deleted)

There is nothing to recover: the private key was only ever on that server. Do
not try to copy an identity from another host - two agents sharing a key means
revoking one revokes both.

1. In the panel, revoke the old identity:
   `POST /api/v1/agent-pki/agents/{agent_id}/revoke` with a reason. The
   certificate that may still exist on a stolen disk is now refused.
2. Mint a fresh enrolment token: `POST /api/v1/agent-pki/enrolments`.
3. On the server, make sure the state directory is empty
   (`/vkai-panel/ssl/agent/`), set `VKAI_AGENT_ENROLMENT_TOKEN` and restart
   `vkai-agent`. It enrols and gets a new identity.
4. Delete the old record once the new agent is reporting:
   `DELETE /api/v1/agent-pki/agents/{old_agent_id}`.

An agent that has been offline longer than its certificate's lifetime is the
same procedure: an expired certificate cannot be renewed, so it re-enrols.

### The panel's own client certificate was lost

Delete `panel-client.crt` and `panel-client.key` from
`/vkai-panel/ssl/agent-pki/` and restart the API. The panel issues itself a new
one at startup. Agents accept it immediately: it comes from the same CA.

If it was not merely lost but *leaked*, revoke it first
(`POST /api/v1/agent-pki/agents/panel/revoke`) so the serial reaches agents on
their next status report, then delete the files and restart.

### The CA key leaked

Everything issued by it is worthless.

1. Move `/vkai-panel/ssl/agent-pki/` aside.
2. Restart the API. A new CA is created on first use.
3. Re-enrol every agent (section 3). There is no shortcut: the agents pin the
   old CA and will refuse the new panel certificate until they are re-enrolled.

### Checking the state of the fleet

```
GET /api/v1/agent-pki/agents          # who is enrolled, with serials and expiry
GET /api/v1/agent-pki/agents/{id}     # one agent, including its previous certificate
GET /api/v1/agent-pki/deny-list       # what is revoked
```

---

## 8. Configuration

Panel side: nothing is required. The CA is created under the panel SSL directory
on first use. `VKAI_SSL_ROOT` moves it with the rest of the SSL tree.

Agent side:

| Variable | Default | Meaning |
|---|---|---|
| `VKAI_PANEL_URL` | (required) | Base URL of the panel, including the entrance prefix if one is set |
| `VKAI_AGENT_ENROLMENT_TOKEN` | (empty) | One-time token, first start only |
| `VKAI_AGENT_STATE_DIR` | `/vkai-panel/ssl/agent` | Key, certificate, CA and state |
| `VKAI_AGENT_PORT` | `30111` | Control channel port |
| `VKAI_AGENT_BIND` | (every interface) | Bind to one interface |
| `VKAI_AGENT_STATUS_INTERVAL` | `30s` | How often it reports in, and how quickly it learns about a revocation |
| `VKAI_PANEL_CA_FILE` | (system roots) | Trust anchor for the panel's *web* certificate, if it is not publicly trusted |
| `VKAI_PANEL_TLS_INSECURE` | `false` | Skip verification of the panel's web certificate. Logged. Avoid |
| `VKAI_AGENT_ALLOW_RAW_EXEC` | `false` | The escape hatch. Leave off |

`VKAI_AGENT_TOKEN` is obsolete. If it is still in the environment file the agent
logs a warning at startup and ignores it; remove it.

---

## 9. Known gaps

- **The agent still listens.** The panel dials in, so an agent behind NAT or a
  restrictive firewall needs an inbound path on port 30111. Reversing the
  direction - the agent holding an outbound stream to the panel - is the next
  step and does not change any of the certificate handling above.
- **Enrolment trusts the network once.** The CA fingerprint in the token stops a
  substituted CA, but a machine in the middle could still steal the token in
  flight and enrol itself. The token is single-use, so the theft is visible: the
  real agent's enrolment fails. Serving the panel over a trusted certificate
  closes it properly.
- **State is a file, not a table.** Enrolments, issued certificates and the deny
  list live in `state.json` next to the CA key. This is deliberate - it is the
  CA's own state and belongs with the CA - but it means one API process. A panel
  that grows to several needs a database-backed `agentpki.Store`; the interface
  is the seam for it.
