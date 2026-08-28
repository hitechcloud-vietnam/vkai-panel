'use client';

/**
 * The one genuinely measured fact on this screen.
 *
 * `GET /api/v1/services/docker` runs `systemctl show docker` on the host and
 * returns ActiveState, SubState, Description, MainPID and MemoryCurrent. That
 * handler is real, so this card shows real values.
 *
 * Two limits are stated on the card rather than hidden, because both change
 * what an operator should conclude from it:
 *
 *  - The backend does not ask systemd for LoadState, so a host with no Docker
 *    installed at all reports `inactive` - exactly like a Docker that is merely
 *    stopped. "Inactive" here means "not running", not "installed and stopped".
 *  - Start, stop and restart are refused. `checkServiceName` in
 *    core/internal/service/service_manager.go allows nginx, mysql, redis and a
 *    dozen others, and `docker` is not among them, so the route answers 500
 *    with `service "docker" is not managed by this panel`. Offering the button
 *    would be offering a 500.
 */

import { useCallback } from 'react';
import { Activity, RefreshCw } from 'lucide-react';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { MetricText } from '@/components/Unavailable';
import { api, unwrap } from '@/services/api';
import { cn } from '@/lib/utils';
import type { DockerDaemonStatus } from '@/types/docker';

import { formatBytes } from './format';
import { ErrorBlock, Panel, SECONDARY_BUTTON_CLASS, SectionHeader } from './PaneChrome';
import { useAsyncResource, type AsyncResource } from './useDockerData';

/** Reads the systemd view of the Docker unit. Admin-only on the API side. */
export function useDaemonStatus(): AsyncResource<DockerDaemonStatus | null> {
  const load = useCallback(async () => {
    const res = await api.get('/api/v1/services/docker');
    return unwrap<DockerDaemonStatus>(res, null);
  }, []);

  return useAsyncResource<DockerDaemonStatus | null>(
    load,
    null,
    'Could not read the Docker service status from the host.'
  );
}

function stateVariant(activeState: string): 'success' | 'warning' | 'danger' | 'neutral' {
  const s = (activeState || '').toLowerCase();
  if (s === 'active') return 'success';
  if (s === 'activating' || s === 'deactivating' || s === 'reloading') return 'warning';
  if (s === 'failed') return 'danger';
  return 'neutral';
}

function DaemonSkeleton() {
  return (
    <div className="grid gap-4 px-4 py-4 sm:grid-cols-2 lg:grid-cols-4" aria-busy="true">
      <span className="sr-only">Loading</span>
      {Array.from({ length: 4 }).map((_, index) => (
        <div key={index}>
          <div className="h-3 w-20 animate-pulse rounded bg-gray-100" />
          <div className="mt-2 h-5 w-24 animate-pulse rounded bg-gray-100" />
        </div>
      ))}
    </div>
  );
}

function Field({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="min-w-0">
      <dt className="text-xs font-semibold uppercase tracking-wide text-gray-500">{label}</dt>
      <dd className="mt-1 truncate text-sm text-gray-900">{children}</dd>
    </div>
  );
}

export function DaemonStatusCard({
  resource,
  /** Settings repeats the restart caveat in full; Overview keeps it to a line. */
  verbose = false,
}: {
  resource: AsyncResource<DockerDaemonStatus | null>;
  verbose?: boolean;
}) {
  const { data, loading, refreshing, error, reload } = resource;
  const activeState = data?.active_state || '';
  const running = activeState.toLowerCase() === 'active';

  return (
    <Panel>
      <SectionHeader
        title="Docker daemon"
        description="Read from systemd on the panel host by GET /api/v1/services/docker."
        actions={
          <Button
            type="button"
            variant="outline"
            onClick={reload}
            disabled={loading || refreshing}
            className={SECONDARY_BUTTON_CLASS}
          >
            <RefreshCw
              className={cn('mr-2 h-4 w-4', (loading || refreshing) && 'animate-spin')}
              aria-hidden="true"
            />
            Refresh
          </Button>
        }
      />

      {loading ? (
        <DaemonSkeleton />
      ) : error ? (
        <ErrorBlock title="Could not read the Docker service status" message={error} onRetry={reload} />
      ) : (
        <>
          <dl className="grid gap-4 px-4 py-4 sm:grid-cols-2 lg:grid-cols-4">
            <Field label="State">
              <div className="flex items-center gap-2">
                <Badge variant={stateVariant(activeState)}>
                  <Activity className="h-3 w-3" aria-hidden="true" />
                  {activeState || 'unknown'}
                </Badge>
                {data?.sub_state && (
                  <span className="text-xs text-gray-500">{data.sub_state}</span>
                )}
              </div>
            </Field>
            <Field label="Unit">
              <MetricText
                value={data?.description || null}
                reason="systemctl returned no Description for the docker unit."
              />
            </Field>
            <Field label="Main PID">
              <MetricText
                value={data?.pid ? String(data.pid) : null}
                reason="The unit is not running, so systemd reports no main PID."
              />
            </Field>
            <Field label="Resident memory">
              <MetricText
                value={data && data.memory > 0 ? formatBytes(data.memory) : null}
                reason="systemd reported no MemoryCurrent for this unit; accounting may be off or the unit stopped."
              />
            </Field>
          </dl>

          <div className="border-t border-gray-200 px-4 py-3">
            {!running && (
              <p className="text-sm text-gray-600">
                <span className="font-medium text-gray-900">
                  &ldquo;{activeState || 'unknown'}&rdquo; means the daemon is not running.
                </span>{' '}
                It does not distinguish a stopped Docker from a host where Docker was never
                installed: the backend asks systemd only for ActiveState, SubState,
                Description, MainPID and MemoryCurrent, not LoadState.
              </p>
            )}
            <p className={cn('text-sm text-gray-600', !running && 'mt-2')}>
              Starting, stopping and restarting the daemon are not offered here.{' '}
              <span className="font-mono text-xs">checkServiceName</span> in{' '}
              <span className="font-mono text-xs">
                core/internal/service/service_manager.go
              </span>{' '}
              does not list <span className="font-mono text-xs">docker</span> among the units
              the panel may control, so those routes answer{' '}
              <span className="font-mono text-xs">
                service &quot;docker&quot; is not managed by this panel
              </span>
              . Use <span className="font-mono text-xs">systemctl</span> over SSH until that
              changes.
            </p>
            {verbose && (
              <p className="mt-2 text-sm text-gray-600">
                The same restriction covers the journal:{' '}
                <span className="font-mono text-xs">GET /api/v1/services/docker/logs</span>{' '}
                runs through the same allow list and is refused, so daemon logs are not
                readable from the panel either.
              </p>
            )}
          </div>
        </>
      )}
    </Panel>
  );
}
