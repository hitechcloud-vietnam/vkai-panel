'use client';

/**
 * Shared state for the WP Toolkit: what the panel can do, and what it knows
 * about each installation.
 *
 * The capability probe is deliberately a hook rather than a constant. Whether
 * the WP-CLI routes are mounted is a property of the running panel, not of this
 * build, and it changes the moment somebody adds the one missing line to
 * router.go. Baking it in at build time would mean shipping a UI that is wrong
 * about the server it is talking to.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  errorMessage,
  probeRuntime,
  records,
  runtime as runtimeApi,
  supporting,
} from './api';
import type {
  InstallationSummary,
  LivePlugin,
  LiveTheme,
  RuntimeAvailability,
  UpdateCounts,
  WordPressSite,
} from '@/types/wordpress';

// ---------------------------------------------------------------------------
// Capability
// ---------------------------------------------------------------------------

export interface RuntimeState {
  availability: RuntimeAvailability;
  /** True only when live readings and one-click updates are real. */
  ready: boolean;
  recheck: () => void;
}

export function useRuntimeAvailability(): RuntimeState {
  const [availability, setAvailability] = useState<RuntimeAvailability>('checking');
  const [nonce, setNonce] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setAvailability('checking');
    probeRuntime().then((value) => {
      if (!cancelled) setAvailability(value);
    });
    return () => {
      cancelled = true;
    };
  }, [nonce]);

  const recheck = useCallback(() => setNonce((n) => n + 1), []);

  return { availability, ready: availability === 'available', recheck };
}

// ---------------------------------------------------------------------------
// The installations the panel knows about
// ---------------------------------------------------------------------------

export interface SitesState {
  sites: WordPressSite[];
  loading: boolean;
  error: string | null;
  reload: () => void;
}

export function useSites(): SitesState {
  const [sites, setSites] = useState<WordPressSite[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [nonce, setNonce] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    records
      .list()
      .then((rows) => {
        if (!cancelled) setSites(rows);
      })
      .catch((err) => {
        if (!cancelled) {
          setError(errorMessage(err, 'The list of WordPress installations could not be loaded.'));
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [nonce]);

  return { sites, loading, error, reload: useCallback(() => setNonce((n) => n + 1), []) };
}

// ---------------------------------------------------------------------------
// Live readings, one request set per installation
// ---------------------------------------------------------------------------

/**
 * Fetch runtime, plugin and theme readings for a list of sites.
 *
 * Runs only when the runtime routes are mounted. When they are not, every
 * summary comes back with null readings and the caller renders the reason
 * instead of a number, which is the whole point of the exercise.
 *
 * Requests are issued a few sites at a time. Each one shells out to WP-CLI on
 * the host, and firing forty of those at once turns a page load into a load
 * spike on the customer's own server.
 */
export function useLiveReadings(sites: WordPressSite[], enabled: boolean) {
  const [summaries, setSummaries] = useState<Record<string, InstallationSummary>>({});
  const [loading, setLoading] = useState(false);
  const [nonce, setNonce] = useState(0);
  const idsKey = useMemo(() => sites.map((s) => s.id).join(','), [sites]);
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  useEffect(() => {
    if (!enabled || sites.length === 0) {
      setSummaries({});
      setLoading(false);
      return;
    }
    let cancelled = false;
    setLoading(true);

    const readOne = async (site: WordPressSite): Promise<InstallationSummary> => {
      const summary: InstallationSummary = {
        site,
        runtime: null,
        plugins: null,
        themes: null,
        liveError: null,
      };
      const [runtimeResult, pluginResult, themeResult] = await Promise.allSettled([
        runtimeApi.view(site.id),
        runtimeApi.livePlugins(site.id),
        runtimeApi.liveThemes(site.id),
      ]);
      if (runtimeResult.status === 'fulfilled') {
        summary.runtime = runtimeResult.value;
      }
      if (pluginResult.status === 'fulfilled') {
        summary.plugins = pluginResult.value;
      }
      if (themeResult.status === 'fulfilled') {
        summary.themes = themeResult.value;
      }
      const firstFailure = [runtimeResult, pluginResult, themeResult].find(
        (r): r is PromiseRejectedResult => r.status === 'rejected',
      );
      if (firstFailure) {
        summary.liveError = errorMessage(
          firstFailure.reason,
          'WP-CLI could not be run against this installation.',
        );
      }
      return summary;
    };

    (async () => {
      const batchSize = 4;
      const collected: Record<string, InstallationSummary> = {};
      for (let i = 0; i < sites.length; i += batchSize) {
        if (cancelled) return;
        const batch = sites.slice(i, i + batchSize);
        const results = await Promise.all(batch.map(readOne));
        results.forEach((summary) => {
          collected[summary.site.id] = summary;
        });
        if (!cancelled && mounted.current) setSummaries({ ...collected });
      }
      if (!cancelled && mounted.current) setLoading(false);
    })();

    return () => {
      cancelled = true;
    };
    // idsKey stands in for the site list: re-reading on every array identity
    // change would re-run WP-CLI on every render of the parent.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [idsKey, enabled, nonce]);

  return { summaries, loading, reload: useCallback(() => setNonce((n) => n + 1), []) };
}

// ---------------------------------------------------------------------------
// Counting what is out of date
// ---------------------------------------------------------------------------

/**
 * Count a live plugin or theme list.
 *
 * WP-CLI reports `update` as "available", "none" or "unavailable". Only
 * "available" is an update. "unavailable" means WP-CLI could not reach
 * wordpress.org to find out, and folding that into "up to date" would report a
 * site as clean when nobody has checked it.
 */
export function countUpdates(items: (LivePlugin | LiveTheme)[] | null): UpdateCounts | null {
  if (items === null) return null;
  let outdated = 0;
  let unknown = 0;
  for (const item of items) {
    const state = (item.update ?? '').toLowerCase();
    if (state === 'available') outdated += 1;
    else if (state !== 'none' && state !== '') unknown += 1;
  }
  return { total: items.length, outdated, unknown };
}

/** The slugs WP-CLI says have an update waiting. */
export function outdatedNames(items: (LivePlugin | LiveTheme)[] | null): string[] {
  if (!items) return [];
  return items
    .filter((item) => (item.update ?? '').toLowerCase() === 'available')
    .map((item) => item.name)
    .filter((name): name is string => Boolean(name));
}

// ---------------------------------------------------------------------------
// Supporting lists for the Add WordPress form
// ---------------------------------------------------------------------------

export interface FormOptionsState {
  servers: { id: string; label: string }[];
  websites: { id: string; domain: string; root_dir: string; server_id: string }[];
  databases: { id: string; name: string; username: string }[];
  phpVersions: string[];
  loading: boolean;
  /** Lists that could not be loaded, so the form can say which picker is empty and why. */
  errors: Record<string, string>;
}

export function useFormOptions(): FormOptionsState {
  const [state, setState] = useState<FormOptionsState>({
    servers: [],
    websites: [],
    databases: [],
    phpVersions: [],
    loading: true,
    errors: {},
  });

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const [serverRes, websiteRes, dbRes, phpRes] = await Promise.allSettled([
        supporting.servers(),
        supporting.websites(),
        supporting.databases(),
        supporting.phpVersions(),
      ]);
      if (cancelled) return;
      const errors: Record<string, string> = {};

      const servers =
        serverRes.status === 'fulfilled'
          ? serverRes.value.map((s) => ({
              id: s.id,
              label: s.name || s.hostname || s.ip_address || s.id,
            }))
          : ((errors.servers = errorMessage(serverRes.reason, 'Servers could not be listed.')), []);

      const websites =
        websiteRes.status === 'fulfilled'
          ? websiteRes.value.map((w) => ({
              id: w.id,
              domain: w.domain || '',
              root_dir: w.root_dir || '',
              server_id: w.server_id || '',
            }))
          : ((errors.websites = errorMessage(websiteRes.reason, 'Websites could not be listed.')), []);

      const databases =
        dbRes.status === 'fulfilled'
          ? dbRes.value.map((d) => ({ id: d.id, name: d.name || '', username: d.username || '' }))
          : ((errors.databases = errorMessage(dbRes.reason, 'Databases could not be listed.')), []);

      const phpVersions =
        phpRes.status === 'fulfilled'
          ? Array.from(
              new Set(
                phpRes.value
                  .map((p) => (p.version || '').trim())
                  .filter((v) => v.length > 0),
              ),
            ).sort()
          : ((errors.php = errorMessage(phpRes.reason, 'PHP versions could not be listed.')), []);

      setState({ servers, websites, databases, phpVersions, loading: false, errors });
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  return state;
}
