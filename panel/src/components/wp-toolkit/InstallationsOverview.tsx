'use client';

/**
 * The screen the toolkit opens on: every WordPress installation the panel knows
 * about, what is out of date on each, and the three buttons that fix it.
 *
 * Two sources of truth are shown side by side and never mixed:
 *
 *   - the panel's RECORD (GET /api/v1/wordpress) - name, domain, path, the
 *     version somebody typed in once. Always available.
 *   - the LIVE reading (WP-CLI, through the runtime routes) - the version that
 *     is really on disk, the plugins that are really installed, and which of
 *     them have an update waiting. Available only when those routes are mounted.
 *
 * Where a live reading is missing the cell shows a dash with the reason on it,
 * not a zero. "0 plugins out of date" and "nobody has looked" must not render
 * identically.
 */

import { useMemo, useState } from 'react';
import Link from 'next/link';
import { ExternalLink, RefreshCw } from 'lucide-react';
import { errorMessage, runtime as runtimeApi } from './api';
import { countUpdates, useLiveReadings, useRuntimeAvailability, useSites } from './hooks';
import { RuntimeBanner } from './RuntimeBanner';
import { siteLabel } from './SitePicker';
import {
  Badge,
  Button,
  Card,
  CardBody,
  CardHeader,
  EmptyState,
  ErrorNote,
  Loading,
  Notice,
  PageHeader,
  Table,
  Td,
  Th,
  Unknown,
} from './ui';
import type { Tone } from './ui';
import type { InstallationSummary, WordPressSite } from '@/types/wordpress';

const NO_LIVE_DATA =
  'Not read: this needs the WP-CLI routes, which are not mounted on this panel.';

function statusTone(status: string | undefined): Tone {
  switch ((status || '').toLowerCase()) {
    case 'active':
    case 'running':
      return 'emerald';
    case 'pending':
    case 'installing':
      return 'sky';
    case 'error':
    case 'failed':
      return 'red';
    case 'suspended':
    case 'inactive':
      return 'amber';
    default:
      return 'gray';
  }
}

/** One "3 of 12" cell, or a dash carrying why it is not a number. */
function CountCell({
  items,
  reason,
}: {
  items: InstallationSummary['plugins'] | InstallationSummary['themes'];
  reason: string;
}) {
  const counts = countUpdates(items);
  if (!counts) return <Unknown reason={reason} />;
  return (
    <span className="whitespace-nowrap">
      <span className="text-gray-900">{counts.total}</span>
      {counts.outdated > 0 ? (
        <span className="ml-2">
          <Badge tone="amber">{counts.outdated} out of date</Badge>
        </span>
      ) : (
        <span className="ml-2 text-xs text-gray-500">up to date</span>
      )}
      {counts.unknown > 0 ? (
        <span
          className="ml-2 text-xs text-gray-500"
          title="WP-CLI could not check these against wordpress.org, so they are counted as unknown rather than current."
        >
          {counts.unknown} unchecked
        </span>
      ) : null}
    </span>
  );
}

export function InstallationsOverview() {
  const { availability, ready, recheck } = useRuntimeAvailability();
  const { sites, loading: sitesLoading, error: sitesError, reload: reloadSites } = useSites();
  const { summaries, loading: liveLoading, reload: reloadLive } = useLiveReadings(sites, ready);

  const [busy, setBusy] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [actionResult, setActionResult] = useState<string | null>(null);

  const runAction = async (key: string, label: string, action: () => Promise<unknown>) => {
    setBusy(key);
    setActionError(null);
    setActionResult(null);
    try {
      await action();
      setActionResult(`${label} finished. The live readings below have been re-taken.`);
      reloadLive();
    } catch (err) {
      setActionError(errorMessage(err, `${label} failed.`));
    } finally {
      setBusy(null);
    }
  };

  const pending = useMemo(() => {
    const rows: { site: WordPressSite; kind: string; name: string; from: string; to: string }[] = [];
    for (const site of sites) {
      const summary = summaries[site.id];
      if (!summary) continue;
      for (const plugin of summary.plugins ?? []) {
        if ((plugin.update ?? '').toLowerCase() === 'available') {
          rows.push({
            site,
            kind: 'Plugin',
            name: plugin.name,
            from: plugin.version || '',
            to: plugin.update_version || '',
          });
        }
      }
      for (const theme of summary.themes ?? []) {
        if ((theme.update ?? '').toLowerCase() === 'available') {
          rows.push({
            site,
            kind: 'Theme',
            name: theme.name,
            from: theme.version || '',
            to: theme.update_version || '',
          });
        }
      }
    }
    return rows;
  }, [sites, summaries]);

  const runtimeOff = ready ? null : NO_LIVE_DATA;

  return (
    <div className="space-y-5">
      <PageHeader
        title="WordPress installations"
        description="Every WordPress installation this panel has a record of, what is out of date on each of them, and the operations that bring them current."
        actions={
          <>
            <Button tone="secondary" onClick={reloadSites} busy={sitesLoading}>
              <RefreshCw className="h-4 w-4" aria-hidden />
              Reload
            </Button>
            <Link
              href="/wp-toolkit/add"
              className="inline-flex items-center gap-2 rounded-md border border-transparent bg-brand-600 px-3 py-2 text-sm font-medium text-white hover:bg-brand-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-1"
            >
              Add WordPress
            </Link>
          </>
        }
      />

      <RuntimeBanner availability={availability} onRecheck={recheck} />

      {sitesError ? <ErrorNote>{sitesError}</ErrorNote> : null}
      {actionError ? <ErrorNote>{actionError}</ErrorNote> : null}
      {actionResult ? <Notice tone="emerald">{actionResult}</Notice> : null}

      <Card>
        <CardHeader
          title="Installations"
          description={
            ready
              ? 'Version, plugin and theme figures are read live through WP-CLI each time this page loads.'
              : 'Only the panel record is shown. Live figures need the WP-CLI routes.'
          }
          actions={
            ready ? (
              <Button tone="secondary" onClick={reloadLive} busy={liveLoading}>
                <RefreshCw className="h-4 w-4" aria-hidden />
                Re-read live data
              </Button>
            ) : null
          }
        />
        {sitesLoading ? (
          <Loading label="Loading installations" />
        ) : sites.length === 0 ? (
          <EmptyState
            title="No WordPress installations are registered"
            action={
              <Link
                href="/wp-toolkit/add"
                className="inline-flex items-center gap-2 rounded-md border border-transparent bg-brand-600 px-3 py-2 text-sm font-medium text-white hover:bg-brand-700"
              >
                Add WordPress
              </Link>
            }
          >
            Nothing has been added to the panel yet. Adding an installation registers it here and, on
            a panel with the WP-CLI routes mounted, installs WordPress on disk.
          </EmptyState>
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Site</Th>
                <Th>WordPress</Th>
                <Th>PHP</Th>
                <Th>Runs as</Th>
                <Th>Plugins</Th>
                <Th>Themes</Th>
                <Th>Status</Th>
                <Th className="text-right">Update</Th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {sites.map((site) => {
                const summary = summaries[site.id];
                const live = summary?.runtime;
                const recordVersion = site.version || '';
                const liveVersion = live?.installed_wordpress_version || '';
                // Why a live figure is missing, in the order the causes matter:
                // the routes are not there at all, or they are and this
                // particular installation could not be read.
                const missingReason = runtimeOff ?? summary?.liveError ?? NO_LIVE_DATA;
                return (
                  <tr key={site.id} className="hover:bg-gray-50">
                    <Td>
                      <div className="font-medium text-gray-900">{site.domain || site.name || site.id}</div>
                      <div className="mt-0.5 flex flex-wrap items-center gap-2 text-xs text-gray-500">
                        {site.name && site.name !== site.domain ? <span>{site.name}</span> : null}
                        {site.path ? <code className="font-mono">{site.path}</code> : null}
                        {site.domain ? (
                          <a
                            href={`http://${site.domain}`}
                            target="_blank"
                            rel="noreferrer"
                            className="inline-flex items-center gap-1 text-brand-600 hover:text-brand-700"
                          >
                            Open <ExternalLink className="h-3 w-3" aria-hidden />
                          </a>
                        ) : null}
                      </div>
                    </Td>
                    <Td>
                      {liveVersion ? (
                        <span className="text-gray-900">{liveVersion}</span>
                      ) : recordVersion ? (
                        <span
                          className="text-gray-600"
                          title="From the panel record, not read from the installation."
                        >
                          {recordVersion}
                          <span className="ml-1 text-xs text-gray-400">(recorded)</span>
                        </span>
                      ) : (
                        <Unknown reason={recordVersion ? missingReason : 'The panel has no version for this installation.'} />
                      )}
                    </Td>
                    <Td>
                      {live?.php_version ? (
                        live.php_version
                      ) : (
                        <Unknown reason={missingReason} />
                      )}
                    </Td>
                    <Td>
                      {live?.run_as_user ? (
                        <code className="font-mono text-xs">
                          {live.run_as_user}
                          {live.run_as_group ? `:${live.run_as_group}` : ''}
                        </code>
                      ) : (
                        <Unknown reason={missingReason} />
                      )}
                    </Td>
                    <Td>
                      <CountCell items={summary?.plugins ?? null} reason={missingReason} />
                    </Td>
                    <Td>
                      <CountCell items={summary?.themes ?? null} reason={missingReason} />
                    </Td>
                    <Td>
                      <Badge tone={statusTone(site.status)}>{site.status || 'unknown'}</Badge>
                      {site.auto_update ? (
                        <span className="ml-2">
                          <Badge tone="sky">auto-update</Badge>
                        </span>
                      ) : null}
                    </Td>
                    <Td className="text-right">
                      <div className="inline-flex flex-wrap justify-end gap-2">
                        <Button
                          tone="secondary"
                          unavailableReason={runtimeOff}
                          busy={busy === `${site.id}:core`}
                          onClick={() =>
                            runAction(`${site.id}:core`, `Updating WordPress core on ${siteLabel(site)}`, () =>
                              runtimeApi.updateCore(site.id),
                            )
                          }
                        >
                          Core
                        </Button>
                        <Button
                          tone="secondary"
                          unavailableReason={runtimeOff}
                          busy={busy === `${site.id}:plugins`}
                          onClick={() =>
                            runAction(`${site.id}:plugins`, `Updating plugins on ${siteLabel(site)}`, () =>
                              runtimeApi.updatePlugins(site.id),
                            )
                          }
                        >
                          Plugins
                        </Button>
                        <Button
                          tone="secondary"
                          unavailableReason={runtimeOff}
                          busy={busy === `${site.id}:themes`}
                          onClick={() =>
                            runAction(`${site.id}:themes`, `Updating themes on ${siteLabel(site)}`, () =>
                              runtimeApi.updateThemes(site.id),
                            )
                          }
                        >
                          Themes
                        </Button>
                      </div>
                    </Td>
                  </tr>
                );
              })}
            </tbody>
          </Table>
        )}
      </Card>

      <Card>
        <CardHeader
          title="Out of date"
          description="Every plugin and theme WP-CLI reports as having an update waiting, across all installations."
        />
        {!ready ? (
          <EmptyState title="Nothing has been checked">
            {NO_LIVE_DATA} An installation whose plugins have never been checked is not the same as
            one with nothing to update, so this list stays empty rather than claiming everything is
            current.
          </EmptyState>
        ) : liveLoading && pending.length === 0 ? (
          <Loading label="Reading plugin and theme versions" />
        ) : pending.length === 0 ? (
          <EmptyState title="Nothing is waiting for an update">
            Every plugin and theme WP-CLI could check is on its current version.
          </EmptyState>
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Installation</Th>
                <Th>Kind</Th>
                <Th>Name</Th>
                <Th>Installed</Th>
                <Th>Available</Th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {pending.map((row) => (
                <tr key={`${row.site.id}:${row.kind}:${row.name}`} className="hover:bg-gray-50">
                  <Td className="text-gray-900">{row.site.domain || row.site.name || row.site.id}</Td>
                  <Td>{row.kind}</Td>
                  <Td className="font-mono text-xs text-gray-900">{row.name}</Td>
                  <Td>{row.from || <Unknown reason="WP-CLI did not report an installed version." />}</Td>
                  <Td>
                    {row.to ? (
                      <Badge tone="amber">{row.to}</Badge>
                    ) : (
                      <Unknown reason="WP-CLI reported an update but not its version number." />
                    )}
                  </Td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>

      <Card>
        <CardHeader
          title="What the plugin and theme record endpoints actually do"
          description="A note about two routes this screen deliberately does not use."
        />
        <CardBody>
          <p className="text-sm text-gray-600">
            <code className="rounded bg-gray-50 px-1 py-0.5 font-mono text-xs">
              POST /api/v1/wordpress/:id/plugins
            </code>{' '}
            and its theme twin write a row into the panel database and return it. They do not reach
            the installation, download anything or run WP-CLI, even though the server logs the line
            &quot;WordPress plugin installed&quot;. This section does not offer them as an install
            button, because a button that records a plugin nobody installed leaves the panel
            confidently wrong. Installing a plugin for real needs a route that calls
            wpcli.Client.InstallPlugin, which is written and is not mounted anywhere.
          </p>
        </CardBody>
      </Card>
    </div>
  );
}

export default InstallationsOverview;
