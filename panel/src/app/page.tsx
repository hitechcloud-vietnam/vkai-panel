'use client';

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { brand } from '@/lib/brand';
import { useT } from '@/i18n';

export default function HomePage() {
  const router = useRouter();
  const t = useT();

  useEffect(() => {
    if (typeof window === 'undefined') return;

    let token: string | null = null;
    try {
      token = window.localStorage.getItem('access_token');
    } catch {
      token = null;
    }

    router.replace(token ? '/dashboard' : '/login');
  }, [router]);

  return (
    <div className="flex min-h-screen items-center justify-center bg-gray-50">
      <div className="flex flex-col items-center gap-3">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-gray-200 border-t-brand-600" />
        <p className="text-sm text-gray-600">
          {t('common.loadingProduct', { product: brand.productName })}
        </p>
      </div>
    </div>
  );
}
