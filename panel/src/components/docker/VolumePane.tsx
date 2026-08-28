'use client';

/**
 * Volume: list, mountpoint, and - the column that matters - the containers with
 * this volume mounted.
 *
 * Removing a volume a container still uses is a data-loss event, and it is the
 * one action on this whole screen that cannot be undone by pulling the image
 * again. The panel is not allowed to offer that removal without showing what is
 * attached, so while the backend reports no attachments the removal is not
 * offered at all.
 */

import { useCallback } from 'react';

import { AlertTriangle } from 'lucide-react';

import { Badge } from '@/components/ui/badge';
import { MetricText } from '@/components/Unavailable';
import { dockerApi, unwrapList } from '@/services/api';
import type { CapabilityGap, DockerVolume } from '@/types/docker';

import { formatBytes, formatDateTime, matches } from './format';
import { ListPane, type ListColumn } from './ListPane';
import { TD_CLASS } from './PaneChrome';
import { useAsyncResource } from './useDockerData';

const COLUMNS: ListColumn[] = [
  { key: 'name', label: 'Name' },
  { key: 'driver', label: 'Driver' },
  { key: 'mountpoint', label: 'Mount point' },
  { key: 'size', label: 'Size' },
  { key: 'containers', label: 'Mounted by' },
  { key: 'created', label: 'Created' },
];

const GAPS: CapabilityGap[] = [
  {
    label: 'The list itself',
    missing:
      'GET /api/v1/docker/volumes is mounted but ListVolumes returns an empty slice - no mountpoint, no driver, no labels.',
  },
  {
    label: 'Containers mounting a volume',
    missing:
      'Nothing joins volumes to containers. This is the blocker for offering removal at all: a delete button that cannot name what is using the volume is a delete button that loses data.',
  },
  {
    label: 'Volume size',
    missing:
      'No `docker system df -v` equivalent, so a volume’s disk footprint is unknown and cannot be shown before a removal.',
  },
  {
    label: 'Create a volume',
    missing:
      'POST /api/v1/docker/volumes accepts a name and driver, logs them and answers 201 without creating anything. It has no driver-options or labels either.',
  },
  {
    label: 'Inspect',
    missing: 'No route. There is no GET /api/v1/docker/volumes/:id.',
  },
  {
    label: 'Remove a volume',
    missing:
      'DELETE /api/v1/docker/volumes/:id answers "Volume deleted" without removing anything.',
  },
];

export function VolumePane() {
  const load = useCallback(async () => {
    const res = await dockerApi.listVolumes();
    return unwrapList<DockerVolume>(res);
  }, []);

  const resource = useAsyncResource<DockerVolume[]>(load, [], 'Could not load volumes.');

  const filter = useCallback(
    (row: DockerVolume, term: string) =>
      matches(row.name, term) || matches(row.driver, term) || matches(row.mountpoint, term),
    []
  );

  return (
    <ListPane<DockerVolume>
      title="Volumes"
      description="Named volumes on this host, with the containers holding them open."
      searchPlaceholder="Search by name, driver or mount point"
      columns={COLUMNS}
      resource={resource}
      rowKey={(row) => row.id || row.name}
      filter={filter}
      errorTitle="Could not load volumes"
      emptyTitle="No volumes on this host"
      emptyDescription="A container that declares a volume creates one on first start, and it appears here."
      stub={{
        resource: 'volumes',
        handler: 'ListVolumes',
        detail: 'it returns an empty slice and never asks a Docker daemon what exists.',
      }}
      gapsTitle="What the Volume tab needs from the backend"
      gapsIntro="Removal is deliberately not offered, and will stay unoffered until the API reports which containers hold a volume open."
      gaps={GAPS}
      renderRow={(row) => (
        <>
          <td className={`${TD_CLASS} font-medium text-gray-900`}>{row.name}</td>
          <td className={TD_CLASS}>
            <Badge variant="neutral">{row.driver || 'local'}</Badge>
          </td>
          <td className={TD_CLASS}>
            <MetricText
              value={row.mountpoint || null}
              reason="The backend did not report a mount point for this volume."
              className="font-mono text-xs"
            />
          </td>
          <td className={TD_CLASS}>
            <MetricText
              value={formatBytes(row.size)}
              reason="Volume sizes need a `docker system df -v` equivalent, which the backend does not have."
            />
          </td>
          <td className={TD_CLASS}>
            {row.containers && row.containers.length > 0 ? (
              <span className="inline-flex items-center gap-1.5 text-gray-900">
                <AlertTriangle className="h-3.5 w-3.5 shrink-0 text-amber-600" aria-hidden="true" />
                {row.containers.join(', ')}
              </span>
            ) : (
              <MetricText
                value={null}
                reason="The backend does not report volume mounts. This is not 'no container is using it' - it is unknown, which is why removal is not offered."
              />
            )}
          </td>
          <td className={TD_CLASS}>
            <MetricText
              value={formatDateTime(row.created_at)}
              reason="The backend did not report a creation time."
            />
          </td>
        </>
      )}
    />
  );
}

export default VolumePane;
