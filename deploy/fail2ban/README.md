# fail2ban integration

The panel defends itself against credential guessing on its own. The limiter in
`core/internal/ratelimit` counts failures per account, per source address and
per address-account pair, delays attempts progressively and locks a pair that
keeps failing. None of that requires fail2ban, and **the panel works exactly the
same whether or not fail2ban is installed.**

What fail2ban adds is where the block happens. Without it, a blocked attacker
still opens a TCP connection, still completes a TLS handshake, still sends a
request the panel has to parse, and still costs a Redis round trip - several
thousand times a minute if they want to. With it, the same attacker is dropped
by the kernel's firewall and pays for a refused connection. The panel decides
who is attacking; fail2ban makes that decision cheap to enforce.

## What the panel provides

The panel writes one line per authentication attempt to its authentication log,
by default `/vkai-panel/logs/auth.log`:

```
2026-08-28T09:14:02Z vkai-auth outcome=failure reason=invalid_credentials ip=203.0.113.9 account=admin scope=login path=/api/v1/auth/login request_id=8f2c1d4e
```

Fields, in a fixed order:

| field | meaning |
|---|---|
| `outcome` | `success`, `failure` (credential presented and rejected) or `blocked` (the panel's limiter refused the attempt before checking) |
| `reason` | `ok`, `invalid_credentials`, `locked`, `throttled`, `limiter_unavailable` |
| `ip` | the individual source address, resolved through the trusted-proxy list, **not** taken from an unverified `X-Forwarded-For` |
| `account` | the account the attempt was aimed at, sanitised to `[A-Za-z0-9._@:/+-]` and truncated |
| `scope` | which credential: `login`, `refresh`, `password_reset`, `api_key`, `two_factor`, `agent_enrol` |
| `path` | the route |
| `request_id` | correlates with the panel's request log |

The `ip` field is the caller's individual address, because that is what
fail2ban has to hand to the firewall. The panel's own limiter counts an IPv6
caller by its `/64` instead - an attacker is routinely handed a whole `/64`, so
counting single IPv6 addresses would be the same as not counting - which means
the panel may lock a prefix while the jail bans the single address that
triggered it. That is the right split: the panel knows about prefixes, iptables
rules do not.

IPv6 lines need fail2ban 0.10 or newer, where `<HOST>` covers IPv6. On 0.9 the
IPv4 lines still match and the IPv6 ones are simply not banned - the panel's own
limiter still handles them.

Every value is sanitised before it is written. This matters: the `account`
field is chosen by the caller, and without sanitising, an attacker could pick a
username containing a newline and a forged `outcome=failure ip=...` line, and
use this jail to have fail2ban ban an address of their choosing - the
operator's, for instance.

## Enabling it

fail2ban must already be installed (`apt install fail2ban`,
`dnf install fail2ban`). Then, as root:

```sh
./enable.sh
```

or by hand:

```sh
install -m 0644 filter.d/vkai-panel-auth.conf /etc/fail2ban/filter.d/
install -m 0644 jail.d/vkai-panel.conf        /etc/fail2ban/jail.d/
systemctl reload fail2ban    # or: systemctl restart fail2ban
```

Check it took:

```sh
fail2ban-client status vkai-panel-auth
```

Test the filter against the real log before trusting it:

```sh
fail2ban-regex /vkai-panel/logs/auth.log /etc/fail2ban/filter.d/vkai-panel-auth.conf
```

That prints how many lines matched. Zero matches with a non-empty log means the
filter and the panel have drifted apart - report it, because the Go test
`TestFail2banFilterMatchesAuthLogLines` should have caught it first.

## Things to adjust

- **`logpath`** if `VKAI_PANEL_ROOT`, `VKAI_LOG_ROOT` or `VKAI_AUTH_LOG` moves
  the log.
- **`port`** to match the panel's configured port (default 30110) and the ports
  of any reverse proxy in front of it.
- **`ignoreip`** - add the operator's own address and any office range. This is
  the single most important line in the jail. Read the warning below.
- **`banaction = nftables-multiport`** on a host that uses nftables rather than
  iptables.

## Do not let it lock you out

The jail bans an address, not an account, and it cannot tell your fingers from
an attacker's script. Two defences:

1. `ignoreip` in the jail, for fail2ban.
2. `VKAI_AUTH_ALLOWLIST` in the panel's environment, a comma-separated list of
   addresses or CIDR blocks that the panel's own limiter never blocks or locks.
   Attempts from those addresses are still logged, so this does not blind the
   jail - set both if you want an address genuinely exempt.

If you are already locked out, `fail2ban-client set vkai-panel-auth unbanip
<address>` releases the firewall side, and the panel's own lock expires on its
own within an hour at worst.

## Related settings on the panel side

| variable | default | effect |
|---|---|---|
| `VKAI_AUTH_LOG` | `<log root>/auth.log` | where the lines above are written |
| `VKAI_TRUSTED_PROXIES` | `127.0.0.0/8,::1/128` | whose `X-Forwarded-For` is believed when resolving `ip=` |
| `VKAI_AUTH_ALLOWLIST` | empty | addresses the panel's limiter never blocks |
| `VKAI_AUTH_LIMIT_PAIR_LOCK` | `8` | failures before an address-account pair locks |
| `VKAI_AUTH_LIMIT_ADDRESS` | `60` | failures per address per 15 minutes |
| `VKAI_AUTH_LIMIT_ACCOUNT` | `30` | failures per account per 15 minutes from addresses that have never signed in to it |
| `VKAI_AUTH_LIMIT_FAIL_OPEN` | `false` | allow authentication when Redis is unreachable - see below |
| `VKAI_AUTH_MIN_RESPONSE` | `250ms` | the floor every failed authentication is padded to |

`VKAI_AUTH_LIMIT_FAIL_OPEN` deserves a sentence of its own. By default the
panel refuses authentication when it cannot reach Redis, because it cannot then
count attempts, and an attacker who can arrange a Redis outage would otherwise
have arranged for the brute force protection to be off. Setting it to `true`
trades that for availability. It is the wrong trade for a panel that holds root
on every server it manages.

## Log rotation

The panel rotates its own authentication log (50 MB, ten generations,
compressed). Do **not** add a separate logrotate rule for it: two rotators on
one file produce gaps, and a gap is a window in which the jail sees nothing.
fail2ban follows the live file and handles rotation on its own.
