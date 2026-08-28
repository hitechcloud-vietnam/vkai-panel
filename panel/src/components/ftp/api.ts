/**
 * Every request this screen makes, and nothing else.
 *
 * There is no `ftpApi` here because there is no FTP API. What the screen calls
 * are three groups of routes that already exist and are already mounted, read
 * from core/internal/handler/router.go:
 *
 *   GET  /api/v1/services                 systemd units on the machine   (admin)
 *   GET  /api/v1/services/:name           ActiveState, SubState, PID, memory
 *   POST /api/v1/services/:name/start|stop|restart|enable|disable
 *   GET  /api/v1/services/:name/logs      journalctl tail, `{ logs: string }`
 *   GET  /api/v1/firewall                 the panel's own rule rows
 *   GET  /api/v1/firewall/active          raw `iptables -L -n -v` output
 *   GET  /api/v1/websites                 paginated; root_dir per site
 *
 * The services and firewall groups are behind middleware.RequireAdmin, so a
 * non-administrator gets a 403 with a message - which the panes render as a
 * message, not as an empty table.
 */

import { api, unwrap, unwrapList } from '@/services/api';

import type { FirewallRule, ServiceInfo, WebsiteRow } from './types';

/** Units on the machine. The list is unfiltered; the caller picks the FTP ones. */
export async function listServices(): Promise<ServiceInfo[]> {
  const res = await api.get('/api/v1/services');
  return unwrapList<ServiceInfo>(res);
}

/** Live state of one unit. */
export async function getServiceStatus(name: string): Promise<ServiceInfo | null> {
  const res = await api.get(`/api/v1/services/${encodeURIComponent(name)}`);
  return unwrap<ServiceInfo>(res, null);
}

export type ServiceAction = 'start' | 'stop' | 'restart' | 'enable' | 'disable';

/** Act on one unit. Refused server-side for any unit outside the allow list. */
export async function runServiceAction(name: string, action: ServiceAction): Promise<void> {
  await api.post(`/api/v1/services/${encodeURIComponent(name)}/${action}`);
}

/** The journal tail for one unit. The handler wraps it as `{ logs: string }`. */
export async function getServiceLogs(name: string, lines = 100): Promise<string> {
  const res = await api.get(`/api/v1/services/${encodeURIComponent(name)}/logs`, {
    params: { lines },
  });
  const body = unwrap<{ logs?: unknown }>(res, null);
  return typeof body?.logs === 'string' ? body.logs : '';
}

/** The firewall rules the panel itself holds. Not the live kernel table. */
export async function listFirewallRules(): Promise<FirewallRule[]> {
  const res = await api.get('/api/v1/firewall');
  return unwrapList<FirewallRule>(res);
}

/** The live kernel table, as text. `iptables -L -n -v`, exactly as it printed. */
export async function getActiveFirewallRules(): Promise<string> {
  const res = await api.get('/api/v1/firewall/active');
  const body = unwrap<{ rules?: unknown }>(res, null);
  return typeof body?.rules === 'string' ? body.rules : '';
}

/** Sites on this panel. Paginated, so it goes through unwrapList. */
export async function listWebsites(): Promise<WebsiteRow[]> {
  const res = await api.get('/api/v1/websites', { params: { page: 1, per_page: 200 } });
  return unwrapList<WebsiteRow>(res);
}
