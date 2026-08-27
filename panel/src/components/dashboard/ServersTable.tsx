'use client';

import Link from 'next/link';
import { Cpu, HardDrive, MemoryStick, Plus, Server } from 'lucide-react';
import { Skeleton, StateMessage } from './StatCard';

/**
 * Bang danh sach may chu dang quan ly.
 * Kieu du lieu giu nguyen theo response cua GET /api/v1/servers.
 */
export interface DashboardServer {
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

export interface ServersTableProps {
  servers: DashboardServer[];
  loading?: boolean;
  error?: string | null;
  /** Ham dinh dang dung luong cua trang - truyen vao de giu nguyen hanh vi cu. */
  formatBytes: (bytes: number) => string;
  /** Ham tra ve class huy hieu trang thai cua trang. */
  statusBadgeClass: (status: string) => string;
  /** Chu goi cho trang thai rong. */
  emptyHint?: string;
  onRetry?: () => void;
}

const COLUMNS = [
  'Máy chủ',
  'Trạng thái',
  'Địa chỉ IP',
  'Hệ điều hành',
  'CPU',
  'RAM',
  'Ổ đĩa',
  'Thao tác',
];

export default function ServersTable({
  servers,
  loading = false,
  error = null,
  formatBytes,
  statusBadgeClass,
  emptyHint,
  onRetry,
}: ServersTableProps) {
  const list = Array.isArray(servers) ? servers : [];

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
        title="Không tải được danh sách máy chủ"
        hint={error}
        action={
          onRetry ? (
            <button
              type="button"
              onClick={onRetry}
              className="rounded-md border border-gray-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
            >
              Thử lại
            </button>
          ) : undefined
        }
      />
    );
  }

  if (list.length === 0) {
    return (
      <StateMessage
        icon={<Server size={36} aria-hidden="true" />}
        title="Chưa có máy chủ nào"
        hint={emptyHint}
        action={
          <Link
            href="/servers/add"
            className="inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2"
          >
            <Plus size={16} aria-hidden="true" />
            Thêm máy chủ đầu tiên
          </Link>
        }
      />
    );
  }

  return (
    <div className="w-full overflow-x-auto">
      <table className="w-full min-w-[900px] border-collapse">
        <thead className="bg-gray-50">
          <tr className="border-b border-gray-200">
            {COLUMNS.map((label) => (
              <th
                key={label}
                scope="col"
                className="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-gray-500"
              >
                {label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {list.map((server) => (
            <tr key={server?.id} className="border-b border-gray-100 last:border-b-0 hover:bg-gray-50">
              <td className="px-4 py-3 text-sm text-gray-700">
                <p className="font-medium text-gray-900">{server?.name || '—'}</p>
                <p className="text-xs text-gray-500">{server?.hostname || '—'}</p>
              </td>
              <td className="px-4 py-3 text-sm">
                <span
                  className={`inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium ${statusBadgeClass(
                    server?.status || ''
                  )}`}
                >
                  {server?.status || 'unknown'}
                </span>
              </td>
              <td className="px-4 py-3 font-mono text-sm text-gray-700">
                {server?.ip_address || '—'}
              </td>
              <td className="px-4 py-3 text-sm text-gray-700">{server?.os || '—'}</td>
              <td className="px-4 py-3 text-sm text-gray-700">
                <span className="flex items-center gap-2">
                  <Cpu size={14} className="text-gray-500" aria-hidden="true" />
                  <span>{(server?.metrics?.cpu_percent ?? 0).toFixed(1)}%</span>
                </span>
              </td>
              <td className="px-4 py-3 text-sm text-gray-700">
                <span className="flex items-center gap-2">
                  <MemoryStick size={14} className="text-gray-500" aria-hidden="true" />
                  <span>
                    {formatBytes(server?.metrics?.ram_used ?? 0)} /{' '}
                    {formatBytes(server?.ram_total ?? 0)}
                  </span>
                </span>
              </td>
              <td className="px-4 py-3 text-sm text-gray-700">
                <span className="flex items-center gap-2">
                  <HardDrive size={14} className="text-gray-500" aria-hidden="true" />
                  <span>
                    {formatBytes(server?.metrics?.disk_used ?? 0)} /{' '}
                    {formatBytes(server?.disk_total ?? 0)}
                  </span>
                </span>
              </td>
              <td className="px-4 py-3 text-sm">
                <Link
                  href={`/servers/${server?.id}`}
                  className="inline-flex items-center rounded-md border border-gray-300 bg-white px-2.5 py-1 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
                >
                  Xem
                </Link>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
