'use client';

/**
 * Local image: what is stored on this host, how big it is, and which containers
 * depend on it.
 *
 * The "containers using this image" column is the one that stops a mistake. An
 * image with a running container behind it cannot be removed, and an operator
 * who deletes on size alone finds that out from a failed deploy rather than
 * from the table.
 *
 * Prune is specified to state how much it will free before it runs. That number
 * comes from `docker system df`, which the backend has no equivalent of, so the
 * control is not offered: a prune button that cannot say what it will delete is
 * a prune button nobody should press.
 */

import { useCallback } from 'react';

import { MetricText } from '@/components/Unavailable';
import { dockerApi, unwrapList } from '@/services/api';
import type { CapabilityGap, DockerImage } from '@/types/docker';

import { formatBytes, formatDateTime, matches } from './format';
import { ListPane, type ListColumn } from './ListPane';
import { TD_CLASS } from './PaneChrome';
import { useAsyncResource } from './useDockerData';

const COLUMNS: ListColumn[] = [
  { key: 'repository', label: 'Repository' },
  { key: 'tag', label: 'Tag' },
  { key: 'id', label: 'Image ID' },
  { key: 'size', label: 'Size' },
  { key: 'containers', label: 'Used by' },
  { key: 'created', label: 'Created' },
];

const GAPS: CapabilityGap[] = [
  {
    label: 'The list itself',
    missing:
      'GET /api/v1/docker/images is mounted but ListImages returns an empty slice. No size, no digest, no tag list.',
  },
  {
    label: 'Containers using an image',
    missing:
      'Nothing joins images to containers. Without it, removal cannot warn that an image is in use.',
  },
  {
    label: 'Remove an image',
    missing:
      'DELETE /api/v1/docker/images/:id answers "Image deleted" without deleting anything, and has no force or no-prune option.',
  },
  {
    label: 'Prune, with the size it will free',
    missing:
      'No prune route, and no `docker system df` equivalent to size dangling layers first. Both are needed together: the figure is the whole reason an operator presses the button.',
  },
  {
    label: 'Export and push',
    missing:
      'aaPanel offers save-to-tar and push-to-registry on this tab. Neither has a route here.',
  },
];

export function LocalImagePane() {
  const load = useCallback(async () => {
    const res = await dockerApi.listImages();
    return unwrapList<DockerImage>(res);
  }, []);

  const resource = useAsyncResource<DockerImage[]>(load, [], 'Could not load local images.');

  const filter = useCallback(
    (row: DockerImage, term: string) =>
      matches(row.repository, term) || matches(row.tag, term) || matches(row.id, term),
    []
  );

  return (
    <ListPane<DockerImage>
      title="Local images"
      description="Images stored on this host, with the containers built from each."
      searchPlaceholder="Search by repository, tag or image ID"
      columns={COLUMNS}
      resource={resource}
      rowKey={(row) => `${row.id}:${row.repository}:${row.tag}`}
      filter={filter}
      errorTitle="Could not load local images"
      emptyTitle="No images on this host"
      emptyDescription="Pull one from the Cloud image tab and it is stored here."
      stub={{
        resource: 'images',
        handler: 'ListImages',
        detail: 'it returns an empty slice and never asks a Docker daemon what is stored.',
      }}
      gapsTitle="What the Local image tab needs from the backend"
      gapsIntro="Removal and prune are not offered. A prune that cannot say how much it frees, and a remove that cannot say what depends on the image, are both unsafe to press."
      gaps={GAPS}
      renderRow={(row) => (
        <>
          <td className={`${TD_CLASS} font-medium text-gray-900`}>{row.repository}</td>
          <td className={TD_CLASS}>
            <span className="font-mono text-xs">{row.tag || '<none>'}</span>
          </td>
          <td className={TD_CLASS}>
            <span className="font-mono text-xs">{(row.id || '').replace(/^sha256:/, '').slice(0, 12)}</span>
          </td>
          <td className={TD_CLASS}>
            <MetricText
              value={formatBytes(row.size)}
              reason="The backend did not report a size for this image."
            />
          </td>
          <td className={TD_CLASS}>
            {row.containers && row.containers.length > 0 ? (
              <span className="text-gray-900">{row.containers.join(', ')}</span>
            ) : (
              <MetricText
                value={null}
                reason="The backend does not report which containers use an image, so this cannot be shown as 'none' either."
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

export default LocalImagePane;
