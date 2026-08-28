'use client';

import { useEffect } from 'react';
import Link from 'next/link';
import { AlertTriangle, RotateCcw, Home } from 'lucide-react';
import { brand } from '@/lib/brand';
import { useT } from '@/i18n';

export default function Error({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  const t = useT();

  useEffect(() => {
    // Logged to the console so a technician can read the real cause. Console
    // output stays in English regardless of the interface locale: it is read
    // by whoever is debugging, not by the operator.
    console.error(`[${brand.productName}] Application error:`, error);
  }, [error]);

  return (
    <div className="flex min-h-screen items-center justify-center bg-[#F7F8FA] px-4 py-10">
      <div className="w-full max-w-lg rounded-lg border border-gray-200 bg-white shadow-sm">
        <div className="flex items-start gap-3 border-b border-gray-200 px-5 py-4">
          <span className="mt-0.5 inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-red-50 text-red-600">
            <AlertTriangle size={18} aria-hidden="true" />
          </span>
          <div>
            <h1 className="text-sm font-semibold text-gray-900">{t('errors.appTitle')}</h1>
            <p className="mt-0.5 text-sm text-gray-600">{t('errors.appBody')}</p>
          </div>
        </div>

        <div className="space-y-3 px-5 py-4">
          <div>
            <p className="text-xs font-medium uppercase tracking-wide text-gray-500">
              {t('errors.detailsLabel')}
            </p>
            <pre className="mt-1.5 max-h-48 overflow-auto whitespace-pre-wrap rounded-md border border-gray-200 bg-gray-50 p-3 text-xs text-gray-700">
              {error?.message || t('errors.noDetails')}
            </pre>
          </div>

          {error?.digest && (
            <div>
              <p className="text-xs font-medium uppercase tracking-wide text-gray-500">
                {t('errors.digestLabel')}
              </p>
              <p className="mt-1 font-mono text-xs text-gray-700">{error.digest}</p>
            </div>
          )}

          <p className="text-sm text-gray-600">
            {t('errors.contactSupportLead')}{' '}
            <a
              href={`mailto:${brand.supportEmail}`}
              className="rounded-md font-medium text-brand-700 hover:text-brand-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500"
            >
              {brand.supportEmail}
            </a>
            .
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2 border-t border-gray-200 px-5 py-4">
          <button
            type="button"
            onClick={() => reset()}
            className="inline-flex items-center gap-2 rounded-md bg-brand-600 px-3.5 py-2 text-sm font-medium text-white hover:bg-brand-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2"
          >
            <RotateCcw size={16} aria-hidden="true" />
            {t('common.retry')}
          </button>
          <Link
            href="/"
            className="inline-flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500 focus-visible:ring-offset-2"
          >
            <Home size={16} aria-hidden="true" />
            {t('common.backToHome')}
          </Link>
        </div>
      </div>
    </div>
  );
}
