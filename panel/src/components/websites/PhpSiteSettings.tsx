'use client';

/**
 * Everything a PHP site has, in one dialog, with each section stating whether
 * the panel can actually change it.
 *
 * Sections, and what is behind each:
 *
 *   Document root, PHP version   PUT /api/v1/websites/:id            works
 *   Pool limits                  PUT /api/v1/php/pools/:id           records only
 *   Rewrite rules                nothing                             no endpoint
 *   SSL                          POST /api/v1/websites/:id/ssl       works
 *   WordPress                    GET /api/v1/wordpress               record only
 */

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { ExternalLink, Loader2, Save, ShieldCheck } from 'lucide-react';

import { errorMessage } from '@/lib/apiError';
import { api, websiteApi } from '@/services/api';
import type { ManagedWebsite, SSLCertificate } from '@/types/server';
import type { PHPPool, WordPressSite } from '@/types/website';

import { phpPoolApi } from './api';
import { PROJECT_TYPE_GAPS } from './projectTypes';
import {
  BTN_PRIMARY,
  BTN_SECONDARY,
  Field,
  FormError,
  INPUT_CLASS,
  Modal,
  Notice,
  Tag,
} from './ui';

/** The PHP runtimes offered per site. An empty value means the site needs none. */
export const PHP_VERSIONS = ['', '8.4', '8.3', '8.2', '8.1', '8.0', '7.4'];

function Section({
  title,
  description,
  children,
}: {
  title: string;
  description?: string;
  children: React.ReactNode;
}) {
  return (
    <section className="rounded-md border border-gray-200 bg-white p-4">
      <h3 className="text-sm font-semibold text-gray-900">{title}</h3>
      {description && <p className="mt-1 text-sm text-gray-600">{description}</p>}
      <div className="mt-3 space-y-3">{children}</div>
    </section>
  );
}

export interface PhpSiteSettingsProps {
  site: ManagedWebsite;
  certificate: SSLCertificate | null;
  wordpress: WordPressSite | null;
  onClose: () => void;
  onSaved: () => void;
}

export default function PhpSiteSettings({
  site,
  certificate,
  wordpress,
  onClose,
  onSaved,
}: PhpSiteSettingsProps) {
  const [rootDir, setRootDir] = useState(site.root_dir || '');
  const [phpVersion, setPhpVersion] = useState(site.php_version || '');
  const [savingCore, setSavingCore] = useState(false);
  const [coreError, setCoreError] = useState('');
  const [coreSaved, setCoreSaved] = useState(false);

  const [pools, setPools] = useState<PHPPool[]>([]);
  const [poolsLoading, setPoolsLoading] = useState(true);
  const [poolsError, setPoolsError] = useState('');
  const [poolDraft, setPoolDraft] = useState<Record<string, string>>({});
  const [savingPool, setSavingPool] = useState(false);
  const [poolSaved, setPoolSaved] = useState(false);

  const [cert, setCert] = useState('');
  const [key, setKey] = useState('');
  const [chain, setChain] = useState('');
  const [sslError, setSslError] = useState('');
  const [sslSaving, setSslSaving] = useState(false);
  const [sslSaved, setSslSaved] = useState(false);

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      setPoolsLoading(true);
      setPoolsError('');
      try {
        const found = await phpPoolApi.listForWebsite(site.id);
        if (cancelled) return;
        setPools(found);
        const first = found[0];
        if (first) {
          setPoolDraft({
            pm: first.pm || 'dynamic',
            pm_max_children: String(first.pm_max_children ?? ''),
            pm_start_servers: String(first.pm_start_servers ?? ''),
            pm_min_spare_servers: String(first.pm_min_spare_servers ?? ''),
            pm_max_spare_servers: String(first.pm_max_spare_servers ?? ''),
            pm_max_requests: String(first.pm_max_requests ?? ''),
          });
        }
      } catch (err) {
        if (!cancelled) setPoolsError(errorMessage(err, 'Failed to load the PHP-FPM pool'));
      } finally {
        if (!cancelled) setPoolsLoading(false);
      }
    };
    load();
    return () => {
      cancelled = true;
    };
  }, [site.id]);

  const saveCore = async () => {
    setCoreError('');
    setCoreSaved(false);
    setSavingCore(true);
    try {
      await websiteApi.update(site.id, {
        root_dir: rootDir.trim(),
        php_version: phpVersion,
      });
      setCoreSaved(true);
      onSaved();
    } catch (err) {
      setCoreError(errorMessage(err, 'Failed to save the site settings'));
    } finally {
      setSavingCore(false);
    }
  };

  const savePool = async () => {
    const pool = pools[0];
    if (!pool) return;
    setPoolsError('');
    setPoolSaved(false);
    setSavingPool(true);
    const asNumber = (value: string) => {
      const n = Number(value);
      return Number.isFinite(n) && value.trim() !== '' ? n : undefined;
    };
    try {
      await phpPoolApi.update(pool.id, {
        pm: poolDraft.pm || undefined,
        pm_max_children: asNumber(poolDraft.pm_max_children || ''),
        pm_start_servers: asNumber(poolDraft.pm_start_servers || ''),
        pm_min_spare_servers: asNumber(poolDraft.pm_min_spare_servers || ''),
        pm_max_spare_servers: asNumber(poolDraft.pm_max_spare_servers || ''),
        pm_max_requests: asNumber(poolDraft.pm_max_requests || ''),
      });
      setPoolSaved(true);
    } catch (err) {
      setPoolsError(errorMessage(err, 'Failed to save the pool limits'));
    } finally {
      setSavingPool(false);
    }
  };

  const saveSsl = async (e: React.FormEvent) => {
    e.preventDefault();
    setSslError('');
    setSslSaved(false);
    if (!cert.trim() || !key.trim()) {
      setSslError('A certificate and a private key are both required.');
      return;
    }
    setSslSaving(true);
    try {
      await api.post(`/api/v1/websites/${site.id}/ssl`, {
        certificate: cert.trim(),
        private_key: key.trim(),
        chain_cert: chain.trim() || undefined,
      });
      setSslSaved(true);
      setCert('');
      setKey('');
      setChain('');
      onSaved();
    } catch (err) {
      setSslError(errorMessage(err, 'Failed to install the certificate'));
    } finally {
      setSslSaving(false);
    }
  };

  const poolFieldIds: Array<{ key: string; label: string }> = [
    { key: 'pm_max_children', label: 'Max children' },
    { key: 'pm_start_servers', label: 'Start servers' },
    { key: 'pm_min_spare_servers', label: 'Min spare servers' },
    { key: 'pm_max_spare_servers', label: 'Max spare servers' },
    { key: 'pm_max_requests', label: 'Max requests per child' },
  ];

  const expiry = certificate?.not_after ? new Date(certificate.not_after) : null;
  const expiryText =
    expiry && !Number.isNaN(expiry.getTime()) ? expiry.toLocaleDateString() : null;

  return (
    <Modal title={`Settings for ${site.domain || 'this site'}`} onClose={onClose} wide>
      <div className="max-h-[70vh] space-y-4 overflow-y-auto pr-1">
        <Section
          title="Document root and PHP version"
          description="Both are stored on the website row and used when the panel next writes this site's web server configuration."
        >
          <FormError message={coreError} />
          {coreSaved && (
            <p className="rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700">
              Saved.
            </p>
          )}
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <Field id="php-root" label="Document root">
              <input
                id="php-root"
                type="text"
                value={rootDir}
                onChange={(e) => setRootDir(e.target.value)}
                className={INPUT_CLASS}
                placeholder="/www/wwwroot/example.vn"
              />
            </Field>
            <Field
              id="php-version"
              label="PHP version"
              hint="Stored on the site. The route that switches the running pool per site is not mounted, so the change reaches the host the next time this site's configuration is regenerated."
            >
              <select
                id="php-version"
                value={phpVersion}
                onChange={(e) => setPhpVersion(e.target.value)}
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
          <div className="flex justify-end">
            <button type="button" onClick={saveCore} className={BTN_PRIMARY} disabled={savingCore}>
              {savingCore ? (
                <Loader2 size={16} className="animate-spin" aria-hidden="true" />
              ) : (
                <Save size={16} aria-hidden="true" />
              )}
              {savingCore ? 'Saving...' : 'Save'}
            </button>
          </div>
        </Section>

        <Section
          title="PHP-FPM pool limits"
          description="The process manager and the child limits for this site's pool."
        >
          <Notice tone="amber" title="Saved to the panel, not yet to the pool file">
            <p>{PROJECT_TYPE_GAPS.php[1]}</p>
          </Notice>
          <FormError message={poolsError} />
          {poolSaved && (
            <p className="rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700">
              Recorded.
            </p>
          )}
          {poolsLoading ? (
            <p className="text-sm text-gray-600">Loading the pool for this site...</p>
          ) : pools.length === 0 ? (
            <p className="text-sm text-gray-600">
              No PHP-FPM pool is recorded for this site, so there are no limits to show. Pools are
              created through POST /api/v1/php/pools; nothing on this screen creates one, because a
              pool the panel invents would not match the pool the host actually runs.
            </p>
          ) : (
            <>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
                <Field id="pool-pm" label="Process manager">
                  <select
                    id="pool-pm"
                    value={poolDraft.pm || 'dynamic'}
                    onChange={(e) => setPoolDraft((p) => ({ ...p, pm: e.target.value }))}
                    className={INPUT_CLASS}
                  >
                    <option value="static">static</option>
                    <option value="dynamic">dynamic</option>
                    <option value="ondemand">ondemand</option>
                  </select>
                </Field>
                {poolFieldIds.map((f) => (
                  <Field key={f.key} id={`pool-${f.key}`} label={f.label}>
                    <input
                      id={`pool-${f.key}`}
                      type="number"
                      min={0}
                      value={poolDraft[f.key] ?? ''}
                      onChange={(e) => setPoolDraft((p) => ({ ...p, [f.key]: e.target.value }))}
                      className={INPUT_CLASS}
                    />
                  </Field>
                ))}
              </div>
              <div className="flex items-center justify-between gap-3">
                <p className="text-xs text-gray-500">
                  Pool <span className="font-mono">{pools[0]?.name || pools[0]?.id}</span>
                  {pools[0]?.listen ? ` listening on ${pools[0].listen}` : ''}
                </p>
                <button type="button" onClick={savePool} className={BTN_PRIMARY} disabled={savingPool}>
                  {savingPool ? (
                    <Loader2 size={16} className="animate-spin" aria-hidden="true" />
                  ) : (
                    <Save size={16} aria-hidden="true" />
                  )}
                  {savingPool ? 'Saving...' : 'Save limits'}
                </button>
              </div>
            </>
          )}
        </Section>

        <Section title="Rewrite rules">
          <Notice tone="amber" title="Not available yet">
            <p>{PROJECT_TYPE_GAPS.php[0]}</p>
            <p>
              Until that endpoint exists, rewrite rules are edited in the site&apos;s web server
              configuration file on the host. No control is shown here, because one that saved
              nowhere would be worse than none.
            </p>
          </Notice>
        </Section>

        <Section title="SSL">
          <div className="flex flex-wrap items-center gap-2 text-sm text-gray-700">
            {certificate ? (
              <>
                <Tag tone="sky">{certificate.source || 'certificate'}</Tag>
                <span>
                  Issued by {certificate.issuer || 'an unrecorded issuer'}
                  {expiryText ? `, expires ${expiryText}` : ''}
                </span>
                {certificate.auto_renew && <Tag>auto renew</Tag>}
              </>
            ) : site.ssl_enabled ? (
              <span>
                This site is marked as having SSL enabled, but no certificate row matches its
                domain. The certificate was installed directly rather than through the panel.
              </span>
            ) : (
              <span>No certificate is installed for this site.</span>
            )}
          </div>
          <p className="text-sm text-gray-600">
            Let&apos;s Encrypt issuing and renewal live on the{' '}
            <Link
              href="/ssl"
              className="font-medium text-brand-700 underline underline-offset-2 hover:text-brand-800"
            >
              SSL screen
            </Link>
            . Paste an existing certificate below to install it on this site.
          </p>
          <form onSubmit={saveSsl} className="space-y-3">
            <FormError message={sslError} />
            {sslSaved && (
              <p className="rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700">
                Certificate installed and the web server reloaded.
              </p>
            )}
            <Field id="ssl-cert" label="Certificate (PEM)" required>
              <textarea
                id="ssl-cert"
                value={cert}
                onChange={(e) => setCert(e.target.value)}
                rows={3}
                className={`${INPUT_CLASS} font-mono text-xs`}
                placeholder="-----BEGIN CERTIFICATE-----"
              />
            </Field>
            <Field id="ssl-key" label="Private key (PEM)" required>
              <textarea
                id="ssl-key"
                value={key}
                onChange={(e) => setKey(e.target.value)}
                rows={3}
                className={`${INPUT_CLASS} font-mono text-xs`}
                placeholder="-----BEGIN PRIVATE KEY-----"
              />
            </Field>
            <Field id="ssl-chain" label="Chain certificate (optional)">
              <textarea
                id="ssl-chain"
                value={chain}
                onChange={(e) => setChain(e.target.value)}
                rows={2}
                className={`${INPUT_CLASS} font-mono text-xs`}
              />
            </Field>
            <div className="flex justify-end">
              <button type="submit" className={BTN_PRIMARY} disabled={sslSaving}>
                {sslSaving ? (
                  <Loader2 size={16} className="animate-spin" aria-hidden="true" />
                ) : (
                  <ShieldCheck size={16} aria-hidden="true" />
                )}
                {sslSaving ? 'Installing...' : 'Install certificate'}
              </button>
            </div>
          </form>
        </Section>

        <Section title="WordPress">
          {wordpress ? (
            <>
              <div className="flex flex-wrap items-center gap-2 text-sm text-gray-700">
                <Tag tone="sky">WordPress</Tag>
                <span>
                  {wordpress.name || wordpress.domain}
                  {wordpress.version ? ` on version ${wordpress.version}` : ''}
                  {wordpress.auto_update ? ', auto update on' : ''}
                </span>
              </div>
              <Notice tone="amber" title="Plugin, theme and core actions run against the panel's record">
                <p>{PROJECT_TYPE_GAPS.php[3]}</p>
              </Notice>
              <Link
                href="/websites/wordpress"
                className={`${BTN_SECONDARY} w-fit`}
              >
                <ExternalLink size={16} aria-hidden="true" />
                Open the WordPress screen
              </Link>
            </>
          ) : (
            <p className="text-sm text-gray-600">
              This site has no WordPress record, so no WordPress actions are offered. A site becomes
              a WordPress site by being registered on the{' '}
              <Link
                href="/websites/wordpress"
                className="font-medium text-brand-700 underline underline-offset-2 hover:text-brand-800"
              >
                WordPress screen
              </Link>
              .
            </p>
          )}
        </Section>
      </div>

      <div className="mt-4 flex justify-end border-t border-gray-200 pt-4">
        <button type="button" onClick={onClose} className={BTN_SECONDARY}>
          Close
        </button>
      </div>
    </Modal>
  );
}
