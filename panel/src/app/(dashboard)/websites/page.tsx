'use client';

import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { Check, Globe, Loader2, Minus, Plus, RefreshCw, X } from 'lucide-react';

import ServerScopeField, {
  SERVER_SCOPE_COPY_EN,
} from '@/components/servers/ServerScopeField';
import { MetricText } from '@/components/Unavailable';
import { useServers } from '@/hooks/useServers';
import { websiteApi, unwrapList } from '@/services/api';
import { nodeWebServers, serverLabel } from '@/lib/servers';
import {
  WEB_SERVER_TYPES,
  type CreateWebsiteRequest,
  type ManagedServer,
  type ManagedWebsite,
} from '@/types/server';
import { errorMessage } from '@/lib/apiError';

/** PHP runtimes offered for a new site. An empty value means the site needs none. */
const PHP_VERSIONS = ['', '8.3', '8.2', '8.1', '8.0', '7.4'];

const SITE_TYPES = [
  { value: 'php', label: 'PHP' },
  { value: 'static', label: 'Static' },
  { value: 'proxy', label: 'Reverse proxy' },
];

const INPUT_CLASS =
  'w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder:text-gray-400 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500';
const LABEL_CLASS = 'mb-1.5 block text-sm font-medium text-gray-700';

const NO_DATE_REASON = 'Not available: the API did not report a creation date for this site.';

interface WebsiteForm {
  domain: string;
  server_id: string;
  web_server_type: string;
  php_version: string;
  site_type: string;
  root_dir: string;
}

const EMPTY_FORM: WebsiteForm = {
  domain: '',
  server_id: '',
  web_server_type: '',
  php_version: '8.3',
  site_type: 'php',
  root_dir: '',
};

/**
 * The web servers a new site can use on the chosen node: what the node reports,
 * or the full supported set when it has reported nothing yet.
 */
function webServerOptions(server: ManagedServer | null | undefined): string[] {
  const reported = nodeWebServers(server);
  return reported.length > 0 ? reported : [...WEB_SERVER_TYPES];
}

export default function WebsitesPage() {
  const {
    servers,
    localNode,
    defaultId,
    singleNode,
    loading: serversLoading,
    reload: reloadServers,
  } = useServers();

  const [websites, setWebsites] = useState<ManagedWebsite[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState<WebsiteForm>(EMPTY_FORM);
  const [formError, setFormError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const loadWebsites = async () => {
    setLoading(true);
    setError('');
    try {
      const res = await websiteApi.list({ page: 1, per_page: 200 });
      setWebsites(unwrapList<ManagedWebsite>(res));
    } catch (err: any) {
      console.error('Failed to load websites:', err);
      setWebsites([]);
      setError(
        errorMessage(err, 'Failed to load websites')
      );
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadWebsites();
  }, []);

  const selectedServer = useMemo(
    () => servers.find((s) => s.id === form.server_id) || null,
    [servers, form.server_id]
  );

  /*
   * The node is chosen for the operator, not asked about: on a single-node
   * panel that node is the machine the panel runs on. The picker in the dialog
   * only appears when there is more than one, and this keeps the form's choice
   * valid as the node list loads or changes underneath it.
   */
  useEffect(() => {
    if (!defaultId) return;
    setForm((prev) => {
      if (prev.server_id && servers.some((s) => s.id === prev.server_id)) return prev;
      return { ...prev, server_id: defaultId };
    });
  }, [defaultId, servers]);

  // Follow the chosen node's web server unless the operator has picked one that
  // node can actually run.
  useEffect(() => {
    const options = webServerOptions(selectedServer);
    setForm((prev) => {
      if (prev.web_server_type && options.includes(prev.web_server_type)) return prev;
      return { ...prev, web_server_type: options[0] || 'nginx' };
    });
  }, [selectedServer]);

  const openForm = () => {
    setFormError('');
    setForm((prev) => {
      const serverId = prev.server_id || defaultId;
      const node = servers.find((s) => s.id === serverId) || null;
      // Reopening the dialog must not leave the web server blank: reset to what
      // the chosen node actually runs.
      return {
        ...EMPTY_FORM,
        server_id: serverId,
        web_server_type: webServerOptions(node)[0] || 'nginx',
      };
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
        'No server is registered yet, so there is nowhere to put this website. Refresh the server list and try again.'
      );
      return;
    }

    const payload: CreateWebsiteRequest = {
      domain,
      server_id: form.server_id,
      web_server_type: form.web_server_type || 'nginx',
    };
    if (form.php_version) payload.php_version = form.php_version;
    if (form.site_type) payload.site_type = form.site_type;
    if (form.root_dir.trim()) payload.root_dir = form.root_dir.trim();

    setSubmitting(true);
    try {
      await websiteApi.create(payload);
      setShowForm(false);
      setForm(EMPTY_FORM);
      await loadWebsites();
    } catch (err: any) {
      setFormError(
        errorMessage(err, 'Failed to create the website')
      );
    } finally {
      setSubmitting(false);
    }
  };

  const getStatusBadge = (status: string) => {
    switch (String(status || '').toLowerCase()) {
      case 'active':
        return 'bg-emerald-50 text-emerald-700';
      case 'inactive':
      case 'error':
        return 'bg-red-50 text-red-700';
      default:
        return 'bg-amber-50 text-amber-700';
    }
  };

  const serverName = (id: string | undefined) => {
    const match = servers.find((s) => s.id === id);
    return match ? serverLabel(match) : null;
  };

  const formatDate = (value: string | undefined) => {
    if (!value) return null;
    const d = new Date(value);
    return Number.isNaN(d.getTime()) ? null : d.toLocaleDateString();
  };

  const showServerColumn = servers.length > 1;

  if (loading || serversLoading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-gray-200 border-b-brand-600" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Websites</h1>
          <p className="mt-1 text-sm text-gray-600">
            {singleNode && localNode
              ? `Sites hosted on ${serverLabel(localNode)}, the machine this panel runs on.`
              : 'Sites hosted on the machines this panel manages.'}
          </p>
        </div>
        <div className="flex items-center gap-3">
          <button
            type="button"
            onClick={() => {
              loadWebsites();
              reloadServers();
            }}
            className="inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
          >
            <RefreshCw size={16} aria-hidden="true" />
            Refresh
          </button>
          <button
            type="button"
            onClick={openForm}
            className="inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2"
          >
            <Plus size={16} aria-hidden="true" />
            Add Website
          </button>
        </div>
      </div>

      {error && (
        <div
          role="alert"
          className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700"
        >
          {error}
        </div>
      )}

      <div className="rounded-lg border border-gray-200 bg-white shadow-sm">
        <div className="border-b border-gray-200 px-5 py-4">
          <h2 className="text-sm font-semibold text-gray-900">All websites</h2>
        </div>

        {websites.length === 0 ? (
          /*
            The empty state names the one action worth taking first, and says
            where it will land - because on a fresh install the answer is "this
            machine", and an operator who does not know that thinks the panel
            needs a second server before it can do anything.
          */
          <div className="px-6 py-14 text-center">
            <Globe className="mx-auto text-gray-300" size={40} aria-hidden="true" />
            <h3 className="mt-4 text-sm font-semibold text-gray-900">
              Create your first website
            </h3>
            <p className="mx-auto mt-2 max-w-lg text-sm text-gray-600">
              {localNode ? (
                <>
                  Point a domain at <span className="font-mono">{serverLabel(localNode)}</span>{' '}
                  and the panel will set up its document root and web server configuration
                  on this machine. A database and a certificate can follow once the site
                  answers.
                </>
              ) : servers.length > 0 ? (
                <>
                  Add a domain and the panel will set up its document root and web server
                  configuration on the machine you choose.
                </>
              ) : (
                <>
                  No server is registered yet, so there is nowhere to put a website. The
                  machine running this panel should register itself; check the Servers
                  screen.
                </>
              )}
            </p>
            <div className="mt-4 flex flex-wrap items-center justify-center gap-3">
              <button
                type="button"
                onClick={openForm}
                disabled={servers.length === 0}
                className="inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-60"
              >
                <Plus size={16} aria-hidden="true" />
                Add Website
              </button>
              <Link
                href="/servers"
                className="inline-flex items-center rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
              >
                View servers
              </Link>
            </div>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[800px] border-collapse">
              <thead className="bg-gray-50">
                <tr className="border-b border-gray-200">
                  {['Domain', ...(showServerColumn ? ['Server'] : []), 'Type', 'Status', 'PHP', 'SSL', 'Created', 'Actions'].map(
                    (label) => (
                      <th
                        key={label}
                        scope="col"
                        className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500"
                      >
                        {label}
                      </th>
                    )
                  )}
                </tr>
              </thead>
              <tbody>
                {websites.map((site) => (
                  <tr key={site.id} className="border-b border-gray-100 hover:bg-gray-50">
                    <td className="px-4 py-3 text-sm font-medium text-gray-900">
                      <MetricText
                        value={site.domain || null}
                        reason="Not available: this site has no domain recorded."
                      />
                    </td>
                    {showServerColumn && (
                      <td className="px-4 py-3 text-sm text-gray-700">
                        <MetricText
                          value={serverName(site.server_id)}
                          reason="Not available: this site points at a server that is no longer in the list."
                        />
                      </td>
                    )}
                    <td className="px-4 py-3 text-sm text-gray-700">
                      <MetricText
                        value={site.site_type || null}
                        reason="Not available: the API did not report a site type."
                      />
                    </td>
                    <td className="px-4 py-3 text-sm">
                      <span
                        className={`inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium ${getStatusBadge(
                          site.status || ''
                        )}`}
                      >
                        {site.status || 'unknown'}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-700">
                      <MetricText
                        value={site.php_version || null}
                        reason="Not available: this site does not use a PHP runtime, or none has been recorded."
                      />
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-700">
                      {site.ssl_enabled ? (
                        <span className="inline-flex items-center gap-1 rounded-md bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700">
                          <Check size={12} aria-hidden="true" />
                          Enabled
                        </span>
                      ) : (
                        <span className="inline-flex items-center gap-1 rounded-md bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600">
                          <Minus size={12} aria-hidden="true" />
                          Off
                        </span>
                      )}
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-700" suppressHydrationWarning>
                      <MetricText value={formatDate(site.created_at)} reason={NO_DATE_REASON} />
                    </td>
                    <td className="px-4 py-3 text-sm">
                      <button
                        type="button"
                        className="inline-flex items-center rounded-md border border-gray-300 bg-white px-2.5 py-1 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
                      >
                        Manage
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {showForm && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-gray-900/40"
          onClick={() => setShowForm(false)}
        >
          <div
            role="dialog"
            aria-modal="true"
            aria-label="Add website"
            className="mx-4 w-full max-w-lg rounded-lg border border-gray-200 bg-white p-6 shadow-lg"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="mb-6 flex items-center justify-between">
              <h2 className="text-sm font-semibold text-gray-900">Add website</h2>
              <button
                type="button"
                onClick={() => setShowForm(false)}
                aria-label="Close dialog"
                className="rounded-md p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
              >
                <X size={16} aria-hidden="true" />
              </button>
            </div>

            <form onSubmit={submit} className="space-y-4">
              {formError && (
                <div
                  role="alert"
                  className="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700"
                >
                  {formError}
                </div>
              )}

              <div>
                <label htmlFor="website-domain" className={LABEL_CLASS}>
                  Domain <span className="text-red-600">*</span>
                </label>
                <input
                  id="website-domain"
                  type="text"
                  value={form.domain}
                  onChange={(e) => setForm((prev) => ({ ...prev, domain: e.target.value }))}
                  className={INPUT_CLASS}
                  placeholder="example.vn"
                  required
                />
              </div>

              {/* One node means one answer; the picker appears only with a real choice. */}
              <ServerScopeField
                id="website-server"
                servers={servers}
                value={form.server_id}
                onChange={(id) => setForm((prev) => ({ ...prev, server_id: id }))}
                copy={SERVER_SCOPE_COPY_EN}
              />

              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div>
                  <label htmlFor="website-webserver" className={LABEL_CLASS}>
                    Web server <span className="text-red-600">*</span>
                  </label>
                  <select
                    id="website-webserver"
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
                  {nodeWebServers(selectedServer).length === 0 && (
                    <p className="mt-1 text-xs text-gray-500">
                      This node has not reported which web servers it runs, so every
                      supported one is offered.
                    </p>
                  )}
                </div>

                <div>
                  <label htmlFor="website-site-type" className={LABEL_CLASS}>
                    Site type
                  </label>
                  <select
                    id="website-site-type"
                    value={form.site_type}
                    onChange={(e) => setForm((prev) => ({ ...prev, site_type: e.target.value }))}
                    className={INPUT_CLASS}
                  >
                    {SITE_TYPES.map((type) => (
                      <option key={type.value} value={type.value}>
                        {type.label}
                      </option>
                    ))}
                  </select>
                </div>
              </div>

              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div>
                  <label htmlFor="website-php" className={LABEL_CLASS}>
                    PHP version
                  </label>
                  <select
                    id="website-php"
                    value={form.php_version}
                    onChange={(e) =>
                      setForm((prev) => ({ ...prev, php_version: e.target.value }))
                    }
                    className={INPUT_CLASS}
                  >
                    {PHP_VERSIONS.map((version) => (
                      <option key={version || 'none'} value={version}>
                        {version || 'None'}
                      </option>
                    ))}
                  </select>
                </div>

                <div>
                  <label htmlFor="website-root" className={LABEL_CLASS}>
                    Document root
                  </label>
                  <input
                    id="website-root"
                    type="text"
                    value={form.root_dir}
                    onChange={(e) => setForm((prev) => ({ ...prev, root_dir: e.target.value }))}
                    className={INPUT_CLASS}
                    placeholder="Left empty: chosen from the domain"
                  />
                </div>
              </div>

              <div className="flex justify-end gap-3 border-t border-gray-200 pt-4">
                <button
                  type="button"
                  onClick={() => setShowForm(false)}
                  className="inline-flex items-center rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={submitting || !form.server_id}
                  className="inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-60"
                >
                  {submitting ? (
                    <Loader2 size={16} className="animate-spin" aria-hidden="true" />
                  ) : (
                    <Plus size={16} aria-hidden="true" />
                  )}
                  {submitting ? 'Creating...' : 'Create website'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
