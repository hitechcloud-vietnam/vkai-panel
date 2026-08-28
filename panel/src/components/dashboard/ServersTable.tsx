'use client';

import Link from 'next/link';
import { Cpu, HardDrive, MemoryStick, RefreshCw, Server } from 'lucide-react';

import LocalNodeBadge from '@/components/servers/LocalNodeBadge';
import { MetricText } from '@/components/Unavailable';
import { useFormatters, useT } from '@/i18n';
import { formatUsage, isLocalNode, serverLabel } from '@/lib/servers';
import type { ManagedServer, ServerMetrics } from '@/types/server';
import { Skeleton, StateMessage } from './StatCard';

/**
 * The table of managed machines.
 *
 * Every figure here comes from a node that has reported it. One that has not is
 * an em dash with the reason in its tooltip - never a zero, which would read as
 * an idle machine rather than a silent one.
 */
export type DashboardServer = ManagedServer;

export interface ServersTableProps {
  servers: DashboardServer[];
  /** Latest sample per node id, from useServerMetrics. */
  metrics?: Record<string, ServerMetrics | null>;
  loading?: boolean;
  error?: string | null;
  emptyHint?: string;
  onRetry?: () => void;
}

/** Column headings, in order. Dictionary keys, resolved at render. */
const COLUMN_KEYS = [
  'common.field.server',
  'common.status',
  'common.field.ipAddress',
  'common.field.os',
  'common.field.cpu',
  'common.field.ram',
  'common.field.disk',
  'common.actions',
];

function statusBadgeClass(status: string | undefined): string {
  switch (String(status || '').toLowerCase()) {
    case 'online':
    case 'active':
      return 'bg-emerald-50 text-emerald-700';
    case 'offline':
    case 'error':
    case 'failed':
      return 'bg-red-50 text-red-700';
    case 'maintenance':
    case 'pending':
    case 'provisioning':
      return 'bg-amber-50 text-amber-700';
    default:
      return 'bg-gray-100 text-gray-700';
  }
}

export default function ServersTable({
  servers,
  metrics = {},
  loading = false,
  error = null,
  emptyHint,
  onRetry,
}: ServersTableProps) {
  const t = useT();
  const { formatNumber } = useFormatters();
  const list = Array.isArray(servers) ? servers : [];
  const noMetricsReason = t('common.reason.noMetrics');
  const noInventoryReason = t('common.reason.noInventory');

  if (loading) {
    return (
      <div className="space-y-3 px-5 py-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <Skeleton key={i} className="h-10 w-full" />
        ))}
      </div>
    );
  }

  if (error) {
    return (
      <StateMessage
        tone="error"
        title={t('dashboard.servers.loadFailed')}
        hint={error}
        action={
          onRetry ? (
            <button
              type="button"
              onClick={onRetry}
              className="rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
            >
              {t('common.retry')}
            </button>
          ) : undefined
        }
      />
    );
  }

  if (list.length === 0) {
    /*
      An empty list on a fresh install means the machine the panel runs on has
      not registered itself yet - not that the operator has no servers. The
      wording says so, and the action is to look again rather than to go and
      find a second machine.
    */
    return (
      <StateMessage
        icon={<Server size={36} aria-hidden="true" />}
        title={t('dashboard.servers.unregisteredTitle')}
        hint={emptyHint || t('dashboard.servers.unregisteredHint')}
        action={
          <div className="flex flex-wrap items-center justify-center gap-3">
            {onRetry && (
              <button
                type="button"
                onClick={onRetry}
                className="inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
              >
                <RefreshCw size={16} aria-hidden="true" />
                {t('common.checkAgain')}
              </button>
            )}
            <Link
              href="/servers"
              className="inline-flex items-center rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2"
            >
              {t('dashboard.servers.openServers')}
            </Link>
          </div>
        }
      />
    );
  }

  return (
    <div className="w-full overflow-x-auto">
      <table className="w-full min-w-[900px] border-collapse">
        <thead className="bg-gray-50">
          <tr className="border-b border-gray-200">
            {COLUMN_KEYS.map((key) => (
              <th
                key={key}
                scope="col"
                className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500"
              >
                {t(key)}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {list.map((server) => {
            const sample = metrics[server.id] || null;
            const cpu =
              typeof sample?.cpu_percent === 'number' && Number.isFinite(sample.cpu_percent)
                ? t('common.percent', {
                    n: formatNumber(sample.cpu_percent, {
                      minimumFractionDigits: 1,
                      maximumFractionDigits: 1,
                    }),
                  })
                : null;
            const ram = formatUsage(sample?.ram_used, sample?.ram_total ?? server.ram_total);
            const disk = formatUsage(sample?.disk_used, sample?.disk_total ?? server.disk_total);

            return (
              <tr
                key={server.id}
                className="border-b border-gray-100 last:border-b-0 hover:bg-gray-50"
              >
                <td className="px-4 py-3 text-sm text-gray-700">
                  <span className="flex flex-wrap items-center gap-2">
                    <span className="font-medium text-gray-900">{serverLabel(server)}</span>
                    {isLocalNode(server) && (
                      <LocalNodeBadge
                        label={t('servers.localBadge')}
                        title={t('servers.localBadgeTitle.managed')}
                      />
                    )}
                  </span>
                  <span className="block text-xs text-gray-500">
                    <MetricText value={server.os || null} reason={noInventoryReason} />
                  </span>
                </td>
                <td className="px-4 py-3 text-sm">
                  <span
                    className={`inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium ${statusBadgeClass(
                      server.status
                    )}`}
                  >
                    {server.status || 'unknown'}
                  </span>
                </td>
                <td className="px-4 py-3 font-mono text-sm text-gray-700">
                  <MetricText value={server.ip_address || null} reason={noInventoryReason} />
                </td>
                <td className="px-4 py-3 text-sm text-gray-700">
                  <MetricText value={server.os || null} reason={noInventoryReason} />
                </td>
                <td className="px-4 py-3 text-sm text-gray-700">
                  <span className="flex items-center gap-2">
                    <Cpu size={14} className="text-gray-500" aria-hidden="true" />
                    <MetricText value={cpu} reason={noMetricsReason} />
                  </span>
                </td>
                <td className="px-4 py-3 text-sm text-gray-700">
                  <span className="flex items-center gap-2">
                    <MemoryStick size={14} className="text-gray-500" aria-hidden="true" />
                    <MetricText value={ram} reason={noMetricsReason} />
                  </span>
                </td>
                <td className="px-4 py-3 text-sm text-gray-700">
                  <span className="flex items-center gap-2">
                    <HardDrive size={14} className="text-gray-500" aria-hidden="true" />
                    <MetricText value={disk} reason={noMetricsReason} />
                  </span>
                </td>
                <td className="px-4 py-3 text-sm">
                  <Link
                    href={`/servers/${server.id}`}
                    className="inline-flex items-center rounded-md border border-gray-300 bg-white px-2.5 py-1 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
                  >
                    {t('common.view')}
                  </Link>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
