'use client';

import { brand } from '@/lib/brand';
import { useT } from '@/i18n';

export default function Loading() {
  const t = useT();

  return (
    <div
      role="status"
      aria-live="polite"
      className="flex min-h-screen items-center justify-center bg-[#F7F8FA]"
    >
      <div className="flex flex-col items-center gap-3">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-brand-100 border-t-brand-600" />
        <p className="text-sm text-gray-600">
          {t('common.loadingProduct', { product: brand.productName })}
        </p>
      </div>
    </div>
  );
}
