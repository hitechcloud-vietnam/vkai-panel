/**
 * Reading the firewall's port field.
 *
 * models.FirewallRule stores `Port string`, and the string is whatever the rule
 * was written with: a single port, a comma list, or a range in either iptables
 * (`40000:40100`) or ufw (`40000-40100`) spelling. An FTP support ticket almost
 * always turns on whether one of those covers the daemon's passive range, so the
 * screen has to understand all of them rather than string-compare "21".
 */

/** One inclusive port interval. A single port is a range of width one. */
export interface PortRange {
  from: number;
  to: number;
}

/** Parse a firewall rule's port field. Unparseable fragments are dropped. */
export function parsePortSpec(spec: string): PortRange[] {
  const out: PortRange[] = [];
  for (const part of (spec ?? '').split(',')) {
    const token = part.trim();
    if (token === '') continue;
    const match = token.match(/^(\d{1,5})\s*[:-]\s*(\d{1,5})$/);
    if (match) {
      const from = Number(match[1]);
      const to = Number(match[2]);
      if (isPort(from) && isPort(to) && from <= to) out.push({ from, to });
      continue;
    }
    if (/^\d{1,5}$/.test(token)) {
      const value = Number(token);
      if (isPort(value)) out.push({ from: value, to: value });
    }
  }
  return out;
}

function isPort(value: number): boolean {
  return Number.isInteger(value) && value >= 0 && value <= 65535;
}

/** True when any interval in the spec contains `port`. */
export function specCoversPort(spec: string, port: number): boolean {
  return parsePortSpec(spec).some((range) => range.from <= port && port <= range.to);
}

/** How many ports the spec covers in total. */
export function spanOf(spec: string): number {
  return parsePortSpec(spec).reduce((total, range) => total + (range.to - range.from + 1), 0);
}

/**
 * A rule that plausibly opens a passive data range: an inbound TCP allow whose
 * port field is a range wider than one port and above the well-known ports.
 * Every daemon puts its passive range in the ephemeral space, so a range down
 * in the hundreds is something else and is not offered as a candidate.
 */
export function looksLikePassiveRange(spec: string): boolean {
  return parsePortSpec(spec).some(
    (range) => range.to > range.from && range.from >= 1024
  );
}

/** Render a spec back for display, normalised to `from-to`. */
export function formatSpec(spec: string): string {
  const ranges = parsePortSpec(spec);
  if (ranges.length === 0) return spec || '—';
  return ranges
    .map((range) => (range.from === range.to ? String(range.from) : `${range.from}-${range.to}`))
    .join(', ');
}
