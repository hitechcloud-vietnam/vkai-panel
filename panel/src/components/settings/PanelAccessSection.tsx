'use client';

/**
 * Panel access settings.
 *
 * This is the one screen in the panel that can make the panel unreachable. It
 * is therefore built around showing, rather than hiding, the consequence of a
 * change: the access URL is always on screen with a copy button, the security
 * entrance is masked until it is deliberately revealed, and nothing that moves
 * the entrance is submitted without a dialog that spells out the new URL.
 */

import { useCallback, useEffect, useState } from 'react';
import {
  AlertTriangle,
  Check,
  CheckCircle,
  Copy,
  Eye,
  EyeOff,
  Globe,
  KeyRound,
  Pencil,
  RefreshCw,
  RotateCcw,
  ServerCog,
  ShieldCheck,
  X,
  XCircle,
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { panelSettingsApi, unwrap } from '@/services/api';
import PanelAccessConfirmDialog from './PanelAccessConfirmDialog';
import {
  PANEL_FIELD_LABELS,
  SESSION_TTL_MAX_MINUTES,
  SESSION_TTL_MIN_MINUTES,
  type PanelAccessFormState,
  type PanelConfirmationPayload,
  type PanelConfirmationReason,
  type PanelPendingAction,
  type PanelSettingChange,
  type PanelSettingsResult,
  type PanelSettingsView,
} from './panel-access-types';

const LABEL_CLASS = 'mb-1.5 block text-sm font-medium text-gray-700';
const HELP_CLASS = 'mt-1 text-xs text-gray-500';
const SELECT_CLASS =
  'h-9 w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function errorMessage(err: unknown, fallback: string): string {
  const response = (err as { response?: { data?: { error?: { message?: string } } } })?.response;
  return response?.data?.error?.message || fallback;
}

function confirmationPayload(err: unknown): PanelConfirmationPayload | null {
  const response = (err as { response?: { status?: number; data?: { data?: PanelConfirmationPayload } } })
    ?.response;
  if (response?.status !== 409) return null;
  const payload = response?.data?.data;
  return payload && payload.confirmation_required ? payload : null;
}

function formFrom(settings: PanelSettingsView): PanelAccessFormState {
  return {
    port: String(settings.port ?? ''),
    entrance: settings.entrance ?? '',
    entranceEnabled: Boolean(settings.entrance_enabled),
    domain: settings.domain ?? '',
    allowedIps: (settings.allowed_ips ?? []).join('\n'),
    sessionTtlMinutes: String(Math.round((settings.session_ttl_seconds ?? 0) / 60)),
    tlsMode: settings.tls?.mode === 'custom' || settings.tls?.mode === 'off'
      ? settings.tls.mode
      : 'self_signed',
    tlsCertFile: settings.tls?.cert_file ?? '',
    tlsKeyFile: settings.tls?.key_file ?? '',
  };
}

function payloadFrom(form: PanelAccessFormState): Record<string, unknown> {
  const allowed = form.allowedIps
    .split(/[\n,;]/)
    .map((entry) => entry.trim())
    .filter(Boolean);

  return {
    port: Number(form.port),
    entrance: form.entrance.trim(),
    entrance_enabled: form.entranceEnabled,
    domain: form.domain.trim(),
    allowed_ips: allowed,
    session_ttl_seconds: Math.round(Number(form.sessionTtlMinutes) * 60),
    tls_enabled: form.tlsMode !== 'off',
    tls_self_signed: form.tlsMode === 'self_signed',
    tls_cert_file: form.tlsCertFile.trim(),
    tls_key_file: form.tlsKeyFile.trim(),
  };
}

function formatSessionTtl(seconds: number): string {
  if (!seconds || seconds <= 0) return 'Not set';
  if (seconds % 86400 === 0) {
    const days = seconds / 86400;
    return `${days} day${days === 1 ? '' : 's'}`;
  }
  if (seconds % 3600 === 0) {
    const hours = seconds / 3600;
    return `${hours} hour${hours === 1 ? '' : 's'}`;
  }
  const minutes = Math.round(seconds / 60);
  return `${minutes} minute${minutes === 1 ? '' : 's'}`;
}

function tlsLabel(settings: PanelSettingsView): string {
  switch (settings.tls?.mode) {
    case 'self_signed':
      return 'HTTPS, self-signed certificate';
    case 'custom':
      return 'HTTPS, operator-supplied certificate';
    default:
      return 'HTTP, no certificate';
  }
}

function formatDate(value: string | null): string {
  if (!value) return 'Unknown';
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return 'Unknown';
  return parsed.toISOString().slice(0, 10);
}

// ---------------------------------------------------------------------------
// Small presentational pieces
// ---------------------------------------------------------------------------

function Row({
  icon,
  label,
  children,
}: {
  icon?: React.ReactNode;
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="grid grid-cols-1 gap-1 border-b border-gray-100 py-3 last:border-b-0 sm:grid-cols-[200px_minmax(0,1fr)] sm:gap-4">
      <div className="flex items-center gap-2 text-sm font-medium text-gray-700">
        {icon ? <span className="text-gray-400">{icon}</span> : null}
        {label}
      </div>
      <div className="min-w-0 text-sm text-gray-900">{children}</div>
    </div>
  );
}

function CopyButton({ value, label }: { value: string; label: string }) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      /* Clipboard blocked: the value is on screen and selectable. */
    }
  };

  return (
    <Button type="button" variant="secondary" size="sm" onClick={copy} aria-label={label}>
      {copied ? <Check size={14} /> : <Copy size={14} />}
      {copied ? 'Copied' : 'Copy'}
    </Button>
  );
}

function LoadingSkeleton() {
  return (
    <div className="animate-pulse space-y-3" aria-hidden="true">
      {Array.from({ length: 7 }).map((_, index) => (
        <div key={index} className="grid grid-cols-1 gap-2 sm:grid-cols-[200px_minmax(0,1fr)] sm:gap-4">
          <div className="h-4 w-40 rounded bg-gray-100" />
          <div className="h-4 w-full max-w-md rounded bg-gray-100" />
        </div>
      ))}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Section
// ---------------------------------------------------------------------------

interface PanelAccessSectionProps {
  /**
   * Called whenever this section starts or stops holding edits that have not
   * been saved. The settings page passes it on to the Updates tab, because an
   * upgrade restarts the panel and would throw those edits away.
   */
  onDirtyChange?: (dirty: boolean) => void;
}

export default function PanelAccessSection({ onDirtyChange }: PanelAccessSectionProps = {}) {
  const [settings, setSettings] = useState<PanelSettingsView | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [editing, setEditing] = useState(false);
  const [form, setForm] = useState<PanelAccessFormState | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const [revealEntrance, setRevealEntrance] = useState(false);
  const [warnings, setWarnings] = useState<string[]>([]);
  const [toast, setToast] = useState<{ type: 'success' | 'error'; message: string } | null>(null);

  const [pending, setPending] = useState<PanelPendingAction | null>(null);
  const [dialogUrl, setDialogUrl] = useState('');
  const [dialogReasons, setDialogReasons] = useState<PanelConfirmationReason[]>([]);
  const [dialogChanges, setDialogChanges] = useState<PanelSettingChange[]>([]);

  // Dirty means "in edit mode and the form no longer matches what is saved".
  // Opening the editor and changing nothing is not unsaved work, and reporting
  // it as such would train an operator to ignore the warning.
  const dirty = Boolean(
    editing && form && settings && JSON.stringify(form) !== JSON.stringify(formFrom(settings)),
  );

  useEffect(() => {
    onDirtyChange?.(dirty);
  }, [dirty, onDirtyChange]);

  // A section that is unmounted is no longer holding anything.
  useEffect(() => () => onDirtyChange?.(false), [onDirtyChange]);

  const showToast = useCallback((type: 'success' | 'error', message: string) => {
    setToast({ type, message });
    window.setTimeout(() => setToast(null), 6000);
  }, []);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await panelSettingsApi.get();
      const data = unwrap<PanelSettingsView>(response, null);
      if (!data) {
        setSettings(null);
        setError(null);
        return;
      }
      setSettings(data);
    } catch (err) {
      setSettings(null);
      setError(errorMessage(err, 'Could not load the panel access settings.'));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const applyResult = useCallback(
    (result: PanelSettingsResult | null, successMessage: string) => {
      if (result?.settings) setSettings(result.settings);
      setWarnings(result?.warnings ?? []);
      setEditing(false);
      setForm(null);
      setFormError(null);
      setRevealEntrance(false);
      showToast('success', successMessage);
    },
    [showToast],
  );

  const closeDialog = useCallback(() => {
    setPending(null);
    setDialogUrl('');
    setDialogReasons([]);
    setDialogChanges([]);
  }, []);

  // -------------------------------------------------------------------------
  // Actions
  // -------------------------------------------------------------------------

  const save = async (confirm: boolean, payload: Record<string, unknown>) => {
    setSubmitting(true);
    setFormError(null);
    try {
      const response = await panelSettingsApi.update({ ...payload, confirm });
      applyResult(unwrap<PanelSettingsResult>(response, null), 'Panel access settings saved.');
      closeDialog();
    } catch (err) {
      const confirmation = confirmationPayload(err);
      if (confirmation) {
        setPending({ kind: 'save', payload });
        setDialogUrl(confirmation.new_url);
        setDialogReasons(confirmation.reasons ?? []);
        setDialogChanges(confirmation.changes ?? []);
        return;
      }
      const message = errorMessage(err, 'Could not save the panel access settings.');
      if (pending) {
        closeDialog();
        showToast('error', message);
      } else {
        setFormError(message);
      }
    } finally {
      setSubmitting(false);
    }
  };

  const regenerateEntrance = async (confirm: boolean) => {
    setSubmitting(true);
    try {
      const response = await panelSettingsApi.regenerateEntrance(confirm);
      applyResult(
        unwrap<PanelSettingsResult>(response, null),
        'A new security entrance is active. Use the new URL from now on.',
      );
      closeDialog();
    } catch (err) {
      const confirmation = confirmationPayload(err);
      if (confirmation) {
        setPending({ kind: 'regenerate' });
        setDialogUrl(confirmation.new_url);
        setDialogReasons(confirmation.reasons ?? []);
        setDialogChanges(confirmation.changes ?? []);
        return;
      }
      closeDialog();
      showToast('error', errorMessage(err, 'Could not generate a new security entrance.'));
    } finally {
      setSubmitting(false);
    }
  };

  const reissueCertificate = async () => {
    setSubmitting(true);
    try {
      const response = await panelSettingsApi.reissueCertificate();
      applyResult(
        unwrap<PanelSettingsResult>(response, null),
        'A new certificate was issued. Restart the panel service to serve it.',
      );
      closeDialog();
    } catch (err) {
      closeDialog();
      showToast('error', errorMessage(err, 'Could not reissue the panel certificate.'));
    } finally {
      setSubmitting(false);
    }
  };

  const confirmPending = () => {
    if (!pending) return;
    if (pending.kind === 'save') void save(true, pending.payload);
    else if (pending.kind === 'regenerate') void regenerateEntrance(true);
    else void reissueCertificate();
  };

  const onSubmit = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!form) return;

    const port = Number(form.port);
    if (!Number.isInteger(port) || port < 1024 || port > 65535) {
      setFormError('Port must be a whole number between 1024 and 65535, and never 80 or 443.');
      return;
    }
    const minutes = Number(form.sessionTtlMinutes);
    if (!Number.isFinite(minutes) || minutes < SESSION_TTL_MIN_MINUTES || minutes > SESSION_TTL_MAX_MINUTES) {
      setFormError(
        `Session lifetime must be between ${SESSION_TTL_MIN_MINUTES} minutes and ${SESSION_TTL_MAX_MINUTES} minutes (30 days).`,
      );
      return;
    }

    void save(false, payloadFrom(form));
  };

  // -------------------------------------------------------------------------
  // Render
  // -------------------------------------------------------------------------

  const dialogTitle =
    pending?.kind === 'regenerate'
      ? 'Generate a new security entrance?'
      : pending?.kind === 'reissue'
        ? 'Reissue the panel certificate?'
        : 'Confirm the new panel address';

  const dialogConfirmLabel =
    pending?.kind === 'regenerate'
      ? 'Generate and apply'
      : pending?.kind === 'reissue'
        ? 'Reissue certificate'
        : 'Save and apply';

  return (
    <div className="space-y-4">
      {toast ? (
        <div
          role="status"
          className={`fixed right-4 top-4 z-50 flex items-center gap-3 rounded-md border px-4 py-3 shadow-lg ${
            toast.type === 'success'
              ? 'border-emerald-200 bg-emerald-50 text-emerald-700'
              : 'border-red-200 bg-red-50 text-red-700'
          }`}
        >
          {toast.type === 'success' ? <CheckCircle size={18} /> : <XCircle size={18} />}
          <span className="text-sm font-medium">{toast.message}</span>
          <button
            type="button"
            onClick={() => setToast(null)}
            aria-label="Dismiss notification"
            className="ml-2 rounded-md p-0.5 hover:bg-white/60 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
          >
            <X size={14} />
          </button>
        </div>
      ) : null}

      {settings?.restart_pending ? (
        <div
          role="status"
          className="flex items-start gap-3 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3"
        >
          <span className="mt-0.5 text-amber-600">
            <AlertTriangle size={18} />
          </span>
          <div className="min-w-0 text-sm text-amber-800">
            <p className="font-medium">Restart required</p>
            <p className="mt-0.5">
              These settings are saved but the running panel is still using the previous values.
              Restart the panel service to apply them.
            </p>
            {(settings.restart_reasons ?? []).length > 0 ? (
              <ul className="mt-1.5 list-inside list-disc">
                {(settings.restart_reasons ?? []).map((reason) => (
                  <li key={reason}>{reason}</li>
                ))}
              </ul>
            ) : null}
          </div>
        </div>
      ) : null}

      {warnings.length > 0 ? (
        <div className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
          <ul className="list-inside list-disc space-y-1">
            {warnings.map((warning) => (
              <li key={warning}>{warning}</li>
            ))}
          </ul>
        </div>
      ) : null}

      <Card>
        <CardHeader className="flex-row items-start justify-between gap-4 space-y-0">
          <div>
            <CardTitle>Panel access</CardTitle>
            <p className="mt-1 text-sm text-gray-600">
              How this panel is reached: its own port, its security entrance, the addresses allowed
              to connect and the certificate it serves. Ports 80 and 443 stay reserved for hosted
              websites.
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <Button
              type="button"
              variant="secondary"
              size="sm"
              onClick={() => void load()}
              disabled={loading || submitting}
            >
              <RefreshCw size={14} />
              Refresh
            </Button>
            {settings && !editing ? (
              <Button
                type="button"
                size="sm"
                onClick={() => {
                  setForm(formFrom(settings));
                  setFormError(null);
                  setEditing(true);
                }}
              >
                <Pencil size={14} />
                Edit
              </Button>
            ) : null}
          </div>
        </CardHeader>

        <CardContent>
          {loading ? <LoadingSkeleton /> : null}

          {!loading && error ? (
            <div className="flex flex-col items-start gap-3 rounded-md border border-red-200 bg-red-50 px-4 py-3">
              <div className="flex items-start gap-3">
                <span className="mt-0.5 text-red-600">
                  <XCircle size={18} />
                </span>
                <div className="text-sm text-red-700">
                  <p className="font-medium">Panel access settings unavailable</p>
                  <p className="mt-0.5">{error}</p>
                </div>
              </div>
              <Button type="button" variant="secondary" size="sm" onClick={() => void load()}>
                <RotateCcw size={14} />
                Try again
              </Button>
            </div>
          ) : null}

          {!loading && !error && !settings ? (
            <div className="rounded-md border border-dashed border-gray-300 px-4 py-8 text-center">
              <ServerCog className="mx-auto text-gray-400" size={24} />
              <p className="mt-2 text-sm font-medium text-gray-900">No access gate configured</p>
              <p className="mt-1 text-sm text-gray-600">
                This instance is not running behind the panel access gate, so there is nothing to
                change here.
              </p>
            </div>
          ) : null}

          {!loading && !error && settings && !editing ? (
            <div className="divide-y divide-gray-100">
              <Row icon={<Globe size={15} />} label="Access URL">
                <div className="flex flex-wrap items-center gap-2">
                  <code className="min-w-0 break-all rounded border border-gray-200 bg-gray-50 px-2 py-1 font-mono text-xs text-gray-900">
                    {settings.access_url}
                  </code>
                  <CopyButton value={settings.access_url} label="Copy the panel access URL" />
                </div>
              </Row>

              <Row icon={<ServerCog size={15} />} label="Port">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-mono">{settings.effective_port}</span>
                  {settings.proxied ? (
                    <Badge variant="info">bound on {settings.port} behind a proxy</Badge>
                  ) : null}
                  {settings.port !== settings.running_port ? (
                    <Badge variant="warning">running on {settings.running_port}</Badge>
                  ) : null}
                </div>
              </Row>

              <Row icon={<KeyRound size={15} />} label="Security entrance">
                {settings.entrance_enabled ? (
                  <div className="flex flex-wrap items-center gap-2">
                    <code className="break-all rounded border border-gray-200 bg-gray-50 px-2 py-1 font-mono text-xs text-gray-900">
                      {revealEntrance ? settings.entrance : settings.entrance_masked}
                    </code>
                    <Button
                      type="button"
                      variant="secondary"
                      size="sm"
                      onClick={() => setRevealEntrance((value) => !value)}
                      aria-pressed={revealEntrance}
                    >
                      {revealEntrance ? <EyeOff size={14} /> : <Eye size={14} />}
                      {revealEntrance ? 'Hide' : 'Reveal'}
                    </Button>
                    {revealEntrance ? (
                      <CopyButton value={settings.entrance} label="Copy the security entrance" />
                    ) : null}
                  </div>
                ) : (
                  <Badge variant="danger">Disabled — the panel answers on every path</Badge>
                )}
              </Row>

              <Row icon={<Globe size={15} />} label="Domain">
                {settings.domain ? (
                  <span className="font-mono">{settings.domain}</span>
                ) : (
                  <span className="text-gray-500">Not pinned — any host name is accepted</span>
                )}
              </Row>

              <Row label="IP allow list">
                {(settings.allowed_ips ?? []).length > 0 ? (
                  <div className="flex flex-wrap gap-1.5">
                    {(settings.allowed_ips ?? []).map((entry) => (
                      <Badge key={entry} variant="neutral" className="font-mono">
                        {entry}
                      </Badge>
                    ))}
                  </div>
                ) : (
                  <span className="text-gray-500">
                    Empty — every address may connect (your address: {settings.client_ip || 'unknown'})
                  </span>
                )}
              </Row>

              <Row icon={<ShieldCheck size={15} />} label="TLS">
                <div className="space-y-1.5">
                  <div className="flex flex-wrap items-center gap-2">
                    <span>{tlsLabel(settings)}</span>
                    {settings.tls?.enabled ? (
                      settings.tls.expired ? (
                        <Badge variant="danger">Expired</Badge>
                      ) : settings.tls.expires_in_days !== null &&
                        settings.tls.expires_in_days <= 30 ? (
                        <Badge variant="warning">
                          Expires in {settings.tls.expires_in_days} day
                          {settings.tls.expires_in_days === 1 ? '' : 's'}
                        </Badge>
                      ) : (
                        <Badge variant="success">Valid</Badge>
                      )
                    ) : (
                      <Badge variant="warning">Off</Badge>
                    )}
                  </div>
                  {settings.tls?.enabled && settings.tls.present ? (
                    <>
                      <p className="text-xs text-gray-600">
                        Expires {formatDate(settings.tls.not_after)}
                        {settings.tls.expires_in_days !== null
                          ? ` (${settings.tls.expires_in_days} days)`
                          : ''}
                      </p>
                      {settings.tls.fingerprint ? (
                        <div className="flex flex-wrap items-center gap-2">
                          <code className="min-w-0 break-all rounded border border-gray-200 bg-gray-50 px-2 py-1 font-mono text-[11px] text-gray-700">
                            {settings.tls.fingerprint}
                          </code>
                          <CopyButton
                            value={settings.tls.fingerprint}
                            label="Copy the certificate fingerprint"
                          />
                        </div>
                      ) : null}
                    </>
                  ) : null}
                  {settings.tls?.enabled && !settings.tls.present ? (
                    <p className="text-xs text-amber-700">
                      No certificate is on disk yet at {settings.tls.cert_file || '(no path set)'}.
                    </p>
                  ) : null}
                </div>
              </Row>

              <Row label="Session lifetime">
                {formatSessionTtl(settings.session_ttl_seconds)}
              </Row>

              {(settings.env_overrides ?? []).length > 0 ? (
                <Row label="Pinned by environment">
                  <div className="flex flex-wrap gap-1.5">
                    {(settings.env_overrides ?? []).map((name) => (
                      <Badge key={name} variant="warning" className="font-mono">
                        {PANEL_FIELD_LABELS[name] ?? name}
                      </Badge>
                    ))}
                  </div>
                  <p className={HELP_CLASS}>
                    These come from environment variables and are restored on the next restart.
                  </p>
                </Row>
              ) : null}
            </div>
          ) : null}

          {!loading && !error && settings && editing && form ? (
            <form onSubmit={onSubmit} className="space-y-4">
              {formError ? (
                <div className="flex items-start gap-2 rounded-md border border-red-200 bg-red-50 px-3 py-2.5 text-sm text-red-700">
                  <XCircle size={16} className="mt-0.5 shrink-0" />
                  <span>{formError}</span>
                </div>
              ) : null}

              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div>
                  <label htmlFor="panel-port" className={LABEL_CLASS}>
                    Port
                  </label>
                  <Input
                    id="panel-port"
                    type="number"
                    min={1024}
                    max={65535}
                    value={form.port}
                    onChange={(event) => setForm({ ...form, port: event.target.value })}
                  />
                  <p className={HELP_CLASS}>
                    1024–65535, never 80 or 443. Changing this needs a service restart.
                  </p>
                </div>

                <div>
                  <label htmlFor="panel-session-ttl" className={LABEL_CLASS}>
                    Session lifetime (minutes)
                  </label>
                  <Input
                    id="panel-session-ttl"
                    type="number"
                    min={SESSION_TTL_MIN_MINUTES}
                    max={SESSION_TTL_MAX_MINUTES}
                    value={form.sessionTtlMinutes}
                    onChange={(event) =>
                      setForm({ ...form, sessionTtlMinutes: event.target.value })
                    }
                  />
                  <p className={HELP_CLASS}>
                    Between {SESSION_TTL_MIN_MINUTES} minutes and {SESSION_TTL_MAX_MINUTES} minutes
                    (30 days).
                  </p>
                </div>

                <div className="sm:col-span-2">
                  <label htmlFor="panel-entrance" className={LABEL_CLASS}>
                    Security entrance
                  </label>
                  <Input
                    id="panel-entrance"
                    type="text"
                    value={form.entrance}
                    disabled={!form.entranceEnabled}
                    placeholder="/vkai_a1b2c3d4"
                    onChange={(event) => setForm({ ...form, entrance: event.target.value })}
                  />
                  <label className="mt-2 flex items-center gap-2 text-sm text-gray-700">
                    <input
                      type="checkbox"
                      checked={form.entranceEnabled}
                      onChange={(event) =>
                        setForm({ ...form, entranceEnabled: event.target.checked })
                      }
                      className="h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500"
                    />
                    Require the security entrance
                  </label>
                  <p className={HELP_CLASS}>
                    A leading slash followed by 4 to 64 letters, digits, hyphens or underscores.
                    Turning it off makes the panel answer on every path.
                  </p>
                </div>

                <div>
                  <label htmlFor="panel-domain" className={LABEL_CLASS}>
                    Domain
                  </label>
                  <Input
                    id="panel-domain"
                    type="text"
                    value={form.domain}
                    placeholder="panel.example.com"
                    onChange={(event) => setForm({ ...form, domain: event.target.value })}
                  />
                  <p className={HELP_CLASS}>
                    Leave empty to accept any host name. A bare host name, no scheme or port.
                  </p>
                </div>

                <div>
                  <label htmlFor="panel-tls-mode" className={LABEL_CLASS}>
                    TLS
                  </label>
                  <select
                    id="panel-tls-mode"
                    className={SELECT_CLASS}
                    value={form.tlsMode}
                    onChange={(event) =>
                      setForm({
                        ...form,
                        tlsMode: event.target.value as PanelAccessFormState['tlsMode'],
                      })
                    }
                  >
                    <option value="self_signed">HTTPS with a self-signed certificate</option>
                    <option value="custom">HTTPS with my own certificate</option>
                    <option value="off">HTTP only (not recommended)</option>
                  </select>
                  <p className={HELP_CLASS}>Changing the TLS mode needs a service restart.</p>
                </div>

                {form.tlsMode === 'custom' ? (
                  <>
                    <div>
                      <label htmlFor="panel-tls-cert" className={LABEL_CLASS}>
                        Certificate file
                      </label>
                      <Input
                        id="panel-tls-cert"
                        type="text"
                        value={form.tlsCertFile}
                        placeholder="/vkai-panel/etc/ssl/panel.crt"
                        onChange={(event) => setForm({ ...form, tlsCertFile: event.target.value })}
                      />
                    </div>
                    <div>
                      <label htmlFor="panel-tls-key" className={LABEL_CLASS}>
                        Private key file
                      </label>
                      <Input
                        id="panel-tls-key"
                        type="text"
                        value={form.tlsKeyFile}
                        placeholder="/vkai-panel/etc/ssl/panel.key"
                        onChange={(event) => setForm({ ...form, tlsKeyFile: event.target.value })}
                      />
                    </div>
                  </>
                ) : null}

                <div className="sm:col-span-2">
                  <label htmlFor="panel-allowed-ips" className={LABEL_CLASS}>
                    IP allow list
                  </label>
                  <textarea
                    id="panel-allowed-ips"
                    rows={4}
                    value={form.allowedIps}
                    placeholder={`203.0.113.7\n10.0.0.0/8`}
                    onChange={(event) => setForm({ ...form, allowedIps: event.target.value })}
                    className="w-full rounded-md border border-gray-300 bg-white px-3 py-2 font-mono text-sm text-gray-900 placeholder:text-gray-400 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
                  />
                  <p className={HELP_CLASS}>
                    One IP address or CIDR block per line. Leave empty to allow every address. Your
                    current address is {settings.client_ip || 'unknown'}.
                  </p>
                </div>
              </div>

              <div className="flex items-center justify-end gap-2 border-t border-gray-200 pt-4">
                <Button
                  type="button"
                  variant="secondary"
                  onClick={() => {
                    setEditing(false);
                    setForm(null);
                    setFormError(null);
                  }}
                  disabled={submitting}
                >
                  Cancel
                </Button>
                <Button type="submit" disabled={submitting}>
                  {submitting ? 'Saving…' : 'Review and save'}
                </Button>
              </div>
            </form>
          ) : null}
        </CardContent>
      </Card>

      {!loading && !error && settings && !editing ? (
        <Card>
          <CardHeader>
            <CardTitle>Maintenance</CardTitle>
            <p className="mt-1 text-sm text-gray-600">
              Rotate the entrance after it has been shared, or replace the certificate after the
              panel changed address.
            </p>
          </CardHeader>
          <CardContent className="flex flex-wrap gap-2">
            <Button
              type="button"
              variant="secondary"
              onClick={() => void regenerateEntrance(false)}
              disabled={submitting || !settings.entrance_enabled}
            >
              <KeyRound size={15} />
              Regenerate entrance
            </Button>
            <Button
              type="button"
              variant="secondary"
              onClick={() => {
                setPending({ kind: 'reissue' });
                setDialogUrl(settings.access_url);
                setDialogReasons([
                  {
                    code: 'certificate_reissue',
                    message:
                      'The current certificate and key are deleted and replaced. Browsers that trusted the old fingerprint will warn again, and the panel service has to restart before the new certificate is served.',
                  },
                ]);
                setDialogChanges([]);
              }}
              disabled={submitting || !settings.tls?.enabled || !settings.tls?.self_signed}
            >
              <ShieldCheck size={15} />
              Reissue certificate
            </Button>
          </CardContent>
        </Card>
      ) : null}

      <PanelAccessConfirmDialog
        open={pending !== null}
        title={dialogTitle}
        newUrl={dialogUrl}
        reasons={dialogReasons}
        changes={dialogChanges}
        submitting={submitting}
        confirmLabel={dialogConfirmLabel}
        onCancel={closeDialog}
        onConfirm={confirmPending}
      />
    </div>
  );
}
