'use client';

import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { AlertTriangle, CheckCircle, Plus, RefreshCw } from 'lucide-react';
import { api } from '@/services/api';
import { useAuthStore } from '@/store/auth';
import { brand } from '@/lib/brand';
import StatCard, { Skeleton } from '@/components/dashboard/StatCard';
import DonutGauge, { DonutGaugeSkeleton } from '@/components/dashboard/DonutGauge';
import OverviewRow, { type OverviewItem } from '@/components/dashboard/OverviewRow';
import SoftwareGrid, { type SoftwareItem } from '@/components/dashboard/SoftwareGrid';
import TrafficPanel, { type TrafficSeries } from '@/components/dashboard/TrafficPanel';
import ServersTable from '@/components/dashboard/ServersTable';

interface DashboardStats {
  servers: number;
  websites: number;
  databases: number;
  sslCerts: number;
}

interface ServerInfo {
  id: string;
  name: string;
  hostname: string;
  status: string;
  ip_address: string;
  os: string;
  cpu_cores: number;
  ram_total: number;
  disk_total: number;
  metrics?: {
    cpu_percent: number;
    ram_used: number;
    disk_used: number;
  };
}

/** Chu thich dung chung cho truong ma API hien chua tra ve. */
const NO_API_NOTE = 'Chưa có dữ liệu từ API';

/** Danh sach phan mem: chua co endpoint nen de rong, khong bia du lieu. */
const SOFTWARE_ITEMS: SoftwareItem[] = [];

/** Chuoi luu luong: chua co endpoint nen de rong, cac o so lieu hien "—". */
const NETWORK_SERIES: TrafficSeries = {
  points: [],
  upLabel: 'Gửi lên',
  downLabel: 'Nhận về',
  metrics: [
    { label: 'Lên', value: null },
    { label: 'Xuống', value: null },
    { label: 'Tổng gửi', value: null },
    { label: 'Tổng nhận', value: null },
  ],
};

const DISK_SERIES: TrafficSeries = {
  points: [],
  upLabel: 'Ghi',
  downLabel: 'Đọc',
  metrics: [
    { label: 'Ghi', value: null },
    { label: 'Đọc', value: null },
    { label: 'Tổng ghi', value: null },
    { label: 'Tổng đọc', value: null },
  ],
};

/** Tinh phan tram an toan; tra null khi thieu du lieu. */
function percentOf(used: unknown, total: unknown): number | null {
  const usedNum = typeof used === 'number' ? used : Number(used);
  const totalNum = typeof total === 'number' ? total : Number(total);
  if (!Number.isFinite(usedNum) || !Number.isFinite(totalNum) || totalNum <= 0) return null;
  return Math.min(100, Math.max(0, (usedNum / totalNum) * 100));
}

export default function DashboardPage() {
  const { user } = useAuthStore();
  const [stats, setStats] = useState<DashboardStats>({
    servers: 0,
    websites: 0,
    databases: 0,
    sslCerts: 0,
  });
  const [servers, setServers] = useState<ServerInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [selectedServerId, setSelectedServerId] = useState('');
  const [lastUpdated, setLastUpdated] = useState('');

  useEffect(() => {
    loadDashboardData();
  }, []);

  const loadDashboardData = async () => {
    setLoading(true);
    setError('');
    try {
      const [serversRes] = await Promise.all([
        api.get('/api/v1/servers'),
      ]);

      const list = Array.isArray(serversRes?.data?.data) ? serversRes.data.data : [];
      setServers(list);
      setStats({
        servers: list.length,
        websites: 0,
        databases: 0,
        sslCerts: 0,
      });
    } catch (err: any) {
      console.error('Failed to load dashboard data:', err);
      setServers([]);
      setError(
        err?.response?.data?.error || err?.message || 'Failed to load dashboard data'
      );
    } finally {
      setLoading(false);
      // Dinh dang thoi gian ngoai luong render de tranh lech hydration.
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
    }
  };

  const formatBytes = (bytes: number) => {
    const value = Number(bytes);
    if (!Number.isFinite(value) || value <= 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.min(Math.floor(Math.log(value) / Math.log(k)), sizes.length - 1);
    return parseFloat((value / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'online':
        return 'text-emerald-600';
      case 'offline':
        return 'text-red-600';
      default:
        return 'text-amber-600';
    }
  };

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'online':
        return 'bg-emerald-50 text-emerald-700';
      case 'offline':
        return 'bg-red-50 text-red-700';
      default:
        return 'bg-amber-50 text-amber-700';
    }
  };

  const onlineCount = servers.filter((s) => s?.status === 'online').length;
  const offlineCount = servers.filter((s) => s?.status === 'offline').length;

  // Giu may chu dang chon luon hop le sau moi lan tai lai danh sach.
  useEffect(() => {
    if (servers.length === 0) {
      if (selectedServerId !== '') setSelectedServerId('');
      return;
    }
    const stillThere = servers.some((s) => s?.id === selectedServerId);
    if (!stillThere) setSelectedServerId(servers[0]?.id || '');
  }, [servers, selectedServerId]);

  const selected = useMemo(
    () => servers.find((s) => s?.id === selectedServerId) || servers[0] || null,
    [servers, selectedServerId]
  );

  const cpuRaw = selected?.metrics?.cpu_percent;
  const cpuPercent = typeof cpuRaw === 'number' && Number.isFinite(cpuRaw) ? cpuRaw : null;
  const cpuCores = Number(selected?.cpu_cores);
  const cpuDetail =
    Number.isFinite(cpuCores) && cpuCores > 0 ? `${cpuCores} nhân` : 'Chưa có số nhân';

  const ramUsed = selected?.metrics?.ram_used;
  const ramTotal = selected?.ram_total;
  const ramPercent = percentOf(ramUsed, ramTotal);
  const ramDetail =
    ramPercent === null
      ? NO_API_NOTE
      : `${formatBytes(Number(ramUsed))} / ${formatBytes(Number(ramTotal))}`;

  const diskUsed = selected?.metrics?.disk_used;
  const diskTotal = selected?.disk_total;
  const diskPercent = percentOf(diskUsed, diskTotal);
  const diskDetail =
    diskPercent === null
      ? NO_API_NOTE
      : `${formatBytes(Number(diskUsed))} / ${formatBytes(Number(diskTotal))}`;

  const overviewItems: OverviewItem[] = useMemo(
    () => [
      { key: 'websites', label: 'Website', value: null, href: '/websites', note: NO_API_NOTE },
      { key: 'databases', label: 'Cơ sở dữ liệu', value: null, href: '/databases', note: NO_API_NOTE },
      { key: 'ssl', label: 'SSL', value: null, href: '/ssl', note: NO_API_NOTE },
      { key: 'security', label: 'Cảnh báo bảo mật', value: null, href: '/security', note: NO_API_NOTE },
    ],
    []
  );

  const hasServer = Boolean(selected);

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
            onClick={loadDashboardData}
            disabled={loading}
            className="inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 disabled:cursor-not-allowed disabled:opacity-60"
          >
            <RefreshCw size={16} aria-hidden="true" />
            Làm mới
          </button>
          <Link
            href="/servers/add"
            className="inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2"
          >
            <Plus size={16} aria-hidden="true" />
            Thêm máy chủ
          </Link>
        </div>
      </div>

      {error && (
        <div
          role="alert"
          className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700"
        >
          <span className="flex min-w-0 items-center gap-2">
            <AlertTriangle size={16} aria-hidden="true" />
            <span className="min-w-0 break-words">{error}</span>
          </span>
          <button
            type="button"
            onClick={loadDashboardData}
            className="shrink-0 rounded-md border border-red-300 bg-white px-3 py-1.5 text-sm font-medium text-red-700 hover:bg-red-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-600"
          >
            Thử lại
          </button>
        </div>
      )}

      {/* 1. Tinh trang he thong + O dia */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-3">
        <StatCard
          title="Tình trạng hệ thống"
          description={
            hasServer && selected?.name
              ? `${selected.name}${selected?.hostname ? ` · ${selected.hostname}` : ''}`
              : 'Chưa chọn máy chủ'
          }
          className="lg:col-span-2"
        >
          {loading ? (
            <div className="grid grid-cols-1 gap-6 sm:grid-cols-3">
              {Array.from({ length: 3 }).map((_, i) => (
                <div key={i} className="flex justify-center">
                  <DonutGaugeSkeleton />
                </div>
              ))}
            </div>
          ) : !hasServer ? (
            <p className="py-8 text-center text-sm text-gray-500">
              Chưa có máy chủ nào để hiển thị tình trạng hệ thống.
            </p>
          ) : (
            <div className="grid grid-cols-1 gap-6 sm:grid-cols-3">
              <DonutGauge
                value={null}
                label="Tải hệ thống"
                detail="API chưa trả tải trung bình"
              />
              <DonutGauge value={cpuPercent} label="CPU" detail={cpuDetail} />
              <DonutGauge value={ramPercent} label="RAM" detail={ramDetail} />
            </div>
          )}
        </StatCard>

        <StatCard title="Ổ đĩa">
          {loading ? (
            <div className="space-y-4">
              <Skeleton className="h-8 w-full" />
              <div className="flex justify-center">
                <DonutGaugeSkeleton size={100} />
              </div>
            </div>
          ) : !hasServer ? (
            <p className="py-8 text-center text-sm text-gray-500">
              Chưa có dữ liệu ổ đĩa.
            </p>
          ) : (
            <div className="space-y-4">
              <div className="flex flex-wrap gap-2" role="group" aria-label="Chọn máy chủ">
                {servers.map((server) => {
                  const active = server?.id === (selected?.id || '');
                  return (
                    <button
                      key={server?.id}
                      type="button"
                      aria-pressed={active}
                      onClick={() => setSelectedServerId(server?.id || '')}
                      className={
                        active
                          ? 'max-w-full truncate rounded-md border border-brand-200 bg-brand-50 px-2.5 py-1 text-xs font-medium text-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500'
                          : 'max-w-full truncate rounded-md border border-gray-300 bg-white px-2.5 py-1 text-xs font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500'
                      }
                    >
                      {server?.name || server?.hostname || '—'}
                    </button>
                  );
                })}
              </div>

              <div className="flex items-center justify-center">
                <DonutGauge
                  value={diskPercent}
                  label="Đã dùng"
                  detail={diskDetail}
                  size={104}
                />
              </div>

              <p className="border-t border-gray-200 pt-3 text-xs text-gray-500">
                API chưa trả chi tiết theo từng điểm gắn kết (mount); số liệu trên là tổng dung
                lượng ổ đĩa của máy chủ đang chọn.
              </p>
            </div>
          )}
        </StatCard>
      </div>

      {/* 2. Tong quan */}
      <StatCard title="Tổng quan" flush>
        <OverviewRow items={overviewItems} loading={loading} />
      </StatCard>

      {/* 3. Phan mem + Luu luong */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-2">
        <StatCard title="Phần mềm">
          <SoftwareGrid
            items={SOFTWARE_ITEMS}
            loading={loading}
            emptyHint={`API hiện chưa trả về danh sách phần mềm đã cài đặt cho ${brand.productName}.`}
          />
        </StatCard>

        <StatCard title="Lưu lượng">
          <TrafficPanel
            network={NETWORK_SERIES}
            disk={DISK_SERIES}
            loading={loading}
            emptyHint="API hiện chưa trả về chuỗi số liệu lưu lượng theo thời gian."
          />
        </StatCard>
      </div>

      {/* 4. May chu */}
      <StatCard
        title="Máy chủ"
        description={
          loading
            ? undefined
            : `${stats.servers} máy chủ · ${onlineCount} trực tuyến${
                offlineCount > 0 ? ` · ${offlineCount} ngoại tuyến` : ''
              }`
        }
        action={
          <>
            <span className={`hidden items-center gap-1.5 text-xs sm:inline-flex ${getStatusColor('online')}`}>
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
          loading={loading}
          error={error || null}
          formatBytes={formatBytes}
          statusBadgeClass={getStatusBadge}
          emptyHint={`Thêm máy chủ đầu tiên để bắt đầu sử dụng ${brand.productName}.`}
          onRetry={loadDashboardData}
        />
      </StatCard>
    </div>
  );
}
