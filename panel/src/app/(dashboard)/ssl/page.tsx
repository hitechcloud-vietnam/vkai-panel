'use client';

import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { Loader2, Plus, RefreshCw, Shield, X } from 'lucide-react';

import ServerScopeField, {
  SERVER_SCOPE_COPY_EN,
} from '@/components/servers/ServerScopeField';
import { MetricText } from '@/components/Unavailable';
import { useServers } from '@/hooks/useServers';
import { sslApi, unwrapList, websiteApi } from '@/services/api';
import { serverLabel } from '@/lib/servers';
import type {
  IssueLetsEncryptRequest,
  ManagedWebsite,
  SSLCertificate,
} from '@/types/server';
import { errorMessage } from '@/lib/apiError';

const INPUT_CLASS =
  'w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder:text-gray-400 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500';
const LABEL_CLASS = 'mb-1.5 block text-sm font-medium text-gray-700';

const CUSTOM_TARGET = '__custom__';

function statusBadge(status: string | undefined): string {
  switch (String(status || '').toLowerCase()) {
    case 'valid':
    case 'active':
    case 'issued':
      return 'bg-emerald-50 text-emerald-700';
    case 'expired':
    case 'revoked':
    case 'failed':
      return 'bg-red-50 text-red-700';
    case 'expiring':
    case 'pending':
      return 'bg-amber-50 text-amber-700';
    default:
      return 'bg-gray-100 text-gray-700';
  }
}

export default function SSLPage() {
  const { servers, localNode, defaultId, singleNode } = useServers();

  const [certificates, setCertificates] = useState<SSLCertificate[]>([]);
  const [websites, setWebsites] = useState<ManagedWebsite[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [renewing, setRenewing] = useState(false);

  const [showForm, setShowForm] = useState(false);
  const [serverId, setServerId] = useState('');
  const [target, setTarget] = useState(CUSTOM_TARGET);
  const [domain, setDomain] = useState('');
  const [webroot, setWebroot] = useState('');
  const [formError, setFormError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const load = async () => {
    setLoading(true);
    setError('');
    const [certRes, siteRes] = await Promise.allSettled([
      sslApi.list(),
      websiteApi.list({ page: 1, per_page: 200 }),
    ]);

    if (certRes.status === 'fulfilled') {
      setCertificates(unwrapList<SSLCertificate>(certRes.value));
    } else {
      setCertificates([]);
      const err: any = certRes.reason;
      setError(
        errorMessage(err, 'Failed to load certificates')
      );
    }

    // A failed website list is not worth an error banner on this screen: it
    // only costs the operator the shortcut of picking a site by name.
    setWebsites(
      siteRes.status === 'fulfilled' ? unwrapList<ManagedWebsite>(siteRes.value) : []
    );
    setLoading(false);
  };

  useEffect(() => {
    load();
  }, []);

  // The certificate is issued on the panel host unless the operator picks
  // another node, and there is no picker to pick with until there is one.
  useEffect(() => {
    if (!defaultId) return;
    setServerId((prev) => (prev && servers.some((s) => s.id === prev) ? prev : defaultId));
  }, [defaultId, servers]);

  const sitesOnServer = useMemo(
    () => websites.filter((site) => !serverId || site.server_id === serverId),
    [websites, serverId]
  );

  // A site chosen on one node is not a valid choice on another.
  useEffect(() => {
    if (target === CUSTOM_TARGET) return;
    if (!sitesOnServer.some((site) => site.id === target)) {
      setTarget(CUSTOM_TARGET);
    }
  }, [sitesOnServer, target]);

  const onTargetChange = (value: string) => {
    setTarget(value);
    if (value === CUSTOM_TARGET) return;
    const site = websites.find((s) => s.id === value);
    if (!site) return;
    setDomain(site.domain || '');
    setWebroot(site.root_dir || '');
  };

  const openForm = () => {
    setFormError('');
    setTarget(CUSTOM_TARGET);
    setDomain('');
    setWebroot('');
    setShowForm(true);
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setFormError('');

    const cleanDomain = domain.trim().toLowerCase();
    const cleanWebroot = webroot.trim();
    if (!cleanDomain) {
      setFormError('A domain is required.');
      return;
    }
    if (!cleanWebroot) {
      setFormError(
        'A document root is required: Let’s Encrypt writes its challenge file there. Pick an existing website to fill it in.'
      );
      return;
    }

    const payload: IssueLetsEncryptRequest = {
      domain: cleanDomain,
      webroot: cleanWebroot,
    };
    if (serverId) payload.server_id = serverId;

    setSubmitting(true);
    try {
      await sslApi.issueLetsEncrypt(payload);
      setShowForm(false);
      await load();
    } catch (err: any) {
      setFormError(
        errorMessage(err, 'Failed to issue the certificate')
      );
    } finally {
      setSubmitting(false);
    }
  };

  const renewAll = async () => {
    setRenewing(true);
    try {
      await sslApi.renewAll();
      await load();
    } catch (err: any) {
      setError(
        errorMessage(err, 'Failed to renew certificates')
      );
    } finally {
      setRenewing(false);
    }
  };

  const formatDate = (value: string | undefined) => {
    if (!value) return null;
    const d = new Date(value);
    return Number.isNaN(d.getTime()) ? null : d.toLocaleDateString();
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">SSL Certificates</h1>
          <p className="mt-1 text-sm text-gray-600">
            {singleNode && localNode
              ? `Certificates issued on ${serverLabel(localNode)}, the machine this panel runs on.`
              : 'Certificates issued on the machines this panel manages.'}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={renewAll}
            disabled={renewing || certificates.length === 0}
            className="inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 disabled:cursor-not-allowed disabled:opacity-60"
          >
            {renewing ? (
              <Loader2 size={16} className="animate-spin" aria-hidden="true" />
            ) : (
              <RefreshCw size={16} aria-hidden="true" />
            )}
            Renew All
          </button>
          <button
            type="button"
            onClick={openForm}
            className="inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2"
          >
            <Plus size={16} aria-hidden="true" />
            Add Certificate
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
          <h2 className="text-sm font-semibold text-gray-900">Certificates</h2>
        </div>

        {loading ? (
          <div className="flex h-40 items-center justify-center">
            <div className="h-8 w-8 animate-spin rounded-full border-2 border-gray-200 border-b-brand-600" />
          </div>
        ) : certificates.length === 0 ? (
          <div className="px-6 py-14 text-center">
            <Shield className="mx-auto text-gray-300" size={40} aria-hidden="true" />
            <h3 className="mt-4 text-sm font-semibold text-gray-900">No certificates yet</h3>
            <p className="mx-auto mt-2 max-w-lg text-sm text-gray-600">
              {websites.length === 0
                ? 'Create a website first: Let’s Encrypt proves ownership by fetching a file from the site’s document root, so the site has to answer before a certificate can be issued.'
                : 'Pick one of your websites and the panel will request a certificate for its domain and install it on the machine the site runs on.'}
            </p>
            <div className="mt-4 flex flex-wrap items-center justify-center gap-3">
              {websites.length === 0 ? (
                <Link
                  href="/websites"
                  className="inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2"
                >
                  Go to websites
                </Link>
              ) : (
                <button
                  type="button"
                  onClick={openForm}
                  className="inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2"
                >
                  <Plus size={16} aria-hidden="true" />
                  Add Certificate
                </button>
              )}
            </div>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[720px] border-collapse">
              <thead className="bg-gray-50">
                <tr className="border-b border-gray-200">
                  {['Domain', 'Issuer', 'Source', 'Status', 'Expires', 'Auto renew'].map((label) => (
                    <th
                      key={label}
                      scope="col"
                      className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500"
                    >
                      {label}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {certificates.map((cert) => (
                  <tr key={cert.id} className="border-b border-gray-100 hover:bg-gray-50">
                    <td className="px-4 py-3 text-sm font-medium text-gray-900">
                      <MetricText
                        value={cert.domain || null}
                        reason="Not available: this certificate has no domain recorded."
                      />
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-700">
                      <MetricText
                        value={cert.issuer || null}
                        reason="Not available: the API did not report an issuer."
                      />
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-700">
                      <MetricText
                        value={cert.source || null}
                        reason="Not available: the API did not report whether this certificate was issued or uploaded."
                      />
                    </td>
                    <td className="px-4 py-3 text-sm">
                      <span
                        className={`inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium ${statusBadge(
                          cert.status
                        )}`}
                      >
                        {cert.status || 'unknown'}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-700" suppressHydrationWarning>
                      <MetricText
                        value={formatDate(cert.not_after)}
                        reason="Not available: the API did not report an expiry date."
                      />
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-700">
                      {cert.auto_renew ? 'Yes' : 'No'}
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
            aria-label="Issue certificate"
            className="mx-4 w-full max-w-lg rounded-lg border border-gray-200 bg-white p-6 shadow-lg"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="mb-6 flex items-center justify-between">
              <h2 className="text-sm font-semibold text-gray-900">
                Issue a Let&rsquo;s Encrypt certificate
              </h2>
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

              {/* One node means one answer; the picker appears only with a real choice. */}
              <ServerScopeField
                id="ssl-server"
                servers={servers}
                value={serverId}
                onChange={setServerId}
                copy={SERVER_SCOPE_COPY_EN}
              />

              <div>
                <label htmlFor="ssl-target" className={LABEL_CLASS}>
                  Website
                </label>
                <select
                  id="ssl-target"
                  value={target}
                  onChange={(e) => onTargetChange(e.target.value)}
                  className={INPUT_CLASS}
                >
                  <option value={CUSTOM_TARGET}>Enter a domain manually</option>
                  {sitesOnServer.map((site) => (
                    <option key={site.id} value={site.id}>
                      {site.domain || site.id.slice(0, 8)}
                    </option>
                  ))}
                </select>
                <p className="mt-1 text-xs text-gray-500">
                  Picking a website fills in its domain and document root.
                </p>
              </div>

              <div>
                <label htmlFor="ssl-domain" className={LABEL_CLASS}>
                  Domain <span className="text-red-600">*</span>
                </label>
                <input
                  id="ssl-domain"
                  type="text"
                  value={domain}
                  onChange={(e) => setDomain(e.target.value)}
                  className={INPUT_CLASS}
                  placeholder="example.vn"
                  required
                />
              </div>

              <div>
                <label htmlFor="ssl-webroot" className={LABEL_CLASS}>
                  Document root <span className="text-red-600">*</span>
                </label>
                <input
                  id="ssl-webroot"
                  type="text"
                  value={webroot}
                  onChange={(e) => setWebroot(e.target.value)}
                  className={INPUT_CLASS}
                  placeholder="/var/www/example.vn"
                  required
                />
                <p className="mt-1 text-xs text-gray-500">
                  The directory the domain already serves. Let&rsquo;s Encrypt fetches its
                  challenge file from there over plain HTTP.
                </p>
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
                  disabled={submitting}
                  className="inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-60"
                >
                  {submitting ? (
                    <Loader2 size={16} className="animate-spin" aria-hidden="true" />
                  ) : (
                    <Shield size={16} aria-hidden="true" />
                  )}
                  {submitting ? 'Requesting...' : 'Request certificate'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
