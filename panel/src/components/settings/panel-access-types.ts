/**
 * Shapes returned by /api/v1/panel/settings.
 *
 * They mirror the Go types in core/internal/service/panel_settings.go. Keeping
 * them in one place means a field renamed on the server fails to compile here
 * rather than rendering as "undefined" in front of an operator who is about to
 * change how the panel is reached.
 */

export interface PanelTLSView {
  enabled: boolean;
  self_signed: boolean;
  /** "off" | "self_signed" | "custom" */
  mode: string;
  cert_file: string;
  key_file: string;
  fingerprint: string;
  subject: string;
  hosts: string[] | null;
  not_before: string | null;
  not_after: string | null;
  expires_in_days: number | null;
  expired: boolean;
  present: boolean;
}

export interface PanelSettingsView {
  enabled: boolean;
  bind: string;
  port: number;
  public_port: number;
  public_scheme: string;
  entrance: string;
  entrance_masked: string;
  entrance_enabled: boolean;
  session_ttl_seconds: number;
  allowed_ips: string[] | null;
  trusted_proxies: string[] | null;
  domain: string;
  tls: PanelTLSView;
  access_url: string;
  access_host: string;
  effective_port: number;
  scheme: string;
  proxied: boolean;
  restart_pending: boolean;
  restart_reasons: string[] | null;
  running_port: number;
  running_bind: string;
  env_overrides: string[] | null;
  state_file: string;
  updated_at: string;
  client_ip: string;
}

export interface PanelSettingChange {
  field: string;
  old: string;
  new: string;
}

export interface PanelSettingsResult {
  settings: PanelSettingsView;
  changes: PanelSettingChange[] | null;
  access_url: string;
  restart_pending: boolean;
  restart_reasons: string[] | null;
  warnings: string[] | null;
}

export interface PanelConfirmationReason {
  code: string;
  message: string;
}

/** Body of the 409 the server sends instead of applying a lockout. */
export interface PanelConfirmationPayload {
  confirmation_required: boolean;
  reasons: PanelConfirmationReason[] | null;
  new_url: string;
  changes: PanelSettingChange[] | null;
}

/** The editable subset, as the form holds it: strings, because inputs are strings. */
export interface PanelAccessFormState {
  port: string;
  entrance: string;
  entranceEnabled: boolean;
  domain: string;
  allowedIps: string;
  sessionTtlMinutes: string;
  tlsMode: 'off' | 'self_signed' | 'custom';
  tlsCertFile: string;
  tlsKeyFile: string;
}

/** What a confirmation dialog is currently asking about. */
export type PanelPendingAction =
  | { kind: 'save'; payload: Record<string, unknown> }
  | { kind: 'regenerate' }
  | { kind: 'reissue' };

export const SESSION_TTL_MIN_MINUTES = 5;
export const SESSION_TTL_MAX_MINUTES = 43200;

/** Human labels for the field names the server reports in a change list. */
export const PANEL_FIELD_LABELS: Record<string, string> = {
  port: 'Port',
  bind: 'Bind address',
  public_port: 'Public port',
  public_scheme: 'Public scheme',
  entrance: 'Security entrance',
  entrance_enabled: 'Security entrance enabled',
  session_ttl_seconds: 'Session lifetime',
  allowed_ips: 'IP allow list',
  trusted_proxies: 'Trusted proxies',
  domain: 'Domain',
  'tls.enabled': 'TLS',
  'tls.self_signed': 'Self-signed certificate',
  'tls.cert_file': 'Certificate file',
  'tls.key_file': 'Key file',
  'tls.fingerprint': 'Certificate fingerprint',
};

/** Values that are secrets are never echoed back into a diff on screen. */
export const PANEL_SECRET_FIELDS = new Set(['entrance']);
