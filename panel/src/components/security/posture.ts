/**
 * Turning what the API answered into what the panel is willing to claim.
 *
 * Every item below is derived from a value the panel actually read back from an
 * endpoint. Nothing here is derived from a compiled-in default, a build flag or
 * an assumption about how the machine is usually set up, because the whole
 * value of this screen is that an operator can trust a green tick on it.
 *
 * Three rules the items obey:
 *
 *  1. A source that failed produces `unknown`, never `ok` and never `off`.
 *     "The panel could not read this" and "this is switched off" are different
 *     facts and an operator acts on them differently.
 *
 *  2. A setting the panel stores but does not apply produces `unenforced`.
 *     The WAF is the live example: internal/service/waf.go persists rules and
 *     never writes a web server configuration, so a page that ticked "WAF: on"
 *     would be describing a database table, not a defence.
 *
 *  3. A control with no endpoint at all still appears, as `unknown`, with the
 *     reason. Omitting it would let an operator conclude the panel had checked
 *     and found nothing wrong.
 */

import type { PostureItem } from './types';
import { sourceReason, type PostureSources } from './usePosture';

/** How far back the failed sign-in count looks. */
export const FAILURE_WINDOW_HOURS = 24;

function withinWindow(iso: string, hours: number): boolean {
  const at = Date.parse(iso);
  if (Number.isNaN(at)) return false;
  return Date.now() - at <= hours * 60 * 60 * 1000;
}

function plural(count: number, singular: string, pluralForm?: string): string {
  return count === 1 ? singular : (pluralForm ?? `${singular}s`);
}

// ---------------------------------------------------------------------------
// Controls with no endpoint behind them.
//
// These are listed here rather than left out so that the Overview count of
// "not verifiable" is the true count. Each one names what would have to exist.
// ---------------------------------------------------------------------------

const UNIMPLEMENTED: PostureItem[] = [
  {
    id: 'ssh-password-auth',
    title: 'SSH password authentication',
    state: 'unknown',
    risk: 'critical',
    detail:
      'Password authentication on SSH is the single most attacked door on a hosting node.',
    reason:
      'This panel has no SSH endpoint. It cannot read sshd_config, so it cannot tell you whether password login is on or off.',
    fix: { label: 'What is missing', tab: 'ssh' },
  },
  {
    id: 'ssh-root-login',
    title: 'SSH root login',
    state: 'unknown',
    risk: 'critical',
    detail: 'Direct root login over SSH removes the audit trail of who did what.',
    reason:
      'This panel has no SSH endpoint. PermitRootLogin is not readable from the API.',
    fix: { label: 'What is missing', tab: 'ssh' },
  },
  {
    id: 'website-protections',
    title: 'Per-site web protections',
    state: 'unknown',
    risk: 'high',
    detail:
      'Directory listing, PHP function restrictions, upload filtering and hotlink protection are per-site settings that live in the web server and PHP configuration.',
    reason:
      'This panel has no endpoint that reads or writes per-site protections. Nothing on this screen can report their state.',
    fix: { label: 'What is missing', tab: 'website-security' },
  },
  {
    id: 'kernel-hardening',
    title: 'Kernel and service hardening',
    state: 'unknown',
    risk: 'medium',
    detail:
      'Kernel network parameters, core dumps, and the accounts that system services run as.',
    reason:
      'This panel has no endpoint that reads sysctl values or service accounts, and none that applies a hardening profile.',
    fix: { label: 'What is missing', tab: 'server-security' },
  },
  {
    id: 'compiler-access',
    title: 'Compiler access from web processes',
    state: 'unknown',
    risk: 'medium',
    detail:
      'On shared hosting a compiler reachable from a web process turns a file upload into a running binary.',
    reason:
      'This panel has no endpoint that reports which compilers are installed or whether the web user can execute them.',
    fix: { label: 'What is missing', tab: 'compiler-access' },
  },
  {
    id: 'brute-force-policy',
    title: 'Credential limiter thresholds',
    state: 'unknown',
    risk: 'high',
    detail:
      'The panel does run a layered credential limiter in front of every credential endpoint, counting failures per address, per account and per pair.',
    reason:
      'No endpoint reports the thresholds in force, the addresses currently locked, or the allow list. The numbers can be changed by environment variables, so this screen will not print the built-in defaults as if they were live.',
    fix: { label: 'What is readable', tab: 'brute-force' },
  },
];

// ---------------------------------------------------------------------------

/** Builds the full posture list from the loaded sources. Unsorted; rank it. */
export function buildPosture(sources: PostureSources): PostureItem[] {
  const items: PostureItem[] = [];

  // -- Panel access ---------------------------------------------------------
  const panel = sources.panel;
  if (panel.status !== 'ok' || !panel.data) {
    const reason = sourceReason(panel, 'the panel access settings');
    for (const [id, title, risk] of [
      ['panel-tls', 'Panel served over TLS', 'critical'],
      ['panel-entrance', 'Security entrance', 'high'],
      ['panel-allowlist', 'Panel IP allow list', 'high'],
      ['panel-port', 'Panel listening port', 'low'],
    ] as const) {
      items.push({
        id,
        title,
        state: 'unknown',
        risk,
        detail: 'Read from GET /api/v1/panel/settings.',
        reason,
        fix: { label: 'Panel settings', href: '/settings' },
      });
    }
  } else {
    const view = panel.data;
    const tls = view.tls;

    // TLS. `source` is what actually produced the certificate on the wire,
    // which is not always the configured `mode` - an ACME order that failed
    // leaves the panel on a self-signed fallback. Reporting `mode` there would
    // be exactly the silent lie this screen exists to avoid.
    if (!tls.enabled) {
      items.push({
        id: 'panel-tls',
        title: 'Panel served over TLS',
        state: 'off',
        risk: 'critical',
        detail:
          'The panel is served over plain HTTP. Every sign-in, every session token and every configuration change crosses the network readable.',
        fix: { label: 'Turn on TLS', href: '/settings' },
      });
    } else if (tls.expired) {
      items.push({
        id: 'panel-tls',
        title: 'Panel served over TLS',
        state: 'attention',
        risk: 'critical',
        detail: 'The certificate on the panel port has expired. Browsers will refuse it.',
        fix: { label: 'Reissue the certificate', href: '/settings' },
      });
    } else if (tls.mode && tls.source && tls.mode !== tls.source) {
      items.push({
        id: 'panel-tls',
        title: 'Panel served over TLS',
        state: 'attention',
        risk: 'high',
        detail: `Configured for "${tls.mode}" but the certificate actually on the wire came from "${tls.source}". Something the panel was asked to do did not happen.`,
        fix: { label: 'Panel settings', href: '/settings' },
      });
    } else if (tls.inconsistency) {
      items.push({
        id: 'panel-tls',
        title: 'Panel served over TLS',
        state: 'attention',
        risk: 'high',
        detail: tls.inconsistency,
        fix: { label: 'Panel settings', href: '/settings' },
      });
    } else if (tls.expiring_soon) {
      const days = tls.expires_in_days;
      items.push({
        id: 'panel-tls',
        title: 'Panel served over TLS',
        state: 'attention',
        risk: 'medium',
        detail:
          days === null
            ? 'The certificate on the panel port expires soon.'
            : `The certificate on the panel port expires in ${days} ${plural(days, 'day')}.`,
        fix: { label: 'Reissue the certificate', href: '/settings' },
      });
    } else if (tls.self_signed || tls.source === 'self-signed') {
      items.push({
        id: 'panel-tls',
        title: 'Panel served over TLS',
        state: 'attention',
        risk: 'medium',
        detail:
          'TLS is on with a self-signed certificate. The traffic is encrypted, but a browser cannot tell this panel apart from an impostor on the same address.',
        fix: { label: 'Panel settings', href: '/settings' },
      });
    } else {
      items.push({
        id: 'panel-tls',
        title: 'Panel served over TLS',
        state: 'ok',
        risk: 'critical',
        detail: `Certificate in use came from "${tls.source || tls.mode || 'the configured source'}"${
          tls.chain_complete ? ' with a complete chain' : ''
        }.`,
      });
    }

    // The security entrance: a secret path prefix in front of the sign-in page.
    items.push(
      view.entrance_enabled
        ? {
            id: 'panel-entrance',
            title: 'Security entrance',
            state: 'ok',
            risk: 'high',
            detail: `A request that does not present the entrance never reaches the sign-in page. Entrance in use: ${view.entrance_masked || 'set'}.`,
          }
        : {
            id: 'panel-entrance',
            title: 'Security entrance',
            state: 'off',
            risk: 'high',
            detail:
              'Anything that can reach the panel port is shown the sign-in page. Every scanner on the internet finds it.',
            fix: { label: 'Turn on the entrance', href: '/settings' },
          }
    );

    // IP allow list.
    const allowed = view.allowed_ips ?? [];
    items.push(
      allowed.length > 0
        ? {
            id: 'panel-allowlist',
            title: 'Panel IP allow list',
            state: 'ok',
            risk: 'high',
            detail: `${allowed.length} ${plural(allowed.length, 'entry', 'entries')} allowed. Connections from anywhere else are refused before authentication.`,
          }
        : {
            id: 'panel-allowlist',
            title: 'Panel IP allow list',
            state: 'attention',
            risk: 'high',
            detail: `The panel accepts connections from any address. Your address is ${view.client_ip || 'not reported'}.`,
            fix: { label: 'Add an allow list', href: '/settings' },
          }
    );

    // Listening port. The configuration layer refuses 80 and 443 outright, so
    // this is normally fine; it is here because an operator looking for the
    // port should find it stated rather than have to guess.
    const onWebPort = view.port === 80 || view.port === 443;
    items.push({
      id: 'panel-port',
      title: 'Panel listening port',
      state: onWebPort ? 'attention' : 'ok',
      risk: onWebPort ? 'high' : 'low',
      detail: onWebPort
        ? `The panel is on port ${view.port}, which belongs to the hosted websites.`
        : `Port ${view.port} on ${view.bind}, separate from the website ports.`,
      fix: onWebPort ? { label: 'Change the port', href: '/settings' } : undefined,
    });

    if (view.restart_pending) {
      const reasons = view.restart_reasons ?? [];
      items.push({
        id: 'panel-restart-pending',
        title: 'Saved settings not yet live',
        state: 'attention',
        risk: 'high',
        detail:
          reasons.length > 0
            ? `Written to disk but not applied in the running process: ${reasons.join('; ')}.`
            : 'Some settings are written to disk but not applied in the running process.',
        fix: { label: 'Panel settings', href: '/settings' },
      });
    }
  }

  // -- Two-factor on the signed-in account ----------------------------------
  const twoFactor = sources.twoFactor;
  if (twoFactor.status !== 'ok' || !twoFactor.data) {
    items.push({
      id: 'two-factor',
      title: 'Two-factor on your account',
      state: 'unknown',
      risk: 'critical',
      detail: 'A second factor is what stops a leaked panel password from being enough.',
      // A 503 here is the documented "the panel master key is missing" state.
      reason: sourceReason(twoFactor, 'your two-factor status'),
      fix: { label: 'Account settings', href: '/settings' },
    });
  } else if (twoFactor.data.enabled) {
    const remaining = twoFactor.data.recovery_codes_remaining;
    items.push(
      twoFactor.data.recovery_codes_low
        ? {
            id: 'two-factor',
            title: 'Two-factor on your account',
            state: 'attention',
            risk: 'medium',
            detail: `Enabled, but only ${remaining} recovery ${plural(remaining, 'code')} left. Running out locks you out of your own panel.`,
            fix: { label: 'Regenerate recovery codes', href: '/settings' },
          }
        : {
            id: 'two-factor',
            title: 'Two-factor on your account',
            state: 'ok',
            risk: 'critical',
            detail: `Enabled, with ${remaining} recovery ${plural(remaining, 'code')} in reserve. This is your account only - it says nothing about the other operators.`,
          }
    );
  } else {
    items.push({
      id: 'two-factor',
      title: 'Two-factor on your account',
      state: twoFactor.data.pending_enrolment ? 'attention' : 'off',
      risk: 'critical',
      detail: twoFactor.data.pending_enrolment
        ? 'Enrolment was started but never confirmed, so your account still signs in with a password alone.'
        : 'Your account signs in with a password alone. This is your account only - it says nothing about the other operators.',
      fix: { label: 'Set up two-factor', href: '/settings' },
    });
  }

  // -- Firewall -------------------------------------------------------------
  const firewall = sources.firewall;
  if (firewall.status !== 'ok' || !firewall.data) {
    items.push({
      id: 'firewall-rules',
      title: 'Firewall rules managed by the panel',
      state: 'unknown',
      risk: 'high',
      detail: 'Rules the panel has applied to this host with iptables.',
      reason: sourceReason(firewall, 'the firewall rules'),
      fix: { label: 'Firewall', tab: 'firewall' },
    });
  } else {
    const rules = firewall.data;
    const active = rules.filter((rule) => rule.status !== 'disabled');
    // Deliberately narrow wording. An empty list means the panel manages no
    // rules; it does NOT mean the host is unprotected, because iptables may
    // well be carrying rules this panel never wrote. Claiming otherwise would
    // be a red cross the panel cannot justify, which is the same defect as a
    // green tick it cannot justify.
    items.push(
      rules.length === 0
        ? {
            id: 'firewall-rules',
            title: 'Firewall rules managed by the panel',
            state: 'attention',
            risk: 'high',
            detail:
              'The panel manages no firewall rules on this host. It cannot tell you whether the host firewall is configured by other means - the Firewall tab shows the live iptables table so you can check.',
            fix: { label: 'Open the firewall', tab: 'firewall' },
          }
        : {
            id: 'firewall-rules',
            title: 'Firewall rules managed by the panel',
            state: 'ok',
            risk: 'high',
            detail: `${active.length} of ${rules.length} panel ${plural(rules.length, 'rule')} applied with iptables. The live table is shown in the Firewall tab.`,
            fix: { label: 'Review the rules', tab: 'firewall' },
          }
    );
  }

  // -- File integrity monitoring -------------------------------------------
  const tamper = sources.tamper;
  if (tamper.status !== 'ok' || !tamper.data) {
    items.push({
      id: 'file-integrity',
      title: 'File integrity monitoring',
      state: 'unknown',
      risk: 'high',
      detail: 'Checksums of the paths the panel watches, compared against a stored baseline.',
      reason: sourceReason(tamper, 'the file integrity statistics'),
      fix: { label: 'Anti Intrusion', tab: 'anti-intrusion' },
    });
  } else {
    const stats = tamper.data;
    if (stats.enabled_paths === 0) {
      items.push({
        id: 'file-integrity',
        title: 'File integrity monitoring',
        state: 'off',
        risk: 'high',
        detail:
          stats.protected_paths === 0
            ? 'No path is being watched. A change to a site file or a system binary would leave no record.'
            : `${stats.protected_paths} ${plural(stats.protected_paths, 'path')} configured but every one of them is disabled.`,
        fix: { label: 'Add a watched path', tab: 'anti-intrusion' },
      });
    } else if (stats.active_alerts > 0) {
      items.push({
        id: 'file-integrity',
        title: 'File integrity monitoring',
        state: 'attention',
        risk: 'critical',
        detail: `${stats.active_alerts} unresolved integrity ${plural(stats.active_alerts, 'alert')} across ${stats.enabled_paths} watched ${plural(stats.enabled_paths, 'path')}. Something under a watched path changed and nobody has signed it off.`,
        fix: { label: 'Review the alerts', tab: 'anti-intrusion' },
      });
    } else if (!stats.last_scan_at) {
      items.push({
        id: 'file-integrity',
        title: 'File integrity monitoring',
        state: 'attention',
        risk: 'high',
        detail: `${stats.enabled_paths} ${plural(stats.enabled_paths, 'path')} watched, but no scan has ever run, so there is nothing to compare against.`,
        fix: { label: 'Run a scan', tab: 'anti-intrusion' },
      });
    } else {
      items.push({
        id: 'file-integrity',
        title: 'File integrity monitoring',
        state: 'ok',
        risk: 'high',
        detail: `${stats.enabled_paths} ${plural(stats.enabled_paths, 'path')} watched over ${stats.total_files} ${plural(stats.total_files, 'file')}, no unresolved alerts.`,
        fix: { label: 'Anti Intrusion', tab: 'anti-intrusion' },
      });
    }

    // Scanning is on demand: nothing in the panel schedules it. A baseline
    // that is never re-checked is a baseline, not a monitor.
    if (stats.enabled_paths > 0) {
      items.push({
        id: 'file-integrity-schedule',
        title: 'Integrity scan schedule',
        state: 'unknown',
        risk: 'medium',
        detail:
          'Integrity scans run when somebody presses the button on the Anti Intrusion tab.',
        reason:
          'Nothing in the panel schedules a periodic integrity scan, and no endpoint reports one, so the panel cannot say when this was last checked automatically.',
        fix: { label: 'Run one now', tab: 'anti-intrusion' },
      });
    }
  }

  // -- Web application firewall --------------------------------------------
  //
  // This is the item this screen exists for. The WAF endpoints store and return
  // rules, and internal/service/waf.go does nothing else with them: no web
  // server configuration is written, no ModSecurity is configured, nothing is
  // reloaded. So however many rules are enabled, none of them is stopping a
  // request, and the honest state is "stored, not enforced" - not "on".
  const waf = sources.waf;
  if (waf.status !== 'ok' || !waf.data) {
    items.push({
      id: 'waf',
      title: 'Web application firewall',
      state: 'unknown',
      risk: 'critical',
      detail: 'Rules that would inspect requests before they reach a site.',
      reason: sourceReason(waf, 'the WAF rules'),
      fix: { label: 'WAF', href: '/waf' },
    });
  } else {
    const enabled = waf.data.filter((rule) => rule.enabled).length;
    items.push({
      id: 'waf',
      title: 'Web application firewall',
      state: 'unenforced',
      risk: 'critical',
      detail:
        enabled > 0
          ? `${enabled} of ${waf.data.length} ${plural(waf.data.length, 'rule')} are marked enabled in the panel database. The panel does not write them into any web server configuration and does not reload one, so no request is being inspected by them.`
          : 'The panel stores WAF rules but does not write them into any web server configuration, so nothing here inspects a request.',
      fix: { label: 'WAF rules', href: '/waf' },
    });
  }

  // -- Authentication pressure ---------------------------------------------
  const failures = sources.failures;
  if (failures.status !== 'ok' || !failures.data) {
    items.push({
      id: 'sign-in-failures',
      title: 'Failed sign-ins',
      state: 'unknown',
      risk: 'medium',
      detail: 'Rejected sign-in attempts recorded in the audit log.',
      reason: sourceReason(failures, 'the audit log'),
      fix: { label: 'Brute force protection', tab: 'brute-force' },
    });
  } else {
    const recent = failures.data.filter((row) => withinWindow(row.created_at, FAILURE_WINDOW_HOURS));
    const addresses = new Set(recent.map((row) => row.ip_address).filter(Boolean));
    const lockouts = recent.filter(
      (row) => String(row.details?.reason ?? '') === 'locked_out'
    ).length;

    if (recent.length === 0) {
      items.push({
        id: 'sign-in-failures',
        title: 'Failed sign-ins',
        state: 'ok',
        risk: 'medium',
        detail: `No rejected sign-in in the last ${FAILURE_WINDOW_HOURS} hours.`,
      });
    } else if (lockouts > 0) {
      items.push({
        id: 'sign-in-failures',
        title: 'Failed sign-ins',
        state: 'attention',
        risk: 'high',
        detail: `${recent.length} rejected ${plural(recent.length, 'attempt')} from ${addresses.size} ${plural(addresses.size, 'address', 'addresses')} in the last ${FAILURE_WINDOW_HOURS} hours, ${lockouts} of them refused because the account was already locked out. Somebody is working through passwords.`,
        fix: { label: 'See who', tab: 'brute-force' },
      });
    } else {
      items.push({
        id: 'sign-in-failures',
        title: 'Failed sign-ins',
        state: 'attention',
        risk: 'medium',
        detail: `${recent.length} rejected ${plural(recent.length, 'attempt')} from ${addresses.size} ${plural(addresses.size, 'address', 'addresses')} in the last ${FAILURE_WINDOW_HOURS} hours.`,
        fix: { label: 'See who', tab: 'brute-force' },
      });
    }
  }

  return items.concat(UNIMPLEMENTED);
}
