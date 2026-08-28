'use client';

/**
 * Overview: the numbers an operator opens this screen to read.
 *
 * The one that matters most is reclaimable space. Nobody visits Docker's
 * overview to admire a container count; they visit it because a disk is filling
 * and they want to know how much of it is dead layers, stopped containers and
 * dangling volumes.
 *
 * Today none of those numbers exist. `GetSummary` in
 * core/internal/handler/docker.go returns five hardcoded zeros and no disk
 * figures at all. So the tiles render the unavailable dash carrying that
 * reason, not the zeros. A zero and an unmeasured value look identical on a
 * dashboard, and an operator who cannot tell them apart cannot trust any tile
 * on the page - which is precisely how "Docker is using 0 GB" gets believed by
 * someone whose disk is at 96%.
 */

import { useCallback } from 'react';

import { MetricText } from '@/components/Unavailable';
import { api, unwrap } from '@/services/api';
import type { CapabilityGap, DockerSummary } from '@/types/docker';

import { CapabilityGaps } from './CapabilityNotice';
import { DaemonStatusCard, useDaemonStatus } from './DaemonStatus';
import { formatBytes, formatCount } from './format';
import { ErrorBlock, Panel, SectionHeader, TileSkeleton } from './PaneChrome';
import { useAsyncResource } from './useDockerData';

/**
 * The summary handler is a literal, so nothing it returns may be printed as a
 * measurement. Every tile carries this as its reason instead.
 */
const STUB_REASON =
  'GetSummary in core/internal/handler/docker.go returns hardcoded zeros and is not connected to a Docker daemon.';

/** For a figure the response has no field for at all, stub or not. */
const NO_FIELD_REASON =
  'GET /api/v1/docker/summary does not return this field at all; there is no `docker system df` call in the backend.';

/**
 * The single switch for this pane. While it is true the counts below are shown
 * as unmeasured whatever the API sends, because what the API sends is a
 * constant. Set it to false in the same change that makes GetSummary count
 * something, and the tiles start reading the response.
 */
const SUMMARY_IS_STUB = true;

/** Reads a count from the response, unless the response is known to be fiction. */
function counted(value: number | null | undefined): string | null {
  if (SUMMARY_IS_STUB) return null;
  return formatCount(value ?? null);
}

const OVERVIEW_GAPS: CapabilityGap[] = [
  {
    label: 'Object counts',
    missing:
      'GET /api/v1/docker/summary answers with a fixed {running_containers: 0, stopped_containers: 0, total_images: 0, total_volumes: 0, total_networks: 0}. It needs to count what the daemon actually reports.',
  },
  {
    label: 'Disk used by Docker, and the reclaimable part',
    missing:
      'No `docker system df` equivalent exists anywhere in core/. The summary response carries no size fields, so images, containers, volumes and build cache cannot be sized or split into used and reclaimable.',
  },
  {
    label: 'Engine version and API version',
    missing:
      'Nothing calls the daemon `/version` endpoint. core/internal/docker/ is an empty directory and neither core/go.mod nor agent/go.mod depends on a Docker client library.',
  },
  {
    label: 'Prune',
    missing:
      'No route exists for pruning containers, images, networks, volumes or build cache, so the reclaimable figure would have nothing to act on even once it is measured.',
  },
];

function StatTile({
  label,
  value,
  reason,
  hint,
}: {
  label: string;
  value: string | null;
  reason: string;
  hint?: string;
}) {
  return (
    <div className="rounded-lg border border-gray-200 bg-white p-4 shadow-sm">
      <p className="text-xs font-semibold uppercase tracking-wide text-gray-500">{label}</p>
      <p className="mt-2 text-2xl font-semibold text-gray-900">
        <MetricText value={value} reason={reason} className="text-2xl" />
      </p>
      {hint && <p className="mt-1 text-xs text-gray-500">{hint}</p>}
    </div>
  );
}

export function OverviewPane() {
  const daemon = useDaemonStatus();

  const loadSummary = useCallback(async () => {
    const res = await api.get('/api/v1/docker/summary');
    return unwrap<Partial<DockerSummary>>(res, null);
  }, []);

  const summary = useAsyncResource<Partial<DockerSummary> | null>(
    loadSummary,
    null,
    'Could not read the Docker summary.'
  );

  return (
    <div className="space-y-4">
      <DaemonStatusCard resource={daemon} />

      <Panel>
        <SectionHeader
          title="Docker objects and disk"
          description="What is on this host and how much of it can be freed."
        />
        <div className="px-4 py-4">
          {summary.loading ? (
            <TileSkeleton tiles={8} />
          ) : summary.error ? (
            <ErrorBlock
              title="Could not read the Docker summary"
              message={summary.error}
              onRetry={summary.reload}
            />
          ) : (
            <>
              <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                <StatTile
                  label="Containers running"
                  value={counted(summary.data?.running_containers)}
                  reason={STUB_REASON}
                  hint="Not measured yet"
                />
                <StatTile
                  label="Containers stopped"
                  value={counted(summary.data?.stopped_containers)}
                  reason={STUB_REASON}
                  hint="Not measured yet"
                />
                <StatTile
                  label="Images"
                  value={counted(summary.data?.total_images)}
                  reason={STUB_REASON}
                  hint="Not measured yet"
                />
                <StatTile
                  label="Networks"
                  value={counted(summary.data?.total_networks)}
                  reason={STUB_REASON}
                  hint="Not measured yet"
                />
                <StatTile
                  label="Volumes"
                  value={counted(summary.data?.total_volumes)}
                  reason={STUB_REASON}
                  hint="Not measured yet"
                />
                <StatTile
                  label="Disk used by Docker"
                  value={formatBytes(summary.data?.disk_used_bytes ?? null)}
                  reason={NO_FIELD_REASON}
                  hint="Images, containers, volumes and build cache"
                />
                <StatTile
                  label="Reclaimable"
                  value={formatBytes(summary.data?.reclaimable_bytes ?? null)}
                  reason={NO_FIELD_REASON}
                  hint="What a full prune would free"
                />
                <StatTile
                  label="Engine version"
                  value={summary.data?.server_version ?? null}
                  reason={NO_FIELD_REASON}
                  hint="Reported by the daemon, not by systemd"
                />
              </div>
              <p className="mt-4 text-sm text-gray-600">
                Every figure above reads{' '}
                <span className="text-gray-400">&mdash;</span> because it is not measured yet,
                not because it is zero. Hover any dash for the reason. The daemon state above
                is the only Docker fact on this page the backend actually collects.
              </p>
            </>
          )}
        </div>
      </Panel>

      <CapabilityGaps
        title="What Overview needs from the backend"
        intro="These are the endpoints that would fill the tiles above. Each is missing, not broken."
        gaps={OVERVIEW_GAPS}
      />
    </div>
  );
}

export default OverviewPane;
