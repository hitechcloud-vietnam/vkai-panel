'use client';

import { Package } from 'lucide-react';
import { useT } from '@/i18n';
import { Skeleton, StateMessage } from './StatCard';

/**
 * A grid of the software installed on a machine (Nginx, MySQL, PHP, Redis,
 * Docker...). Each cell: name + version + running/stopped state.
 */
export type SoftwareStatus = 'running' | 'stopped' | 'unknown';

export interface SoftwareItem {
  name: string;
  /** `null` or empty while the API has not reported a version. */
  version?: string | null;
  status?: SoftwareStatus;
}

export interface SoftwareGridProps {
  items: SoftwareItem[];
  loading?: boolean;
  error?: string | null;
  /** Note shown in the empty state, already translated. */
  emptyHint?: string;
}

/** Dictionary key per status. The status itself is a machine value. */
const STATUS_KEY: Record<SoftwareStatus, string> = {
  running: 'dashboard.software.status.running',
  stopped: 'dashboard.software.status.stopped',
  unknown: 'dashboard.software.status.unknown',
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
  const t = useT();
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
        title={t('dashboard.software.loadFailed')}
        hint={error}
      />
    );
  }

  if (list.length === 0) {
    return (
      <StateMessage
        icon={<Package size={32} aria-hidden="true" />}
        title={t('dashboard.software.emptyTitle')}
        hint={emptyHint || t('dashboard.software.emptyHint')}
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
              <span className={STATUS_LABEL[status]}>{t(STATUS_KEY[status])}</span>
            </p>
          </div>
        );
      })}
    </div>
  );
}
