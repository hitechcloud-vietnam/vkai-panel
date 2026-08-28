'use client';

/**
 * Network: list, and the containers attached to each.
 *
 * The attachment column is not decoration. Removing a network that a container
 * still sits on breaks that container's addressing, and the daemon's refusal
 * arrives as a generic error somewhere else in the panel. Showing the
 * attachments in the row is what turns a surprise into a decision.
 *
 * Create and remove are not offered: both routes exist, both answer success,
 * neither touches a daemon.
 */

import { useCallback } from 'react';

import { Badge } from '@/components/ui/badge';
import { MetricText } from '@/components/Unavailable';
import { dockerApi, unwrapList } from '@/services/api';
import type { CapabilityGap, DockerNetwork } from '@/types/docker';

import { formatDateTime, matches } from './format';
import { ListPane, type ListColumn } from './ListPane';
import { TD_CLASS } from './PaneChrome';
import { useAsyncResource } from './useDockerData';

const COLUMNS: ListColumn[] = [
  { key: 'name', label: 'Name' },
  { key: 'driver', label: 'Driver' },
  { key: 'scope', label: 'Scope' },
  { key: 'subnet', label: 'Subnet' },
  { key: 'gateway', label: 'Gateway' },
  { key: 'containers', label: 'Attached containers' },
  { key: 'created', label: 'Created' },
];

const GAPS: CapabilityGap[] = [
  {
    label: 'The list itself',
    missing:
      'GET /api/v1/docker/networks is mounted but ListNetworks returns an empty slice - no driver, no IPAM, no attachments.',
  },
  {
    label: 'Attached containers',
    missing:
      'Nothing reads a network’s Containers map, so the panel cannot warn before a removal that would cut a running container off its network.',
  },
  {
    label: 'Create a network',
    missing:
      'POST /api/v1/docker/networks accepts a name and driver, logs them and answers 201 without creating anything. It also has no subnet, gateway, IP range or internal/attachable options.',
  },
  {
    label: 'Inspect',
    missing: 'No route. There is no GET /api/v1/docker/networks/:id.',
  },
  {
    label: 'Remove a network',
    missing:
      'DELETE /api/v1/docker/networks/:id answers "Network deleted" without removing anything.',
  },
];

export function NetworkPane() {
  const load = useCallback(async () => {
    const res = await dockerApi.listNetworks();
    return unwrapList<DockerNetwork>(res);
  }, []);

  const resource = useAsyncResource<DockerNetwork[]>(load, [], 'Could not load networks.');

  const filter = useCallback(
    (row: DockerNetwork, term: string) =>
      matches(row.name, term) || matches(row.driver, term) || matches(row.subnet, term),
    []
  );

  return (
    <ListPane<DockerNetwork>
      title="Networks"
      description="Docker networks on this host and what is attached to them."
      searchPlaceholder="Search by name, driver or subnet"
      columns={COLUMNS}
      resource={resource}
      rowKey={(row) => row.id || row.name}
      filter={filter}
      errorTitle="Could not load networks"
      emptyTitle="No networks reported"
      emptyDescription="A Docker host always has bridge, host and none. Seeing nothing here means the panel is not reading the daemon."
      stub={{
        resource: 'networks',
        handler: 'ListNetworks',
        detail:
          'it returns an empty slice. A real Docker host always has at least bridge, host and none, so an empty list here can only mean the panel is not looking.',
      }}
      gapsTitle="What the Network tab needs from the backend"
      gapsIntro="Create, inspect and remove are not offered while their handlers answer success without acting."
      gaps={GAPS}
      renderRow={(row) => (
        <>
          <td className={`${TD_CLASS} font-medium text-gray-900`}>{row.name}</td>
          <td className={TD_CLASS}>
            <Badge variant="neutral">{row.driver || 'unknown'}</Badge>
          </td>
          <td className={TD_CLASS}>{row.scope || '-'}</td>
          <td className={TD_CLASS}>
            <MetricText
              value={row.subnet || null}
              reason="The backend did not report an IPAM subnet for this network."
              className="font-mono text-xs"
            />
          </td>
          <td className={TD_CLASS}>
            <MetricText
              value={row.gateway || null}
              reason="The backend did not report a gateway for this network."
              className="font-mono text-xs"
            />
          </td>
          <td className={TD_CLASS}>
            {row.containers && row.containers.length > 0 ? (
              <span className="text-gray-900">{row.containers.join(', ')}</span>
            ) : (
              <MetricText
                value={null}
                reason="The backend does not report network attachments, so this cannot be shown as 'none' either."
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

export default NetworkPane;
