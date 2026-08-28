'use client';

import Link from 'next/link';
import { Home, LayoutDashboard } from 'lucide-react';
import { brand } from '@/lib/brand';
import { useT } from '@/i18n';

export default function NotFound() {
  const t = useT();

  return (
    <div className="flex min-h-screen items-center justify-center bg-[#F7F8FA] px-4 py-10">
      <div className="w-full max-w-md rounded-lg border border-gray-200 bg-white shadow-sm">
        <div className="border-b border-gray-200 px-5 py-4">
          <p className="text-xs font-semibold uppercase tracking-wide text-gray-500">
            {t('errors.notFoundLabel')}
          </p>
          <h1 className="mt-1 text-sm font-semibold text-gray-900">{t('errors.notFoundTitle')}</h1>
        </div>
        <div className="px-5 py-4">
          <p className="text-sm text-gray-600">
            {t('errors.notFoundBody', { product: brand.productName })}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2 border-t border-gray-200 px-5 py-4">
          <Link
            href="/dashboard"
            className="inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2"
          >
            <LayoutDashboard size={16} aria-hidden="true" />
            {t('common.backToDashboard')}
          </Link>
          <Link
            href="/"
            className="inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2"
          >
            <Home size={16} aria-hidden="true" />
            {t('common.home')}
          </Link>
        </div>
      </div>
    </div>
  );
}
