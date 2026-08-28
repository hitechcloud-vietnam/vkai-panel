'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { AlertTriangle, CheckCircle, Plus, RefreshCw } from 'lucide-react';

import AddNodeCallout from '@/components/servers/AddNodeCallout';
import LocalNodeBadge from '@/components/servers/LocalNodeBadge';
import { MetricText } from '@/components/Unavailable';
import DonutGauge, { DonutGaugeSkeleton } from '@/components/dashboard/DonutGauge';
import OverviewRow, { type OverviewItem } from '@/components/dashboard/OverviewRow';
import ServersTable from '@/components/dashboard/ServersTable';
import SoftwareGrid, { type SoftwareItem } from '@/components/dashboard/SoftwareGrid';
import StatCard, { Skeleton } from '@/components/dashboard/StatCard';
import TrafficPanel, { type TrafficSeries } from '@/components/dashboard/TrafficPanel';
import { LOCALE_TAG, useFormatters, useLocale, useT, useTn } from '@/i18n';
import { useServerMetrics } from '@/hooks/useServerMetrics';
import { useServers } from '@/hooks/useServers';
import { brand } from '@/lib/brand';
import {
  finiteOrNull,
  formatBytesOrNull,
  formatUsage,
  isLocalNode,
  loadPercent,
  percentOf,
  serverLabel,
} from '@/lib/servers';
import { databaseApi, sslApi, unwrapList, websiteApi } from '@/services/api';
import { useAuthStore } from '@/store/auth';

/** Software inventory has no endpoint yet, so the grid stays empty rather than inventing rows. */
const SOFTWARE_ITEMS: SoftwareItem[] = [];

/**
 * A count that failed to load, told apart from a count that is genuinely zero.
 *
 * The state holds keys rather than sentences: the fetch callback must not close
 * over the translator, or a translator that is a fresh function on every render
 * would make the callback fresh too and the effect that runs it would refetch
 * forever.
 */
interface CountState {
  value: number | null;
  status: 'loading' | 'loaded' | 'failed';
  /** Dictionary key naming what was being listed, for the failure note. */
  entityKey: string;
}

const LOADING_COUNT: CountState = { value: null, status: 'loading', entityKey: '' };

/** The "updated at" clock, in the operator's locale. Empty when Intl refuses. */
function formatClock(at: Date, localeTag: string): string {
  try {
    return at.toLocaleTimeString(localeTag, {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
  } catch {
    return '';
  }
}

export default function DashboardPage() {
  const t = useT();
  const tn = useTn();
  const { locale } = useLocale();
  const { formatNumber } = useFormatters();
  const { user } = useAuthStore();
  const {
    servers,
    defaultId,
    loading: serversLoading,
    error: serversError,
    reload: reloadServers,
  } = useServers();

  /*
   * Why a figure on this page can be missing. Each reason names the link in the
   * chain that has not reported, so an operator who hovers a dash learns
   * something instead of only being told the value is absent. Nothing here ever
   * degrades to 0: a machine that is not reporting and a machine that is idle
   * must not look the same.
   */
  const noMetricsReason = t('common.reason.noMetrics');
  const noInventoryReason = t('common.reason.noInventory');
  const noNodeReason = t('common.reason.noNode');
  const noSoftwareEndpoint = t('dashboard.software.noEndpoint', {
    product: brand.productName,
  });
  const noSeriesEndpoint = t('dashboard.traffic.noSeriesEndpoint');

  const [selectedServerId, setSelectedServerId] = useState('');
  const [lastUpdatedAt, setLastUpdatedAt] = useState<Date | null>(null);
  const [websiteCount, setWebsiteCount] = useState<CountState>(LOADING_COUNT);
  const [databaseCount, setDatabaseCount] = useState<CountState>(LOADING_COUNT);
  const [sslCount, setSslCount] = useState<CountState>(LOADING_COUNT);

  const serverIds = useMemo(() => servers.map((s) => s.id).filter(Boolean), [servers]);
  const { metrics, reload: reloadMetrics } = useServerMetrics(serverIds);

  /**
   * Counts come from the same endpoints the list screens use, each settled on
   * its own: one failing endpoint must not blank the other three, and a failure
   * must not read as zero.
   */
  const loadCounts = useCallback(async () => {
    setWebsiteCount(LOADING_COUNT);
    setDatabaseCount(LOADING_COUNT);
    setSslCount(LOADING_COUNT);

    const [websitesRes, databasesRes, sslRes] = await Promise.allSettled([
      websiteApi.list({ page: 1, per_page: 200 }),
      databaseApi.list(),
      sslApi.list(),
    ]);

    const settle = (
      result: PromiseSettledResult<unknown>,
      entityKey: string
    ): CountState =>
      result.status === 'fulfilled'
        ? { value: unwrapList(result.value).length, status: 'loaded', entityKey }
        : { value: null, status: 'failed', entityKey };

    setWebsiteCount(settle(websitesRes, 'common.entity.website'));
    setDatabaseCount(settle(databasesRes, 'common.entity.database'));
    setSslCount(settle(sslRes, 'common.entity.sslCertificate'));
  }, []);

  useEffect(() => {
    loadCounts();
  }, [loadCounts]);

  const refreshAll = useCallback(() => {
    reloadServers();
    reloadMetrics();
    loadCounts();
  }, [reloadServers, reloadMetrics, loadCounts]);

  // The instant is captured after the data settles, never during render, so
  // server and client cannot disagree about the clock. It is formatted at
  // render time instead, which lets a locale switch restate the same instant
  // rather than move it.
  useEffect(() => {
    if (serversLoading) return;
    setLastUpdatedAt(new Date());
  }, [serversLoading]);

  const lastUpdated = lastUpdatedAt ? formatClock(lastUpdatedAt, LOCALE_TAG[locale]) : '';

  // The machine the panel runs on is the one an operator wants to see first.
  useEffect(() => {
    if (servers.length === 0) {
      if (selectedServerId !== '') setSelectedServerId('');
      return;
    }
    if (!servers.some((s) => s.id === selectedServerId)) {
      setSelectedServerId(defaultId || servers[0].id);
    }
  }, [servers, selectedServerId, defaultId]);

  const selected = useMemo(
    () => servers.find((s) => s.id === selectedServerId) || servers[0] || null,
    [servers, selectedServerId]
  );
  const selectedMetrics = selected ? metrics[selected.id] || null : null;
  const hasServer = Boolean(selected);
  const metricsReason = hasServer ? noMetricsReason : noNodeReason;
  const inventoryReason = hasServer ? noInventoryReason : noNodeReason;

  /** "8 cores" - tn picks the plural branch; Vietnamese has one form. */
  const coreCount = (cores: number) => tn('common.coreCount', cores);

  const cpuPercent = finiteOrNull(selectedMetrics?.cpu_percent);
  const cpuCores = finiteOrNull(selected?.cpu_cores);
  const cpuDetail = cpuCores && cpuCores > 0 ? coreCount(cpuCores) : undefined;

  const ramUsed = finiteOrNull(selectedMetrics?.ram_used);
  const ramTotal = finiteOrNull(selectedMetrics?.ram_total) ?? finiteOrNull(selected?.ram_total);
  const ramPercent = percentOf(ramUsed, ramTotal);
  const ramDetail = formatUsage(ramUsed, ramTotal) || undefined;

  const diskUsed = finiteOrNull(selectedMetrics?.disk_used);
  const diskTotal = finiteOrNull(selectedMetrics?.disk_total) ?? finiteOrNull(selected?.disk_total);
  const diskPercent = percentOf(diskUsed, diskTotal);
  const diskDetail = formatUsage(diskUsed, diskTotal) || undefined;

  const systemLoad = loadPercent(selectedMetrics, selected?.cpu_cores);
  const loadDetail =
    finiteOrNull(selectedMetrics?.load1) !== null && cpuCores
      ? t('dashboard.load1Detail', {
          load: formatNumber(Number(selectedMetrics?.load1), {
            minimumFractionDigits: 2,
            maximumFractionDigits: 2,
          }),
          cores: cpuCores,
        })
      : undefined;

  const onlineCount = servers.filter(
    (s) => String(s.status || '').toLowerCase() === 'online' ||
      String(s.status || '').toLowerCase() === 'active'
  ).length;
  const offlineCount = servers.filter(
    (s) => String(s.status || '').toLowerCase() === 'offline'
  ).length;

  /** The line under a count: why it is loading, or why it is missing. */
  const countNote = (count: CountState): string | undefined => {
    if (count.status === 'loading') return t('common.loading');
    if (count.status === 'failed') {
      return t('common.reason.listLoadFailed', { what: t(count.entityKey) });
    }
    return undefined;
  };

  const overviewItems: OverviewItem[] = useMemo(
    () => [
      {
        key: 'websites',
        label: t('dashboard.overview.websites'),
        value: websiteCount.value,
        href: '/websites',
        note: countNote(websiteCount),
      },
      {
        key: 'databases',
        label: t('dashboard.overview.databases'),
        value: databaseCount.value,
        href: '/databases',
        note: countNote(databaseCount),
      },
      {
        key: 'ssl',
        label: t('dashboard.overview.ssl'),
        value: sslCount.value,
        href: '/ssl',
        note: countNote(sslCount),
      },
      {
        key: 'servers',
        label: t('dashboard.overview.servers'),
        value: serversError ? null : servers.length,
        href: '/servers',
        note: serversError
          ? t('common.reason.listLoadFailed', { what: t('common.entity.server') })
          : undefined,
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [websiteCount, databaseCount, sslCount, servers.length, serversError, t]
  );

  const networkSeries: TrafficSeries = useMemo(
    () => ({
      points: [],
      upLabel: t('dashboard.traffic.upload'),
      downLabel: t('dashboard.traffic.download'),
      metrics: [
        { label: t('dashboard.traffic.up'), value: null, reason: noSeriesEndpoint },
        { label: t('dashboard.traffic.down'), value: null, reason: noSeriesEndpoint },
        {
          label: t('dashboard.traffic.totalSent'),
          value: formatBytesOrNull(selectedMetrics?.net_out),
          reason: metricsReason,
        },
        {
          label: t('dashboard.traffic.totalReceived'),
          value: formatBytesOrNull(selectedMetrics?.net_in),
          reason: metricsReason,
        },
      ],
    }),
    [selectedMetrics, metricsReason, noSeriesEndpoint, t]
  );

  const diskSeries: TrafficSeries = useMemo(
    () => ({
      points: [],
      upLabel: t('dashboard.disk.write'),
      downLabel: t('dashboard.disk.read'),
      metrics: [
        { label: t('dashboard.disk.write'), value: null, reason: noSeriesEndpoint },
        { label: t('dashboard.disk.read'), value: null, reason: noSeriesEndpoint },
        {
          label: t('dashboard.disk.used'),
          value: formatBytesOrNull(diskUsed),
          reason: metricsReason,
        },
        {
          label: t('dashboard.disk.total'),
          value: formatBytesOrNull(diskTotal),
          reason: inventoryReason,
        },
      ],
    }),
    [diskUsed, diskTotal, metricsReason, inventoryReason, noSeriesEndpoint, t]
  );

  const nodeTitle = selected ? serverLabel(selected) : t('dashboard.noServer');

  return (
    <div className="space-y-5">
      {/* Page heading */}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="text-xl font-semibold text-gray-900">{t('dashboard.title')}</h1>
          <p className="mt-1 text-sm text-gray-600">
            {t('dashboard.greeting', {
              name:
                user?.first_name || user?.username || t('dashboard.greetingFallbackName'),
            })}
            {lastUpdated ? ` ${t('dashboard.updatedAt', { time: lastUpdated })}` : ''}
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <button
            type="button"
            onClick={refreshAll}
            disabled={serversLoading}
            className="inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 disabled:cursor-not-allowed disabled:opacity-60"
          >
            <RefreshCw size={16} aria-hidden="true" />
            {t('common.refresh')}
          </button>
          <Link
            href="/websites"
            className="inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2"
          >
            <Plus size={16} aria-hidden="true" />
            {t('dashboard.createWebsite')}
          </Link>
        </div>
      </div>

      {serversError && (
        <div
          role="alert"
          className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700"
        >
          <span className="flex min-w-0 items-center gap-2">
            <AlertTriangle size={16} aria-hidden="true" />
            <span className="min-w-0 break-words">{serversError}</span>
          </span>
          <button
            type="button"
            onClick={refreshAll}
            className="shrink-0 rounded-md border border-red-300 bg-white px-3 py-1.5 text-sm font-medium text-red-700 hover:bg-red-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-600"
          >
            {t('common.retry')}
          </button>
        </div>
      )}

      {/* 0. The machine in view - the node's identity, before any figure */}
      <StatCard
        title={t('dashboard.currentMachine.title')}
        description={
          servers.length > 1
            ? t('dashboard.currentMachine.pickHint')
            : t('dashboard.currentMachine.selfHint')
        }
        action={
          selected ? (
            <Link
              href="/servers"
              className="inline-flex items-center rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
            >
              {t('dashboard.manageServers')}
            </Link>
          ) : undefined
        }
      >
        {serversLoading ? (
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
            {Array.from({ length: 4 }).map((_, i) => (
              <div key={i}>
                <Skeleton className="h-3 w-20" />
                <Skeleton className="mt-2 h-5 w-28" />
              </div>
            ))}
          </div>
        ) : !selected ? (
          <p className="py-6 text-center text-sm text-gray-500">
            {t('dashboard.currentMachine.unregistered')}
          </p>
        ) : (
          <div className="space-y-4">
            <div className="flex flex-wrap items-center gap-2">
              <span className="truncate text-base font-semibold text-gray-900">{nodeTitle}</span>
              {isLocalNode(selected) && (
                <LocalNodeBadge
                  label={t('servers.localBadge')}
                  title={t('servers.localBadgeTitle.full')}
                />
              )}
            </div>

            {servers.length > 1 && (
              <div
                className="flex flex-wrap gap-2"
                role="group"
                aria-label={t('dashboard.currentMachine.pickerLabel')}
              >
                {servers.map((server) => {
                  const active = server.id === selected.id;
                  return (
                    <button
                      key={server.id}
                      type="button"
                      aria-pressed={active}
                      onClick={() => setSelectedServerId(server.id)}
                      className={
                        active
                          ? 'max-w-full truncate rounded-md border border-brand-200 bg-brand-50 px-2.5 py-1 text-xs font-medium text-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500'
                          : 'max-w-full truncate rounded-md border border-gray-300 bg-white px-2.5 py-1 text-xs font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500'
                      }
                    >
                      {serverLabel(server)}
                    </button>
                  );
                })}
              </div>
            )}

            <dl className="grid grid-cols-2 gap-4 sm:grid-cols-4">
              <div className="min-w-0">
                <dt className="text-xs text-gray-500">{t('common.field.ipAddress')}</dt>
                <dd className="truncate font-mono text-sm text-gray-900">
                  <MetricText value={selected.ip_address || null} reason={inventoryReason} />
                </dd>
              </div>
              <div className="min-w-0">
                <dt className="text-xs text-gray-500">{t('common.field.os')}</dt>
                <dd className="truncate text-sm text-gray-900">
                  <MetricText value={selected.os || null} reason={inventoryReason} />
                </dd>
              </div>
              <div className="min-w-0">
                <dt className="text-xs text-gray-500">{t('common.field.cpu')}</dt>
                <dd className="truncate text-sm text-gray-900">
                  <MetricText
                    value={cpuCores && cpuCores > 0 ? coreCount(cpuCores) : null}
                    reason={inventoryReason}
                  />
                </dd>
              </div>
              <div className="min-w-0">
                <dt className="text-xs text-gray-500">{t('common.field.ram')}</dt>
                <dd className="truncate text-sm text-gray-900">
                  <MetricText value={formatBytesOrNull(ramTotal)} reason={inventoryReason} />
                </dd>
              </div>
              <div className="min-w-0">
                <dt className="text-xs text-gray-500">{t('common.field.disk')}</dt>
                <dd className="truncate text-sm text-gray-900">
                  <MetricText value={formatBytesOrNull(diskTotal)} reason={inventoryReason} />
                </dd>
              </div>
              <div className="min-w-0">
                <dt className="text-xs text-gray-500">{t('common.field.kernel')}</dt>
                <dd className="truncate text-sm text-gray-900">
                  <MetricText value={selected.kernel || null} reason={inventoryReason} />
                </dd>
              </div>
              <div className="min-w-0">
                <dt className="text-xs text-gray-500">{t('common.field.agent')}</dt>
                <dd className="truncate text-sm text-gray-900">
                  <MetricText
                    value={selected.agent_status || null}
                    reason={t('common.reason.noAgentStatus')}
                  />
                </dd>
              </div>
              <div className="min-w-0">
                <dt className="text-xs text-gray-500">{t('common.field.webServer')}</dt>
                <dd className="truncate text-sm text-gray-900">
                  <MetricText
                    value={
                      Array.isArray(selected.web_servers) && selected.web_servers.length > 0
                        ? selected.web_servers.join(', ')
                        : selected.web_server_type || null
                    }
                    reason={t('common.reason.noWebServer')}
                  />
                </dd>
              </div>
            </dl>
          </div>
        )}
      </StatCard>

      {/* 1. System health + disk */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-3">
        <StatCard
          title={t('dashboard.systemHealth')}
          description={selected ? serverLabel(selected) : t('dashboard.noServer')}
          className="lg:col-span-2"
        >
          {serversLoading ? (
            <div className="grid grid-cols-1 gap-6 sm:grid-cols-3">
              {Array.from({ length: 3 }).map((_, i) => (
                <div key={i} className="flex justify-center">
                  <DonutGaugeSkeleton />
                </div>
              ))}
            </div>
          ) : (
            <div className="grid grid-cols-1 gap-6 sm:grid-cols-3">
              <DonutGauge
                value={systemLoad}
                label={t('dashboard.systemLoad')}
                detail={loadDetail}
                unavailableReason={metricsReason}
              />
              <DonutGauge
                value={cpuPercent}
                label={t('common.field.cpu')}
                detail={cpuDetail}
                unavailableReason={metricsReason}
              />
              <DonutGauge
                value={ramPercent}
                label={t('common.field.ram')}
                detail={ramDetail}
                unavailableReason={ramTotal === null ? inventoryReason : metricsReason}
              />
            </div>
          )}
        </StatCard>

        <StatCard title={t('common.field.disk')}>
          {serversLoading ? (
            <div className="space-y-4">
              <Skeleton className="h-8 w-full" />
              <div className="flex justify-center">
                <DonutGaugeSkeleton size={100} />
              </div>
            </div>
          ) : (
            <div className="space-y-4">
              <div className="flex items-center justify-center">
                <DonutGauge
                  value={diskPercent}
                  label={t('dashboard.disk.used')}
                  detail={diskDetail}
                  size={104}
                  unavailableReason={diskTotal === null ? inventoryReason : metricsReason}
                />
              </div>
              <p className="border-t border-gray-200 pt-3 text-xs text-gray-500">
                {t('dashboard.diskMountNote')}
              </p>
            </div>
          )}
        </StatCard>
      </div>

      {/* 2. Overview */}
      <StatCard title={t('dashboard.overview.title')} flush>
        <OverviewRow items={overviewItems} loading={serversLoading} />
      </StatCard>

      {/* 3. Software + traffic */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-2">
        <StatCard title={t('dashboard.software.title')}>
          <SoftwareGrid
            items={SOFTWARE_ITEMS}
            loading={serversLoading}
            emptyHint={noSoftwareEndpoint}
          />
        </StatCard>

        <StatCard title={t('dashboard.traffic.title')}>
          <TrafficPanel
            network={networkSeries}
            disk={diskSeries}
            loading={serversLoading}
            emptyHint={noSeriesEndpoint}
          />
        </StatCard>
      </div>

      {/* 4. Servers */}
      <StatCard
        title={t('common.field.servers')}
        description={
          serversLoading
            ? undefined
            : [
                tn('common.serverCount', servers.length),
                t('dashboard.servers.onlineCount', { n: onlineCount }),
                ...(offlineCount > 0
                  ? [t('dashboard.servers.offlineCount', { n: offlineCount })]
                  : []),
              ].join(' · ')
        }
        action={
          <>
            <span className="hidden items-center gap-1.5 text-xs text-emerald-600 sm:inline-flex">
              <CheckCircle size={14} aria-hidden="true" />
              {t('dashboard.servers.onlineCount', { n: onlineCount })}
            </span>
            <Link
              href="/servers"
              className="inline-flex items-center rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
            >
              {t('common.viewAll')}
            </Link>
          </>
        }
        flush
      >
        <ServersTable
          servers={servers}
          metrics={metrics}
          loading={serversLoading}
          error={serversError}
          onRetry={refreshAll}
        />
      </StatCard>

      {/* 5. Adding a machine - an optional layer, said to be optional */}
      <AddNodeCallout docsHref={brand.docsUrl} />
    </div>
  );
}
