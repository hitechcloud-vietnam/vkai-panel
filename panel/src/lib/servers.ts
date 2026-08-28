/**
 * Everything the UI needs to reason about managed nodes, and in particular
 * about the one node that is always there: the machine the panel is installed
 * on.
 *
 * The rule the whole interface follows is here rather than in each page: a
 * figure the backend has not reported is unavailable, not zero. `null` travels
 * through these helpers unchanged so a caller can render an em dash and say why
 * instead of inventing a number.
 */

import {
  LOCAL_NODE_ROLE_ALIASES,
  type ManagedServer,
  type ServerMetrics,
} from '@/types/server';

/** Rendered in place of a figure the backend has not reported. */
export const UNAVAILABLE = '—';

/** True when this row is the machine the panel itself runs on. */
export function isLocalNode(server: ManagedServer | null | undefined): boolean {
  const role = String(server?.role || '').trim().toLowerCase();
  if (!role) return false;
  return LOCAL_NODE_ROLE_ALIASES.includes(role);
}

/** The local node, or null when the list does not contain one. */
export function findLocalNode(
  servers: ManagedServer[] | null | undefined
): ManagedServer | null {
  if (!Array.isArray(servers)) return null;
  return servers.find((s) => isLocalNode(s)) || null;
}

/**
 * What to call a node on screen. There is no `name` column, so the hostname is
 * the name; an address or a short id stands in while a node is still
 * registering and has not reported its hostname.
 */
export function serverLabel(server: ManagedServer | null | undefined): string {
  const hostname = String(server?.hostname || '').trim();
  if (hostname) return hostname;
  const ip = String(server?.ip_address || '').trim();
  if (ip) return ip;
  const id = String(server?.id || '').trim();
  if (id) return id.slice(0, 8);
  return UNAVAILABLE;
}

/**
 * The node an action should apply to when the operator has not chosen one: the
 * panel host if it is known, otherwise the only node there is, otherwise
 * nothing. Returns '' when the list is empty.
 */
export function defaultServerId(servers: ManagedServer[] | null | undefined): string {
  const list = Array.isArray(servers) ? servers : [];
  const local = findLocalNode(list);
  if (local?.id) return local.id;
  if (list.length === 1) return String(list[0]?.id || '');
  return '';
}

/** Badge classes for a node's status. Status colours are the panel's standard set. */
export function serverStatusBadgeClass(status: string | null | undefined): string {
  switch (String(status || '').trim().toLowerCase()) {
    case 'online':
    case 'active':
      return 'bg-emerald-50 text-emerald-700';
    case 'offline':
    case 'error':
    case 'failed':
      return 'bg-red-50 text-red-700';
    case 'maintenance':
    case 'pending':
    case 'provisioning':
      return 'bg-amber-50 text-amber-700';
    default:
      return 'bg-gray-100 text-gray-700';
  }
}

/** A finite number, or null. Never a silent zero. */
export function finiteOrNull(value: unknown): number | null {
  if (value === null || value === undefined || value === '') return null;
  const num = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(num) ? num : null;
}

/** used/total as a percentage in 0-100, or null when either side is missing. */
export function percentOf(used: unknown, total: unknown): number | null {
  const u = finiteOrNull(used);
  const t = finiteOrNull(total);
  if (u === null || t === null || t <= 0) return null;
  return Math.min(100, Math.max(0, (u / t) * 100));
}

/** Bytes in a human unit, or null when the figure is missing. Zero is a real answer and stays "0 B". */
export function formatBytesOrNull(bytes: unknown, decimals = 1): string | null {
  const value = finiteOrNull(bytes);
  if (value === null || value < 0) return null;
  if (value === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  const i = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  return `${(value / Math.pow(1024, i)).toFixed(i === 0 ? 0 : decimals)} ${units[i]}`;
}

/** "12.3 GB / 31.0 GB", or null when either side is missing. */
export function formatUsage(used: unknown, total: unknown): string | null {
  const u = formatBytesOrNull(used);
  const t = formatBytesOrNull(total);
  if (u === null || t === null) return null;
  return `${u} / ${t}`;
}

/**
 * Load average as a share of the node's cores, in 0-100. A machine at load 1.0
 * with 4 cores is at 25 per cent, which is what an operator means by system
 * load. Null when either figure is missing.
 */
export function loadPercent(
  metrics: ServerMetrics | null | undefined,
  cpuCores: unknown
): number | null {
  const load = finiteOrNull(metrics?.load1);
  const cores = finiteOrNull(cpuCores);
  if (load === null || cores === null || cores <= 0) return null;
  return Math.min(100, Math.max(0, (load / cores) * 100));
}

/**
 * The web servers a node can host a site on. Prefers what the node reports; a
 * node that reports only one type still offers that one; a node that reports
 * nothing yields an empty list so the caller can offer the supported set.
 */
export function nodeWebServers(server: ManagedServer | null | undefined): string[] {
  const list = Array.isArray(server?.web_servers) ? server?.web_servers : null;
  if (list && list.length > 0) {
    return list.map((v) => String(v).trim().toLowerCase()).filter(Boolean);
  }
  const single = String(server?.web_server_type || '').trim().toLowerCase();
  return single ? [single] : [];
}
