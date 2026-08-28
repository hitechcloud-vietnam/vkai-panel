/**
 * Which rule wins when two of them cover the same traffic.
 *
 * This is the thing operators get wrong, and it is worth being exact about how
 * the answer is derived, because the panel is not reading it back from the
 * kernel.
 *
 * internal/service/firewall.go applies every rule with `iptables -A INPUT`
 * (or `-A OUTPUT`), which APPENDS. iptables evaluates a chain top to bottom and
 * stops at the first terminating match, so the rule that was applied FIRST
 * wins over any later rule covering the same traffic. Creation order is
 * therefore chain order - with one exception the UI has to state rather than
 * hide: `Update` deletes and re-applies, which moves an edited rule to the
 * BOTTOM of the chain. A rule edited after it was created may sit later than
 * its creation time suggests.
 *
 * Everything below works on the rules the panel stores. It is a prediction from
 * the code path, not an observation of the running chain, and the Firewall pane
 * says so next to it - the live `iptables -L -n -v` listing is on the same tab
 * for anyone who needs the truth rather than the prediction.
 */

import type { FirewallRule } from './types';

export interface PortRange {
  from: number;
  to: number;
}

/** Parses "80" or "8000:8100". An empty port means every port. */
export function parsePortRange(port: string): PortRange | null {
  const trimmed = (port ?? '').trim();
  if (trimmed === '') return { from: 0, to: 65535 };
  const parts = trimmed.split(':');
  if (parts.length > 2) return null;
  const from = Number(parts[0]);
  const to = parts.length === 2 ? Number(parts[1]) : from;
  if (!Number.isInteger(from) || !Number.isInteger(to)) return null;
  if (from < 0 || to > 65535 || from > to) return null;
  return { from, to };
}

function portsIntersect(a: string, b: string): boolean {
  const left = parsePortRange(a);
  const right = parsePortRange(b);
  // An unparseable port is not claimed to overlap anything: a guess here would
  // print a shadowing warning that is not true.
  if (!left || !right) return false;
  return left.from <= right.to && right.from <= left.to;
}

/**
 * iptables only receives `--dport` for tcp and udp (see applyRule), so an icmp
 * or "all" rule ignores the port field entirely. Two protocols overlap when
 * they are equal or when either side is "all".
 */
function protocolsIntersect(a: string, b: string): boolean {
  const left = (a || 'all').toLowerCase();
  const right = (b || 'all').toLowerCase();
  if (left === right) return true;
  return left === 'all' || right === 'all';
}

/** Whether the port field is even consulted for this protocol pair. */
function portIsMeaningful(protocol: string): boolean {
  const value = (protocol || '').toLowerCase();
  return value === 'tcp' || value === 'udp';
}

interface CidrV4 {
  base: number;
  prefix: number;
}

function parseIPv4(value: string): number | null {
  const octets = value.split('.');
  if (octets.length !== 4) return null;
  let result = 0;
  for (const octet of octets) {
    if (!/^\d{1,3}$/.test(octet)) return null;
    const n = Number(octet);
    if (n > 255) return null;
    result = result * 256 + n;
  }
  return result >>> 0;
}

/** Parses "10.0.0.0/8", "203.0.113.5", "any" or "". IPv6 is not attempted. */
export function parseSource(source: string): CidrV4 | null {
  const trimmed = (source ?? '').trim().toLowerCase();
  if (trimmed === '' || trimmed === 'any') return { base: 0, prefix: 0 };
  const [addr, maskPart] = trimmed.split('/');
  const base = parseIPv4(addr ?? '');
  if (base === null) return null;
  if (maskPart === undefined) return { base, prefix: 32 };
  if (!/^\d{1,2}$/.test(maskPart)) return null;
  const prefix = Number(maskPart);
  if (prefix > 32) return null;
  return { base, prefix };
}

function maskOf(prefix: number): number {
  return prefix === 0 ? 0 : (0xffffffff << (32 - prefix)) >>> 0;
}

function sourcesIntersect(a: string, b: string): boolean {
  const left = parseSource(a);
  const right = parseSource(b);
  // An IPv6 address or anything else unparseable: claim an overlap only when
  // the two are literally the same string, so the warning is never invented.
  if (!left || !right) return a.trim().toLowerCase() === b.trim().toLowerCase();
  const prefix = Math.min(left.prefix, right.prefix);
  const mask = maskOf(prefix);
  return ((left.base & mask) >>> 0) === ((right.base & mask) >>> 0);
}

/** Do these two rules ever see the same packet? */
export function rulesOverlap(a: FirewallRule, b: FirewallRule): boolean {
  const directionA = (a.direction || 'in').toLowerCase();
  const directionB = (b.direction || 'in').toLowerCase();
  if (directionA !== directionB) return false;
  if (!protocolsIntersect(a.protocol, b.protocol)) return false;
  if (!sourcesIntersect(a.source, b.source)) return false;
  // The port only narrows the match when iptables was given --dport for both.
  if (portIsMeaningful(a.protocol) && portIsMeaningful(b.protocol)) {
    return portsIntersect(a.port, b.port);
  }
  return true;
}

export type ConflictKind = 'shadowed' | 'duplicate';

export interface RuleConflict {
  kind: ConflictKind;
  /** The rule that is applied first and therefore wins the shared traffic. */
  winnerId: string;
  /** A one-line explanation written for an operator, not for a log. */
  message: string;
}

export interface AnalysedRule {
  rule: FirewallRule;
  /** 1-based position in the chain, as the panel applied them. */
  position: number;
  /** True when the rule was edited, which re-appended it to the chain bottom. */
  reappended: boolean;
  conflicts: RuleConflict[];
}

function describe(rule: FirewallRule): string {
  const protocol = (rule.protocol || 'all').toLowerCase();
  const port = rule.port ? `:${rule.port}` : '';
  const source = rule.source && rule.source.toLowerCase() !== 'any' ? rule.source : 'any source';
  return `${protocol}${port} from ${source}`;
}

/** Timestamps differing by under a second are the same write, not an edit. */
const EDIT_TOLERANCE_MS = 1000;

/**
 * Orders the rules the way the panel applied them and works out, for each one,
 * whether an earlier rule already decided its traffic.
 */
export function analyseRules(rules: FirewallRule[]): AnalysedRule[] {
  const ordered = [...rules].sort((a, b) => {
    const left = Date.parse(a.created_at);
    const right = Date.parse(b.created_at);
    if (Number.isNaN(left) || Number.isNaN(right)) return 0;
    return left - right;
  });

  return ordered.map((rule, index) => {
    const created = Date.parse(rule.created_at);
    const updated = Date.parse(rule.updated_at);
    const reappended =
      !Number.isNaN(created) && !Number.isNaN(updated) && updated - created > EDIT_TOLERANCE_MS;

    const conflicts: RuleConflict[] = [];
    for (let earlier = 0; earlier < index; earlier += 1) {
      const other = ordered[earlier];
      if (!rulesOverlap(other, rule)) continue;

      const sameAction = (other.action || '').toUpperCase() === (rule.action || '').toUpperCase();
      conflicts.push(
        sameAction
          ? {
              kind: 'duplicate',
              winnerId: other.id,
              message: `Rule ${earlier + 1} (${describe(other)} → ${other.action}) already covers this traffic with the same action, so this rule never changes an outcome.`,
            }
          : {
              kind: 'shadowed',
              winnerId: other.id,
              message: `Rule ${earlier + 1} (${describe(other)} → ${other.action}) is applied first and wins the traffic these two share. This rule's ${rule.action} does not apply to it.`,
            }
      );
    }

    return { rule, position: index + 1, reappended, conflicts };
  });
}
