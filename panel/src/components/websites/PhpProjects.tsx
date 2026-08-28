'use client';

/**
 * The PHP Project tab: sites served from a document root.
 *
 * Everything in the row comes from a route that exists:
 *
 *   domain, status, SSL flag   GET /api/v1/websites
 *   certificate expiry         GET /api/v1/ssl, matched on domain
 *   disk use                   GET /api/v1/files/disk-usage, on demand
 *   last deployment            GET /api/v1/git-deployments, matched on website_id
 *   WordPress badge            GET /api/v1/wordpress, matched on website_id or domain
 *
 * A row's secondary lists (certificates, deployments, WordPress) are optional:
 * they sit behind their own permissions, so a failure to read one leaves a
 * column unavailable with its reason rather than taking the whole tab down.
 */

import { useCallback, useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import {
  ExternalLink,
  FileText,
  FolderOpen,
  Globe,
  HardDrive,
  Loader2,
  Plus,
  RefreshCw,
  RotateCw,
  Settings,
  Trash2,
} from 'lucide-react';

import ServerScopeField, { SERVER_SCOPE_COPY_EN } from '@/components/servers/ServerScopeField';
import { MetricText, Unavailable } from '@/components/Unavailable';
import { errorMessage } from '@/lib/apiError';
import { nodeWebServers, serverLabel } from '@/lib/servers';
import { websiteApi } from '@/services/api';
import {
  WEB_SERVER_TYPES,
  type CreateWebsiteRequest,
  type ManagedServer,
  type ManagedWebsite,
  type SSLCertificate,
} from '@/types/server';
import type { GitDeployment, WordPressSite } from '@/types/website';

import { siteFilesApi } from './api';
import { certificateForDomain, deploymentForWebsite, type SiteContext } from './useSiteContext';
import PhpSiteSettings, { PHP_VERSIONS } from './PhpSiteSettings';
import { PHP_TAB_SITE_TYPES } from './projectTypes';
import { RemoveWebsiteDialog } from './RemoveDialogs';
import {
  BTN_PRIMARY,
  BTN_ROW,
  BTN_SECONDARY,
  Field,
  FormError,
  INPUT_CLASS,
  Modal,
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

const NO_LOGS_REASON =
  'Not available: the API has no per-site log endpoint. /api/v1/logs/search reads log sources registered on the Logs screen, not a site’s access and error logs.';
const NO_RESTART_REASON =
  'Not available: there is no per-site restart. Reloading PHP-FPM is a host-wide action on /api/v1/services, which is restricted to administrators.';
const NO_FILES_REASON =
  'Not available: the file endpoints /api/v1/files/* exist, but the panel has no file browser screen to open them in yet.';

function webServerOptions(server: ManagedServer | null | undefined): string[] {
  const reported = nodeWebServers(server);
  return reported.length > 0 ? reported : [...WEB_SERVER_TYPES];
}

interface PhpForm {
  domain: string;
  server_id: string;
  web_server_type: string;
  php_version: string;
  root_dir: string;
}

const EMPTY_FORM: PhpForm = {
  domain: '',
  server_id: '',
  web_server_type: '',
  php_version: '8.3',
  root_dir: '',
};

export interface PhpProjectsProps {
  servers: ManagedServer[];
  defaultServerId: string;
  ctx: SiteContext;
}

export default function PhpProjects({ servers, defaultServerId, ctx }: PhpProjectsProps) {
  const {
    websites,
    websitesLoading: loading,
    websitesError: error,
    certificates,
    certificatesError: certError,
    deployments,
    deploymentsError: deployError,
    wordpress,
    reload,
  } = ctx;

  /** The website rows that belong on this tab. See PHP_TAB_SITE_TYPES. */
  const sites = useMemo(
    () =>
      websites.filter((s) =>
        PHP_TAB_SITE_TYPES.includes(String(s.site_type || '').toLowerCase())
      ),
    [websites]
  );

  const [disk, setDisk] = useState<Record<string, string>>({});
  const [diskBusy, setDiskBusy] = useState<Record<string, boolean>>({});
  const [diskError, setDiskError] = useState<Record<string, string>>({});

  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState<PhpForm>(EMPTY_FORM);
  const [formError, setFormError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const [settingsFor, setSettingsFor] = useState<ManagedWebsite | null>(null);
  const [removing, setRemoving] = useState<ManagedWebsite | null>(null);

  useEffect(() => {
    if (!defaultServerId) return;
    setForm((prev) => {
      if (prev.server_id && servers.some((s) => s.id === prev.server_id)) return prev;
      return { ...prev, server_id: defaultServerId };
    });
  }, [defaultServerId, servers]);

  const selectedServer = useMemo(
    () => servers.find((s) => s.id === form.server_id) || null,
    [servers, form.server_id]
  );

  useEffect(() => {
    const options = webServerOptions(selectedServer);
    setForm((prev) => {
      if (prev.web_server_type && options.includes(prev.web_server_type)) return prev;
      return { ...prev, web_server_type: options[0] || 'nginx' };
    });
  }, [selectedServer]);

  const certFor = useCallback(
    (site: ManagedWebsite): SSLCertificate | null =>
      certificateForDomain(certificates, site.domain, site.id),
    [certificates]
  );

  const deploymentFor = useCallback(
    (site: ManagedWebsite): GitDeployment | null => deploymentForWebsite(deployments, site.id),
    [deployments]
  );

  const wordpressFor = useCallback(
    (site: ManagedWebsite): WordPressSite | null =>
      wordpress.find(
        (w) =>
          (w.website_id && w.website_id === site.id) ||
          (w.domain && site.domain && w.domain.toLowerCase() === site.domain.toLowerCase())
      ) || null,
    [wordpress]
  );

  /**
   * Disk use is measured on request, not on load. The backend answers with
   * `du -sh` over the whole document root, and firing that for every site every
   * time the tab opens would make the panel the slowest thing on the machine.
   */
  const measure = async (site: ManagedWebsite) => {
    const path = site.root_dir;
    if (!path) return;
    setDiskBusy((prev) => ({ ...prev, [site.id]: true }));
    setDiskError((prev) => ({ ...prev, [site.id]: '' }));
    try {
      const usage = await siteFilesApi.diskUsage(path);
      setDisk((prev) => ({ ...prev, [site.id]: usage?.size || '' }));
    } catch (err) {
      setDiskError((prev) => ({
        ...prev,
        [site.id]: errorMessage(err, 'The size could not be measured'),
      }));
    } finally {
      setDiskBusy((prev) => ({ ...prev, [site.id]: false }));
    }
  };

  const openForm = () => {
    setFormError('');
    const serverId = form.server_id || defaultServerId;
    const node = servers.find((s) => s.id === serverId) || null;
    setForm({
      ...EMPTY_FORM,
      server_id: serverId,
      web_server_type: webServerOptions(node)[0] || 'nginx',
    });
    setShowForm(true);
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setFormError('');
    const domain = form.domain.trim().toLowerCase();
    if (!domain) {
      setFormError('A domain is required.');
      return;
    }
    if (!form.server_id) {
      setFormError(
        'No server is registered yet, so there is nowhere to put this site. Refresh the server list and try again.'
      );
      return;
    }
    const payload: CreateWebsiteRequest = {
      domain,
      server_id: form.server_id,
      web_server_type: form.web_server_type || 'nginx',
      site_type: 'php',
    };
    if (form.php_version) payload.php_version = form.php_version;
    if (form.root_dir.trim()) payload.root_dir = form.root_dir.trim();

    setSubmitting(true);
    try {
      await websiteApi.create(payload);
      setShowForm(false);
      setForm(EMPTY_FORM);
      await reload();
    } catch (err) {
      setFormError(errorMessage(err, 'Failed to create the site'));
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
      <Surface>
        <SurfaceHeader
          title="PHP projects"
          description="Sites the web server serves from a document root."
          actions={
            <>
              <button
                type="button"
                onClick={reload}
                className={BTN_SECONDARY}
              >
                <RefreshCw size={16} aria-hidden="true" />
                Refresh
              </button>
              <button type="button" onClick={openForm} className={BTN_PRIMARY}>
                <Plus size={16} aria-hidden="true" />
                Add PHP site
              </button>
            </>
          }
        />

        {loading ? (
          <PanelLoading label="Loading PHP sites..." />
        ) : error ? (
          <PanelError message={error} onRetry={reload} />
        ) : sites.length === 0 ? (
          <PanelEmpty
            icon={<Globe size={40} aria-hidden="true" />}
            title="No PHP site yet"
            body={
              servers.length > 0 ? (
                <>
                  Add a domain and the panel writes its document root and its web server
                  configuration, then reloads the web server. A certificate and a database can
                  follow once the site answers.
                </>
              ) : (
                <>
                  No server is registered yet, so there is nowhere to put a site. The machine
                  running this panel registers itself; check the Servers screen.
                </>
              )
            }
            action={
              <>
                <button
                  type="button"
                  onClick={openForm}
                  disabled={servers.length === 0}
                  className={BTN_PRIMARY}
                >
                  <Plus size={16} aria-hidden="true" />
                  Add PHP site
                </button>
                <Link href="/servers" className={BTN_SECONDARY}>
                  View servers
                </Link>
              </>
            }
          />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[1100px] border-collapse">
              <thead className="bg-gray-50">
                <tr className="border-b border-gray-200">
                  <TH>Domain</TH>
                  {showServerColumn && <TH>Server</TH>}
                  <TH>Type</TH>
                  <TH>Status</TH>
                  <TH>SSL</TH>
                  <TH>Disk use</TH>
                  <TH>Last deployment</TH>
                  <TH>Actions</TH>
                </tr>
              </thead>
              <tbody>
                {sites.map((site) => {
                  const cert = certFor(site);
                  const deployment = deploymentFor(site);
                  const wp = wordpressFor(site);
                  const expiry = formatDate(cert?.not_after);
                  const url = `${site.ssl_enabled ? 'https' : 'http'}://${site.domain || ''}`;
                  return (
                    <tr key={site.id} className="border-b border-gray-100 hover:bg-gray-50">
                      <TD className="font-medium text-gray-900">
                        <div className="flex flex-col gap-1">
                          <MetricText
                            value={site.domain || null}
                            reason="Not available: this site has no domain recorded."
                          />
                          <span className="break-all font-mono text-xs text-gray-500">
                            {site.root_dir || 'no document root recorded'}
                          </span>
                        </div>
                      </TD>
                      {showServerColumn && (
                        <TD>
                          <MetricText
                            value={serverName(site.server_id)}
                            reason="Not available: this site points at a server that is no longer in the list."
                          />
                        </TD>
                      )}
                      <TD>
                        <div className="flex flex-wrap items-center gap-1.5">
                          <Tag>{site.php_version ? `PHP ${site.php_version}` : 'No PHP runtime'}</Tag>
                          {wp && <Tag tone="sky">WordPress</Tag>}
                        </div>
                      </TD>
                      <TD>
                        <StatusBadge status={site.status} />
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
                        ) : site.ssl_enabled ? (
                          <div className="flex flex-col gap-0.5">
                            <span className="inline-flex w-fit items-center rounded-md bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700">
                              enabled
                            </span>
                            <span className="text-xs text-gray-500">
                              {certError ? 'expiry unavailable' : 'no certificate row'}
                            </span>
                          </div>
                        ) : (
                          <span className="inline-flex items-center rounded-md bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600">
                            off
                          </span>
                        )}
                      </TD>
                      <TD>
                        {disk[site.id] ? (
                          <span className="font-mono text-xs">{disk[site.id]}</span>
                        ) : diskError[site.id] ? (
                          <Unavailable reason={diskError[site.id]} />
                        ) : !site.root_dir ? (
                          <Unavailable reason="Not available: this site has no document root to measure." />
                        ) : (
                          <button
                            type="button"
                            onClick={() => measure(site)}
                            disabled={diskBusy[site.id]}
                            className={BTN_ROW}
                            title="Runs du -sh over the document root on the host. It is not measured automatically because it walks every file."
                          >
                            {diskBusy[site.id] ? (
                              <Loader2 size={12} className="animate-spin" aria-hidden="true" />
                            ) : (
                              <HardDrive size={12} aria-hidden="true" />
                            )}
                            {diskBusy[site.id] ? 'Measuring' : 'Measure'}
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
                              deployError
                                ? `Not available: ${deployError}`
                                : 'Not available: no git deployment is attached to this site.'
                            }
                          />
                        )}
                      </TD>
                      <TD>
                        <div className="flex flex-wrap items-center gap-1.5">
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
                          <button type="button" className={BTN_ROW} disabled title={NO_FILES_REASON}>
                            <FolderOpen size={12} aria-hidden="true" />
                            Files
                          </button>
                          <button type="button" className={BTN_ROW} disabled title={NO_LOGS_REASON}>
                            <FileText size={12} aria-hidden="true" />
                            Logs
                          </button>
                          <button type="button" className={BTN_ROW} disabled title={NO_RESTART_REASON}>
                            <RotateCw size={12} aria-hidden="true" />
                            Restart
                          </button>
                          <button
                            type="button"
                            className={BTN_ROW}
                            onClick={() => setSettingsFor(site)}
                          >
                            <Settings size={12} aria-hidden="true" />
                            Settings
                          </button>
                          <button
                            type="button"
                            className={`${BTN_ROW} text-red-700 hover:bg-red-50`}
                            onClick={() => setRemoving(site)}
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

      {showForm && (
        <Modal title="Add PHP site" onClose={() => setShowForm(false)}>
          <form onSubmit={submit} className="space-y-4">
            <FormError message={formError} />

            <Field id="php-form-domain" label="Domain" required>
              <input
                id="php-form-domain"
                type="text"
                value={form.domain}
                onChange={(e) => setForm((prev) => ({ ...prev, domain: e.target.value }))}
                className={INPUT_CLASS}
                placeholder="example.vn"
                required
              />
            </Field>

            <ServerScopeField
              id="php-form-server"
              servers={servers}
              value={form.server_id}
              onChange={(id) => setForm((prev) => ({ ...prev, server_id: id }))}
              copy={SERVER_SCOPE_COPY_EN}
            />

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field
                id="php-form-webserver"
                label="Web server"
                required
                hint={
                  nodeWebServers(selectedServer).length === 0
                    ? 'This node has not reported which web servers it runs, so every supported one is offered.'
                    : undefined
                }
              >
                <select
                  id="php-form-webserver"
                  value={form.web_server_type}
                  onChange={(e) =>
                    setForm((prev) => ({ ...prev, web_server_type: e.target.value }))
                  }
                  className={INPUT_CLASS}
                >
                  {webServerOptions(selectedServer).map((type) => (
                    <option key={type} value={type}>
                      {type}
                    </option>
                  ))}
                </select>
              </Field>

              <Field id="php-form-version" label="PHP version">
                <select
                  id="php-form-version"
                  value={form.php_version}
                  onChange={(e) => setForm((prev) => ({ ...prev, php_version: e.target.value }))}
                  className={INPUT_CLASS}
                >
                  {PHP_VERSIONS.map((version) => (
                    <option key={version || 'none'} value={version}>
                      {version || 'None'}
                    </option>
                  ))}
                </select>
              </Field>
            </div>

            <Field
              id="php-form-root"
              label="Document root"
              hint="Left empty, the panel derives it from the domain."
            >
              <input
                id="php-form-root"
                type="text"
                value={form.root_dir}
                onChange={(e) => setForm((prev) => ({ ...prev, root_dir: e.target.value }))}
                className={INPUT_CLASS}
                placeholder="/www/wwwroot/example.vn"
              />
            </Field>

            <div className="flex justify-end gap-3 border-t border-gray-200 pt-4">
              <button type="button" onClick={() => setShowForm(false)} className={BTN_SECONDARY}>
                Cancel
              </button>
              <button
                type="submit"
                disabled={submitting || !form.server_id}
                className={BTN_PRIMARY}
              >
                {submitting ? (
                  <Loader2 size={16} className="animate-spin" aria-hidden="true" />
                ) : (
                  <Plus size={16} aria-hidden="true" />
                )}
                {submitting ? 'Creating...' : 'Create site'}
              </button>
            </div>
          </form>
        </Modal>
      )}

      {settingsFor && (
        <PhpSiteSettings
          site={settingsFor}
          certificate={certFor(settingsFor)}
          wordpress={wordpressFor(settingsFor)}
          onClose={() => setSettingsFor(null)}
          onSaved={reload}
        />
      )}

      {removing && (
        <RemoveWebsiteDialog
          site={removing}
          onClose={() => setRemoving(null)}
          onRemoved={reload}
        />
      )}
    </>
  );
}
