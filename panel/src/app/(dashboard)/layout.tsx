'use client';

import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import Sidebar from '@/components/Sidebar';
import Header from '@/components/Header';
import { useAuthStore } from '@/store/auth';
import { brand } from '@/lib/brand';

function DashboardSkeleton() {
  return (
    <div className="flex h-screen overflow-hidden bg-gray-50">
      <aside className="hidden w-64 shrink-0 border-r border-gray-200 bg-white lg:block">
        <div className="h-16 border-b border-gray-200 px-4 py-4">
          <div className="h-8 w-32 animate-pulse rounded-md bg-gray-100" />
        </div>
        <div className="space-y-2 p-4">
          {Array.from({ length: 10 }).map((_, i) => (
            <div key={i} className="h-8 w-full animate-pulse rounded-md bg-gray-100" />
          ))}
        </div>
      </aside>
      <div className="flex flex-1 flex-col overflow-hidden">
        <div className="flex h-16 shrink-0 items-center justify-between border-b border-gray-200 bg-white px-6">
          <div className="h-5 w-48 animate-pulse rounded-md bg-gray-100" />
          <div className="h-8 w-40 animate-pulse rounded-md bg-gray-100" />
        </div>
        <div className="flex-1 space-y-4 overflow-y-auto p-6">
          <div className="h-7 w-64 animate-pulse rounded-md bg-gray-100" />
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
            {Array.from({ length: 4 }).map((_, i) => (
              <div
                key={i}
                className="h-24 animate-pulse rounded-lg border border-gray-200 bg-white"
              />
            ))}
          </div>
          <div className="h-64 animate-pulse rounded-lg border border-gray-200 bg-white" />
        </div>
      </div>
    </div>
  );
}

export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const router = useRouter();
  const { isAuthenticated, loadUser } = useAuthStore();
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;

    let token: string | null = null;
    try {
      token = window.localStorage.getItem('access_token');
    } catch {
      token = null;
    }

    if (!token) {
      router.replace('/login');
      return;
    }

    loadUser()
      .catch((err) => {
        // loadUser da tu xu ly dang xuat; bat them de tranh unhandled rejection.
        console.error(`[${brand.productName}] Không tải được thông tin người dùng:`, err);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (!loading && !isAuthenticated) {
      router.replace('/login');
    }
  }, [loading, isAuthenticated, router]);

  if (loading || !isAuthenticated) {
    return <DashboardSkeleton />;
  }

  return (
    <div className="flex h-screen overflow-hidden bg-gray-50">
      <Sidebar />
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        <Header />
        <main className="flex-1 overflow-y-auto bg-gray-50 p-6">{children}</main>
      </div>
    </div>
  );
}
