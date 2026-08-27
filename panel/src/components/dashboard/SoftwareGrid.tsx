'use client';

import { Package } from 'lucide-react';
import { Skeleton, StateMessage } from './StatCard';

/**
 * Luoi cac o phan mem da cai dat tren may chu (Nginx, MySQL, PHP, Redis, Docker...).
 * Moi o: ten + phien ban + trang thai chay/dung.
 */
export type SoftwareStatus = 'running' | 'stopped' | 'unknown';

export interface SoftwareItem {
  name: string;
  /** `null`/bo trong khi API chua tra phien ban. */
  version?: string | null;
  status?: SoftwareStatus;
}

export interface SoftwareGridProps {
  items: SoftwareItem[];
  loading?: boolean;
  error?: string | null;
  /** Chu thich hien o trang thai rong. */
  emptyHint?: string;
}

const STATUS_TEXT: Record<SoftwareStatus, string> = {
  running: 'Đang chạy',
  stopped: 'Đã dừng',
  unknown: 'Chưa rõ',
};

const STATUS_DOT: Record<SoftwareStatus, string> = {
  running: 'bg-emerald-600',
  stopped: 'bg-red-600',
  unknown: 'bg-gray-400',
};

const STATUS_LABEL: Record<SoftwareStatus, string> = {
  running: 'text-emerald-700',
  stopped: 'text-red-700',
  unknown: 'text-gray-500',
};

export default function SoftwareGrid({
  items,
  loading = false,
  error = null,
  emptyHint,
}: SoftwareGridProps) {
  const list = Array.isArray(items) ? items : [];

  if (loading) {
    return (
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
        {Array.from({ length: 6 }).map((_, i) => (
          <div key={i} className="rounded-md border border-gray-200 p-3">
            <Skeleton className="h-4 w-20" />
            <Skeleton className="mt-2 h-3 w-14" />
            <Skeleton className="mt-3 h-3 w-16" />
          </div>
        ))}
      </div>
    );
  }

  if (error) {
    return (
      <StateMessage
        tone="error"
        title="Không tải được danh sách phần mềm"
        hint={error}
      />
    );
  }

  if (list.length === 0) {
    return (
      <StateMessage
        icon={<Package size={32} aria-hidden="true" />}
        title="Chưa có dữ liệu phần mềm"
        hint={emptyHint || 'API hiện chưa trả về danh sách phần mềm đã cài đặt.'}
      />
    );
  }

  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
      {list.map((item, index) => {
        const status: SoftwareStatus = item?.status || 'unknown';
        return (
          <div
            key={`${item?.name || 'sw'}-${index}`}
            className="rounded-md border border-gray-200 p-3 hover:bg-gray-50"
          >
            <p className="truncate text-sm font-medium text-gray-900">{item?.name || '—'}</p>
            <p className="mt-0.5 truncate font-mono text-xs text-gray-500">
              {item?.version || '—'}
            </p>
            <p className="mt-2 flex items-center gap-1.5 text-xs">
              <span
                className={`h-1.5 w-1.5 shrink-0 rounded-full ${STATUS_DOT[status]}`}
                aria-hidden="true"
              />
              <span className={STATUS_LABEL[status]}>{STATUS_TEXT[status]}</span>
            </p>
          </div>
        );
      })}
    </div>
  );
}
