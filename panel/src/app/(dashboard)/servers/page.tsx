'use client';

import { useMemo, useState } from 'react';
import Link from 'next/link';
import {
  Cpu,
  HardDrive,
  MemoryStick,
  Plus,
  RefreshCw,
  Search,
  Server,
} from 'lucide-react';

import AddNodeCallout, { ADD_NODE_COPY_EN } from '@/components/servers/AddNodeCallout';
import LocalNodeBadge from '@/components/servers/LocalNodeBadge';
import PanelHostPlaceholder, {
  PANEL_HOST_PLACEHOLDER_COPY_EN,
} from '@/components/servers/PanelHostPlaceholder';
import { MetricText, Unavailable } from '@/components/Unavailable';
import { useServerMetrics } from '@/hooks/useServerMetrics';
import { useServers } from '@/hooks/useServers';
import {
  finiteOrNull,
  formatBytesOrNull,
  formatUsage,
  isLocalNode,
  percentOf,
  serverLabel,
  serverStatusBadgeClass,
} from '@/lib/servers';
import type { ManagedServer, ServerMetrics } from '@/types/server';

/**
 * Why a figure on this page can be missing. Each one is specific: an operator
 * who hovers a dash learns which part of the chain has not reported, not merely
 * that something is absent.
 */
const NO_METRICS_REASON =
  'Not available: no agent on this node has reported a sample yet.';
const NO_INVENTORY_REASON =
  'Not available: this node has not reported its hardware inventory yet.';
const NO_HEARTBEAT_REASON = 'Not available: this node has never reported in.';

/** A usage bar. Missing figures leave the track empty rather than drawing a zero-width bar as if it were idle. */
function UsageBar({ percent, color }: { percent: number | null; color: string }) {
  if (percent === null) {
    return <div className="h-1.5 w-full overflow-hidden rounded-md bg-gray-100" />;
  }
  return (
    <div className="h-1.5 w-full overflow-hidden rounded-md bg-gray-100">
      <div className={`h-full rounded-md ${color}`} style={{ width: `${percent}%` }} />
    </div>
  );
}

function ServerCard({
  server,
  metrics,
  lastSeen,
}: {
  server: ManagedServer;
  metrics: ServerMetrics | null;
  lastSeen: string | null;
}) {
  const local = isLocalNode(server);

  const cpuPercent = finiteOrNull(metrics?.cpu_percent);
  const ramUsed = finiteOrNull(metrics?.ram_used);
  const ramTotal = finiteOrNull(metrics?.ram_total) ?? finiteOrNull(server.ram_total);
  const diskUsed = finiteOrNull(metrics?.disk_used);
  const diskTotal = finiteOrNull(metrics?.disk_total) ?? finiteOrNull(server.disk_total);

  return (
    <Link
      href={`/servers/${server.id}`}
      className="block rounded-lg border border-gray-200 bg-white p-5 shadow-sm hover:border-gray-300 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
    >
      <div className="mb-4 flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <div className="rounded-md border border-gray-200 bg-gray-50 p-2">
            <Server className="text-gray-600" size={18} aria-hidden="true" />
          </div>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="truncate text-sm font-semibold text-gray-900">
                {serverLabel(server)}
              </h3>
              {local && (
                <LocalNodeBadge
                  label={PANEL_HOST_PLACEHOLDER_COPY_EN.localBadge}
                  title="The machine this panel runs on. It is managed directly, with no second machine involved."
                />
              )}
            </div>
            <p className="truncate text-xs text-gray-500">
              {server.ip_address || (
                <Unavailable reason={NO_INVENTORY_REASON} className="text-xs" />
              )}
            </p>
          </div>
        </div>
        <span
          className={`inline-flex shrink-0 items-center rounded-md px-2 py-0.5 text-xs font-medium ${serverStatusBadgeClass(
            server.status
          )}`}
        >
          {server.status || 'unknown'}
        </span>
      </div>

      <div className="mb-4 grid grid-cols-2 gap-4">
        <div>
          <p className="text-xs text-gray-500">Operating system</p>
          <p className="text-sm text-gray-900">
            <MetricText value={server.os || null} reason={NO_INVENTORY_REASON} />
          </p>
        </div>
        <div>
          <p className="text-xs text-gray-500">Cores</p>
          <p className="text-sm text-gray-900">
            <MetricText
              value={
                finiteOrNull(server.cpu_cores) && Number(server.cpu_cores) > 0
                  ? String(server.cpu_cores)
                  : null
              }
              reason={NO_INVENTORY_REASON}
            />
          </p>
        </div>
        <div>
          <p className="text-xs text-gray-500">Agent</p>
          <p className="text-sm text-gray-900">
            <MetricText
              value={server.agent_status || server.agent_version || null}
              reason="Not available: the API has not reported an agent state for this node."
            />
          </p>
        </div>
        <div>
          <p className="text-xs text-gray-500">Last seen</p>
          <p className="text-sm text-gray-900" suppressHydrationWarning>
            <MetricText value={lastSeen} reason={NO_HEARTBEAT_REASON} />
          </p>
        </div>
      </div>

      <div className="space-y-3">
        <div>
          <div className="mb-1 flex items-center justify-between">
            <span className="flex items-center gap-2 text-xs text-gray-500">
              <Cpu size={12} aria-hidden="true" />
              CPU
            </span>
            <span className="text-xs text-gray-700">
              <MetricText
                value={cpuPercent === null ? null : `${cpuPercent.toFixed(1)}%`}
                reason={NO_METRICS_REASON}
              />
            </span>
          </div>
          <UsageBar percent={cpuPercent} color="bg-brand-600" />
        </div>

        <div>
          <div className="mb-1 flex items-center justify-between">
            <span className="flex items-center gap-2 text-xs text-gray-500">
              <MemoryStick size={12} aria-hidden="true" />
              RAM
            </span>
            <span className="text-xs text-gray-700">
              <MetricText
                value={formatUsage(ramUsed, ramTotal)}
                reason={ramTotal === null ? NO_INVENTORY_REASON : NO_METRICS_REASON}
              />
            </span>
          </div>
          <UsageBar percent={percentOf(ramUsed, ramTotal)} color="bg-emerald-600" />
        </div>

        <div>
          <div className="mb-1 flex items-center justify-between">
            <span className="flex items-center gap-2 text-xs text-gray-500">
              <HardDrive size={12} aria-hidden="true" />
              Disk
            </span>
            <span className="text-xs text-gray-700">
              <MetricText
                value={
                  formatUsage(diskUsed, diskTotal) ??
                  (diskUsed === null ? formatBytesOrNull(diskTotal) : null)
                }
                reason={diskTotal === null ? NO_INVENTORY_REASON : NO_METRICS_REASON}
              />
            </span>
          </div>
          <UsageBar percent={percentOf(diskUsed, diskTotal)} color="bg-gray-600" />
        </div>
      </div>
    </Link>
  );
}

export default function ServersPage() {
  const { servers, loading, error, reload } = useServers();
  const [search, setSearch] = useState('');

  // The panel host first: it is the node the operator is standing on.
  const ordered = useMemo(() => {
    const list = [...servers];
    list.sort((a, b) => {
      const localDiff = Number(isLocalNode(b)) - Number(isLocalNode(a));
      if (localDiff !== 0) return localDiff;
      return serverLabel(a).localeCompare(serverLabel(b));
    });
    return list;
  }, [servers]);

  const ids = useMemo(() => ordered.map((s) => s.id).filter(Boolean), [ordered]);
  const { metrics } = useServerMetrics(ids);

  /*
   * Safe to format with the browser's locale: the first paint of this page is
   * the loading spinner below, so a card only ever renders after mount and
   * there is no server-rendered string to disagree with.
   */
  const formatLastSeen = (value: string | null | undefined): string | null => {
    if (!value) return null;
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return null;
    return date.toLocaleString();
  };

  const query = search.trim().toLowerCase();
  const filtered = ordered.filter((server) => {
    if (!query) return true;
    return (
      serverLabel(server).toLowerCase().includes(query) ||
      String(server.ip_address || '').toLowerCase().includes(query) ||
      String(server.os || '').toLowerCase().includes(query)
    );
  });

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-gray-200 border-b-brand-600" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">Servers</h1>
          <p className="mt-1 text-sm text-gray-600">
            Every machine this panel manages, starting with the one it runs on.
          </p>
        </div>
        <div className="flex items-center gap-3">
          <button
            type="button"
            onClick={reload}
            className="inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
          >
            <RefreshCw size={16} aria-hidden="true" />
            Refresh
          </button>
          <Link
            href="/servers/add"
            className="inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2"
          >
            <Plus size={16} aria-hidden="true" />
            Add Server
          </Link>
        </div>
      </div>

      {error && (
        <div
          role="alert"
          className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700"
        >
          <span className="min-w-0 break-words">{error}</span>
          <button
            type="button"
            onClick={reload}
            className="shrink-0 rounded-md border border-red-300 bg-white px-3 py-1.5 text-sm font-medium text-red-700 hover:bg-red-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-600"
          >
            Try again
          </button>
        </div>
      )}

      {servers.length > 1 && (
        <div className="rounded-lg border border-gray-200 bg-white px-4 py-3 shadow-sm">
          <div className="relative">
            <Search
              className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
              size={16}
              aria-hidden="true"
            />
            <input
              id="server-search"
              type="text"
              aria-label="Search servers"
              placeholder="Search by hostname, address or operating system..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="w-full rounded-md border border-gray-300 bg-white py-2 pl-9 pr-3 text-sm text-gray-900 placeholder:text-gray-400 focus:border-brand-500 focus:outline-none focus:ring-1 focus:ring-brand-500"
            />
          </div>
        </div>
      )}

      {/*
        An empty list on a fresh install is the panel host waiting to be
        registered, not an absence of servers - so it is shown as itself.
      */}
      {servers.length === 0 && error ? null : servers.length === 0 ? (
        <PanelHostPlaceholder copy={PANEL_HOST_PLACEHOLDER_COPY_EN} onRefresh={reload} />
      ) : filtered.length === 0 ? (
        <div className="rounded-lg border border-gray-200 bg-white px-6 py-14 text-center shadow-sm">
          <Server className="mx-auto text-gray-300" size={40} aria-hidden="true" />
          <h3 className="mt-4 text-sm font-semibold text-gray-900">No servers match</h3>
          <p className="mt-1 text-sm text-gray-600">Try a different search term.</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          {filtered.map((server) => (
            <ServerCard
              key={server.id}
              server={server}
              metrics={metrics[server.id] || null}
              lastSeen={formatLastSeen(server.last_seen_at)}
            />
          ))}
        </div>
      )}

      <AddNodeCallout copy={ADD_NODE_COPY_EN} />
    </div>
  );
}
