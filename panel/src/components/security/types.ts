/**
 * The Security screen's vocabulary.
 *
 * Two things live here: the tab identity (which is also the URL), and the
 * posture model the Overview and System Hardening panes both render.
 *
 * The posture model has one rule behind every type in it: a state is only ever
 * `ok` when the panel READ the setting from an endpoint AND that setting is
 * actually applied by something. Everything else gets a state that says so.
 * A control the panel stores but nothing enforces is `unenforced`, not `ok`.
 * A control the panel cannot read at all is `unknown`, not `ok`. This screen is
 * the one an operator uses to decide the machine is safe, so a green tick it
 * cannot justify is worse than a red cross it can.
 */

/** The tabs, in the order aaPanel puts them. The id is the `tab` query value. */
export const SECURITY_TABS = [
  { id: 'overview', label: 'Overview' },
  { id: 'firewall', label: 'Firewall' },
  { id: 'ssh', label: 'SSH' },
  { id: 'server-security', label: 'Server security' },
  { id: 'website-security', label: 'Website Security' },
  { id: 'brute-force', label: 'Brute force protection' },
  { id: 'compiler-access', label: 'Compiler Access' },
  { id: 'anti-intrusion', label: 'Anti Intrusion' },
  { id: 'system-hardening', label: 'System Hardening' },
] as const;

export type SecurityTabId = (typeof SECURITY_TABS)[number]['id'];

export const DEFAULT_SECURITY_TAB: SecurityTabId = 'overview';

/** Narrows an arbitrary query value to a tab id, falling back to Overview. */
export function toSecurityTab(value: string | null | undefined): SecurityTabId {
  const found = SECURITY_TABS.find((tab) => tab.id === value);
  return found ? found.id : DEFAULT_SECURITY_TAB;
}

// ---------------------------------------------------------------------------
// Posture
// ---------------------------------------------------------------------------

/**
 * What the panel is able to say about one control.
 *
 *   ok          read from an endpoint, switched on, and applied by something
 *   attention   read and on, but with a caveat an operator should act on
 *   off         read, and it is switched off
 *   unenforced  the panel stores this setting but nothing applies it - the
 *               rows are real, the protection is not
 *   unknown     the panel cannot read it: no endpoint exists, or the request
 *               that would have answered failed. Never rendered as safe.
 */
export type PostureState = 'ok' | 'attention' | 'off' | 'unenforced' | 'unknown';

export type PostureRisk = 'critical' | 'high' | 'medium' | 'low';

export interface PostureFix {
  label: string;
  /** Another tab on this screen. */
  tab?: SecurityTabId;
  /** Another page in the panel. */
  href?: string;
}

export interface PostureItem {
  id: string;
  /** The control, named the way an operator would name it. */
  title: string;
  state: PostureState;
  risk: PostureRisk;
  /** What the panel actually observed. Never a guess, never a default. */
  detail: string;
  /** Present on `unknown`: why the state could not be established. */
  reason?: string;
  fix?: PostureFix;
}

/**
 * Problems first, then the things the panel cannot see, then what is fine -
 * and inside each of those, most dangerous first. Sorting by risk alone would
 * bury a switched-off control under a critical control that is already on, and
 * sorting alphabetically would bury it under everything.
 */
const STATE_BUCKET: Record<PostureState, number> = {
  off: 0,
  unenforced: 0,
  attention: 1,
  unknown: 2,
  ok: 3,
};

const RISK_ORDER: Record<PostureRisk, number> = {
  critical: 0,
  high: 1,
  medium: 2,
  low: 3,
};

export function rankPosture(items: PostureItem[]): PostureItem[] {
  return [...items].sort((a, b) => {
    const bucket = STATE_BUCKET[a.state] - STATE_BUCKET[b.state];
    if (bucket !== 0) return bucket;
    const risk = RISK_ORDER[a.risk] - RISK_ORDER[b.risk];
    if (risk !== 0) return risk;
    return a.title.localeCompare(b.title);
  });
}

/** How many items sit in each state, for the summary strip. */
export function countStates(items: PostureItem[]): Record<PostureState, number> {
  const counts: Record<PostureState, number> = {
    ok: 0,
    attention: 0,
    off: 0,
    unenforced: 0,
    unknown: 0,
  };
  for (const item of items) counts[item.state] += 1;
  return counts;
}

// ---------------------------------------------------------------------------
// API row shapes
// ---------------------------------------------------------------------------

/** GET /api/v1/firewall - one stored rule. */
export interface FirewallRule {
  id: string;
  tenant_id: string;
  server_id: string;
  protocol: string;
  port: string;
  source: string;
  action: string;
  direction: string;
  status: string;
  created_at: string;
  updated_at: string;
}

/** GET /api/v1/panel/settings - only the fields this screen reads. */
export interface PanelTLSView {
  enabled: boolean;
  self_signed: boolean;
  mode: string;
  managed: boolean;
  source: string;
  present: boolean;
  expired: boolean;
  expiring_soon: boolean;
  expires_in_days: number | null;
  chain_complete: boolean;
  warnings: string[] | null;
  inconsistency: string;
}

export interface PanelSettingsView {
  enabled: boolean;
  bind: string;
  port: number;
  entrance_masked: string;
  entrance_enabled: boolean;
  session_ttl_seconds: number;
  allowed_ips: string[] | null;
  trusted_proxies: string[] | null;
  domain: string;
  tls: PanelTLSView;
  access_url: string;
  scheme: string;
  proxied: boolean;
  restart_pending: boolean;
  restart_reasons: string[] | null;
  hot_reload: boolean;
  client_ip: string;
}

/** GET /api/v1/two-factor/status, for the signed-in operator only. */
export interface TwoFactorStatus {
  enabled: boolean;
  pending_enrolment: boolean;
  recovery_codes_remaining: number;
  recovery_codes_total: number;
  recovery_codes_low: boolean;
}

/** GET /api/v1/tamper-proof/stats. */
export interface TamperStats {
  protected_paths: number;
  enabled_paths: number;
  total_files: number;
  active_alerts: number;
  resolved_alerts: number;
  alerts_today: number;
  last_scan_at: string | null;
  total_scans: number;
  clean_scans: number;
  violation_scans: number;
}

/** GET /api/v1/tamper-proof/paths. */
export interface ProtectedPath {
  id: string;
  path: string;
  path_type: string;
  recursive: boolean;
  algorithm: string;
  is_enabled: boolean;
  file_count: number;
  description: string;
  last_scan_at: string | null;
  last_alert_at: string | null;
}

/** GET /api/v1/tamper-proof/alerts. */
export interface TamperAlert {
  id: string;
  protected_id: string;
  file_path: string;
  alert_type: string;
  severity: string;
  old_checksum: string;
  new_checksum: string;
  old_mode: string;
  new_mode: string;
  is_resolved: boolean;
  notes: string;
  created_at: string;
}

/** GET /api/v1/tamper-proof/scan-results. */
export interface TamperScanResult {
  id: string;
  protected_id: string;
  status: string;
  total_files: number;
  scanned_files: number;
  violations: number;
  new_files: number;
  deleted_files: number;
  modified_files: number;
  duration: number;
  created_at: string;
}

/** GET /api/v1/waf/rules. */
export interface WafRule {
  id: string;
  name: string;
  rule_type: string;
  severity: string;
  action: string;
  enabled: boolean;
}

/** GET /api/v1/audit/search - one authentication event. */
export interface AuditLog {
  id: string;
  user_id: string | null;
  action: string;
  resource: string;
  details: Record<string, unknown> | null;
  ip_address: string;
  user_agent: string;
  status: string;
  created_at: string;
}
