'use client';

import Link from 'next/link';
import { ChevronRight } from 'lucide-react';
import { Skeleton } from './StatCard';

/**
 * Hang "Tong quan": cac o so lieu ngang nhau, ngan cach bang duong doc tren man lon,
 * xep chong tren man nho. Moi o la mot lien ket next/link co mui ten chevron.
 */
export interface OverviewItem {
  /** Khoa React, nen la slug on dinh. */
  key: string;
  label: string;
  /** Truyen `null` khi API chua tra truong nay - o se hien "—" kem chu thich. */
  value: number | null;
  href: string;
  /** Chu thich ngan hien duoi con so (vi du khi chua co du lieu). */
  note?: string;
}

export interface OverviewRowProps {
  items: OverviewItem[];
  loading?: boolean;
}

export default function OverviewRow({ items, loading = false }: OverviewRowProps) {
  const list = Array.isArray(items) ? items : [];

  if (loading) {
    return (
      <div className="grid grid-cols-1 divide-y divide-gray-200 lg:grid-cols-4 lg:divide-x lg:divide-y-0">
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="px-5 py-4">
            <Skeleton className="h-3 w-24" />
            <Skeleton className="mt-3 h-7 w-16" />
          </div>
        ))}
      </div>
    );
  }

  if (list.length === 0) {
    return <p className="px-5 py-6 text-sm text-gray-500">Chưa có mục tổng quan nào.</p>;
  }

  return (
    <div className="grid grid-cols-1 divide-y divide-gray-200 lg:grid-cols-4 lg:divide-x lg:divide-y-0">
      {list.map((item) => {
        const hasValue = typeof item?.value === 'number' && Number.isFinite(item.value);
        return (
          <Link
            key={item?.key}
            href={item?.href || '#'}
            className="group flex items-center justify-between gap-3 px-5 py-4 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-inset"
          >
            <span className="min-w-0">
              <span className="block truncate text-xs font-medium uppercase tracking-wide text-gray-500">
                {item?.label}
              </span>
              <span className="mt-1 block text-2xl font-semibold text-gray-900">
                {hasValue ? item.value : '—'}
              </span>
              {!hasValue && item?.note && (
                <span className="mt-0.5 block text-xs text-gray-500">{item.note}</span>
              )}
            </span>
            <ChevronRight
              size={18}
              className="shrink-0 text-gray-400 group-hover:text-brand-700"
              aria-hidden="true"
            />
          </Link>
        );
      })}
    </div>
  );
}
