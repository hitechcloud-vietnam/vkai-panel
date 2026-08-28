'use client';

/**
 * Container: name, image, state, ports, CPU and memory now, uptime.
 *
 * The operations aaPanel offers here - start, stop, restart, logs, exec,
 * inspect, remove - are deliberately absent rather than present and disabled.
 * All seven routes are mounted and all seven are stubs: StartContainer logs a
 * line and returns `{"message": "Container started"}` without touching a
 * daemon. A button that reports success for work that never happened is worse
 * than a missing button, because an operator believes it: they see "Container
 * stopped", walk away, and the container is still serving traffic.
 *
 * The confirm-before-removing-a-running-container step is specified and will be
 * built with the route. It is listed in the gaps below so it is not lost.
 */

import { useCallback } from 'react';

import { Badge } from '@/components/ui/badge';
import { MetricText } from '@/components/Unavailable';
import { dockerApi, unwrapList } from '@/services/api';
import type { CapabilityGap, DockerContainer } from '@/types/docker';

import {
  containerStateVariant,
  formatBytes,
  formatPercent,
  formatUptime,
  matches,
} from './format';
import { ListPane, type ListColumn } from './ListPane';
import { TD_CLASS } from './PaneChrome';
import { useAsyncResource } from './useDockerData';

const COLUMNS: ListColumn[] = [
  { key: 'name', label: 'Name' },
  { key: 'image', label: 'Image' },
  { key: 'state', label: 'State' },
  { key: 'ports', label: 'Ports' },
  { key: 'ip', label: 'IP' },
  { key: 'cpu', label: 'CPU' },
  { key: 'memory', label: 'Memory' },
  { key: 'uptime', label: 'Uptime' },
];

const GAPS: CapabilityGap[] = [
  {
    label: 'The list itself',
    missing:
      'GET /api/v1/docker/containers is mounted but ListContainers returns an empty slice with a "TODO: integrate with Docker SDK / agent" comment.',
  },
  {
    label: 'Start, stop, restart',
    missing:
      'The three POST routes exist but StartContainer, StopContainer and RestartContainer only write a log line and answer with a success message. Nothing reaches a daemon.',
  },
  {
    label: 'Remove, with a confirmation for a running container',
    missing:
      'DELETE /api/v1/docker/containers/:id answers "Container deleted" without deleting anything. The confirmation step is a UI concern and will be added with the route; it needs the container state, which the list does not report yet.',
  },
  {
    label: 'Logs',
    missing:
      'No route. There is no /docker/containers/:id/logs in core/internal/handler/router.go, and no streaming channel for it.',
  },
  {
    label: 'Exec into a container',
    missing:
      'No route. The panel has a terminal WebSocket for the host but nothing that attaches to a container.',
  },
  {
    label: 'Inspect',
    missing:
      'GET /api/v1/docker/containers/:id exists but GetContainer echoes back only the id it was given.',
  },
  {
    label: 'CPU and memory now',
    missing:
      'No stats route and no stats field on the list response, so live per-container usage cannot be shown even for a container the list eventually returns.',
  },
];

export function ContainerPane() {
  const load = useCallback(async () => {
    const res = await dockerApi.listContainers();
    return unwrapList<DockerContainer>(res);
  }, []);

  const resource = useAsyncResource<DockerContainer[]>(
    load,
    [],
    'Could not load containers.'
  );

  const filter = useCallback(
    (row: DockerContainer, term: string) =>
      matches(row.name, term) ||
      matches(row.image, term) ||
      matches(row.state, term) ||
      matches(row.ports, term) ||
      matches(row.ip, term),
    []
  );

  return (
    <ListPane<DockerContainer>
      title="Containers"
      description="Every container on this host, running or not."
      searchPlaceholder="Search by name, image, state, port or IP"
      columns={COLUMNS}
      resource={resource}
      rowKey={(row) => row.id || row.name}
      filter={filter}
      errorTitle="Could not load containers"
      emptyTitle="No containers on this host"
      emptyDescription="Pull an image from Cloud image, or bring up a project from Docker Compose, and its containers appear here."
      stub={{
        resource: 'containers',
        handler: 'ListContainers',
        detail:
          'it returns an empty slice and never calls a Docker daemon.',
      }}
      gapsTitle="What the Container tab needs from the backend"
      gapsIntro="Every operation aaPanel offers on this tab is a mounted route with an empty handler. None is wired here, because each one would report success for work that never happened."
      gaps={GAPS}
      renderRow={(row) => (
        <>
          <td className={`${TD_CLASS} font-medium text-gray-900`}>{row.name}</td>
          <td className={TD_CLASS}>
            <span className="font-mono text-xs">{row.image}</span>
          </td>
          <td className={TD_CLASS}>
            <Badge variant={containerStateVariant(row.state)}>{row.state || 'unknown'}</Badge>
            {row.status && <div className="mt-1 text-xs text-gray-500">{row.status}</div>}
          </td>
          <td className={TD_CLASS}>
            <MetricText
              value={row.ports || null}
              reason="This container publishes no ports, or the backend did not report its port map."
              className="font-mono text-xs"
            />
          </td>
          <td className={TD_CLASS}>
            <MetricText
              value={row.ip || null}
              reason="The backend did not report an address for this container."
              className="font-mono text-xs"
            />
          </td>
          <td className={TD_CLASS}>
            <MetricText
              value={formatPercent(row.cpu_percent)}
              reason="Live container stats are not collected by the backend."
            />
          </td>
          <td className={TD_CLASS}>
            <MetricText
              value={formatBytes(row.memory_bytes)}
              reason="Live container stats are not collected by the backend."
            />
            {row.memory_limit_bytes ? (
              <div className="text-xs text-gray-500">
                of {formatBytes(row.memory_limit_bytes)}
              </div>
            ) : null}
          </td>
          <td className={TD_CLASS}>
            <MetricText
              value={formatUptime(row.started_at)}
              reason="The backend did not report when this container started."
            />
          </td>
        </>
      )}
    />
  );
}

export default ContainerPane;
