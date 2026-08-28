'use client';

/**
 * Docker Compose: the projects on this host, their services, and their state.
 *
 * Bringing a project up and down, reading its file, editing it, and reading the
 * logs of one service are the four things an operator does here. None is
 * offered:
 *
 *  - up/down: POST /docker/compose/deploy and /compose/stop are mounted, and
 *    both answer "Compose stack deployed" / "Compose stack stopped" after
 *    writing a log line. Neither runs `docker compose`.
 *  - view and edit the file: there is no route. The panel does have a file
 *    manager, but ListComposeStacks returns no project path, so there is
 *    nothing to hand it - and the file routes are gated on the `website`
 *    permission, not `docker`.
 *  - per-service logs: no route anywhere.
 *
 * `deploy` takes the whole YAML in the request body, which is worth noting for
 * whoever implements it: the panel has no way to fetch a project's existing
 * file, so an edit today would mean re-typing it from memory.
 */

import { useCallback } from 'react';

import { Badge } from '@/components/ui/badge';
import { MetricText } from '@/components/Unavailable';
import { dockerApi, unwrapList } from '@/services/api';
import type { CapabilityGap, DockerComposeProject } from '@/types/docker';

import { formatCount, formatDateTime, matches } from './format';
import { ListPane, type ListColumn } from './ListPane';
import { TD_CLASS } from './PaneChrome';
import { useAsyncResource } from './useDockerData';

const COLUMNS: ListColumn[] = [
  { key: 'name', label: 'Project' },
  { key: 'status', label: 'State' },
  { key: 'services', label: 'Services' },
  { key: 'path', label: 'Compose file' },
  { key: 'created', label: 'Created' },
];

const GAPS: CapabilityGap[] = [
  {
    label: 'The project list',
    missing:
      'GET /api/v1/docker/compose is mounted but ListComposeStacks returns an empty slice. It reports no project path, no service names and no state.',
  },
  {
    label: 'Up',
    missing:
      'POST /api/v1/docker/compose/deploy takes {name, content}, logs the name and answers success. It never writes the file or runs `docker compose up`.',
  },
  {
    label: 'Down',
    missing:
      'POST /api/v1/docker/compose/stop takes {name}, logs it and answers success. It never runs `docker compose down`, and there is no way to say whether volumes should go with it.',
  },
  {
    label: 'View and edit the compose file',
    missing:
      'No route reads a project’s YAML. /api/v1/files/read exists but is gated on the `website` permission and needs an absolute path, which the compose list does not report.',
  },
  {
    label: 'Logs per service',
    missing:
      'No route. `docker compose logs <service>` has no equivalent, and there is no streaming channel for it.',
  },
  {
    label: 'Compose templates',
    missing:
      'aaPanel keeps a separate Compose-template tab. The panel stores no templates and has no route for them.',
  },
];

export function ComposePane() {
  const load = useCallback(async () => {
    const res = await dockerApi.listComposeStacks();
    return unwrapList<DockerComposeProject>(res);
  }, []);

  const resource = useAsyncResource<DockerComposeProject[]>(
    load,
    [],
    'Could not load Compose projects.'
  );

  const filter = useCallback(
    (row: DockerComposeProject, term: string) =>
      matches(row.name, term) || matches(row.path, term) || matches(row.status, term),
    []
  );

  return (
    <ListPane<DockerComposeProject>
      title="Compose projects"
      description="Multi-container projects defined by a compose file on this host."
      searchPlaceholder="Search by project name, path or state"
      columns={COLUMNS}
      resource={resource}
      rowKey={(row) => row.name}
      filter={filter}
      errorTitle="Could not load Compose projects"
      emptyTitle="No Compose projects on this host"
      emptyDescription="A project appears here once its compose file is deployed from the panel or found on disk."
      stub={{
        resource: 'Compose projects',
        handler: 'ListComposeStacks',
        detail: 'it returns an empty slice and never inspects the host for compose files.',
      }}
      gapsTitle="What the Docker Compose tab needs from the backend"
      gapsIntro="Up and down are mounted routes that answer success without running anything, so neither is wired to a button here."
      gaps={GAPS}
      renderRow={(row) => (
        <>
          <td className={`${TD_CLASS} font-medium text-gray-900`}>{row.name}</td>
          <td className={TD_CLASS}>
            <Badge variant={(row.status || '').toLowerCase() === 'running' ? 'success' : 'neutral'}>
              {row.status || 'unknown'}
            </Badge>
          </td>
          <td className={TD_CLASS}>
            {row.services && row.services.length > 0 ? (
              <span className="text-gray-900">{row.services.join(', ')}</span>
            ) : (
              <MetricText
                value={formatCount(row.service_count)}
                reason="The backend did not report the services in this project."
              />
            )}
          </td>
          <td className={TD_CLASS}>
            <MetricText
              value={row.path || null}
              reason="The backend does not report a compose file path, which is why viewing and editing the file are not offered."
              className="font-mono text-xs"
            />
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

export default ComposePane;
