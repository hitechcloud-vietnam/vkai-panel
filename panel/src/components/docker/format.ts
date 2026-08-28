/**
 * Formatting shared by the Docker panes.
 *
 * Every function takes `null` to mean "the backend did not report this" and
 * returns `null` for it, so callers hand the result to `MetricText` and get the
 * unavailable dash with a reason instead of a confident, wrong zero.
 */

const UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];

/** Bytes as a short human string. `null` in, `null` out. */
export function formatBytes(bytes: number | null | undefined): string | null {
  if (bytes === null || bytes === undefined || !Number.isFinite(bytes)) return null;
  if (bytes <= 0) return '0 B';
  const exponent = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), UNITS.length - 1);
  const value = bytes / Math.pow(1024, exponent);
  return `${value >= 100 || exponent === 0 ? Math.round(value) : value.toFixed(1)} ${UNITS[exponent]}`;
}

/** A percentage with one decimal. `null` in, `null` out. */
export function formatPercent(value: number | null | undefined): string | null {
  if (value === null || value === undefined || !Number.isFinite(value)) return null;
  return `${value.toFixed(1)}%`;
}

/** An integer count. `null` in, `null` out - a missing count is never "0". */
export function formatCount(value: number | null | undefined): string | null {
  if (value === null || value === undefined || !Number.isFinite(value)) return null;
  return String(Math.round(value));
}

/** An absolute timestamp an operator can paste into a ticket. */
export function formatDateTime(value: string | null | undefined): string | null {
  if (!value) return null;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString('en-GB', {
    year: 'numeric',
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  });
}

/**
 * How long something has been up, counted from a start timestamp.
 * Coarse on purpose: "12 days" is what an operator reads, not "12d 4h 09m".
 */
export function formatUptime(startedAt: string | null | undefined): string | null {
  if (!startedAt) return null;
  const start = new Date(startedAt).getTime();
  if (Number.isNaN(start)) return null;
  const seconds = Math.floor((Date.now() - start) / 1000);
  if (seconds < 0) return null;
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 48) return `${hours}h ${minutes % 60}m`;
  const days = Math.floor(hours / 24);
  return `${days} days`;
}

/** Case-insensitive substring match used by every pane's search box. */
export function matches(value: string | null | undefined, term: string): boolean {
  if (!term) return true;
  return (value || '').toLowerCase().includes(term.toLowerCase());
}

/** The status colour for a container state, on the panel's fixed scale. */
export function containerStateVariant(
  state: string
): 'success' | 'danger' | 'warning' | 'info' | 'neutral' {
  const s = (state || '').toLowerCase();
  if (s.includes('running') || s.startsWith('up')) return 'success';
  if (s.includes('restarting') || s.includes('paused') || s.includes('created')) return 'warning';
  if (s.includes('dead')) return 'danger';
  if (s.includes('exited') || s.includes('stopped')) return 'neutral';
  return 'neutral';
}
