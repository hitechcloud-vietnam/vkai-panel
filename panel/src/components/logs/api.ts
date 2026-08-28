/**
 * The log and audit endpoints this page is allowed to call.
 *
 * Every function here corresponds to a route that is actually registered in
 * core/internal/handler/router.go. Nothing is declared speculatively: a wrapper
 * for an endpoint that does not exist is how a button that does nothing gets
 * written, so where the backend has no route the UI renders a gap notice
 * instead and there is no function for it below.
 *
 * The one deliberate exception is the audit chain block. Those handlers are
 * written (handler/audit.go) but RegisterAuditRoutes is never called from
 * router.go, so they answer 404 today. They are called anyway, and a 404 is
 * reported to the user as "verification is not wired up" rather than being
 * hidden - the moment somebody adds the one missing line, the panel starts
 * showing real verification with no further frontend change.
 */

import { api, unwrap, unwrapList } from '@/services/api';
import type {
  ApiFailure,
  AuditLog,
  AuditStats,
  ChainStatus,
  LogEntry,
  LogSource,
  ServiceInfo,
  VerifyReport,
  WebsiteSummary,
} from './types';

// ---------------------------------------------------------------------------
// Failure description
// ---------------------------------------------------------------------------

/**
 * Turn a thrown request into something worth showing an operator.
 *
 * A blanked page teaches nothing. The status code matters on its own: 404 means
 * the route was never mounted (a backend gap, not a user error) and 403 means
 * the signed-in account lacks the permission, and those two need different
 * words on screen.
 */
export function describeError(err: unknown, fallback: string): ApiFailure {
  const anyErr = err as {
    response?: { status?: number; data?: unknown };
    message?: string;
    code?: string;
  } | null;

  const status = typeof anyErr?.response?.status === 'number' ? anyErr.response.status : null;
  const body = anyErr?.response?.data;

  let message = '';
  if (typeof body === 'string' && body.trim()) {
    message = body.trim();
  } else if (body && typeof body === 'object') {
    const record = body as Record<string, unknown>;
    const candidate = record.error ?? record.message;
    if (typeof candidate === 'string' && candidate.trim()) message = candidate.trim();
  }
  if (!message && anyErr?.code === 'ECONNABORTED') {
    message = 'The request timed out before the server answered.';
  }
  if (!message && typeof anyErr?.message === 'string' && anyErr.message) {
    message = anyErr.message;
  }
  if (!message) message = fallback;

  return {
    message,
    status,
    notMounted: status === 404,
    forbidden: status === 403,
  };
}

// ---------------------------------------------------------------------------
// Panel logs - GET /api/v1/logs/search
// ---------------------------------------------------------------------------

export interface LogSearchParams {
  source?: string;
  level?: string;
  query?: string;
  server_id?: string;
  start?: string;
  end?: string;
  limit: number;
  offset: number;
}

export interface LogSearchResult {
  entries: LogEntry[];
  total: number;
}

export async function searchLogEntries(params: LogSearchParams): Promise<LogSearchResult> {
  const res = await api.get('/api/v1/logs/search', { params: compact(params) });
  const body = unwrap<{ entries?: LogEntry[] | null; total?: number }>(res, null);
  return {
    entries: Array.isArray(body?.entries) ? body!.entries! : [],
    total: typeof body?.total === 'number' ? body!.total! : 0,
  };
}

/** GET /api/v1/logs/sources - the log files the panel has been told about. */
export async function listLogSources(serverId?: string): Promise<LogSource[]> {
  const res = await api.get('/api/v1/logs/sources', {
    params: compact({ server_id: serverId }),
  });
  const body = unwrap<{ sources?: LogSource[] | null }>(res, null);
  return Array.isArray(body?.sources) ? body!.sources! : [];
}

// ---------------------------------------------------------------------------
// Audit trail - GET /api/v1/audit/search, /api/v1/audit/stats
// ---------------------------------------------------------------------------

export interface AuditSearchParams {
  action?: string;
  resource?: string;
  status?: string;
  user_id?: string;
  start?: string;
  end?: string;
  limit: number;
  offset: number;
}

export interface AuditSearchResult {
  logs: AuditLog[];
  total: number;
}

export async function searchAuditLogs(params: AuditSearchParams): Promise<AuditSearchResult> {
  const res = await api.get('/api/v1/audit/search', { params: compact(params) });
  const body = unwrap<{ logs?: AuditLog[] | null; total?: number }>(res, null);
  return {
    logs: Array.isArray(body?.logs) ? body!.logs! : [],
    total: typeof body?.total === 'number' ? body!.total! : 0,
  };
}

export async function getAuditStats(days: number): Promise<AuditStats | null> {
  const res = await api.get('/api/v1/audit/stats', { params: { days } });
  return unwrap<AuditStats>(res, null);
}

// ---------------------------------------------------------------------------
// Audit chain - GET /api/v1/audit/chain/*
//
// Handlers exist; the route group is not mounted. See the file header.
// ---------------------------------------------------------------------------

export async function getChainStatus(): Promise<ChainStatus | null> {
  const res = await api.get('/api/v1/audit/chain/status');
  return unwrap<ChainStatus>(res, null);
}

/**
 * Run a verification pass.
 *
 * A broken chain answers 409 Conflict with the report as its body - deliberately,
 * so that a monitor watching status codes cannot read tampering as health. That
 * makes the 409 a RESULT, not an error: it is unwrapped and returned like any
 * other report, and only a genuine failure to run is rethrown.
 */
export async function verifyChain(opts: { deep: boolean; full: boolean }): Promise<VerifyReport> {
  try {
    const res = await api.get('/api/v1/audit/chain/verify', {
      params: { deep: String(opts.deep), full: String(opts.full) },
      timeout: 120000,
    });
    const report = unwrap<VerifyReport>(res, null);
    if (!report) throw new Error('The verification endpoint returned an empty report.');
    return report;
  } catch (err) {
    const response = (err as { response?: { status?: number; data?: unknown } } | null)?.response;
    const body = response?.data;
    if (response?.status === 409 && body && typeof body === 'object' && 'ok' in body) {
      return body as VerifyReport;
    }
    throw err;
  }
}

/** GET /api/v1/audit/chain/export - the bundle an outside auditor can check. */
export async function exportChainBundle(): Promise<{ payload: unknown; verified: boolean }> {
  try {
    const res = await api.get('/api/v1/audit/chain/export', { timeout: 120000 });
    return { payload: unwrap<unknown>(res, null), verified: true };
  } catch (err) {
    const response = (err as { response?: { status?: number; data?: any } } | null)?.response;
    // A range that does not verify still comes back, with the reason attached.
    // Withholding it would deny an investigator the evidence of the tampering.
    if (response?.status === 409 && response.data?.bundle) {
      return { payload: response.data.bundle, verified: false };
    }
    throw err;
  }
}

// ---------------------------------------------------------------------------
// Websites - GET /api/v1/websites
// ---------------------------------------------------------------------------

export async function listWebsites(): Promise<WebsiteSummary[]> {
  const res = await api.get('/api/v1/websites', { params: { page: 1, per_page: 200 } });
  return unwrapList<WebsiteSummary>(res);
}

// ---------------------------------------------------------------------------
// Installed software - GET /api/v1/services, /api/v1/services/:name/logs
// ---------------------------------------------------------------------------

export async function listServices(): Promise<ServiceInfo[]> {
  const res = await api.get('/api/v1/services');
  const value = unwrap<ServiceInfo[] | null>(res, null);
  return Array.isArray(value) ? value : [];
}

/**
 * Read the tail of one unit's journal.
 *
 * `lines` is the whole point: journalctl is asked for the last N lines on the
 * SERVER, so the response is bounded before it is serialised. The panel never
 * receives a whole journal and so can never try to render one. The backend caps
 * this at 5000 (service_manager.go) and silently falls back to 100 above that,
 * which is why the picker never offers more.
 */
export async function getServiceLogs(name: string, lines: number): Promise<string> {
  const res = await api.get(`/api/v1/services/${encodeURIComponent(name)}/logs`, {
    params: { lines },
    timeout: 60000,
  });
  const body = unwrap<{ logs?: string }>(res, null);
  return typeof body?.logs === 'string' ? body.logs : '';
}

// ---------------------------------------------------------------------------

/** Drop empty values so an unset filter never becomes `?level=` on the wire. */
function compact(params: object): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  Object.entries(params).forEach(([key, value]) => {
    if (value === undefined || value === null || value === '') return;
    out[key] = value;
  });
  return out;
}
