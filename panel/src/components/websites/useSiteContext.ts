'use client';

/**
 * The four lists every tab needs, loaded once for the whole screen.
 *
 * Websites, certificates, deployments and WordPress records are read by more
 * than one tab: a Node.js project shows the domain and certificate of the
 * website it is attached to, and so does a proxy. Loading them per tab meant
 * the same four requests on every tab switch.
 *
 * Each list fails on its own. They sit behind four different permissions
 * (website, ssl, terminal:execute, wordpress), so an operator who cannot read
 * deployments must still see their sites - with the deployment column
 * unavailable and the reason attached, never a blank screen.
 */

import { useCallback, useEffect, useState } from 'react';

import { errorMessage } from '@/lib/apiError';
import { sslApi, unwrapList, websiteApi } from '@/services/api';
import type { ManagedWebsite, SSLCertificate } from '@/types/server';
import type { GitDeployment, WordPressSite } from '@/types/website';

import { siteDeploymentApi, wordpressApi } from './api';

export interface SiteContext {
  websites: ManagedWebsite[];
  websitesLoading: boolean;
  /** Non-empty when the websites list itself failed; the tabs show it as an error state. */
  websitesError: string;

  certificates: SSLCertificate[];
  certificatesError: string;

  deployments: GitDeployment[];
  deploymentsError: string;

  wordpress: WordPressSite[];

  reload: () => Promise<void>;
}

export function useSiteContext(): SiteContext {
  const [websites, setWebsites] = useState<ManagedWebsite[]>([]);
  const [websitesLoading, setWebsitesLoading] = useState(true);
  const [websitesError, setWebsitesError] = useState('');

  const [certificates, setCertificates] = useState<SSLCertificate[]>([]);
  const [certificatesError, setCertificatesError] = useState('');

  const [deployments, setDeployments] = useState<GitDeployment[]>([]);
  const [deploymentsError, setDeploymentsError] = useState('');

  const [wordpress, setWordpress] = useState<WordPressSite[]>([]);

  const reload = useCallback(async () => {
    setWebsitesLoading(true);
    setWebsitesError('');
    try {
      setWebsites(unwrapList<ManagedWebsite>(await websiteApi.list({ page: 1, per_page: 200 })));
    } catch (err) {
      setWebsites([]);
      setWebsitesError(errorMessage(err, 'Failed to load websites'));
    } finally {
      setWebsitesLoading(false);
    }

    setCertificatesError('');
    try {
      setCertificates(unwrapList<SSLCertificate>(await sslApi.list()));
    } catch (err) {
      setCertificates([]);
      setCertificatesError(errorMessage(err, 'certificates could not be read'));
    }

    setDeploymentsError('');
    try {
      setDeployments(await siteDeploymentApi.list());
    } catch (err) {
      setDeployments([]);
      setDeploymentsError(errorMessage(err, 'deployments could not be read'));
    }

    try {
      setWordpress(await wordpressApi.list());
    } catch {
      // Only costs a badge. Not worth an error banner.
      setWordpress([]);
    }
  }, []);

  useEffect(() => {
    reload();
  }, [reload]);

  return {
    websites,
    websitesLoading,
    websitesError,
    certificates,
    certificatesError,
    deployments,
    deploymentsError,
    wordpress,
    reload,
  };
}

/** The certificate that belongs to a domain, matched the way the API allows. */
export function certificateForDomain(
  certificates: SSLCertificate[],
  domain: string | undefined,
  websiteId?: string
): SSLCertificate | null {
  if (!domain && !websiteId) return null;
  return (
    certificates.find(
      (c) =>
        (websiteId && c.website_id === websiteId) ||
        (domain && c.domain && c.domain.toLowerCase() === domain.toLowerCase())
    ) || null
  );
}

/** The deployment attached to a website row, when there is one. */
export function deploymentForWebsite(
  deployments: GitDeployment[],
  websiteId: string | null | undefined
) {
  if (!websiteId) return null;
  return deployments.find((d) => d.website_id === websiteId) || null;
}

export default useSiteContext;
