/**
 * The endpoints this screen is allowed to call.
 *
 * Every function here corresponds to a route that is mounted in
 * core/internal/handler/router.go and backed by a service that does something.
 * Nothing else is in this file: a wrapper for an endpoint that does not exist
 * is how a dead control gets written, because the call site then looks exactly
 * like a live one.
 *
 * Response envelopes are NOT uniform in this API and pretending otherwise is
 * how a populated list renders as empty. Two shapes are in use:
 *
 *   utils.Success  ->  { success: true, data: <payload> }     firewall, panel,
 *                                                             two-factor
 *   raw gin.H      ->  { alerts: [...] } / { stats: {...} }   tamper-proof,
 *                                                             WAF, audit
 *
 * so each reader below unwraps the shape its own endpoint actually returns.
 */

import { api, unwrap } from '@/services/api';

import type {
  AuditLog,
  FirewallRule,
  PanelSettingsView,
  ProtectedPath,
  TamperAlert,
  TamperScanResult,
  TamperStats,
  TwoFactorStatus,
  WafRule,
} from './types';

/** Reads a named array out of a raw `gin.H` body. Go serialises nil as null. */
function rawList<T>(res: unknown, key: string): T[] {
  const body = (res as { data?: Record<string, unknown> } | undefined)?.data;
  if (!body || typeof body !== 'object') return [];
  const value = body[key];
  return Array.isArray(value) ? (value as T[]) : [];
}

/** Reads a named object out of a raw `gin.H` body. */
function rawObject<T>(res: unknown, key: string): T | null {
  const body = (res as { data?: Record<string, unknown> } | undefined)?.data;
  if (!body || typeof body !== 'object') return null;
  const value = body[key];
  return value && typeof value === 'object' ? (value as T) : null;
}

// ---------------------------------------------------------------------------
// Firewall - real: the service shells out to iptables for every rule it stores.
// ---------------------------------------------------------------------------

export const firewall = {
  async list(): Promise<FirewallRule[]> {
    const res = await api.get('/api/v1/firewall');
    const value = unwrap<FirewallRule[]>(res, []);
    return Array.isArray(value) ? value : [];
  },
  /** The kernel's own view: `iptables -L -n -v`, as text. */
  async active(): Promise<string> {
    const res = await api.get('/api/v1/firewall/active');
    const value = unwrap<{ rules?: string }>(res, null);
    return typeof value?.rules === 'string' ? value.rules : '';
  },
  create(body: {
    server_id: string;
    protocol: string;
    port?: string;
    source?: string;
    action: string;
    direction?: string;
  }) {
    return api.post('/api/v1/firewall', body);
  },
  update(
    id: string,
    body: {
      protocol?: string;
      port?: string;
      source?: string;
      action?: string;
      direction?: string;
      status?: string;
    }
  ) {
    return api.put(`/api/v1/firewall/${encodeURIComponent(id)}`, body);
  },
  remove(id: string) {
    return api.delete(`/api/v1/firewall/${encodeURIComponent(id)}`);
  },
  /** Persists the live iptables table so it survives a reboot. */
  save() {
    return api.post('/api/v1/firewall/save');
  },
};

// ---------------------------------------------------------------------------
// Panel access - real, and live-applying: port, entrance, allow list, TLS.
// ---------------------------------------------------------------------------

export const panelAccess = {
  async get(): Promise<PanelSettingsView | null> {
    const res = await api.get('/api/v1/panel/settings');
    return unwrap<PanelSettingsView>(res, null);
  },
};

// ---------------------------------------------------------------------------
// Two-factor - real for the signed-in operator. Answers 503 with a reason when
// the panel master key is missing, which is a state this screen must show as
// "not verifiable" rather than as "off".
// ---------------------------------------------------------------------------

export const twoFactor = {
  async status(): Promise<TwoFactorStatus | null> {
    const res = await api.get('/api/v1/two-factor/status');
    return unwrap<TwoFactorStatus>(res, null);
  },
};

// ---------------------------------------------------------------------------
// Tamper-proof - real: it walks the paths, hashes every file and stores a
// baseline, then reports what changed. This is the file integrity monitor the
// Anti Intrusion tab is built on.
// ---------------------------------------------------------------------------

export const tamperProof = {
  async stats(): Promise<TamperStats | null> {
    const res = await api.get('/api/v1/tamper-proof/stats');
    return rawObject<TamperStats>(res, 'stats');
  },
  async paths(): Promise<ProtectedPath[]> {
    const res = await api.get('/api/v1/tamper-proof/paths');
    return rawList<ProtectedPath>(res, 'paths');
  },
  async alerts(resolved?: boolean): Promise<TamperAlert[]> {
    const res = await api.get('/api/v1/tamper-proof/alerts', {
      params: resolved === undefined ? undefined : { resolved: String(resolved) },
    });
    return rawList<TamperAlert>(res, 'alerts');
  },
  async scanResults(limit = 20): Promise<TamperScanResult[]> {
    const res = await api.get('/api/v1/tamper-proof/scan-results', { params: { limit } });
    return rawList<TamperScanResult>(res, 'results');
  },
  resolveAlert(id: string, notes: string) {
    return api.post(`/api/v1/tamper-proof/alerts/${encodeURIComponent(id)}/resolve`, { notes });
  },
  scanPath(id: string) {
    return api.post(`/api/v1/tamper-proof/paths/${encodeURIComponent(id)}/scan`);
  },
  scanAll() {
    return api.post('/api/v1/tamper-proof/scan-all');
  },
  refreshBaseline(id: string) {
    return api.post(`/api/v1/tamper-proof/paths/${encodeURIComponent(id)}/baselines/refresh`);
  },
};

// ---------------------------------------------------------------------------
// WAF - the rules are stored and returned, and that is all. Nothing in
// internal/service/waf.go writes a web server configuration or reloads one, so
// a rule read from here is a database row and not a request that got blocked.
// This screen reads them only to be able to SAY that.
// ---------------------------------------------------------------------------

export const waf = {
  async rules(): Promise<WafRule[]> {
    const res = await api.get('/api/v1/waf/rules');
    return rawList<WafRule>(res, 'rules');
  },
};

// ---------------------------------------------------------------------------
// Audit - real. Failed sign-ins are recorded here by AuthService with the
// source address, the account tried and the reason, which is the only
// machine-readable record of brute force pressure the API exposes.
// ---------------------------------------------------------------------------

export const AUTH_FAILED_ACTION = 'auth.sign_in_failed';
export const AUTH_SUCCESS_ACTION = 'auth.sign_in';

export const audit = {
  async signInFailures(limit = 200): Promise<AuditLog[]> {
    const res = await api.get('/api/v1/audit/search', {
      params: { action: AUTH_FAILED_ACTION, limit },
    });
    return rawList<AuditLog>(res, 'logs');
  },
  async signInSuccesses(limit = 50): Promise<AuditLog[]> {
    const res = await api.get('/api/v1/audit/search', {
      params: { action: AUTH_SUCCESS_ACTION, limit },
    });
    return rawList<AuditLog>(res, 'logs');
  },
};

// ---------------------------------------------------------------------------
// Servers - a firewall rule is stored against a server id, so the create form
// needs the list to be able to send a valid one.
// ---------------------------------------------------------------------------

export interface SecurityServerOption {
  id: string;
  name: string;
  hostname: string;
  ip_address: string;
  status: string;
}

export const servers = {
  async list(): Promise<SecurityServerOption[]> {
    const res = await api.get('/api/v1/servers', { params: { per_page: 100 } });
    const value = unwrap<unknown>(res, null);
    if (Array.isArray(value)) return value as SecurityServerOption[];
    const items = (value as { items?: unknown } | null)?.items;
    return Array.isArray(items) ? (items as SecurityServerOption[]) : [];
  },
};
