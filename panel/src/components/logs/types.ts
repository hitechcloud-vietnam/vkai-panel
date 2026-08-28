/**
 * Shapes returned by the log and audit endpoints.
 *
 * Every field name here was copied from the Go struct tags rather than guessed,
 * because the previous Audit page read `stats.actions` / `log.resource_type`
 * while the API sends `by_action` / `resource`, and so rendered "N/A" for data
 * that was actually present. The source of truth for each block is named above
 * it so the next person can re-check it in one grep.
 */

/** core/internal/models/log.go - LogEntry */
export interface LogEntry {
  id: string;
  server_id: string;
  tenant_id: string;
  source: string;
  level: string;
  message: string;
  details: Record<string, unknown> | null;
  timestamp: string;
  created_at: string;
}

/** core/internal/models/log.go - LogSource */
export interface LogSource {
  id: string;
  tenant_id: string;
  server_id: string;
  name: string;
  type: string;
  path: string;
  format: string;
  is_active: boolean;
  last_read_at: string | null;
  created_at: string;
  updated_at: string;
}

/** core/internal/models/audit.go - AuditLog */
export interface AuditLog {
  id: string;
  tenant_id: string;
  user_id: string | null;
  action: string;
  resource: string;
  resource_id: string | null;
  details: Record<string, unknown> | null;
  ip_address: string;
  user_agent: string;
  status: string;
  created_at: string;
}

/** core/internal/models/audit.go - AuditLogStats */
export interface AuditStats {
  total_logs: number;
  by_action: Record<string, number> | null;
  by_resource: Record<string, number> | null;
  by_status: Record<string, number> | null;
  by_user: Record<string, number> | null;
  recent_logs: AuditLog[] | null;
}

/** core/internal/repository/audit.go - ChainVerification */
export interface ChainVerification {
  mode: string;
  ok: boolean;
  checked: number;
  from_seq: number;
  to_seq: number;
  first_seq?: number | null;
  last_seq?: number | null;
  break_seq?: number | null;
  break_at?: string | null;
  break_reason?: string | null;
  break_log_id?: string | null;
  head_seq?: number | null;
  head_ok: boolean;
  duration_ms: number;
  ran_at: string;
}

/** core/internal/repository/audit.go - ChainStatus */
export interface ChainStatus {
  tenant_id: string;
  entries: number;
  first_seq: number;
  last_seq: number;
  head_seq: number;
  head_hash: string;
  oldest_at?: string | null;
  newest_at?: string | null;
  seals: number;
  last_verification?: ChainVerification | null;
}

/** core/internal/service/audit.go - VerifyReport (embeds ChainVerification) */
export interface VerifyReport extends ChainVerification {
  resumed_from: number;
  note: string;
}

/** core/internal/models/models.go - Website */
export interface WebsiteSummary {
  id: string;
  domain: string;
  server_id: string;
  status: string;
  web_server_type: string;
  root_dir: string;
}

/** core/internal/service/service_manager.go - ServiceInfo */
export interface ServiceInfo {
  name: string;
  status: string;
  active_state: string;
  sub_state: string;
  description: string;
  pid: number;
  memory: number;
}

/** A request that did not succeed, described well enough to render honestly. */
export interface ApiFailure {
  /** Human-readable reason, taken from the response body where there is one. */
  message: string;
  /** HTTP status, or null when the request never reached the server. */
  status: number | null;
  /** True when the endpoint answered 404: the route is not mounted at all. */
  notMounted: boolean;
  /** True when the caller lacks the permission the route requires. */
  forbidden: boolean;
}
