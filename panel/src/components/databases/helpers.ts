/**
 * Small facts the panes share: where an instance listens, what to call a node,
 * and how to build a connection string that a client will actually accept.
 */

import { serverLabel } from '@/lib/servers';
import type { ManagedServer } from '@/types/server';
import {
  type DBEntry,
  type DBServer,
  type DatabaseEngine,
  type DatabaseEngineId,
} from '@/types/databases';

/** Rendered where the backend has reported nothing. Matches lib/servers. */
export const DASH = '—';

/** The node a database server runs on, or null when the list has no such row. */
export function nodeFor(
  server: DBServer | null | undefined,
  nodes: ManagedServer[]
): ManagedServer | null {
  const id = String(server?.server_id || '');
  if (!id) return null;
  return nodes.find((n) => n?.id === id) || null;
}

/** What to call the node an instance runs on. */
export function nodeName(
  server: DBServer | null | undefined,
  nodes: ManagedServer[]
): string {
  const node = nodeFor(server, nodes);
  if (node) return serverLabel(node);
  const id = String(server?.server_id || '');
  return id ? `${id.slice(0, 8)}…` : DASH;
}

/**
 * The address a client would dial. The panel knows the node's IP; it does not
 * know whether the instance binds a different interface, so an instance on the
 * panel host reads as `127.0.0.1` and any other node as its recorded address.
 */
export function instanceHost(
  server: DBServer | null | undefined,
  nodes: ManagedServer[]
): string {
  const node = nodeFor(server, nodes);
  const role = String(node?.role || '').trim().toLowerCase();
  if (role === 'local' || role === 'localhost' || role === 'panel' || role === 'self') {
    return '127.0.0.1';
  }
  const ip = String(node?.ip_address || '').trim();
  return ip || DASH;
}

/** The port, falling back to the engine's default when the row carries none. */
export function instancePort(
  server: DBServer | null | undefined,
  engine: DatabaseEngine
): number {
  const port = Number(server?.port);
  return Number.isFinite(port) && port > 0 ? port : engine.defaultPort;
}

/** Databases belonging to the instances shown on one engine's tab. */
export function entriesForEngine(
  entries: DBEntry[],
  servers: DBServer[]
): DBEntry[] {
  const ids = new Set(servers.map((s) => String(s?.id || '')));
  return entries.filter((e) => ids.has(String(e?.database_server_id || '')));
}

/** Instances whose `type` belongs to one engine's tab. */
export function serversForEngine(
  servers: DBServer[],
  engine: DatabaseEngine
): DBServer[] {
  return servers.filter((s) =>
    engine.serverTypes.includes(String(s?.type || '').trim().toLowerCase())
  );
}

/**
 * A connection string a client library will take. Passwords are never
 * interpolated: a URI carrying a secret ends up in shell history and in server
 * logs, so the placeholder stays a placeholder.
 */
export function connectionString(
  engineId: DatabaseEngineId,
  host: string,
  port: number,
  database: string,
  account: string
): string | undefined {
  if (!host || host === DASH) return undefined;
  switch (engineId) {
    case 'mysql':
      return `mysql://${account}@${host}:${port}/${database}`;
    case 'postgresql':
      return `postgresql://${account}@${host}:${port}/${database}`;
    case 'mongodb':
      return `mongodb://${account}@${host}:${port}/${database}`;
    case 'redis':
      return `redis://${host}:${port}`;
    case 'sqlserver':
      return `sqlserver://${account}@${host}:${port}?database=${database}`;
    default:
      return undefined;
  }
}

/** A deterministic date, so the server and client renders never disagree. */
export function formatDate(value: string | null | undefined): string {
  if (!value) return DASH;
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return DASH;
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${pad(d.getDate())}/${pad(d.getMonth() + 1)}/${d.getFullYear()}`;
}

/** Case-insensitive substring match across a row's searchable fields. */
export function matchesSearch(term: string, ...fields: (string | undefined)[]): boolean {
  const needle = term.trim().toLowerCase();
  if (!needle) return true;
  return fields.some((f) => String(f || '').toLowerCase().includes(needle));
}

/** Badge classes for a status word. The panel's standard status colours. */
export function statusBadgeVariant(
  status: string | null | undefined
): 'success' | 'danger' | 'warning' | 'neutral' {
  switch (String(status || '').trim().toLowerCase()) {
    case 'active':
    case 'online':
    case 'running':
      return 'success';
    case 'error':
    case 'failed':
    case 'inactive':
      return 'danger';
    case 'pending':
    case 'provisioning':
      return 'warning';
    default:
      return 'neutral';
  }
}
