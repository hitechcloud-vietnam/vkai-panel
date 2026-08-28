'use client';

/**
 * The Node.js Project tab: long-running processes, not document roots.
 *
 * Every control here is behind a mounted route:
 *
 *   list / create / delete   /api/v1/node-apps
 *   start / stop / restart   /api/v1/node-apps/:id/{start,stop,restart}
 *   status                   /api/v1/node-apps/:id/status   (systemd is-active)
 *   logs                     /api/v1/node-apps/:id/logs     (journalctl)
 *   environment variables    /api/v1/node-apps/:id/environments
 *
 * The fields the form asks for are exactly the ones models.CreateNodeAppRequest
 * accepts. Two fields an operator would expect - the entry file and the package
 * manager - are NOT asked for, because the backend has nowhere to put them; the
 * note above the form says so rather than collecting them into nothing.
 */

import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  ExternalLink,
  FileText,
  FolderOpen,
  HardDrive,
  Loader2,
  Play,
  Plus,
  RefreshCw,
  RotateCw,
  Server,
  Square,
  Trash2,
  Variable,
} from 'lucide-react';

import ServerScopeField, { SERVER_SCOPE_COPY_EN } from '@/components/servers/ServerScopeField';
import { MetricText, Unavailable } from '@/components/Unavailable';
import { errorMessage } from '@/lib/apiError';
import { serverLabel } from '@/lib/servers';
import type { ManagedServer, ManagedWebsite } from '@/types/server';
import type { CreateNodeAppRequest, NodeApp, NodeAppEnvironment } from '@/types/website';

import { nodeAppApi, siteFilesApi } from './api';
import { PROJECT_TYPE_GAPS } from './projectTypes';
import { RemoveNodeAppDialog } from './RemoveDialogs';
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
import { certificateForDomain, deploymentForWebsite, type SiteContext } from './useSiteContext';

const NO_FILES_REASON =
  'Not available: the file endpoints /api/v1/files/* exist, but the panel has no file browser screen to open them in yet.';

interface NodeForm {
  name: string;
  server_id: string;
  website_id: string;
  path: string;
  port: string;
  node_version: string;
  start_script: string;
  stop_script: string;
  restart_script: string;
  auto_restart: boolean;
  max_restarts: string;
  description: string;
}

const EMPTY_FORM: NodeForm = {
  name: '',
  server_id: '',
  website_id: '',
  path: '',
  port: '',
  node_version: '',
  start_script: 'npm start',
  stop_script: '',
  restart_script: '',
  auto_restart: true,
  max_restarts: '5',
  description: '',
};

export interface NodeProjectsProps {
  servers: ManagedServer[];
  defaultServerId: string;
  ctx: SiteContext;
  onCount: (count: number | null) => void;
}

export default function NodeProjects({
  servers,
  defaultServerId,
  ctx,
  onCount,
}: NodeProjectsProps) {
  const [apps, setApps] = useState<NodeApp[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const [busy, setBusy] = useState<Record<string, string>>({});
  const [rowError, setRowError] = useState<Record<string, string>>({});
  const [disk, setDisk] = useState<Record<string, string>>({});
  const [diskBusy, setDiskBusy] = useState<Record<string, boolean>>({});
  const [diskError, setDiskError] = useState<Record<string, string>>({});

  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState<NodeForm>(EMPTY_FORM);
  const [formError, setFormError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const [logsFor, setLogsFor] = useState<NodeApp | null>(null);
  const [envFor, setEnvFor] = useState<NodeApp | null>(null);
  const [removing, setRemoving] = useState<NodeApp | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const list = await nodeAppApi.list();
      setApps(list);
      onCount(list.length);
    } catch (err) {
      setApps([]);
      onCount(null);
      setError(errorMessage(err, 'Failed to load Node.js projects'));
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

  const runAction = async (app: NodeApp, action: 'start' | 'stop' | 'restart') => {
    setBusy((prev) => ({ ...prev, [app.id]: action }));
    setRowError((prev) => ({ ...prev, [app.id]: '' }));
    try {
      if (action === 'start') await nodeAppApi.start(app.id);
      if (action === 'stop') await nodeAppApi.stop(app.id);
      if (action === 'restart') await nodeAppApi.restart(app.id);
      await load();
    } catch (err) {
      setRowError((prev) => ({
        ...prev,
        [app.id]: errorMessage(err, `Failed to ${action} this project`),
      }));
    } finally {
      setBusy((prev) => ({ ...prev, [app.id]: '' }));
    }
  };

  const measure = async (app: NodeApp) => {
    if (!app.path) return;
    setDiskBusy((prev) => ({ ...prev, [app.id]: true }));
    setDiskError((prev) => ({ ...prev, [app.id]: '' }));
    try {
      const usage = await siteFilesApi.diskUsage(app.path);
      setDisk((prev) => ({ ...prev, [app.id]: usage?.size || '' }));
    } catch (err) {
      setDiskError((prev) => ({
        ...prev,
        [app.id]: errorMessage(err, 'The size could not be measured'),
      }));
    } finally {
      setDiskBusy((prev) => ({ ...prev, [app.id]: false }));
    }
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setFormError('');
    const name = form.name.trim();
    const path = form.path.trim();
    const port = Number(form.port);
    if (!name) {
      setFormError('A project name is required.');
      return;
    }
    if (!path.startsWith('/')) {
      setFormError('The working directory must be an absolute path.');
      return;
    }
    if (!Number.isInteger(port) || port < 1 || port > 65535) {
      setFormError('A port between 1 and 65535 is required.');
      return;
    }
    if (!form.server_id) {
      setFormError('No server is registered yet, so there is nowhere to run this project.');
      return;
    }

    const payload: CreateNodeAppRequest = {
      server_id: form.server_id,
      name,
      path,
      port,
      auto_restart: form.auto_restart,
    };
    if (form.website_id) payload.website_id = form.website_id;
    if (form.description.trim()) payload.description = form.description.trim();
    if (form.node_version.trim()) payload.node_version = form.node_version.trim();
    if (form.start_script.trim()) payload.start_script = form.start_script.trim();
    if (form.stop_script.trim()) payload.stop_script = form.stop_script.trim();
    if (form.restart_script.trim()) payload.restart_script = form.restart_script.trim();
    const maxRestarts = Number(form.max_restarts);
    if (Number.isInteger(maxRestarts) && maxRestarts >= 0) payload.max_restarts = maxRestarts;

    setSubmitting(true);
    try {
      await nodeAppApi.create(payload);
      setShowForm(false);
      setForm({ ...EMPTY_FORM, server_id: form.server_id });
      await load();
    } catch (err) {
      setFormError(errorMessage(err, 'Failed to create the Node.js project'));
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
        <Notice tone="sky" title="What this backend stores for a Node.js project">
          {PROJECT_TYPE_GAPS.nodejs.map((line) => (
            <p key={line}>{line}</p>
          ))}
        </Notice>

        <Surface>
          <SurfaceHeader
            title="Node.js projects"
            description="Processes supervised by systemd on the host, each holding its own port."
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
                  Add Node.js project
                </button>
              </>
            }
          />

          {loading ? (
            <PanelLoading label="Loading Node.js projects..." />
          ) : error ? (
            <PanelError message={error} onRetry={load} />
          ) : apps.length === 0 ? (
            <PanelEmpty
              icon={<Server size={40} aria-hidden="true" />}
              title="No Node.js project yet"
              body={
                <>
                  Point the panel at a directory containing an application and the command that
                  starts it. The panel writes a systemd unit, starts it, and reads its journal
                  back into this screen.
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
                  Add Node.js project
                </button>
              }
            />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full min-w-[1200px] border-collapse">
                <thead className="bg-gray-50">
                  <tr className="border-b border-gray-200">
                    <TH>Project</TH>
                    {showServerColumn && <TH>Server</TH>}
                    <TH>Runtime</TH>
                    <TH>Status</TH>
                    <TH>SSL</TH>
                    <TH>Disk use</TH>
                    <TH>Last deployment</TH>
                    <TH>Actions</TH>
                  </tr>
                </thead>
                <tbody>
                  {apps.map((app) => {
                    const site = app.website_id ? websiteById.get(app.website_id) || null : null;
                    const cert = certificateForDomain(
                      ctx.certificates,
                      site?.domain,
                      site?.id
                    );
                    const deployment = deploymentForWebsite(ctx.deployments, app.website_id);
                    const expiry = formatDate(cert?.not_after);
                    const url = site?.domain
                      ? `${site.ssl_enabled ? 'https' : 'http'}://${site.domain}`
                      : app.port
                        ? `http://localhost:${app.port}`
                        : '';
                    const running = String(app.status || '').toLowerCase() === 'running';
                    return (
                      <tr key={app.id} className="border-b border-gray-100 align-top hover:bg-gray-50">
                        <TD className="font-medium text-gray-900">
                          <div className="flex flex-col gap-1">
                            <span>{app.name || 'unnamed project'}</span>
                            <span className="break-all font-mono text-xs text-gray-500">
                              {site?.domain ? `${site.domain} — ` : ''}
                              {app.path || 'no working directory recorded'}
                            </span>
                            {rowError[app.id] && (
                              <span className="text-xs text-red-700">{rowError[app.id]}</span>
                            )}
                          </div>
                        </TD>
                        {showServerColumn && (
                          <TD>
                            <MetricText
                              value={serverName(app.server_id)}
                              reason="Not available: this project points at a server that is no longer in the list."
                            />
                          </TD>
                        )}
                        <TD>
                          <div className="flex flex-wrap items-center gap-1.5">
                            <Tag>{app.node_version ? `Node ${app.node_version}` : 'Node (host default)'}</Tag>
                            <Tag tone="sky">port {app.port ?? '—'}</Tag>
                            <Tag>{app.auto_restart ? `restart, max ${app.max_restarts ?? 0}` : 'no auto restart'}</Tag>
                          </div>
                          <p className="mt-1 break-all font-mono text-xs text-gray-500">
                            {app.start_script || 'no start command recorded'}
                          </p>
                        </TD>
                        <TD>
                          <StatusBadge status={app.status} />
                        </TD>
                        <TD>
                          {!site ? (
                            <Unavailable reason="Not available: this project is not attached to a website, so it has no hostname and no certificate of its own." />
                          ) : cert ? (
                            <div className="flex flex-col gap-0.5">
                              <span className="inline-flex w-fit items-center rounded-md bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700">
                                {cert.source || 'installed'}
                              </span>
                              <span className="text-xs text-gray-500">
                                {expiry ? `expires ${expiry}` : 'no expiry recorded'}
                              </span>
                            </div>
                          ) : (
                            <span className="inline-flex items-center rounded-md bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600">
                              off
                            </span>
                          )}
                        </TD>
                        <TD>
                          {disk[app.id] ? (
                            <span className="font-mono text-xs">{disk[app.id]}</span>
                          ) : diskError[app.id] ? (
                            <Unavailable reason={diskError[app.id]} />
                          ) : !app.path ? (
                            <Unavailable reason="Not available: this project has no working directory to measure." />
                          ) : (
                            <button
                              type="button"
                              onClick={() => measure(app)}
                              disabled={diskBusy[app.id]}
                              className={BTN_ROW}
                              title="Runs du -sh over the working directory on the host."
                            >
                              {diskBusy[app.id] ? (
                                <Loader2 size={12} className="animate-spin" aria-hidden="true" />
                              ) : (
                                <HardDrive size={12} aria-hidden="true" />
                              )}
                              {diskBusy[app.id] ? 'Measuring' : 'Measure'}
                            </button>
                          )}
                        </TD>
                        <TD>
                          {deployment ? (
                            <div className="flex flex-col gap-0.5">
                              <MetricText
                                value={formatDate(deployment.last_deploy_at)}
                                reason="Not available: this deployment has never run."
                                className="text-xs"
                              />
                              <span className="font-mono text-xs text-gray-500">
                                {deployment.branch || 'no branch'}
                                {deployment.last_commit_hash
                                  ? ` @ ${deployment.last_commit_hash.slice(0, 7)}`
                                  : ''}
                              </span>
                            </div>
                          ) : (
                            <Unavailable
                              reason={
                                ctx.deploymentsError
                                  ? `Not available: ${ctx.deploymentsError}`
                                  : 'Not available: no git deployment is attached to this project.'
                              }
                            />
                          )}
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
                                title="Not available: this project has neither a website nor a port recorded, so there is no address to open."
                              >
                                <ExternalLink size={12} aria-hidden="true" />
                                Open
                              </button>
                            )}
                            <button
                              type="button"
                              className={BTN_ROW}
                              disabled
                              title={NO_FILES_REASON}
                            >
                              <FolderOpen size={12} aria-hidden="true" />
                              Files
                            </button>
                            <button
                              type="button"
                              className={BTN_ROW}
                              onClick={() => setLogsFor(app)}
                            >
                              <FileText size={12} aria-hidden="true" />
                              Logs
                            </button>
                            <button
                              type="button"
                              className={BTN_ROW}
                              disabled={Boolean(busy[app.id])}
                              onClick={() => runAction(app, 'restart')}
                            >
                              {busy[app.id] === 'restart' ? (
                                <Loader2 size={12} className="animate-spin" aria-hidden="true" />
                              ) : (
                                <RotateCw size={12} aria-hidden="true" />
                              )}
                              Restart
                            </button>
                            <button
                              type="button"
                              className={BTN_ROW}
                              disabled={Boolean(busy[app.id])}
                              onClick={() => runAction(app, running ? 'stop' : 'start')}
                            >
                              {busy[app.id] === 'start' || busy[app.id] === 'stop' ? (
                                <Loader2 size={12} className="animate-spin" aria-hidden="true" />
                              ) : running ? (
                                <Square size={12} aria-hidden="true" />
                              ) : (
                                <Play size={12} aria-hidden="true" />
                              )}
                              {running ? 'Stop' : 'Start'}
                            </button>
                            <button
                              type="button"
                              className={BTN_ROW}
                              onClick={() => setEnvFor(app)}
                            >
                              <Variable size={12} aria-hidden="true" />
                              Env
                            </button>
                            <button
                              type="button"
                              className={`${BTN_ROW} text-red-700 hover:bg-red-50`}
                              onClick={() => setRemoving(app)}
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
        <Modal title="Add Node.js project" onClose={() => setShowForm(false)} wide>
          <form onSubmit={submit} className="space-y-4">
            <FormError message={formError} />

            <Notice tone="sky" title="Only the fields the backend stores are asked for">
              <p>
                There is no entry file and no package manager here because models.NodeApp has
                neither. The start command is the whole answer to &ldquo;what runs&rdquo;, and the
                supervisor is always systemd.
              </p>
            </Notice>

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field id="node-name" label="Project name" required>
                <input
                  id="node-name"
                  type="text"
                  value={form.name}
                  onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))}
                  className={INPUT_CLASS}
                  placeholder="checkout-api"
                  required
                />
              </Field>
              <Field
                id="node-port"
                label="Port"
                required
                hint="The port the process listens on. It is written into the unit's environment."
              >
                <input
                  id="node-port"
                  type="number"
                  min={1}
                  max={65535}
                  value={form.port}
                  onChange={(e) => setForm((p) => ({ ...p, port: e.target.value }))}
                  className={INPUT_CLASS}
                  placeholder="3000"
                  required
                />
              </Field>
            </div>

            <ServerScopeField
              id="node-server"
              servers={servers}
              value={form.server_id}
              onChange={(id) => setForm((p) => ({ ...p, server_id: id }))}
              copy={SERVER_SCOPE_COPY_EN}
            />

            <Field
              id="node-path"
              label="Working directory"
              required
              hint="Absolute path the service runs in. The start command is executed here."
            >
              <input
                id="node-path"
                type="text"
                value={form.path}
                onChange={(e) => setForm((p) => ({ ...p, path: e.target.value }))}
                className={INPUT_CLASS}
                placeholder="/www/wwwroot/checkout-api"
                required
              />
            </Field>

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field
                id="node-start"
                label="Start command"
                hint="Whatever starts the app: npm start, pnpm run serve, node dist/server.js."
              >
                <input
                  id="node-start"
                  type="text"
                  value={form.start_script}
                  onChange={(e) => setForm((p) => ({ ...p, start_script: e.target.value }))}
                  className={INPUT_CLASS}
                  placeholder="npm start"
                />
              </Field>
              <Field
                id="node-version"
                label="Node version"
                hint="Left empty, the unit uses whichever Node is on the host's PATH."
              >
                <input
                  id="node-version"
                  type="text"
                  value={form.node_version}
                  onChange={(e) => setForm((p) => ({ ...p, node_version: e.target.value }))}
                  className={INPUT_CLASS}
                  placeholder="20.11.0"
                />
              </Field>
            </div>

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field
                id="node-stop"
                label="Stop command (optional)"
                hint="Left empty, systemd stops the process itself."
              >
                <input
                  id="node-stop"
                  type="text"
                  value={form.stop_script}
                  onChange={(e) => setForm((p) => ({ ...p, stop_script: e.target.value }))}
                  className={INPUT_CLASS}
                />
              </Field>
              <Field id="node-restart" label="Restart command (optional)">
                <input
                  id="node-restart"
                  type="text"
                  value={form.restart_script}
                  onChange={(e) => setForm((p) => ({ ...p, restart_script: e.target.value }))}
                  className={INPUT_CLASS}
                />
              </Field>
            </div>

            <fieldset className="rounded-md border border-gray-200 p-4">
              <legend className="px-1 text-sm font-medium text-gray-700">Restart policy</legend>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <label className="flex items-start gap-3 text-sm text-gray-700">
                  <input
                    type="checkbox"
                    checked={form.auto_restart}
                    onChange={(e) => setForm((p) => ({ ...p, auto_restart: e.target.checked }))}
                    className="mt-0.5 h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500"
                  />
                  <span>
                    Restart the service when it exits
                    <span className="mt-0.5 block text-xs text-gray-500">
                      Written into the systemd unit as its restart policy.
                    </span>
                  </span>
                </label>
                <Field id="node-max-restarts" label="Maximum restarts">
                  <input
                    id="node-max-restarts"
                    type="number"
                    min={0}
                    value={form.max_restarts}
                    onChange={(e) => setForm((p) => ({ ...p, max_restarts: e.target.value }))}
                    className={INPUT_CLASS}
                    disabled={!form.auto_restart}
                  />
                </Field>
              </div>
            </fieldset>

            <Field
              id="node-website"
              label="Attached website (optional)"
              hint="Attaching a website is what gives this project a hostname, a certificate and a deployment in the list above."
            >
              <select
                id="node-website"
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
                {submitting ? 'Creating...' : 'Create project'}
              </button>
            </div>
          </form>
        </Modal>
      )}

      {logsFor && <NodeLogsDialog app={logsFor} onClose={() => setLogsFor(null)} />}
      {envFor && <NodeEnvDialog app={envFor} onClose={() => setEnvFor(null)} />}
      {removing && (
        <RemoveNodeAppDialog
          app={removing}
          onClose={() => setRemoving(null)}
          onRemoved={load}
        />
      )}
    </>
  );
}

/** The journal of one project, from GET /api/v1/node-apps/:id/logs. */
function NodeLogsDialog({ app, onClose }: { app: NodeApp; onClose: () => void }) {
  const [lines, setLines] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      setLines(await nodeAppApi.logs(app.id, 200));
    } catch (err) {
      setLines([]);
      setError(errorMessage(err, 'Failed to read the project log'));
    } finally {
      setLoading(false);
    }
  }, [app.id]);

  useEffect(() => {
    load();
  }, [load]);

  return (
    <Modal title={`Log for ${app.name || 'this project'}`} onClose={onClose} wide>
      <div className="space-y-3">
        <div className="flex items-center justify-between gap-3">
          <p className="text-sm text-gray-600">The last 200 lines the host&apos;s journal holds for this unit.</p>
          <button type="button" onClick={load} className={BTN_SECONDARY}>
            <RefreshCw size={16} aria-hidden="true" />
            Refresh
          </button>
        </div>
        {loading ? (
          <PanelLoading label="Reading the journal..." />
        ) : error ? (
          <PanelError message={error} onRetry={load} />
        ) : lines.length === 0 ? (
          <p className="rounded-md border border-gray-200 bg-gray-50 px-4 py-6 text-center text-sm text-gray-600">
            The journal returned nothing for this unit. That is what an application which has never
            started looks like, as well as one that logs nowhere.
          </p>
        ) : (
          <pre className="max-h-[50vh] overflow-auto rounded-md border border-gray-200 bg-gray-50 p-3 font-mono text-xs text-gray-800">
            {lines.join('\n')}
          </pre>
        )}
      </div>
    </Modal>
  );
}

/** Environment variables of one project. */
function NodeEnvDialog({ app, onClose }: { app: NodeApp; onClose: () => void }) {
  const [vars, setVars] = useState<NodeAppEnvironment[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [key, setKey] = useState('');
  const [value, setValue] = useState('');
  const [secret, setSecret] = useState(false);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      setVars(await nodeAppApi.listEnv(app.id));
    } catch (err) {
      setVars([]);
      setError(errorMessage(err, 'Failed to read the environment variables'));
    } finally {
      setLoading(false);
    }
  }, [app.id]);

  useEffect(() => {
    load();
  }, [load]);

  const add = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    if (!key.trim() || !value.trim()) {
      setError('A name and a value are both required.');
      return;
    }
    setBusy(true);
    try {
      await nodeAppApi.addEnv(app.id, { key: key.trim(), value, is_secret: secret });
      setKey('');
      setValue('');
      setSecret(false);
      await load();
    } catch (err) {
      setError(errorMessage(err, 'Failed to add the variable'));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (envId: string) => {
    setBusy(true);
    try {
      await nodeAppApi.removeEnv(app.id, envId);
      await load();
    } catch (err) {
      setError(errorMessage(err, 'Failed to remove the variable'));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal title={`Environment for ${app.name || 'this project'}`} onClose={onClose} wide>
      <div className="space-y-4">
        <Notice tone="sky" title="Read into the unit when the service is installed">
          <p>
            Variables are written into the systemd unit the first time the project starts. Changing
            one takes effect on the next restart, not immediately.
          </p>
        </Notice>

        <FormError message={error} />

        {loading ? (
          <PanelLoading label="Reading environment variables..." />
        ) : (
          <div className="overflow-x-auto rounded-md border border-gray-200">
            <table className="w-full border-collapse">
              <thead className="bg-gray-50">
                <tr className="border-b border-gray-200">
                  <TH>Name</TH>
                  <TH>Value</TH>
                  <TH>Actions</TH>
                </tr>
              </thead>
              <tbody>
                {vars.length === 0 ? (
                  <tr>
                    <td colSpan={3} className="px-4 py-6 text-center text-sm text-gray-600">
                      No variable is set for this project.
                    </td>
                  </tr>
                ) : (
                  vars.map((v) => (
                    <tr key={v.id} className="border-b border-gray-100">
                      <TD className="font-mono text-xs">{v.key}</TD>
                      <TD className="font-mono text-xs">
                        {v.is_secret ? <Tag>secret, not shown</Tag> : v.value}
                      </TD>
                      <TD>
                        <button
                          type="button"
                          className={`${BTN_ROW} text-red-700 hover:bg-red-50`}
                          disabled={busy}
                          onClick={() => remove(v.id)}
                        >
                          <Trash2 size={12} aria-hidden="true" />
                          Remove
                        </button>
                      </TD>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        )}

        <form onSubmit={add} className="grid grid-cols-1 items-end gap-3 sm:grid-cols-[1fr_1fr_auto]">
          <Field id="env-key" label="Name">
            <input
              id="env-key"
              type="text"
              value={key}
              onChange={(e) => setKey(e.target.value)}
              className={INPUT_CLASS}
              placeholder="DATABASE_URL"
            />
          </Field>
          <Field id="env-value" label="Value">
            <input
              id="env-value"
              type="text"
              value={value}
              onChange={(e) => setValue(e.target.value)}
              className={INPUT_CLASS}
            />
          </Field>
          <button type="submit" className={BTN_PRIMARY} disabled={busy}>
            <Plus size={16} aria-hidden="true" />
            Add
          </button>
          <label className="flex items-center gap-2 text-sm text-gray-700 sm:col-span-3">
            <input
              type="checkbox"
              checked={secret}
              onChange={(e) => setSecret(e.target.checked)}
              className="h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500"
            />
            Hold this value as a secret and do not show it back
          </label>
        </form>
      </div>
    </Modal>
  );
}
