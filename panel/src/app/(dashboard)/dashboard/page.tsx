'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { AlertTriangle, CheckCircle, Plus, RefreshCw } from 'lucide-react';

import AddNodeCallout, { ADD_NODE_COPY_VI } from '@/components/servers/AddNodeCallout';
import LocalNodeBadge from '@/components/servers/LocalNodeBadge';
import { MetricText } from '@/components/Unavailable';
import DonutGauge, { DonutGaugeSkeleton } from '@/components/dashboard/DonutGauge';
import OverviewRow, { type OverviewItem } from '@/components/dashboard/OverviewRow';
import ServersTable from '@/components/dashboard/ServersTable';
import SoftwareGrid, { type SoftwareItem } from '@/components/dashboard/SoftwareGrid';
import StatCard, { Skeleton } from '@/components/dashboard/StatCard';
import TrafficPanel, { type TrafficSeries } from '@/components/dashboard/TrafficPanel';
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

/*
 * Why a figure on this page can be missing. Each reason names the link in the
 * chain that has not reported, so an operator who hovers a dash learns
 * something instead of only being told the value is absent. Nothing here ever
 * degrades to 0: a machine that is not reporting and a machine that is idle
 * must not look the same.
 */
const NO_METRICS_REASON = 'Chưa có dữ liệu: agent trên máy này chưa gửi mẫu đo nào.';
const NO_INVENTORY_REASON = 'Chưa có dữ liệu: máy này chưa báo cáo cấu hình phần cứng.';
const NO_NODE_REASON = 'Chưa có dữ liệu: chưa có máy chủ nào được đăng ký trong panel.';
const NO_SOFTWARE_ENDPOINT = `API hiện chưa trả về danh sách phần mềm đã cài đặt cho ${brand.productName}.`;
const NO_SERIES_ENDPOINT = 'API hiện chưa trả về chuỗi số liệu lưu lượng theo thời gian.';

/** Software inventory has no endpoint yet, so the grid stays empty rather than inventing rows. */
const SOFTWARE_ITEMS: SoftwareItem[] = [];

/** A count that failed to load, told apart from a count that is genuinely zero. */
interface CountState {
  value: number | null;
  reason: string;
}

const LOADING_COUNT: CountState = { value: null, reason: 'Đang tải…' };

export default function DashboardPage() {
  const { user } = useAuthStore();
  const {
    servers,
    defaultId,
    loading: serversLoading,
    error: serversError,
    reload: reloadServers,
  } = useServers();

  const [selectedServerId, setSelectedServerId] = useState('');
  const [lastUpdated, setLastUpdated] = useState('');
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
      what: string
    ): CountState =>
      result.status === 'fulfilled'
        ? { value: unwrapList(result.value).length, reason: '' }
        : { value: null, reason: `Không tải được: API trả về lỗi khi lấy danh sách ${what}.` };

    setWebsiteCount(settle(websitesRes, 'website'));
    setDatabaseCount(settle(databasesRes, 'cơ sở dữ liệu'));
    setSslCount(settle(sslRes, 'chứng chỉ SSL'));
  }, []);

  useEffect(() => {
    loadCounts();
  }, [loadCounts]);

  const refreshAll = useCallback(() => {
    reloadServers();
    reloadMetrics();
    loadCounts();
  }, [reloadServers, reloadMetrics, loadCounts]);

  // Formatted after the data settles, off the render path, so server and client
  // never disagree about the clock.
  useEffect(() => {
    if (serversLoading) return;
    try {
      setLastUpdated(
        new Date().toLocaleTimeString('vi-VN', {
          hour: '2-digit',
          minute: '2-digit',
          second: '2-digit',
        })
      );
    } catch {
      setLastUpdated('');
    }
  }, [serversLoading]);

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
  const metricsReason = hasServer ? NO_METRICS_REASON : NO_NODE_REASON;
  const inventoryReason = hasServer ? NO_INVENTORY_REASON : NO_NODE_REASON;

  const cpuPercent = finiteOrNull(selectedMetrics?.cpu_percent);
  const cpuCores = finiteOrNull(selected?.cpu_cores);
  const cpuDetail = cpuCores && cpuCores > 0 ? `${cpuCores} nhân` : undefined;

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
      ? `Tải 1 phút ${Number(selectedMetrics?.load1).toFixed(2)} trên ${cpuCores} nhân`
      : undefined;

  const onlineCount = servers.filter(
    (s) => String(s.status || '').toLowerCase() === 'online' ||
      String(s.status || '').toLowerCase() === 'active'
  ).length;
  const offlineCount = servers.filter(
    (s) => String(s.status || '').toLowerCase() === 'offline'
  ).length;

  const overviewItems: OverviewItem[] = useMemo(
    () => [
      {
        key: 'websites',
        label: 'Website',
        value: websiteCount.value,
        href: '/websites',
        note: websiteCount.reason,
      },
      {
        key: 'databases',
        label: 'Cơ sở dữ liệu',
        value: databaseCount.value,
        href: '/databases',
        note: databaseCount.reason,
      },
      { key: 'ssl', label: 'SSL', value: sslCount.value, href: '/ssl', note: sslCount.reason },
      {
        key: 'servers',
        label: 'Máy chủ',
        value: serversError ? null : servers.length,
        href: '/servers',
        note: serversError ? 'Không tải được: API trả về lỗi khi lấy danh sách máy chủ.' : undefined,
      },
    ],
    [websiteCount, databaseCount, sslCount, servers.length, serversError]
  );

  const networkSeries: TrafficSeries = useMemo(
    () => ({
      points: [],
      upLabel: 'Gửi lên',
      downLabel: 'Nhận về',
      metrics: [
        { label: 'Lên', value: null, reason: NO_SERIES_ENDPOINT },
        { label: 'Xuống', value: null, reason: NO_SERIES_ENDPOINT },
        {
          label: 'Tổng gửi',
          value: formatBytesOrNull(selectedMetrics?.net_out),
          reason: metricsReason,
        },
        {
          label: 'Tổng nhận',
          value: formatBytesOrNull(selectedMetrics?.net_in),
          reason: metricsReason,
        },
      ],
    }),
    [selectedMetrics, metricsReason]
  );

  const diskSeries: TrafficSeries = useMemo(
    () => ({
      points: [],
      upLabel: 'Ghi',
      downLabel: 'Đọc',
      metrics: [
        { label: 'Ghi', value: null, reason: NO_SERIES_ENDPOINT },
        { label: 'Đọc', value: null, reason: NO_SERIES_ENDPOINT },
        {
          label: 'Đã dùng',
          value: formatBytesOrNull(diskUsed),
          reason: metricsReason,
        },
        {
          label: 'Tổng dung lượng',
          value: formatBytesOrNull(diskTotal),
          reason: inventoryReason,
        },
      ],
    }),
    [diskUsed, diskTotal, metricsReason, inventoryReason]
  );

  const nodeTitle = selected ? serverLabel(selected) : 'Chưa có máy chủ';

  return (
    <div className="space-y-5">
      {/* Tieu de trang */}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="text-xl font-semibold text-gray-900">Bảng điều khiển</h1>
          <p className="mt-1 text-sm text-gray-600">
            Xin chào {user?.first_name || user?.username || 'bạn'}, đây là tình trạng hạ tầng của bạn.
            {lastUpdated ? ` Cập nhật lúc ${lastUpdated}.` : ''}
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
            Làm mới
          </button>
          <Link
            href="/websites"
            className="inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2"
          >
            <Plus size={16} aria-hidden="true" />
            Tạo website
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
            Thử lại
          </button>
        </div>
      )}

      {/* 0. May dang xem - danh tinh cua node, truoc moi con so */}
      <StatCard
        title="Máy đang xem"
        description={
          servers.length > 1
            ? 'Chọn máy chủ để xem số liệu của máy đó.'
            : 'Panel quản lý chính máy mà nó đang chạy.'
        }
        action={
          selected ? (
            <Link
              href="/servers"
              className="inline-flex items-center rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
            >
              Quản lý máy chủ
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
            Máy cài panel chưa được đăng ký thành node quản lý, nên chưa có thông số nào để
            hiển thị.
          </p>
        ) : (
          <div className="space-y-4">
            <div className="flex flex-wrap items-center gap-2">
              <span className="truncate text-base font-semibold text-gray-900">{nodeTitle}</span>
              {isLocalNode(selected) && (
                <LocalNodeBadge
                  label="Máy cài panel"
                  title="Máy đang chạy panel. Panel quản lý trực tiếp máy này, không cần máy thứ hai."
                />
              )}
            </div>

            {servers.length > 1 && (
              <div className="flex flex-wrap gap-2" role="group" aria-label="Chọn máy chủ">
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
                <dt className="text-xs text-gray-500">Địa chỉ IP</dt>
                <dd className="truncate font-mono text-sm text-gray-900">
                  <MetricText value={selected.ip_address || null} reason={inventoryReason} />
                </dd>
              </div>
              <div className="min-w-0">
                <dt className="text-xs text-gray-500">Hệ điều hành</dt>
                <dd className="truncate text-sm text-gray-900">
                  <MetricText value={selected.os || null} reason={inventoryReason} />
                </dd>
              </div>
              <div className="min-w-0">
                <dt className="text-xs text-gray-500">CPU</dt>
                <dd className="truncate text-sm text-gray-900">
                  <MetricText
                    value={cpuCores && cpuCores > 0 ? `${cpuCores} nhân` : null}
                    reason={inventoryReason}
                  />
                </dd>
              </div>
              <div className="min-w-0">
                <dt className="text-xs text-gray-500">RAM</dt>
                <dd className="truncate text-sm text-gray-900">
                  <MetricText value={formatBytesOrNull(ramTotal)} reason={inventoryReason} />
                </dd>
              </div>
              <div className="min-w-0">
                <dt className="text-xs text-gray-500">Ổ đĩa</dt>
                <dd className="truncate text-sm text-gray-900">
                  <MetricText value={formatBytesOrNull(diskTotal)} reason={inventoryReason} />
                </dd>
              </div>
              <div className="min-w-0">
                <dt className="text-xs text-gray-500">Nhân hệ điều hành</dt>
                <dd className="truncate text-sm text-gray-900">
                  <MetricText value={selected.kernel || null} reason={inventoryReason} />
                </dd>
              </div>
              <div className="min-w-0">
                <dt className="text-xs text-gray-500">Agent</dt>
                <dd className="truncate text-sm text-gray-900">
                  <MetricText
                    value={selected.agent_status || null}
                    reason="Chưa có dữ liệu: API chưa báo cáo trạng thái agent của máy này."
                  />
                </dd>
              </div>
              <div className="min-w-0">
                <dt className="text-xs text-gray-500">Máy chủ web</dt>
                <dd className="truncate text-sm text-gray-900">
                  <MetricText
                    value={
                      Array.isArray(selected.web_servers) && selected.web_servers.length > 0
                        ? selected.web_servers.join(', ')
                        : selected.web_server_type || null
                    }
                    reason="Chưa có dữ liệu: máy này chưa báo cáo máy chủ web nào đang chạy."
                  />
                </dd>
              </div>
            </dl>
          </div>
        )}
      </StatCard>

      {/* 1. Tinh trang he thong + O dia */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-3">
        <StatCard
          title="Tình trạng hệ thống"
          description={selected ? serverLabel(selected) : 'Chưa có máy chủ'}
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
                label="Tải hệ thống"
                detail={loadDetail}
                unavailableReason={metricsReason}
              />
              <DonutGauge
                value={cpuPercent}
                label="CPU"
                detail={cpuDetail}
                unavailableReason={metricsReason}
              />
              <DonutGauge
                value={ramPercent}
                label="RAM"
                detail={ramDetail}
                unavailableReason={ramTotal === null ? inventoryReason : metricsReason}
              />
            </div>
          )}
        </StatCard>

        <StatCard title="Ổ đĩa">
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
                  label="Đã dùng"
                  detail={diskDetail}
                  size={104}
                  unavailableReason={diskTotal === null ? inventoryReason : metricsReason}
                />
              </div>
              <p className="border-t border-gray-200 pt-3 text-xs text-gray-500">
                API chưa trả chi tiết theo từng điểm gắn kết (mount); số liệu trên là tổng dung
                lượng ổ đĩa của máy đang xem.
              </p>
            </div>
          )}
        </StatCard>
      </div>

      {/* 2. Tong quan */}
      <StatCard title="Tổng quan" flush>
        <OverviewRow items={overviewItems} loading={serversLoading} />
      </StatCard>

      {/* 3. Phan mem + Luu luong */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-2">
        <StatCard title="Phần mềm">
          <SoftwareGrid
            items={SOFTWARE_ITEMS}
            loading={serversLoading}
            emptyHint={NO_SOFTWARE_ENDPOINT}
          />
        </StatCard>

        <StatCard title="Lưu lượng">
          <TrafficPanel
            network={networkSeries}
            disk={diskSeries}
            loading={serversLoading}
            emptyHint={NO_SERIES_ENDPOINT}
          />
        </StatCard>
      </div>

      {/* 4. May chu */}
      <StatCard
        title="Máy chủ"
        description={
          serversLoading
            ? undefined
            : `${servers.length} máy chủ · ${onlineCount} trực tuyến${
                offlineCount > 0 ? ` · ${offlineCount} ngoại tuyến` : ''
              }`
        }
        action={
          <>
            <span className="hidden items-center gap-1.5 text-xs text-emerald-600 sm:inline-flex">
              <CheckCircle size={14} aria-hidden="true" />
              {onlineCount} trực tuyến
            </span>
            <Link
              href="/servers"
              className="inline-flex items-center rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
            >
              Xem tất cả
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

      {/* 5. Them may - lop tuy chon, noi ro la tuy chon */}
      <AddNodeCallout copy={ADD_NODE_COPY_VI} docsHref={brand.docsUrl} />
    </div>
  );
}
