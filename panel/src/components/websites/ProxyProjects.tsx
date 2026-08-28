'use client';

/**
 * The Proxy Project tab: a hostname forwarded to an upstream.
 *
 * A proxy has no document root, so this list has no document root column and
 * the form asks for none. That absence is the point of splitting the screen.
 *
 * Mounted and wired:
 *   list / create / delete   /api/v1/reverse-proxy
 *   access logs              /api/v1/reverse-proxy/:id/access-logs
 *
 * NOT wired, because it does not exist: anything that makes the proxy live.
 * ReverseProxyService.Create writes a row and never touches the web server, so
 * the tab carries a standing notice saying exactly that. An operator who reads
 * this screen must not walk away believing traffic is being forwarded.
 */

import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  ExternalLink,
  FileText,
  Loader2,
  Plus,
  RefreshCw,
  RotateCw,
  Share2,
  Trash2,
} from 'lucide-react';

import ServerScopeField, { SERVER_SCOPE_COPY_EN } from '@/components/servers/ServerScopeField';
import { MetricText, Unavailable } from '@/components/Unavailable';
import { errorMessage } from '@/lib/apiError';
import { serverLabel } from '@/lib/servers';
import type { ManagedServer, ManagedWebsite } from '@/types/server';
import type {
  CreateReverseProxyRequest,
  ReverseProxy,
  ReverseProxyAccessLog,
} from '@/types/website';

import { reverseProxyApi } from './api';
import { PROJECT_TYPE_GAPS } from './projectTypes';
import { RemoveProxyDialog } from './RemoveDialogs';
import {
  BTN_PRIMARY,
  BTN_ROW,
  BTN_SECONDARY,
  Field,
  FormError,
  INPUT_CLASS,
  Modal,
  Notice,
  PanelEmpty,
  PanelError,
  PanelLoading,
  StatusBadge,
  Surface,
  SurfaceHeader,
  TD,
  TH,
  Tag,
} from './ui';
import { certificateForDomain, type SiteContext } from './useSiteContext';

const NO_RESTART_REASON =
  'Not available: a proxy has no process of its own to restart. The route that would reload the web server after a proxy changes does not exist yet.';

interface ProxyForm {
  name: string;
  server_id: string;
  website_id: string;
  domain: string;
  listen_port: string;
  protocol: string;
  target_host: string;
  target_port: string;
  target_url: string;
  host_header: string;
  websocket: boolean;
  ssl_enabled: boolean;
  ssl_redirect: boolean;
  health_check: string;
  health_interval: string;
}

const EMPTY_FORM: ProxyForm = {
  name: '',
  server_id: '',
  website_id: '',
  domain: '',
  listen_port: '80',
  protocol: 'http',
  target_host: '127.0.0.1',
  target_port: '',
  target_url: '',
  host_header: '',
  websocket: true,
  ssl_enabled: false,
  ssl_redirect: false,
  health_check: '',
  health_interval: '30',
};

export interface ProxyProjectsProps {
  servers: ManagedServer[];
  defaultServerId: string;
  ctx: SiteContext;
  onCount: (count: number | null) => void;
}

export default function ProxyProjects({
  servers,
  defaultServerId,
  ctx,
  onCount,
}: ProxyProjectsProps) {
  const [proxies, setProxies] = useState<ReverseProxy[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState<ProxyForm>(EMPTY_FORM);
  const [formError, setFormError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const [logsFor, setLogsFor] = useState<ReverseProxy | null>(null);
  const [removing, setRemoving] = useState<ReverseProxy | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const list = await reverseProxyApi.list();
      setProxies(list);
      onCount(list.length);
    } catch (err) {
      setProxies([]);
      onCount(null);
      setError(errorMessage(err, 'Failed to load proxy projects'));
    } finally {
      setLoading(false);
    }
  }, [onCount]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    if (!defaultServerId) return;
    setForm((prev) => {
      if (prev.server_id && servers.some((s) => s.id === prev.server_id)) return prev;
      return { ...prev, server_id: defaultServerId };
    });
  }, [defaultServerId, servers]);

  const websiteById = useMemo(() => {
    const map = new Map<string, ManagedWebsite>();
    ctx.websites.forEach((w) => map.set(w.id, w));
    return map;
  }, [ctx.websites]);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setFormError('');
    const name = form.name.trim();
    const domain = form.domain.trim().toLowerCase();
    const targetHost = form.target_host.trim();
    const targetPort = Number(form.target_port);
    if (!name) {
      setFormError('A project name is required.');
      return;
    }
    if (!domain) {
      setFormError('The hostname this proxy answers on is required.');
      return;
    }
    if (!targetHost) {
      setFormError('An upstream host is required.');
      return;
    }
    if (!Number.isInteger(targetPort) || targetPort < 1 || targetPort > 65535) {
      setFormError('An upstream port between 1 and 65535 is required.');
      return;
    }
    if (!form.server_id) {
      setFormError('No server is registered yet, so there is nowhere to put this proxy.');
      return;
    }

    const payload: CreateReverseProxyRequest = {
      server_id: form.server_id,
      name,
      domain,
      target_host: targetHost,
      target_port: targetPort,
      protocol: form.protocol || 'http',
      websocket: form.websocket,
      ssl_enabled: form.ssl_enabled,
      ssl_redirect: form.ssl_redirect,
    };
    const listenPort = Number(form.listen_port);
    if (Number.isInteger(listenPort) && listenPort > 0) payload.listen_port = listenPort;
    if (form.target_url.trim()) payload.target_url = form.target_url.trim();
    if (form.website_id) payload.website_id = form.website_id;
    if (form.host_header.trim()) payload.headers = { Host: form.host_header.trim() };
    if (form.health_check.trim()) {
      payload.health_check = form.health_check.trim();
      const interval = Number(form.health_interval);
      if (Number.isInteger(interval) && interval > 0) payload.health_interval = interval;
    }

    setSubmitting(true);
    try {
      await reverseProxyApi.create(payload);
      setShowForm(false);
      setForm({ ...EMPTY_FORM, server_id: form.server_id });
      await load();
    } catch (err) {
      setFormError(errorMessage(err, 'Failed to create the proxy project'));
    } finally {
      setSubmitting(false);
    }
  };

  const showServerColumn = servers.length > 1;
  const serverName = (id: string | undefined) => {
    const match = servers.find((s) => s.id === id);
    return match ? serverLabel(match) : null;
  };
  const formatDate = (value: string | null | undefined) => {
    if (!value) return null;
    const d = new Date(value);
    return Number.isNaN(d.getTime()) ? null : d.toLocaleDateString();
  };

  return (
    <>
      <div className="space-y-4">
        <Notice tone="amber" title="A proxy created here is recorded, not yet served">
          {PROJECT_TYPE_GAPS.proxy.map((line) => (
            <p key={line}>{line}</p>
          ))}
        </Notice>

        <Surface>
          <SurfaceHeader
            title="Proxy projects"
            description="A hostname forwarded to an upstream. No document root, and none is shown."
            actions={
              <>
                <button type="button" onClick={load} className={BTN_SECONDARY}>
                  <RefreshCw size={16} aria-hidden="true" />
                  Refresh
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setFormError('');
                    setForm((prev) => ({ ...EMPTY_FORM, server_id: prev.server_id || defaultServerId }));
                    setShowForm(true);
                  }}
                  className={BTN_PRIMARY}
                >
                  <Plus size={16} aria-hidden="true" />
                  Add proxy project
                </button>
              </>
            }
          />

          {loading ? (
            <PanelLoading label="Loading proxy projects..." />
          ) : error ? (
            <PanelError message={error} onRetry={load} />
          ) : proxies.length === 0 ? (
            <PanelEmpty
              icon={<Share2 size={40} aria-hidden="true" />}
              title="No proxy project yet"
              body={
                <>
                  A proxy project records a hostname and the upstream it belongs in front of, plus
                  the host header, websocket support and the health check. Read the notice above
                  before you rely on one: the record is real, the forwarding is not written to the
                  web server yet.
                </>
              }
              action={
                <button
                  type="button"
                  onClick={() => {
                    setForm((prev) => ({ ...EMPTY_FORM, server_id: prev.server_id || defaultServerId }));
                    setShowForm(true);
                  }}
                  disabled={servers.length === 0}
                  className={BTN_PRIMARY}
                >
                  <Plus size={16} aria-hidden="true" />
                  Add proxy project
                </button>
              }
            />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full min-w-[1100px] border-collapse">
                <thead className="bg-gray-50">
                  <tr className="border-b border-gray-200">
                    <TH>Hostname</TH>
                    {showServerColumn && <TH>Server</TH>}
                    <TH>Upstream</TH>
                    <TH>Status</TH>
                    <TH>SSL</TH>
                    <TH>Options</TH>
                    <TH>Actions</TH>
                  </tr>
                </thead>
                <tbody>
                  {proxies.map((proxy) => {
                    const site = proxy.website_id ? websiteById.get(proxy.website_id) || null : null;
                    const cert = certificateForDomain(
                      ctx.certificates,
                      proxy.domain || site?.domain,
                      site?.id
                    );
                    const expiry = formatDate(cert?.not_after);
                    const upstream =
                      proxy.target_url ||
                      (proxy.target_host
                        ? `${proxy.protocol || 'http'}://${proxy.target_host}:${proxy.target_port ?? ''}`
                        : '');
                    const hostHeader = proxy.headers?.Host || proxy.headers?.host || '';
                    const url = proxy.domain
                      ? `${proxy.ssl_enabled ? 'https' : 'http'}://${proxy.domain}`
                      : '';
                    return (
                      <tr key={proxy.id} className="border-b border-gray-100 align-top hover:bg-gray-50">
                        <TD className="font-medium text-gray-900">
                          <div className="flex flex-col gap-1">
                            <MetricText
                              value={proxy.domain || null}
                              reason="Not available: this proxy has no hostname recorded."
                            />
                            <span className="text-xs text-gray-500">
                              {proxy.name || 'unnamed'} · listening on port {proxy.listen_port ?? 80}
                            </span>
                          </div>
                        </TD>
                        {showServerColumn && (
                          <TD>
                            <MetricText
                              value={serverName(proxy.server_id)}
                              reason="Not available: this proxy points at a server that is no longer in the list."
                            />
                          </TD>
                        )}
                        <TD>
                          <div className="flex flex-col gap-1">
                            <MetricText
                              value={upstream || null}
                              reason="Not available: no upstream is recorded for this proxy."
                              className="break-all font-mono text-xs"
                            />
                            <span className="text-xs text-gray-500">
                              Host header:{' '}
                              {hostHeader ? (
                                <span className="font-mono">{hostHeader}</span>
                              ) : (
                                'passed through unchanged'
                              )}
                            </span>
                          </div>
                        </TD>
                        <TD>
                          <StatusBadge status={proxy.status} />
                        </TD>
                        <TD>
                          {cert ? (
                            <div className="flex flex-col gap-0.5">
                              <span className="inline-flex w-fit items-center rounded-md bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700">
                                {cert.source || 'installed'}
                              </span>
                              <span className="text-xs text-gray-500">
                                {expiry ? `expires ${expiry}` : 'no expiry recorded'}
                              </span>
                            </div>
                          ) : proxy.ssl_enabled ? (
                            <div className="flex flex-col gap-0.5">
                              <span className="inline-flex w-fit items-center rounded-md bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700">
                                enabled
                              </span>
                              <span className="text-xs text-gray-500">no certificate row</span>
                            </div>
                          ) : (
                            <span className="inline-flex items-center rounded-md bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600">
                              off
                            </span>
                          )}
                        </TD>
                        <TD>
                          <div className="flex flex-wrap items-center gap-1.5">
                            <Tag tone={proxy.websocket ? 'sky' : 'gray'}>
                              {proxy.websocket ? 'websocket' : 'no websocket'}
                            </Tag>
                            {proxy.ssl_redirect && <Tag>force https</Tag>}
                            {proxy.load_balancer && <Tag>load balanced</Tag>}
                            {proxy.health_check ? (
                              <Tag>
                                health {proxy.health_check} / {proxy.health_interval ?? 0}s
                              </Tag>
                            ) : null}
                          </div>
                          <p className="mt-1 text-xs text-gray-500">
                            <Unavailable reason="Not available: models.ReverseProxy has no cache fields, so cache lifetime, cache keys and purge have nowhere to be stored." />{' '}
                            cache settings
                          </p>
                        </TD>
                        <TD>
                          <div className="flex flex-wrap items-center gap-1.5">
                            {url ? (
                              <a
                                href={url}
                                target="_blank"
                                rel="noreferrer noopener"
                                className={BTN_ROW}
                                title={`Opens ${url} in a new tab`}
                              >
                                <ExternalLink size={12} aria-hidden="true" />
                                Open
                              </a>
                            ) : (
                              <button
                                type="button"
                                className={BTN_ROW}
                                disabled
                                title="Not available: this proxy has no hostname to open."
                              >
                                <ExternalLink size={12} aria-hidden="true" />
                                Open
                              </button>
                            )}
                            <button
                              type="button"
                              className={BTN_ROW}
                              onClick={() => setLogsFor(proxy)}
                            >
                              <FileText size={12} aria-hidden="true" />
                              Logs
                            </button>
                            <button
                              type="button"
                              className={BTN_ROW}
                              disabled
                              title={NO_RESTART_REASON}
                            >
                              <RotateCw size={12} aria-hidden="true" />
                              Restart
                            </button>
                            <button
                              type="button"
                              className={`${BTN_ROW} text-red-700 hover:bg-red-50`}
                              onClick={() => setRemoving(proxy)}
                            >
                              <Trash2 size={12} aria-hidden="true" />
                              Remove
                            </button>
                          </div>
                        </TD>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </Surface>
      </div>

      {showForm && (
        <Modal title="Add proxy project" onClose={() => setShowForm(false)} wide>
          <form onSubmit={submit} className="space-y-4">
            <FormError message={formError} />

            <Notice tone="amber" title="This records the forwarding rule; it does not start it">
              <p>{PROJECT_TYPE_GAPS.proxy[0]}</p>
            </Notice>

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field id="proxy-name" label="Project name" required>
                <input
                  id="proxy-name"
                  type="text"
                  value={form.name}
                  onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))}
                  className={INPUT_CLASS}
                  placeholder="checkout-frontdoor"
                  required
                />
              </Field>
              <Field
                id="proxy-domain"
                label="Hostname"
                required
                hint="The name clients ask for. A proxy has no document root, so this is the whole of its identity."
              >
                <input
                  id="proxy-domain"
                  type="text"
                  value={form.domain}
                  onChange={(e) => setForm((p) => ({ ...p, domain: e.target.value }))}
                  className={INPUT_CLASS}
                  placeholder="app.example.vn"
                  required
                />
              </Field>
            </div>

            <ServerScopeField
              id="proxy-server"
              servers={servers}
              value={form.server_id}
              onChange={(id) => setForm((p) => ({ ...p, server_id: id }))}
              copy={SERVER_SCOPE_COPY_EN}
            />

            <fieldset className="rounded-md border border-gray-200 p-4">
              <legend className="px-1 text-sm font-medium text-gray-700">Upstream target</legend>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
                <Field id="proxy-protocol" label="Protocol">
                  <select
                    id="proxy-protocol"
                    value={form.protocol}
                    onChange={(e) => setForm((p) => ({ ...p, protocol: e.target.value }))}
                    className={INPUT_CLASS}
                  >
                    <option value="http">http</option>
                    <option value="https">https</option>
                  </select>
                </Field>
                <Field id="proxy-target-host" label="Upstream host" required>
                  <input
                    id="proxy-target-host"
                    type="text"
                    value={form.target_host}
                    onChange={(e) => setForm((p) => ({ ...p, target_host: e.target.value }))}
                    className={INPUT_CLASS}
                    placeholder="127.0.0.1"
                    required
                  />
                </Field>
                <Field id="proxy-target-port" label="Upstream port" required>
                  <input
                    id="proxy-target-port"
                    type="number"
                    min={1}
                    max={65535}
                    value={form.target_port}
                    onChange={(e) => setForm((p) => ({ ...p, target_port: e.target.value }))}
                    className={INPUT_CLASS}
                    placeholder="3000"
                    required
                  />
                </Field>
              </div>
              <div className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
                <Field
                  id="proxy-target-url"
                  label="Full upstream URL (optional)"
                  hint="Overrides the three fields above when the upstream is not a plain host and port."
                >
                  <input
                    id="proxy-target-url"
                    type="text"
                    value={form.target_url}
                    onChange={(e) => setForm((p) => ({ ...p, target_url: e.target.value }))}
                    className={INPUT_CLASS}
                    placeholder="http://127.0.0.1:3000/api"
                  />
                </Field>
                <Field
                  id="proxy-host-header"
                  label="Host header sent upstream"
                  hint="Left empty, the client's Host header is passed through unchanged. Set it when the upstream serves by name."
                >
                  <input
                    id="proxy-host-header"
                    type="text"
                    value={form.host_header}
                    onChange={(e) => setForm((p) => ({ ...p, host_header: e.target.value }))}
                    className={INPUT_CLASS}
                    placeholder="internal.example.vn"
                  />
                </Field>
              </div>
            </fieldset>

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field id="proxy-listen" label="Listen port">
                <input
                  id="proxy-listen"
                  type="number"
                  min={1}
                  max={65535}
                  value={form.listen_port}
                  onChange={(e) => setForm((p) => ({ ...p, listen_port: e.target.value }))}
                  className={INPUT_CLASS}
                />
              </Field>
              <Field
                id="proxy-health"
                label="Health check path (optional)"
                hint="Recorded with its interval below. Nothing polls it yet."
              >
                <input
                  id="proxy-health"
                  type="text"
                  value={form.health_check}
                  onChange={(e) => setForm((p) => ({ ...p, health_check: e.target.value }))}
                  className={INPUT_CLASS}
                  placeholder="/healthz"
                />
              </Field>
            </div>

            {form.health_check.trim() && (
              <Field id="proxy-health-interval" label="Health check interval (seconds)">
                <input
                  id="proxy-health-interval"
                  type="number"
                  min={1}
                  value={form.health_interval}
                  onChange={(e) => setForm((p) => ({ ...p, health_interval: e.target.value }))}
                  className={INPUT_CLASS}
                />
              </Field>
            )}

            <fieldset className="space-y-2 rounded-md border border-gray-200 p-4">
              <legend className="px-1 text-sm font-medium text-gray-700">Connection handling</legend>
              <label className="flex items-start gap-3 text-sm text-gray-700">
                <input
                  type="checkbox"
                  checked={form.websocket}
                  onChange={(e) => setForm((p) => ({ ...p, websocket: e.target.checked }))}
                  className="mt-0.5 h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500"
                />
                <span>
                  Allow websocket upgrades
                  <span className="mt-0.5 block text-xs text-gray-500">
                    Required by anything that holds a connection open: live dashboards, chat,
                    hot reload.
                  </span>
                </span>
              </label>
              <label className="flex items-start gap-3 text-sm text-gray-700">
                <input
                  type="checkbox"
                  checked={form.ssl_enabled}
                  onChange={(e) => setForm((p) => ({ ...p, ssl_enabled: e.target.checked }))}
                  className="mt-0.5 h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500"
                />
                <span>This hostname is served over HTTPS</span>
              </label>
              <label className="flex items-start gap-3 text-sm text-gray-700">
                <input
                  type="checkbox"
                  checked={form.ssl_redirect}
                  disabled={!form.ssl_enabled}
                  onChange={(e) => setForm((p) => ({ ...p, ssl_redirect: e.target.checked }))}
                  className="mt-0.5 h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500 disabled:cursor-not-allowed"
                />
                <span>Redirect plain HTTP to HTTPS</span>
              </label>
              <p className="text-xs text-gray-500">
                Cache settings are not offered: the proxy model has no cache fields, so anything
                entered here would be dropped.
              </p>
            </fieldset>

            <Field
              id="proxy-website"
              label="Attached website (optional)"
              hint="Attaching a website is what lets the panel show this proxy's certificate and expiry."
            >
              <select
                id="proxy-website"
                value={form.website_id}
                onChange={(e) => setForm((p) => ({ ...p, website_id: e.target.value }))}
                className={INPUT_CLASS}
              >
                <option value="">Not attached</option>
                {ctx.websites.map((w) => (
                  <option key={w.id} value={w.id}>
                    {w.domain || w.id}
                  </option>
                ))}
              </select>
            </Field>

            <div className="flex justify-end gap-3 border-t border-gray-200 pt-4">
              <button type="button" onClick={() => setShowForm(false)} className={BTN_SECONDARY}>
                Cancel
              </button>
              <button type="submit" className={BTN_PRIMARY} disabled={submitting || !form.server_id}>
                {submitting ? (
                  <Loader2 size={16} className="animate-spin" aria-hidden="true" />
                ) : (
                  <Plus size={16} aria-hidden="true" />
                )}
                {submitting ? 'Creating...' : 'Create proxy'}
              </button>
            </div>
          </form>
        </Modal>
      )}

      {logsFor && <ProxyLogsDialog proxy={logsFor} onClose={() => setLogsFor(null)} />}
      {removing && (
        <RemoveProxyDialog
          proxy={removing}
          onClose={() => setRemoving(null)}
          onRemoved={load}
        />
      )}
    </>
  );
}

/** Access log lines recorded for one proxy. */
function ProxyLogsDialog({ proxy, onClose }: { proxy: ReverseProxy; onClose: () => void }) {
  const [entries, setEntries] = useState<ReverseProxyAccessLog[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      setEntries(await reverseProxyApi.accessLogs(proxy.id));
    } catch (err) {
      setEntries([]);
      setError(errorMessage(err, 'Failed to read the access log'));
    } finally {
      setLoading(false);
    }
  }, [proxy.id]);

  useEffect(() => {
    load();
  }, [load]);

  return (
    <Modal title={`Access log for ${proxy.domain || proxy.name || 'this proxy'}`} onClose={onClose} wide>
      <div className="space-y-3">
        <div className="flex items-center justify-between gap-3">
          <p className="text-sm text-gray-600">
            The last 100 requests the panel has recorded for this proxy.
          </p>
          <button type="button" onClick={load} className={BTN_SECONDARY}>
            <RefreshCw size={16} aria-hidden="true" />
            Refresh
          </button>
        </div>
        {loading ? (
          <PanelLoading label="Reading the access log..." />
        ) : error ? (
          <PanelError message={error} onRetry={load} />
        ) : entries.length === 0 ? (
          <p className="rounded-md border border-gray-200 bg-gray-50 px-4 py-6 text-center text-sm text-gray-600">
            No request has been recorded for this proxy. Nothing writes to this table yet: the
            proxy is not serving traffic, so there is nothing to log.
          </p>
        ) : (
          <div className="max-h-[50vh] overflow-auto rounded-md border border-gray-200">
            <table className="w-full border-collapse">
              <thead className="bg-gray-50">
                <tr className="border-b border-gray-200">
                  <TH>Time</TH>
                  <TH>Client</TH>
                  <TH>Request</TH>
                  <TH>Status</TH>
                </tr>
              </thead>
              <tbody>
                {entries.map((entry) => (
                  <tr key={entry.id} className="border-b border-gray-100">
                    <TD className="whitespace-nowrap text-xs" >
                      {entry.created_at ? new Date(entry.created_at).toLocaleString() : '—'}
                    </TD>
                    <TD className="font-mono text-xs">{entry.remote_addr || '—'}</TD>
                    <TD className="break-all font-mono text-xs">
                      {entry.method || ''} {entry.request_uri || ''}
                    </TD>
                    <TD className="text-xs">{entry.status ?? '—'}</TD>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </Modal>
  );
}
