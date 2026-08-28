'use client';

/**
 * Statistics - one card per installation, with the figures an operator checks
 * before they decide whether a site needs their attention today.
 *
 * The discipline of this screen is that every figure says where it came from,
 * and a figure the panel cannot obtain renders as a dash with the reason on it.
 * Four of the requested figures are available and two are not, and pretending
 * otherwise would make the four that are true unusable, because nobody can tell
 * which is which.
 *
 * Where each one comes from:
 *
 *   WordPress version   live from WP-CLI, falling back to the panel record with
 *                       the fallback labelled.
 *   PHP version         the runtime row, when the runtime routes are mounted.
 *   Plugins and themes  live lists, counting only update == "available" as out
 *                       of date. "unavailable" means WP-CLI could not check and
 *                       is reported separately.
 *   Disk use            NOT AVAILABLE. See the note at the foot of the page.
 *   Last backup         derived from the backup jobs and records the panel
 *                       already keeps, matched to the installation's website.
 *   Security warnings   derived from pending updates, which are real security
 *                       warnings. A WordPress vulnerability scan is not
 *                       available - see the note.
 */

import { useEffect, useMemo, useState } from 'react';
import { RefreshCw } from 'lucide-react';
import { errorMessage, supporting } from './api';
import { countUpdates, useLiveReadings, useRuntimeAvailability, useSites } from './hooks';
import { RuntimeBanner } from './RuntimeBanner';
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
  Unknown,
} from './ui';
import type { Tone } from './ui';
import { formatBytes } from '@/lib/utils';
import type { WordPressSite } from '@/types/wordpress';

const NO_LIVE = 'Not read: this needs the WP-CLI routes, which are not mounted on this panel.';

const NO_DISK_USE =
  'The panel has no endpoint that measures the size of one WordPress directory. ' +
  'GET /api/v1/files/disk-usage reports free space on a filesystem, which is a different number ' +
  'and would be wrong here.';

const NO_VULN_SCAN =
  'The panel stores security vulnerabilities against a scan, not against a WordPress ' +
  'installation, and nothing scans WordPress. Matching a finding to a site by its text would be a guess.';

interface BackupFact {
  at: string;
  size: number;
  status: string;
}

/**
 * Find the most recent completed backup for each installation.
 *
 * The link is indirect and worth stating: a backup job carries a type and a
 * resource_id, and a WordPress installation carries a website_id. A job of type
 * "website" whose resource_id is that website_id is this site's backup job.
 * An installation with no website_id cannot be matched at all, and says so
 * rather than reporting "never backed up".
 */
function useLastBackups(sites: WordPressSite[]) {
  const [byId, setById] = useState<Record<string, BackupFact | null>>({});
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const idsKey = sites.map((s) => s.id).join(',');

  useEffect(() => {
    if (sites.length === 0) return;
    let cancelled = false;
    setLoading(true);
    setError(null);
    (async () => {
      try {
        const [jobs, runs] = await Promise.all([supporting.backupJobs(), supporting.backupRecords()]);
        if (cancelled) return;
        const jobByResource = new Map<string, string>();
        for (const job of jobs) {
          if ((job.type || '').toLowerCase() === 'website' && job.resource_id) {
            jobByResource.set(job.resource_id, job.id);
          }
        }
        const latestByJob = new Map<string, BackupFact>();
        for (const run of runs) {
          if (!run.job_id) continue;
          const when = run.completed_at || run.started_at;
          if (!when) continue;
          const current = latestByJob.get(run.job_id);
          if (!current || when > current.at) {
            latestByJob.set(run.job_id, {
              at: when,
              size: typeof run.size === 'number' ? run.size : 0,
              status: run.status || 'unknown',
            });
          }
        }
        const result: Record<string, BackupFact | null> = {};
        for (const site of sites) {
          const jobId = site.website_id ? jobByResource.get(site.website_id) : undefined;
          result[site.id] = jobId ? latestByJob.get(jobId) ?? null : null;
        }
        setById(result);
      } catch (err) {
        if (!cancelled) setError(errorMessage(err, 'Backup history could not be read.'));
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [idsKey]);

  return { byId, error, loading };
}

function Figure({
  label,
  value,
  reason,
  tone,
  note,
}: {
  label: string;
  value: string | null;
  reason?: string;
  tone?: Tone;
  note?: string;
}) {
  return (
    <div className="rounded-md border border-gray-200 px-4 py-3">
      <p className="text-xs font-medium uppercase tracking-wide text-gray-500">{label}</p>
      <p className="mt-1 text-sm font-medium text-gray-900">
        {value === null ? (
          <Unknown reason={reason || 'Not available.'} />
        ) : tone ? (
          <Badge tone={tone}>{value}</Badge>
        ) : (
          value
        )}
      </p>
      {note ? <p className="mt-1 text-xs text-gray-500">{note}</p> : null}
    </div>
  );
}

export function Statistics() {
  const { availability, ready, recheck } = useRuntimeAvailability();
  const { sites, loading: sitesLoading, error: sitesError } = useSites();
  const { summaries, loading: liveLoading, reload } = useLiveReadings(sites, ready);
  const backups = useLastBackups(sites);

  const rows = useMemo(
    () =>
      sites.map((site) => {
        const summary = summaries[site.id];
        const pluginCounts = countUpdates(summary?.plugins ?? null);
        const themeCounts = countUpdates(summary?.themes ?? null);
        const warnings: string[] = [];
        if (pluginCounts && pluginCounts.outdated > 0) {
          warnings.push(
            `${pluginCounts.outdated} plugin${pluginCounts.outdated === 1 ? '' : 's'} out of date`,
          );
        }
        if (themeCounts && themeCounts.outdated > 0) {
          warnings.push(`${themeCounts.outdated} theme${themeCounts.outdated === 1 ? '' : 's'} out of date`);
        }
        if (pluginCounts && pluginCounts.unknown > 0) {
          warnings.push(`${pluginCounts.unknown} plugins could not be checked`);
        }
        if (!site.auto_update) {
          warnings.push('automatic updates are not recorded as enabled');
        }
        const backup = backups.byId[site.id];
        if (site.website_id && backup === null) {
          warnings.push('no completed backup has been recorded');
        }
        return { site, summary, pluginCounts, themeCounts, warnings, backup };
      }),
    [sites, summaries, backups.byId],
  );

  return (
    <div className="space-y-5">
      <PageHeader
        title="Statistics"
        description="What each WordPress installation is running, what is out of date on it, and when it was last backed up."
        actions={
          ready ? (
            <Button tone="secondary" busy={liveLoading} onClick={reload}>
              <RefreshCw className="h-4 w-4" aria-hidden />
              Re-read live data
            </Button>
          ) : null
        }
      />

      <RuntimeBanner availability={availability} onRecheck={recheck} />

      {sitesError ? <ErrorNote>{sitesError}</ErrorNote> : null}
      {backups.error ? <ErrorNote>{backups.error}</ErrorNote> : null}

      {sitesLoading ? (
        <Card>
          <Loading label="Loading installations" />
        </Card>
      ) : sites.length === 0 ? (
        <Card>
          <EmptyState title="No WordPress installations are registered">
            There is nothing to report on yet.
          </EmptyState>
        </Card>
      ) : (
        rows.map(({ site, summary, pluginCounts, themeCounts, warnings, backup }) => {
          const live = summary?.runtime;
          const liveVersion = live?.installed_wordpress_version || '';
          const recordVersion = site.version || '';
          // The reason a live figure is missing, most specific first.
          const missingReason = !ready ? NO_LIVE : summary?.liveError ?? NO_LIVE;
          return (
            <Card key={site.id}>
              <CardHeader
                title={site.domain || site.name || site.id}
                description={site.path ? <code className="font-mono text-xs">{site.path}</code> : undefined}
                actions={
                  warnings.length > 0 ? (
                    <Badge tone="amber">
                      {warnings.length} warning{warnings.length === 1 ? '' : 's'}
                    </Badge>
                  ) : ready ? (
                    <Badge tone="emerald">nothing outstanding</Badge>
                  ) : null
                }
              />
              <CardBody className="space-y-4">
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
                  <Figure
                    label="WordPress version"
                    value={liveVersion || recordVersion || null}
                    reason={missingReason}
                    note={
                      liveVersion
                        ? 'Read from the installation.'
                        : recordVersion
                          ? 'From the panel record, not read from the installation.'
                          : undefined
                    }
                  />
                  <Figure
                    label="PHP version"
                    value={live?.php_version || null}
                    reason={missingReason}
                  />
                  <Figure
                    label="Runs as"
                    value={live?.run_as_user ? `${live.run_as_user}${live.run_as_group ? `:${live.run_as_group}` : ''}` : null}
                    reason={missingReason}
                  />
                  <Figure
                    label="Plugins"
                    value={
                      pluginCounts
                        ? `${pluginCounts.total} installed, ${pluginCounts.outdated} out of date`
                        : null
                    }
                    reason={missingReason}
                    note={
                      pluginCounts && pluginCounts.unknown > 0
                        ? `${pluginCounts.unknown} could not be checked against wordpress.org.`
                        : undefined
                    }
                  />
                  <Figure
                    label="Themes"
                    value={
                      themeCounts ? `${themeCounts.total} installed, ${themeCounts.outdated} out of date` : null
                    }
                    reason={missingReason}
                  />
                  <Figure label="Disk use" value={null} reason={NO_DISK_USE} />
                  <Figure
                    label="Last backup"
                    value={backup ? new Date(backup.at).toLocaleString() : null}
                    reason={
                      !site.website_id
                        ? 'This installation is not linked to a website, so no backup job can be matched to it.'
                        : backups.loading
                          ? 'Reading backup history.'
                          : 'No completed backup run is recorded for the backup job that targets this website.'
                    }
                    note={backup ? `${formatBytes(backup.size)}, ${backup.status}` : undefined}
                  />
                  <Figure
                    label="Last WP-CLI command"
                    value={live?.last_command ? `${live.last_command}${live.last_ran_at ? ` at ${live.last_ran_at}` : ''}` : null}
                    reason={live ? 'No command has been run against this installation yet.' : missingReason}
                  />
                  <Figure label="Vulnerability scan" value={null} reason={NO_VULN_SCAN} />
                </div>

                {warnings.length > 0 ? (
                  <Notice tone="amber" title="Security warnings">
                    <ul className="list-disc space-y-1 pl-5">
                      {warnings.map((warning) => (
                        <li key={warning}>{warning}</li>
                      ))}
                    </ul>
                  </Notice>
                ) : null}
              </CardBody>
            </Card>
          );
        })
      )}

      <Card>
        <CardHeader title="Two figures this panel cannot report yet" />
        <CardBody className="space-y-3 text-sm text-gray-600">
          <p>
            <span className="font-medium text-gray-900">Disk use per installation.</span> {NO_DISK_USE}{' '}
            It would need an endpoint along the lines of{' '}
            <code className="rounded bg-gray-50 px-1 py-0.5 font-mono text-xs">
              GET /api/v1/wordpress/:id/disk
            </code>{' '}
            returning the size of the WordPress root, the uploads directory and the database
            separately - three numbers an operator acts on differently.
          </p>
          <p>
            <span className="font-medium text-gray-900">WordPress security warnings.</span>{' '}
            {NO_VULN_SCAN} The warnings above are derived from what is genuinely known: pending core,
            plugin and theme updates, which are the most common way a WordPress site is compromised.
            A real scan would need{' '}
            <code className="rounded bg-gray-50 px-1 py-0.5 font-mono text-xs">
              POST /api/v1/wordpress/:id/scan
            </code>{' '}
            checking file permissions, the presence of wp-config.php outside the web root, exposed
            debug logs, the admin username, and installed plugin versions against a vulnerability
            feed.
          </p>
        </CardBody>
      </Card>
    </div>
  );
}

export default Statistics;
